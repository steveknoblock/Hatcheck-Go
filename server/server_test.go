package main

import (
	"archive/tar"
	"compress/gzip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// --- Test helpers ---

// newTestEnv creates temp object and metadata directories with a metadata store.
func newTestEnv(t *testing.T) (objPath, metaPath string, meta *metadata.Store) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hatcheck-server-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	objPath = filepath.Join(dir, "objects")
	metaPath = filepath.Join(dir, "metadata")
	os.MkdirAll(objPath, 0755)
	os.MkdirAll(metaPath, 0755)

	meta, err = metadata.New(metaPath,
		&metadata.TagIndex{},
		&metadata.DateIndex{},
		&metadata.NameIndex{},
		&metadata.RelationIndex{},
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

// --- stashHandler ---

func TestStashHandler_Success(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader("hello #world"))
	w := httptest.NewRecorder()
	stashHandler(w, req, objPath, meta)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	hash := strings.TrimSpace(w.Body.String())
	if len(hash) != 32 {
		t.Errorf("expected 32-char hash, got %q", hash)
	}
}

func TestStashHandler_WrongMethod(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/stash", nil)
	w := httptest.NewRecorder()
	stashHandler(w, req, objPath, meta)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- fetchHandler ---

func TestFetchHandler_Success(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	// Stash first.
	stashReq := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader("fetch me"))
	stashW := httptest.NewRecorder()
	stashHandler(stashW, stashReq, objPath, meta)
	hash := strings.TrimSpace(stashW.Body.String())

	// Now fetch.
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash="+hash, nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, objPath)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "fetch me") {
		t.Errorf("expected content in response, got %q", w.Body.String())
	}
}

func TestFetchHandler_MissingHash(t *testing.T) {
	objPath, _, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/fetch", nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, objPath)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestFetchHandler_NotFound(t *testing.T) {
	objPath, _, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=aabbccddeeff00112233445566778899", nil)
	w := httptest.NewRecorder()
	fetchHandler(w, req, objPath)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- listHandler ---

func TestListHandler_EmptyStore(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	listHandler(w, req, objPath, meta)

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
	objPath, _, meta := newTestEnv(t)

	stashReq := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader("list me #tag"))
	stashW := httptest.NewRecorder()
	stashHandler(stashW, stashReq, objPath, meta)

	req := httptest.NewRequest(http.MethodGet, "/list", nil)
	w := httptest.NewRecorder()
	listHandler(w, req, objPath, meta)

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
	objPath, _, meta := newTestEnv(t)

	stashReq := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader("query me #ideas"))
	stashW := httptest.NewRecorder()
	stashHandler(stashW, stashReq, objPath, meta)
	hash := strings.TrimSpace(stashW.Body.String())

	req := httptest.NewRequest(http.MethodGet, "/query?index=tag&key=ideas", nil)
	w := httptest.NewRecorder()
	queryHandler(w, req, meta)

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
	_, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/query?index=tag", nil)
	w := httptest.NewRecorder()
	queryHandler(w, req, meta)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- namespacesHandler ---

func TestNamespacesHandler_Empty(t *testing.T) {
	_, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/namespaces", nil)
	w := httptest.NewRecorder()
	namespacesHandler(w, req, meta)

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
	_, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")
	meta.AppendNameCreate("alice/her-doc", "bbccdd112233445566778899001122bb")

	req := httptest.NewRequest(http.MethodGet, "/namespaces", nil)
	w := httptest.NewRecorder()
	namespacesHandler(w, req, meta)

	var result []string
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Errorf("expected 2 namespaces, got %v", result)
	}
}

// --- namesHandler ---

func TestNamesHandler_MissingNamespace(t *testing.T) {
	_, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/names", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNamesHandler_ReturnsNamesInNamespace(t *testing.T) {
	_, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")
	meta.AppendNameCreate("bob/other-doc", "bbccdd112233445566778899001122bb")
	meta.AppendNameCreate("alice/her-doc", "ccddee223344556677889900112233cc")

	req := httptest.NewRequest(http.MethodGet, "/names?namespace=bob", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta)

	var result []metadata.NameEntry
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Errorf("expected 2 names in bob namespace, got %d", len(result))
	}
}

func TestNamesHandler_PrefixStripped(t *testing.T) {
	_, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")

	req := httptest.NewRequest(http.MethodGet, "/names?namespace=bob", nil)
	w := httptest.NewRecorder()
	namesHandler(w, req, meta)

	var result []metadata.NameEntry
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 1 || result[0].Label != "my-doc" {
		t.Errorf("expected label 'my-doc' without prefix, got %v", result)
	}
}

// --- nameHandler ---

func TestNameHandler_CreateNew(t *testing.T) {
	_, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=my-doc&hash=aabbcc001122334455667788990011aa", nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "bob/my-doc") {
		t.Errorf("expected full label in response, got %q", w.Body.String())
	}
}

func TestNameHandler_UpdateExisting(t *testing.T) {
	_, _, meta := newTestEnv(t)
	meta.AppendNameCreate("bob/my-doc", "aabbcc001122334455667788990011aa")

	req := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=my-doc&hash=bbccdd112233445566778899001122bb", nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify the name now points to the new hash.
	results := meta.Query("name", "bob/my-doc")
	if len(results) != 1 || results[0] != "bbccdd112233445566778899001122bb" {
		t.Errorf("expected updated hash, got %v", results)
	}
}

func TestNameHandler_MissingParams(t *testing.T) {
	_, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/name?label=my-doc", nil)
	w := httptest.NewRecorder()
	nameHandler(w, req, meta)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- collectionHandler ---

func TestCollectionHandler_Success(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	body := `["aabbcc001122334455667788990011aa","bbccdd112233445566778899001122bb"]`
	req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader(body))
	w := httptest.NewRecorder()
	collectionHandler(w, req, objPath, meta)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	hash := strings.TrimSpace(w.Body.String())
	if len(hash) != 32 {
		t.Errorf("expected 32-char hash, got %q", hash)
	}
}

func TestCollectionHandler_InvalidBody(t *testing.T) {
	objPath, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/collection", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	collectionHandler(w, req, objPath, meta)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// --- exportHandler ---

func TestExportHandler_MissingSource(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/export", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportHandler_ReturnsGzip(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/export?source=bob", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "gzip") && !strings.Contains(ct, "octet-stream") {
		t.Errorf("expected gzip content type, got %q", ct)
	}
}

func TestExportHandler_WrongMethod(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/export?source=bob", nil)
	w := httptest.NewRecorder()
	exportHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- importHandler ---

func TestImportHandler_Success(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	archive := makeArchive(t, "bob")
	req := httptest.NewRequest(http.MethodPost, "/import", bytes.NewReader(archive))
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "import successful") {
		t.Errorf("expected success message, got %q", w.Body.String())
	}
}

func TestImportHandler_WrongMethod(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/import", nil)
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestImportHandler_InvalidArchive(t *testing.T) {
	objPath, metaPath, _ := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/import", strings.NewReader("not a tar.gz"))
	w := httptest.NewRecorder()
	importHandler(w, req, objPath, metaPath)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestImportExportRoundTrip(t *testing.T) {
	objPath, metaPath, meta := newTestEnv(t)

	// Stash an object and name it.
	stashReq := httptest.NewRequest(http.MethodPost, "/stash", strings.NewReader("round trip content"))
	stashW := httptest.NewRecorder()
	stashHandler(stashW, stashReq, objPath, meta)
	hash := strings.TrimSpace(stashW.Body.String())

	nameReq := httptest.NewRequest(http.MethodPost,
		"/name?namespace=bob&label=roundtrip&hash="+hash, nil)
	nameW := httptest.NewRecorder()
	nameHandler(nameW, nameReq, meta)

	// Export.
	exportReq := httptest.NewRequest(http.MethodGet, "/export?source=bob", nil)
	exportW := httptest.NewRecorder()
	exportHandler(exportW, exportReq, objPath, metaPath)

	// Import into a fresh environment.
	objPath2, metaPath2, _ := newTestEnv(t)
	importReq := httptest.NewRequest(http.MethodPost, "/import",
		bytes.NewReader(exportW.Body.Bytes()))
	importW := httptest.NewRecorder()
	importHandler(importW, importReq, objPath2, metaPath2)

	if importW.Code != http.StatusOK {
		t.Fatalf("import failed: %s", importW.Body.String())
	}

	// Verify object exists in destination.
	shard := hash[0:2]
	file := hash[2:]
	if _, err := os.Stat(filepath.Join(objPath2, shard, file)); err != nil {
		t.Error("expected object to exist in destination after round trip")
	}
}
