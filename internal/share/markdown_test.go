package share

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// nameEntryAt is like nameEntry but with an explicit timestamp, for tests
// that need to assert a specific date in front matter.
func nameEntryAt(label, hash string, created time.Time) map[string]interface{} {
	e := nameEntry(label, hash)
	e["created"] = created
	return e
}

// relationEntry builds a log entry map for a relation operation.
func relationEntry(hash, from, rel, to string) map[string]interface{} {
	payload, _ := json.Marshal(map[string]string{"hash": hash, "from": from, "rel": rel, "to": to})
	return map[string]interface{}{
		"op":      "relation",
		"created": time.Now().UTC(),
		"payload": payload,
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(data)
}

func TestExportMarkdown_UnknownNamespace(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	outDir := filepath.Join(dir, "out")

	err := ExportMarkdown(objPath, metaPath, "nonexistent", outDir)
	if err == nil {
		t.Error("expected error for unknown namespace, got nil")
	}
}

func TestExportMarkdown_LeafBecomesFile(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	hash := "aabbcc001122334455667788990011aa"
	created := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)

	writeObject(t, objPath, hash, "Hello, world. #ideas #draft")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(hash, []string{"ideas", "draft"}),
		nameEntryAt("bob/hello world", hash, created),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	path := filepath.Join(outDir, "hello-world.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}

	got := readFile(t, path)
	if !strings.HasPrefix(got, "---\n") {
		t.Fatalf("expected front matter block, got:\n%s", got)
	}
	if !strings.Contains(got, `title: "hello world"`) {
		t.Errorf("expected title in front matter, got:\n%s", got)
	}
	if !strings.Contains(got, "date: 2026-03-15T12:00:00Z") {
		t.Errorf("expected date in front matter, got:\n%s", got)
	}
	if !strings.Contains(got, `tags: ["ideas", "draft"]`) {
		t.Errorf("expected tags in front matter, got:\n%s", got)
	}
	if !strings.Contains(got, "Hello, world. #ideas #draft") {
		t.Errorf("expected body content, got:\n%s", got)
	}
}

func TestExportMarkdown_CollectionBecomesDirectory(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	collectionHash := "cc00112233445566778899001122ccdd"
	leaf1 := "aabbcc001122334455667788990011aa"
	leaf2 := "bbccdd112233445566778899001122bb"

	writeObject(t, objPath, leaf1, "first line")
	writeObject(t, objPath, leaf2, "second line")
	writeObject(t, objPath, collectionHash, `["`+leaf1+`","`+leaf2+`"]`)
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(leaf1, nil),
		stashEntry(leaf2, nil),
		nameEntry("bob/haiku", collectionHash),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	haikuDir := filepath.Join(outDir, "haiku")
	info, err := os.Stat(haikuDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected %s to be a directory: %v", haikuDir, err)
	}

	indexPath := filepath.Join(haikuDir, "_index.md")
	indexContent := readFile(t, indexPath)
	if !strings.Contains(indexContent, `title: "haiku"`) {
		t.Errorf("expected _index.md to have collection's title, got:\n%s", indexContent)
	}

	// Anonymous members (no Name of their own) fall back to shortened hash.
	member1Path := filepath.Join(haikuDir, leaf1[:8]+".md")
	member2Path := filepath.Join(haikuDir, leaf2[:8]+".md")
	if content := readFile(t, member1Path); !strings.Contains(content, "first line") {
		t.Errorf("expected member1 content, got:\n%s", content)
	}
	if content := readFile(t, member2Path); !strings.Contains(content, "second line") {
		t.Errorf("expected member2 content, got:\n%s", content)
	}
}

func TestExportMarkdown_AnonymousMemberHasNoFrontMatter(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	collectionHash := "cc00112233445566778899001122ccdd"
	leaf := "aabbcc001122334455667788990011aa"

	writeObject(t, objPath, leaf, "anonymous content #shouldnotappear")
	writeObject(t, objPath, collectionHash, `["`+leaf+`"]`)
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(leaf, []string{"shouldnotappear"}),
		nameEntry("bob/doc", collectionHash),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	memberPath := filepath.Join(outDir, "doc", leaf[:8]+".md")
	got := readFile(t, memberPath)
	if strings.HasPrefix(got, "---\n") {
		t.Errorf("expected no front matter for anonymous member, got:\n%s", got)
	}
	if !strings.Contains(got, "anonymous content #shouldnotappear") {
		t.Errorf("expected plain content preserved, got:\n%s", got)
	}
}

func TestExportMarkdown_NamedCollectionMemberUsesItsOwnName(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	collectionHash := "cc00112233445566778899001122ccdd"
	leaf := "aabbcc001122334455667788990011aa"
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	writeObject(t, objPath, leaf, "a poem #haiku")
	writeObject(t, objPath, collectionHash, `["`+leaf+`"]`)
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(leaf, []string{"haiku"}),
		nameEntryAt("bob/cicada song", leaf, created), // leaf is ALSO its own top-level name
		nameEntry("bob/collected works", collectionHash),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	// As a top-level name, it's its own file...
	topLevelPath := filepath.Join(outDir, "cicada-song.md")
	if _, err := os.Stat(topLevelPath); err != nil {
		t.Fatalf("expected top-level %s to exist: %v", topLevelPath, err)
	}

	// ...and reused inside the collection, it should use its real name
	// ("cicada-song.md"), not fall back to a shortened hash, and it should
	// carry front matter since it does have a title.
	memberPath := filepath.Join(outDir, "collected-works", "cicada-song.md")
	got := readFile(t, memberPath)
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("expected front matter for a named member, got:\n%s", got)
	}
	if !strings.Contains(got, `title: "cicada song"`) {
		t.Errorf("expected member's own title, got:\n%s", got)
	}
	if !strings.Contains(got, "a poem #haiku") {
		t.Errorf("expected content, got:\n%s", got)
	}

	// The shortened-hash fallback path should NOT exist for a named member.
	fallbackPath := filepath.Join(outDir, "collected-works", leaf[:8]+".md")
	if _, err := os.Stat(fallbackPath); err == nil {
		t.Errorf("did not expect a shortened-hash fallback file for a named member: %s", fallbackPath)
	}
}

func TestExportMarkdown_RelationGetsDescriptiveFile(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	relationHash := "dd00112233445566778899001122ddee"
	from := "aabbcc001122334455667788990011aa"
	to := "bbccdd112233445566778899001122bb"

	writeObject(t, objPath, from, "source doc")
	writeObject(t, objPath, to, "target doc")
	writeObject(t, objPath, relationHash, `{"from":"`+from+`","rel":"contextualizes","to":"`+to+`"}`)
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(from, nil),
		stashEntry(to, nil),
		nameEntryAt("bob/source doc", from, time.Now().UTC()),
		nameEntryAt("bob/target doc", to, time.Now().UTC()),
		nameEntry("bob/the link", relationHash),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	got := readFile(t, filepath.Join(outDir, "the-link.md"))
	if !strings.Contains(got, "Relation: contextualizes") {
		t.Errorf("expected relation type in body, got:\n%s", got)
	}
	if !strings.Contains(got, "From: source doc") || !strings.Contains(got, "To: target doc") {
		t.Errorf("expected from/to resolved to their names, got:\n%s", got)
	}
}

func TestExportMarkdown_SlugCollisionGetsSuffix(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	hash1 := "aabbcc001122334455667788990011aa"
	hash2 := "bbccdd112233445566778899001122bb"

	writeObject(t, objPath, hash1, "first")
	writeObject(t, objPath, hash2, "second")
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(hash1, nil),
		stashEntry(hash2, nil),
		// Same label modulo punctuation -> same slug "my-doc".
		nameEntry("bob/My Doc!", hash1),
		nameEntry("bob/my doc?", hash2),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "my-doc.md")); err != nil {
		t.Errorf("expected my-doc.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "my-doc-2.md")); err != nil {
		t.Errorf("expected collision suffix my-doc-2.md to exist: %v", err)
	}
}

func TestExportMarkdown_SharedContentWrittenAtEveryLocation(t *testing.T) {
	objPath, metaPath, dir := newTestEnv(t)
	sharedLeaf := "aabbcc001122334455667788990011aa"
	collectionA := "cc00112233445566778899001122ccdd"
	collectionB := "dd00112233445566778899001122ddee"

	writeObject(t, objPath, sharedLeaf, "shared content")
	writeObject(t, objPath, collectionA, `["`+sharedLeaf+`"]`)
	writeObject(t, objPath, collectionB, `["`+sharedLeaf+`"]`)
	writeLog(t, metaPath, []map[string]interface{}{
		stashEntry(sharedLeaf, nil),
		nameEntry("bob/collection-a", collectionA),
		nameEntry("bob/collection-b", collectionB),
	})

	outDir := filepath.Join(dir, "out")
	if err := ExportMarkdown(objPath, metaPath, "bob", outDir); err != nil {
		t.Fatalf("ExportMarkdown() error: %v", err)
	}

	// The same anonymous hash is a member of two unrelated top-level
	// collections. Both should get real content, not have the second one
	// come up missing because the hash was "already written" for the
	// first — that dedup logic is correct for the flat CAS archive, but
	// wrong for a markdown file tree.
	pathA := filepath.Join(outDir, "collection-a", sharedLeaf[:8]+".md")
	pathB := filepath.Join(outDir, "collection-b", sharedLeaf[:8]+".md")

	contentA := readFile(t, pathA)
	contentB := readFile(t, pathB)
	if !strings.Contains(contentA, "shared content") {
		t.Errorf("expected shared content at %s, got:\n%s", pathA, contentA)
	}
	if !strings.Contains(contentB, "shared content") {
		t.Errorf("expected shared content at %s, got:\n%s", pathB, contentB)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"browsing the shelves": "browsing-the-shelves",
		"A book gathers dust":  "a-book-gathers-dust",
		"Haiku":                "haiku",
		"  extra   spaces  ":   "extra-spaces",
		"already-slugged_ok":   "already-slugged_ok",
	}
	for input, want := range cases {
		if got := slug(input); got != want {
			t.Errorf("slug(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseTagsForExport(t *testing.T) {
	got := parseTagsForExport("Hello #World, this is #ideas and #World again")
	want := []string{"world", "ideas"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("expected %v, got %v", want, got)
		}
	}
}
