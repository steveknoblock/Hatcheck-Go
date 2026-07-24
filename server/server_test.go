package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"crypto/md5"
	"encoding/hex"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// --- Test helpers ---

// serverTestKey is the HMAC signing key used for capabilities issued during
// these tests. Unrelated to middlewareKey in capability_middleware_test.go —
// each file's tests sign/verify independently, so there's no need to share one.
var serverTestKey = []byte("server-test-key")

// testConfig returns a Config suitable for handler tests. CapabilityExpiry
// is set to a real, positive duration — a zero-value Config would issue
// capabilities that expire the instant they're created (Expires =
// time.Now().Add(0)), which doesn't reflect how the server actually runs
// and could make expiry-sensitive assertions fail spuriously.
func testConfig() Config {
	return Config{
		CapabilityExpiry: time.Hour,
	}
}

// newTestEnv creates temp directories with a CAS store and metadata store.
func newTestEnv(t *testing.T) (store *cas.Store, objPath, metaPath string, meta *metadata.Store) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hatcheck-server-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	objPath = filepath.Join(dir, "objects")
	metaPath = filepath.Join(dir, "metadata")

	store, err = cas.New(objPath, func(content string) string {
		sum := md5.Sum([]byte(content))
		return hex.EncodeToString(sum[:])
	})
	if err != nil {
		t.Fatalf("failed to create CAS store: %v", err)
	}

	// Constructors used here (not zero-value struct literals) to match how
	// main() registers indexes in production, and to keep CapabilityIndex
	// available for stashAndIssue's email-backfill lookup (resolveEmail)
	// during these tests — though it degrades gracefully to "" if omitted.
	meta, err = metadata.New(metaPath,
		metadata.NewTagIndex(),
		metadata.NewDateIndex(),
		metadata.NewNameIndex(),
		metadata.NewRelationIndex(),
		metadata.NewCapabilityIndex(),
	)
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	return
}

// makeArchive creates a minimal valid tar.gz archive in memory for import tests.
func makeArchive(t *testing.T, source string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifest := `{"source":"` + source + `","version":"1","objects":0}`
	hdr := &tar.Header{Name: "manifest.json", Mode: 0644, Size: int64(len(manifest))}
	tw.WriteHeader(hdr)
	tw.Write([]byte(manifest))

	tw.Close()
	gz.Close()
	return buf.Bytes()
}

// stashOne is a test helper that stashes content and returns the hash.
// stashHandler's response body is JSON ({"hash":..., "capability":...}) —
// this must unmarshal it rather than return the raw body, or every caller
// downstream ends up treating the whole JSON blob as "the hash".
func stashOne(t *testing.T, store *cas.Store, meta *metadata.Store, content string) string {
	t.Helper()

	// Compute the hash the same way the store will so we can build a matching VR.
	sum := md5.Sum([]byte(content))
	hash := hex.EncodeToString(sum[:])
	vr := VerifiedRequest{
		Capability: metadata.CapabilityPayload{Hash: hash},
		Principal:  "test-user",
	}

	req := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader(content))
	w := httptest.NewRecorder()
	stashHandler(w, req, store, meta, serverTestKey, testConfig(), vr)
	if w.Code != http.StatusCreated {
		t.Fatalf("stashOne failed: %d %s", w.Code, w.Body.String())
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("stashOne: failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	return result.Hash
}

// vrForHash returns a VerifiedRequest with the given hash and a test principal.
func vrForHash(hash string) VerifiedRequest {
	return VerifiedRequest{
		Capability: metadata.CapabilityPayload{Hash: hash},
		Principal:  "test-user",
	}
}

// vrEmpty returns a zero-value VerifiedRequest for handlers that don't check hash.
func vrEmpty() VerifiedRequest {
	return VerifiedRequest{}
}

// --- stashHandler ---

func TestStashHandler_Success(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	content := "hello #world"
	sum := md5.Sum([]byte(content))
	wantHash := hex.EncodeToString(sum[:])

	req := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader(content))
	w := httptest.NewRecorder()
	stashHandler(w, req, store, meta, serverTestKey, testConfig(), vrForHash(wantHash))

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	if result.Hash != wantHash {
		t.Errorf("expected hash %q, got %q", wantHash, result.Hash)
	}
	if result.Capability.Hash != wantHash {
		t.Errorf("expected capability to cover %q, got %q", wantHash, result.Capability.Hash)
	}
	if result.Capability.Perm != PermWrite {
		t.Errorf("expected write capability, got perm %q", result.Capability.Perm)
	}
}

func TestStashHandler_WrongMethod(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/stash", nil)
	w := httptest.NewRecorder()
	stashHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestStashHandler_CapabilityIrrelevant documents a real design point, not
// an oversight: stash is auth-only (see routes.go — no cm.Protect wrapper).
// There's no pre-existing capability to check the content's hash against,
// because stash is the ownership-creation event itself — the server always
// issues a fresh write capability for whatever hash the content produces,
// regardless of what (if anything) is in vr.Capability. An older version of
// this test asserted stash should 403 on a capability/content hash mismatch;
// that behavior was intentionally removed, not broken.
func TestStashHandler_CapabilityIrrelevant(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	content := "hello #world"
	sum := md5.Sum([]byte(content))
	wantHash := hex.EncodeToString(sum[:])

	// vr carries a capability for an unrelated hash — stash must still succeed.
	req := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader(content))
	w := httptest.NewRecorder()
	stashHandler(w, req, store, meta, serverTestKey, testConfig(), vrForHash("completely-unrelated-hash"))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	if result.Hash != wantHash {
		t.Errorf("expected hash %q, got %q", wantHash, result.Hash)
	}
	// The issued capability must cover the real content hash, not the
	// unrelated one that happened to be sitting in vr.Capability.
	if result.Capability.Hash != wantHash {
		t.Errorf("expected issued capability to cover %q, got %q", wantHash, result.Capability.Hash)
	}
}

// --- fetchHandler ---

func TestFetchHandler_Success(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	hash := stashOne(t, store, meta, "fetch me")

	req := httptest.NewRequest(http.MethodGet, "/fetch?hash="+hash, nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, store, vrForHash(hash))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "fetch me") {
		t.Errorf("expected content in response, got %q", w.Body.String())
	}
}

func TestFetchHandler_MissingHash(t *testing.T) {
	store, _, _, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/fetch", nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, store, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFetchHandler_NotFound(t *testing.T) {
	store, _, _, _ := newTestEnv(t)
	hash := "aabbccddeeff00112233445566778899"

	req := httptest.NewRequest(http.MethodGet, "/fetch?hash="+hash, nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, store, vrForHash(hash))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestFetchHandler_WrongHash(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	hash := stashOne(t, store, meta, "fetch me")

	// Request asks for hash but capability covers a different one.
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash="+hash, nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, store, vrForHash("different-hash"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- listHandler ---

func TestListHandler_EmptyStore(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	listHandler(w, req, store, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestListHandler_WithObjects(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	stashOne(t, store, meta, "list me #tag")

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	listHandler(w, req, store, meta, vrEmpty())

	var result []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result))
	}
	if result[0]["hash"] == "" {
		t.Error("expected hash in result")
	}
}

// --- queryHandler ---

func TestQueryHandler_ByTag(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	hash := stashOne(t, store, meta, "query me #ideas")

	req := httptest.NewRequest(http.MethodGet, "/query?index=tag&key=ideas", nil)
	w := httptest.NewRecorder()
	queryHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 || result[0] != hash {
		t.Errorf("expected [%s], got %v", hash, result)
	}
}

func TestQueryHandler_MissingParams(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/query?index=tag", nil)
	w := httptest.NewRecorder()
	queryHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- namespacesHandler ---

func TestNamespacesHandler_Empty(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/namespaces", nil)
	w := httptest.NewRecorder()
	namespacesHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected empty namespaces, got %v", result)
	}
}

func TestNamespacesHandler_WithNames(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")
	meta.AppendNameCreate("alice/her-doc", "bbccdd112233445566778899001122bb")

	req := httptest.NewRequest(http.MethodGet, "/namespaces", nil)
	w := httptest.NewRecorder()
	namespacesHandler(w, req, meta, vrEmpty())

	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Errorf("expected 2 namespaces, got %v", result)
	}
}

// --- namesHandler ---

func TestNamesHandler_MissingNamespace(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/names", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNamesHandler_ReturnsNamesInNamespace(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")
	meta.AppendNameCreate("bob/other-doc", "bbccdd112233445566778899001122bb")
	meta.AppendNameCreate("alice/her-doc", "ccddee223344556677889900112233cc")

	req := httptest.NewRequest(http.MethodGet, "/names?namespace=bob", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta, vrEmpty())

	var result []metadata.NameEntry
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Errorf("expected 2 names in bob namespace, got %d", len(result))
	}
}

func TestNamesHandler_PrefixStripped(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")

	req := httptest.NewRequest(http.MethodGet, "/names?namespace=bob", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta, vrEmpty())

	var result []metadata.NameEntry
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 || result[0].Label != "my-doc" {
		t.Errorf("expected label 'my-doc' without prefix, got %v", result)
	}
}

// --- nameHandler ---

func TestNameHandler_CreateNew(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"

	req := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=my-doc&hash="+hash, nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta, vrForHash(hash))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "bob/my-doc") {
		t.Errorf("expected full label in response, got %q", w.Body.String())
	}
}

func TestNameHandler_UpdateExisting(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	hash := "bbccdd112233445566778899001122bb"
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")

	req := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=my-doc&hash="+hash, nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta, vrForHash(hash))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	results := meta.Query("name", "bob/my-doc")
	if len(results) != 1 || results[0] != hash {
		t.Errorf("expected updated hash, got %v", results)
	}
}

func TestNameHandler_MissingParams(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/name?label=my-doc", nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNameHandler_WrongHash(t *testing.T) {
	_, _, _, meta := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"

	req := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=my-doc&hash="+hash, nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta, vrForHash("different-hash"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// --- collectionHandler ---

func TestCollectionHandler_Success(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	body := `["aabbcc001122334455667788990011aa","bbccdd112233445566778899001122bb"]`
	req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader(body))
	w := httptest.NewRecorder()
	collectionHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	if len(result.Hash) != 32 {
		t.Errorf("expected 32-char hash, got %q", result.Hash)
	}
	if result.Capability.Hash != result.Hash {
		t.Errorf("expected capability to cover %q, got %q", result.Hash, result.Capability.Hash)
	}
}

func TestCollectionHandler_InvalidBody(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	collectionHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- relationHandler ---

func TestRelationHandler_Success(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	from := stashOne(t, store, meta, "from object #source")
	to := stashOne(t, store, meta, "to object #target")

	body := `{"from":"` + from + `","rel":"contextualizes","to":"` + to + `"}`
	req := httptest.NewRequest(http.MethodPost, "/relation", strings.NewReader(body))
	w := httptest.NewRecorder()
	relationHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	if len(result.Hash) != 32 {
		t.Errorf("expected 32-char hash, got %q", result.Hash)
	}
}

func TestRelationHandler_MissingParams(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	body := `{"from":"abc","rel":"contextualizes"}` // "to" missing
	req := httptest.NewRequest(http.MethodPost, "/relation", strings.NewReader(body))
	w := httptest.NewRecorder()
	relationHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRelationHandler_MalformedBody(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/relation", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	relationHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRelationHandler_WrongMethod(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/relation", nil)
	w := httptest.NewRecorder()
	relationHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- relationsHandler ---

func TestRelationsHandler_NoRelations(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	hash := stashOne(t, store, meta, "lonely object")

	req := httptest.NewRequest(http.MethodGet, "/relations?hash="+hash, nil)
	w := httptest.NewRecorder()
	relationsHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string][]metadata.RelationPayload
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result["outgoing"]) != 0 || len(result["incoming"]) != 0 {
		t.Errorf("expected empty relations, got %v", result)
	}
}

func TestRelationsHandler_WithRelations(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	from := stashOne(t, store, meta, "from object")
	to := stashOne(t, store, meta, "to object")

	relBody := `{"from":"` + from + `","rel":"contextualizes","to":"` + to + `"}`
	relReq := httptest.NewRequest(http.MethodPost, "/relation", strings.NewReader(relBody))
	relW := httptest.NewRecorder()
	relationHandler(relW, relReq, store, meta, serverTestKey, testConfig(), vrEmpty())
	if relW.Code != http.StatusCreated {
		t.Fatalf("setup: failed to create relation: %d %s", relW.Code, relW.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/relations?hash="+from, nil)
	w := httptest.NewRecorder()
	relationsHandler(w, req, meta, vrEmpty())

	var result map[string][]metadata.RelationPayload
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result["outgoing"]) != 1 {
		t.Errorf("expected 1 outgoing relation, got %d", len(result["outgoing"]))
	}
	if result["outgoing"][0].Rel != "contextualizes" {
		t.Errorf("expected rel 'contextualizes', got %q", result["outgoing"][0].Rel)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/relations?hash="+to, nil)
	w2 := httptest.NewRecorder()
	relationsHandler(w2, req2, meta, vrEmpty())

	var result2 map[string][]metadata.RelationPayload
	json.Unmarshal(w2.Body.Bytes(), &result2)
	if len(result2["incoming"]) != 1 {
		t.Errorf("expected 1 incoming relation, got %d", len(result2["incoming"]))
	}
}

func TestRelationsHandler_MissingHash(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/relations", nil)
	w := httptest.NewRecorder()
	relationsHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- tagsHandler ---

func TestTagsHandler_Empty(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	tagsHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected empty tags, got %v", result)
	}
}

func TestTagsHandler_WithTags(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	stashOne(t, store, meta, "content with #ideas and #notes")

	req := httptest.NewRequest(http.MethodGet, "/tags", nil)
	w := httptest.NewRecorder()
	tagsHandler(w, req, meta, vrEmpty())

	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Errorf("expected 2 tags, got %v", result)
	}
}

func TestTagsHandler_WrongMethod(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/tags", nil)
	w := httptest.NewRecorder()
	tagsHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- datesHandler ---

func TestDatesHandler_Empty(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/dates", nil)
	w := httptest.NewRecorder()
	datesHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 0 {
		t.Errorf("expected empty dates, got %v", result)
	}
}

func TestDatesHandler_WithDates(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	stashOne(t, store, meta, "content stashed today")

	req := httptest.NewRequest(http.MethodGet, "/dates", nil)
	w := httptest.NewRecorder()
	datesHandler(w, req, meta, vrEmpty())

	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 {
		t.Fatalf("expected 1 date, got %v", result)
	}
	// Format sanity check: YYYY-MM-DD, 10 characters.
	if len(result[0]) != 10 {
		t.Errorf("expected YYYY-MM-DD formatted date, got %q", result[0])
	}
}

func TestDatesHandler_WrongMethod(t *testing.T) {
	_, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/dates", nil)
	w := httptest.NewRecorder()
	datesHandler(w, req, meta, vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// TestDatesAndQueryHandler_RoundTrip confirms /dates and
// /query?index=date&key=<date> work together — /dates tells the client
// which dates have content, /query fetches what was stashed on one.
func TestDatesAndQueryHandler_RoundTrip(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	hash := stashOne(t, store, meta, "content stashed today")

	datesReq := httptest.NewRequest(http.MethodGet, "/dates", nil)
	datesW := httptest.NewRecorder()
	datesHandler(datesW, datesReq, meta, vrEmpty())

	var dates []string
	json.Unmarshal(datesW.Body.Bytes(), &dates)
	if len(dates) != 1 {
		t.Fatalf("expected 1 date, got %v", dates)
	}

	queryReq := httptest.NewRequest(http.MethodGet, "/query?index=date&key="+dates[0], nil)
	queryW := httptest.NewRecorder()
	queryHandler(queryW, queryReq, meta, vrEmpty())

	var hashes []string
	json.Unmarshal(queryW.Body.Bytes(), &hashes)
	if len(hashes) != 1 || hashes[0] != hash {
		t.Errorf("expected [%s], got %v", hash, hashes)
	}
}

// --- exportHandler ---

func TestExportHandler_MissingSource(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportHandler_ReturnsGzip(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/export?source=bob", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "gzip") && !strings.Contains(ct, "octet-stream") {
		t.Errorf("expected gzip content type, got %q", ct)
	}
}

func TestExportHandler_WrongMethod(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/export?source=bob", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- importHandler ---

func TestImportHandler_Success(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	archive := makeArchive(t, "bob")
	req := httptest.NewRequest(http.MethodPost, "/import", bytes.NewReader(archive))
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "import successful") {
		t.Errorf("expected success message, got %q", w.Body.String())
	}
}

func TestImportHandler_WrongMethod(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/import", nil)
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestImportHandler_InvalidArchive(t *testing.T) {
	_, objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader("not a tar.gz"))
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath, vrEmpty())

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	store, objPath, metaPath, meta := newTestEnv(t)

	hash := stashOne(t, store, meta, "round trip content")

	nameReq := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=roundtrip&hash="+hash, nil)
	nameW := httptest.NewRecorder()
	nameHandler(nameW, nameReq, meta, vrForHash(hash))

	exportReq := httptest.NewRequest(http.MethodGet, "/export?source=bob", nil)
	exportW := httptest.NewRecorder()
	exportHandler(exportW, exportReq, objPath, metaPath, vrEmpty())

	_, objPath2, metaPath2, _ := newTestEnv(t)
	importReq := httptest.NewRequest(http.MethodPost, "/import",
		bytes.NewReader(exportW.Body.Bytes()))
	importW := httptest.NewRecorder()
	importHandler(importW, importReq, objPath2, metaPath2, vrEmpty())

	if importW.Code != http.StatusOK {
		t.Fatalf("import failed: %s", importW.Body.String())
	}

	shard := hash[0:2]
	file := hash[2:]
	if _, err := os.Stat(filepath.Join(objPath2, shard, file)); err != nil {
		t.Error("expected object to exist in destination after round trip")
	}
}
