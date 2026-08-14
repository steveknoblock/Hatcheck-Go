package share

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Test helpers ---

// newTestEnv creates a temporary environment with objects and metadata directories.
func newTestEnv(t *testing.T) (objPath, metaPath, dir string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "hatcheck-share-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	objPath = filepath.Join(dir, "objects")
	metaPath = filepath.Join(dir, "metadata")

	if err := os.MkdirAll(objPath, 0755); err != nil {
		t.Fatalf("failed to create objects dir: %v", err)
	}
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		t.Fatalf("failed to create metadata dir: %v", err)
	}

	return objPath, metaPath, dir
}

// writeObject writes a fake CAS object with the given hash and content.
func writeObject(t *testing.T, objPath, hash, content string) {
	t.Helper()
	shard := hash[0:2]
	file := hash[2:]
	shardDir := filepath.Join(objPath, shard)
	if err := os.MkdirAll(shardDir, 0755); err != nil {
		t.Fatalf("failed to create shard dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shardDir, file), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write object: %v", err)
	}
}

// writeLog writes a minimal log file with the given raw entries.
func writeLog(t *testing.T, metaPath string, entries []map[string]interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(metaPath, "log.json"), data, 0644); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}
}

// nameEntry builds a log entry map for a name-create operation.
func nameEntry(label, hash string) map[string]interface{} {
	payload, _ := json.Marshal(map[string]string{"label": label, "hash": hash})
	return map[string]interface{}{
		"op":      "name-create",
		"created": time.Now().UTC(),
		"payload": json.RawMessage(payload),
	}
}

// stashEntry builds a log entry map for a stash operation.
func stashEntry(hash string, tags []string) map[string]interface{} {
	payload, _ := json.Marshal(map[string]interface{}{"hash": hash, "size": 10, "tags": tags})
	return map[string]interface{}{
		"op":      "stash",
		"created": time.Now().UTC(),
		"payload": json.RawMessage(payload),
	}
}

// archiveContains returns the set of file names present in a tar.gz archive.
func archiveContains(t *testing.T, archivePath string) map[string]bool {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to read gzip: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	names := make(map[string]bool)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		names[hdr.Name] = true
	}
	return names
}

// readManifestFromArchive reads and parses the manifest from a tar.gz archive.
func readManifestFromArchive(t *testing.T, archivePath string) Manifest {
	t.Helper()
	f, _ := os.Open(archivePath)
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "manifest.json" {
			data, _ := io.ReadAll(tr)
			var m Manifest
			json.Unmarshal(data, &m)
			return m
		}
	}
	t.Fatal("manifest.json not found in archive")
	return Manifest{}
}

// readLogFromArchive reads the raw metadata/log.json contents from a tar.gz
// archive as a string, for substring assertions against name labels.
func readLogFromArchive(t *testing.T, archivePath string) string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to read gzip: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "metadata/log.json" {
			data, _ := io.ReadAll(tr)
			return string(data)
		}
	}
	t.Fatal("metadata/log.json not found in archive")
	return ""
}

// mustMarshal marshals v to JSON, failing the test on error.
func mustMarshal(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	return data
}

// --- Export tests ---

func TestExport_FullExport(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)

	writeObject(t, objPath, "aabbcc001122334455667788990011aa", "hello world #test")
	writeObject(t, objPath, "bbccdd112233445566778899001122bb", "another object #test")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry("aabbcc001122334455667788990011aa", []string{"test"}),
		stashEntry("bbccdd112233445566778899001122bb", []string{"test"}),
	})

	outPath := filepath.Join(dir, "full.tar.gz")
	err := Export(objPath, metaPath, "bob", "", "", outPath)
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	names := archiveContains(t, outPath)
	if !names["manifest.json"] {
		t.Error("expected manifest.json in archive")
	}
	if !names["metadata/log.json"] {
		t.Error("expected metadata/log.json in archive")
	}
	if !names["objects/aa/bbcc001122334455667788990011aa"] {
		t.Error("expected first object in archive")
	}
	if !names["objects/bb/ccdd112233445566778899001122bb"] {
		t.Error("expected second object in archive")
	}
}

func TestExport_ManifestContents(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	writeObject(t, objPath, "aabbcc001122334455667788990011aa", "content")

	outPath := filepath.Join(dir, "test.tar.gz")
	Export(objPath, metaPath, "alice", "", "", outPath)

	m := readManifestFromArchive(t, outPath)
	if m.Source != "alice" {
		t.Errorf("expected source 'alice', got %q", m.Source)
	}
	if m.Version != manifestVersion {
		t.Errorf("expected version %q, got %q", manifestVersion, m.Version)
	}
	if m.Objects != 1 {
		t.Errorf("expected 1 object, got %d", m.Objects)
	}
	if m.Name != "" {
		t.Errorf("expected empty name for full export, got %q", m.Name)
	}
}

func TestExport_DefaultOutputFilename(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	writeObject(t, objPath, "aabbcc001122334455667788990011aa", "content")

	// Change to temp dir so default output file lands there.
	orig, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(orig)

	err := Export(objPath, metaPath, "bob", "", "", "")
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "bob.tar.gz")); err != nil {
		t.Error("expected bob.tar.gz to be created")
	}
}

func TestExport_EmptyObjectStore(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	outPath := filepath.Join(dir, "empty.tar.gz")

	err := Export(objPath, metaPath, "bob", "", "", outPath)
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	names := archiveContains(t, outPath)
	if !names["manifest.json"] {
		t.Error("expected manifest.json even for empty store")
	}
}

func TestExport_PartialExport_SingleObject(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)

	hash := "aabbcc001122334455667788990011aa"
	otherHash := "bbccdd112233445566778899001122bb"

	writeObject(t, objPath, hash, "my document content")
	writeObject(t, objPath, otherHash, "unrelated content")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(hash, []string{}),
		stashEntry(otherHash, []string{}),
		nameEntry("my-doc", hash),
	})

	outPath := filepath.Join(dir, "partial.tar.gz")
	err := Export(objPath, metaPath, "bob", "my-doc", "", outPath)
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	names := archiveContains(t, outPath)
	if !names["objects/aa/bbcc001122334455667788990011aa"] {
		t.Error("expected named object in archive")
	}
	if names["objects/bb/ccdd112233445566778899001122bb"] {
		t.Error("expected unrelated object to be excluded from partial export")
	}
}

func TestExport_PartialExport_ManifestHasName(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, objPath, hash, "content")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(hash, []string{}),
		nameEntry("my-doc", hash),
	})

	outPath := filepath.Join(dir, "partial.tar.gz")
	Export(objPath, metaPath, "bob", "my-doc", "", outPath)

	m := readManifestFromArchive(t, outPath)
	if m.Name != "my-doc" {
		t.Errorf("expected manifest name 'my-doc', got %q", m.Name)
	}
}

func TestExport_PartialExport_UnknownName(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	outPath := filepath.Join(dir, "out.tar.gz")

	err := Export(objPath, metaPath, "bob", "nonexistent", "", outPath)
	if err == nil {
		t.Error("expected error for unknown name, got nil")
	}
}

func TestExport_NameAndNamespaceMutuallyExclusive(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	outPath := filepath.Join(dir, "out.tar.gz")

	err := Export(objPath, metaPath, "bob", "my-doc", "bob-ns", outPath)
	if err == nil {
		t.Error("expected error when both name and namespace are set, got nil")
	}
}

func TestExport_NamespaceExport_UnionsReachableObjects(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)

	doc1 := "aabbcc001122334455667788990011aa"
	doc2 := "bbccdd112233445566778899001122bb"
	unrelated := "ccddee223344556677889900112233cc"

	writeObject(t, objPath, doc1, "first doc")
	writeObject(t, objPath, doc2, "second doc")
	writeObject(t, objPath, unrelated, "unrelated content")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(doc1, []string{}),
		stashEntry(doc2, []string{}),
		stashEntry(unrelated, []string{}),
		nameEntry("bob/doc-one", doc1),
		nameEntry("bob/doc-two", doc2),
		nameEntry("alice/other-doc", unrelated),
	})

	outPath := filepath.Join(dir, "namespace.tar.gz")
	err := Export(objPath, metaPath, "bob", "", "bob", outPath)
	if err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	names := archiveContains(t, outPath)
	if !names["objects/aa/bbcc001122334455667788990011aa"] {
		t.Error("expected doc1 in namespace export")
	}
	if !names["objects/bb/ccdd112233445566778899001122bb"] {
		t.Error("expected doc2 in namespace export")
	}
	if names["objects/cc/ddee223344556677889900112233cc"] {
		t.Error("expected object from a different namespace to be excluded")
	}
}

func TestExport_NamespaceExport_ManifestHasNamespace(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, objPath, hash, "content")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(hash, []string{}),
		nameEntry("bob/my-doc", hash),
	})

	outPath := filepath.Join(dir, "namespace.tar.gz")
	Export(objPath, metaPath, "bob", "", "bob", outPath)

	m := readManifestFromArchive(t, outPath)
	if m.Namespace != "bob" {
		t.Errorf("expected manifest namespace 'bob', got %q", m.Namespace)
	}
	if m.Name != "" {
		t.Errorf("expected empty manifest name for namespace export, got %q", m.Name)
	}
}

func TestExport_NamespaceExport_LogIncludesAllNamesInNamespace(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	doc1 := "aabbcc001122334455667788990011aa"
	doc2 := "bbccdd112233445566778899001122bb"
	writeObject(t, objPath, doc1, "first doc")
	writeObject(t, objPath, doc2, "second doc")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(doc1, []string{}),
		stashEntry(doc2, []string{}),
		nameEntry("bob/doc-one", doc1),
		nameEntry("bob/doc-two", doc2),
	})

	outPath := filepath.Join(dir, "namespace.tar.gz")
	Export(objPath, metaPath, "bob", "", "bob", outPath)

	logData := readLogFromArchive(t, outPath)
	if !strings.Contains(logData, "bob/doc-one") {
		t.Error("expected 'bob/doc-one' name entry in filtered log")
	}
	if !strings.Contains(logData, "bob/doc-two") {
		t.Error("expected 'bob/doc-two' name entry in filtered log")
	}
}

func TestExport_NamespaceExport_UnknownNamespace(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	outPath := filepath.Join(dir, "out.tar.gz")

	err := Export(objPath, metaPath, "bob", "", "nonexistent", outPath)
	if err == nil {
		t.Error("expected error for unknown namespace, got nil")
	}
}

func TestNamesInNamespace_FiltersByPrefix(t *testing.T) {
	_, metaPath, _ := newTestEnv(t)
	writeLog(t, metaPath, []map[string]interface{}{
		nameEntry("bob/doc-one", "aabbcc001122334455667788990011aa"),
		nameEntry("bob/doc-two", "bbccdd112233445566778899001122bb"),
		nameEntry("alice/other-doc", "ccddee223344556677889900112233cc"),
	})

	names, err := namesInNamespace("bob", metaPath)
	if err != nil {
		t.Fatalf("namesInNamespace() error: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 names in 'bob' namespace, got %d: %v", len(names), names)
	}
}

func TestNamesInNamespace_DeduplicatesUpdatedLabels(t *testing.T) {
	_, metaPath, _ := newTestEnv(t)
	writeLog(t, metaPath, []map[string]interface{}{
		nameEntry("bob/doc-one", "aabbcc001122334455667788990011aa"),
		{
			"op":      "name-update",
			"created": time.Now().UTC(),
			"payload": mustMarshal(t, map[string]string{"label": "bob/doc-one", "hash": "bbccdd112233445566778899001122bb"}),
		},
	})

	names, err := namesInNamespace("bob", metaPath)
	if err != nil {
		t.Fatalf("namesInNamespace() error: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected label to be deduplicated to 1 entry, got %d: %v", len(names), names)
	}
}

// --- Import tests ---

func TestImport_RoundTrip(t *testing.T) {
	// Set up source environment.
	srcObj, srcMeta, srcDir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, srcObj, hash, "hello world")
	writeLog(t, srcMeta, []map[string]interface{}{
		stashEntry(hash, []string{}),
		nameEntry("my-doc", hash),
	})

	archivePath := filepath.Join(srcDir, "export.tar.gz")
	if err := Export(srcObj, srcMeta, "bob", "", "", archivePath); err != nil {
		t.Fatalf("Export() error: %v", err)
	}

	// Set up destination environment.
	dstObj, dstMeta, _ := newTestEnv(t)

	if err := Import(archivePath, dstObj, dstMeta); err != nil {
		t.Fatalf("Import() error: %v", err)
	}

	// Verify object exists in destination.
	objPath := filepath.Join(dstObj, "aa", "bbcc001122334455667788990011aa")
	if _, err := os.Stat(objPath); err != nil {
		t.Error("expected object to exist in destination after import")
	}
}

func TestImport_NamePrefixedWithSource(t *testing.T) {
	srcObj, srcMeta, srcDir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, srcObj, hash, "content")
	writeLog(t, srcMeta, []map[string]interface{}{
		stashEntry(hash, []string{}),
		nameEntry("my-doc", hash),
	})

	archivePath := filepath.Join(srcDir, "export.tar.gz")
	Export(srcObj, srcMeta, "bob", "", "", archivePath)

	dstObj, dstMeta, _ := newTestEnv(t)
	Import(archivePath, dstObj, dstMeta)

	// Read destination log and verify name label is prefixed.
	logData, err := os.ReadFile(filepath.Join(dstMeta, "log.json"))
	if err != nil {
		t.Fatalf("failed to read destination log: %v", err)
	}

	if !strings.Contains(string(logData), "bob/my-doc") {
		t.Error("expected name label to be prefixed with 'bob/' in destination log")
	}
}

func TestImport_SkipsDuplicateObjects(t *testing.T) {
	srcObj, srcMeta, srcDir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, srcObj, hash, "original content")
	writeLog(t, srcMeta, []map[string]interface{}{
		stashEntry(hash, []string{}),
	})

	archivePath := filepath.Join(srcDir, "export.tar.gz")
	Export(srcObj, srcMeta, "bob", "", "", archivePath)

	// Pre-populate destination with different content at the same hash path.
	dstObj, dstMeta, _ := newTestEnv(t)
	writeObject(t, dstObj, hash, "destination content")

	Import(archivePath, dstObj, dstMeta)

	// Verify destination content was not overwritten.
	data, _ := os.ReadFile(filepath.Join(dstObj, "aa", "bbcc001122334455667788990011aa"))
	if string(data) != "destination content" {
		t.Error("expected existing object to not be overwritten on import")
	}
}

func TestImport_MergesIntoExistingLog(t *testing.T) {
	srcObj, srcMeta, srcDir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, srcObj, hash, "content")
	writeLog(t, srcMeta, []map[string]interface{}{
		stashEntry(hash, []string{}),
	})

	archivePath := filepath.Join(srcDir, "export.tar.gz")
	Export(srcObj, srcMeta, "bob", "", "", archivePath)

	// Destination already has one log entry.
	dstObj, dstMeta, _ := newTestEnv(t)
	existingHash := "ccddee223344556677889900112233cc"
	writeLog(t, dstMeta, []map[string]interface{}{
		stashEntry(existingHash, []string{}),
	})

	Import(archivePath, dstObj, dstMeta)

	// Destination log should now have two entries. The merged file is
	// NDJSON (mergeLog's on-disk format), so read it back the same way
	// readLog does rather than assuming the old whole-array shape.
	entries, err := readLog(dstMeta)
	if err != nil {
		t.Fatalf("failed to read merged log: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 log entries after merge, got %d", len(entries))
	}
}

func TestImport_MissingManifest(t *testing.T) {
	_, _, dir := newTestEnv(t)

	// Create a tar.gz without a manifest.
	archivePath := filepath.Join(dir, "bad.tar.gz")
	f, _ := os.Create(archivePath)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.Close()
	gz.Close()
	f.Close()

	dstObj, dstMeta, _ := newTestEnv(t)
	err := Import(archivePath, dstObj, dstMeta)
	if err == nil {
		t.Error("expected error for archive missing manifest, got nil")
	}
}

// --- Traverse / cycle detection tests ---

func TestTraverse_PlainObject(t *testing.T) {
	objPath, _, _ := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	writeObject(t, objPath, hash, "plain text content")

	visited := make(map[string]bool)
	err := traverse(hash, objPath, visited)
	if err != nil {
		t.Fatalf("traverse() error: %v", err)
	}
	if !visited[hash] {
		t.Error("expected hash to be in visited set")
	}
	if len(visited) != 1 {
		t.Errorf("expected 1 visited hash, got %d", len(visited))
	}
}

func TestTraverse_Collection(t *testing.T) {
	objPath, _, _ := newTestEnv(t)

	child1 := "aabbcc001122334455667788990011aa"
	child2 := "bbccdd112233445566778899001122bb"
	writeObject(t, objPath, child1, "child one")
	writeObject(t, objPath, child2, "child two")

	colContent, _ := json.Marshal([]string{child1, child2})
	colHash := "ccddee223344556677889900112233cc"
	writeObject(t, objPath, colHash, string(colContent))

	visited := make(map[string]bool)
	err := traverse(colHash, objPath, visited)
	if err != nil {
		t.Fatalf("traverse() error: %v", err)
	}
	if len(visited) != 3 {
		t.Errorf("expected 3 visited hashes (collection + 2 children), got %d", len(visited))
	}
}

func TestTraverse_CycleDetection(t *testing.T) {
	objPath, _, _ := newTestEnv(t)

	// hash1 is a collection pointing to hash2
	// hash2 is a collection pointing back to hash1 — a cycle
	hash1 := "aabbcc001122334455667788990011aa"
	hash2 := "bbccdd112233445566778899001122bb"

	col1, _ := json.Marshal([]string{hash2})
	col2, _ := json.Marshal([]string{hash1})
	writeObject(t, objPath, hash1, string(col1))
	writeObject(t, objPath, hash2, string(col2))

	visited := make(map[string]bool)
	err := traverse(hash1, objPath, visited)
	if err != nil {
		t.Fatalf("traverse() should not error on cycle, got: %v", err)
	}
	if len(visited) != 2 {
		t.Errorf("expected 2 visited hashes despite cycle, got %d", len(visited))
	}
}

func TestTraverse_Relation(t *testing.T) {
	objPath, _, _ := newTestEnv(t)

	fromHash := "aabbcc001122334455667788990011aa"
	toHash := "bbccdd112233445566778899001122bb"
	writeObject(t, objPath, fromHash, "from object")
	writeObject(t, objPath, toHash, "to object")

	relContent, _ := json.Marshal(map[string]string{
		"from": fromHash,
		"rel":  "contextualizes",
		"to":   toHash,
	})
	relHash := "ccddee223344556677889900112233cc"
	writeObject(t, objPath, relHash, string(relContent))

	visited := make(map[string]bool)
	err := traverse(relHash, objPath, visited)
	if err != nil {
		t.Fatalf("traverse() error: %v", err)
	}
	if len(visited) != 3 {
		t.Errorf("expected 3 visited hashes (relation + from + to), got %d", len(visited))
	}
}
