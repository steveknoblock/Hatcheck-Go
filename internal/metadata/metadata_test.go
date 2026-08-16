package metadata

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// newTestStore creates a Store with all four indexes backed by a temp directory.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	store, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return store
}

// --- ParseTags ---

func TestParseTags_Basic(t *testing.T) {
	tags := ParseTags("This is about #ideas and #go development.")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
	if tags[0] != "ideas" || tags[1] != "go" {
		t.Errorf("expected [ideas go], got %v", tags)
	}
}

func TestParseTags_Uppercase(t *testing.T) {
	tags := ParseTags("Some #Ideas and #GO tags.")
	for _, tag := range tags {
		for _, c := range tag {
			if c >= 'A' && c <= 'Z' {
				t.Errorf("tag %q contains uppercase character", tag)
			}
		}
	}
}

func TestParseTags_Deduplicated(t *testing.T) {
	tags := ParseTags("#ideas and more #ideas and #IDEAS")
	if len(tags) != 1 {
		t.Errorf("expected 1 unique tag, got %d: %v", len(tags), tags)
	}
}

func TestParseTags_NoTags(t *testing.T) {
	tags := ParseTags("No hashtags here at all.")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d: %v", len(tags), tags)
	}
}

func TestParseTags_Empty(t *testing.T) {
	tags := ParseTags("")
	if len(tags) != 0 {
		t.Errorf("expected 0 tags for empty string, got %d", len(tags))
	}
}

func TestParseTags_Underscore(t *testing.T) {
	tags := ParseTags("Tagged as #my_project today.")
	if len(tags) != 1 || tags[0] != "my_project" {
		t.Errorf("expected [my_project], got %v", tags)
	}
}

// --- New ---

func TestNew_EmptyStore(t *testing.T) {
	store := newTestStore(t)
	if len(store.Log) != 0 {
		t.Errorf("expected empty log, got %d entries", len(store.Log))
	}
}

func TestNew_RegistersIndexes(t *testing.T) {
	store := newTestStore(t)
	names := store.IndexNames()
	if len(names) != 4 {
		t.Fatalf("expected 4 indexes, got %d: %v", len(names), names)
	}
}

func TestNew_PersistsAcrossReload(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if err := store.AppendStash("abc123", 42, "Hello #world"); err != nil {
		t.Fatalf("AppendStash() error: %v", err)
	}

	store2, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() reload error: %v", err)
	}
	if len(store2.Log) != 1 {
		t.Fatalf("expected 1 log entry after reload, got %d", len(store2.Log))
	}
	if store2.Log[0].Op != OpStash {
		t.Errorf("expected op %q, got %q", OpStash, store2.Log[0].Op)
	}
}

func TestNew_RebuildIndexesOnReload(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, _ := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	store.AppendStash("hash1", 10, "Note #ideas")

	store2, _ := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	results := store2.Query("tag", "ideas")
	if len(results) != 1 {
		t.Errorf("expected tag index rebuilt after reload, got %d results", len(results))
	}
}

// --- AppendStash ---

func TestAppendStash_AddsToLog(t *testing.T) {
	store := newTestStore(t)

	if err := store.AppendStash("hash1", 10, "Content with #ideas"); err != nil {
		t.Fatalf("AppendStash() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Op != OpStash {
		t.Errorf("expected op %q, got %q", OpStash, store.Log[0].Op)
	}
}

func TestAppendStash_ExtractsTags(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "Content with #ideas and #notes")

	tags := store.TagsForHash("hash1")
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
}

func TestAppendStash_SameHashTwice(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "First #draft")
	store.AppendStash("hash1", 10, "Second #draft #published")

	if len(store.Log) != 2 {
		t.Errorf("expected 2 log entries, got %d", len(store.Log))
	}
}

// --- AppendCollection ---

func TestAppendCollection_AddsToLog(t *testing.T) {
	store := newTestStore(t)

	err := store.AppendCollection("col1", []string{"hash1", "hash2"})
	if err != nil {
		t.Fatalf("AppendCollection() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Op != OpCollection {
		t.Errorf("expected op %q, got %q", OpCollection, store.Log[0].Op)
	}
}

// --- AppendRelation ---

func TestAppendRelation_AddsToLog(t *testing.T) {
	store := newTestStore(t)

	err := store.AppendRelation("rel1", "hash1", "contextualizes", "hash2")
	if err != nil {
		t.Fatalf("AppendRelation() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Op != OpRelation {
		t.Errorf("expected op %q, got %q", OpRelation, store.Log[0].Op)
	}
}

func TestAppendRelation_QueryByFrom(t *testing.T) {
	store := newTestStore(t)
	store.AppendRelation("rel1", "hash1", "contextualizes", "hash2")
	store.AppendRelation("rel2", "hash1", "similar-to", "hash3")

	results := store.Query("relation", "from:hash1")
	if len(results) != 2 {
		t.Errorf("expected 2 relations from hash1, got %d: %v", len(results), results)
	}
}

func TestAppendRelation_QueryByTo(t *testing.T) {
	store := newTestStore(t)
	store.AppendRelation("rel1", "hash1", "contextualizes", "hash2")
	store.AppendRelation("rel2", "hash3", "contextualizes", "hash2")

	results := store.Query("relation", "to:hash2")
	if len(results) != 2 {
		t.Errorf("expected 2 relations to hash2, got %d: %v", len(results), results)
	}
}

func TestAppendRelation_QueryByRel(t *testing.T) {
	store := newTestStore(t)
	store.AppendRelation("rel1", "hash1", "contextualizes", "hash2")
	store.AppendRelation("rel2", "hash3", "similar-to", "hash4")
	store.AppendRelation("rel3", "hash5", "contextualizes", "hash6")

	results := store.Query("relation", "rel:contextualizes")
	if len(results) != 2 {
		t.Errorf("expected 2 contextualizes relations, got %d: %v", len(results), results)
	}
}

// --- AppendNameCreate / AppendNameUpdate ---

func TestAppendNameCreate_AddsToLog(t *testing.T) {
	store := newTestStore(t)

	err := store.AppendNameCreate("my-doc", "hash1")
	if err != nil {
		t.Fatalf("AppendNameCreate() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Op != OpNameCreate {
		t.Errorf("expected op %q, got %q", OpNameCreate, store.Log[0].Op)
	}
}

func TestAppendNameCreate_RejectsDuplicate(t *testing.T) {
	store := newTestStore(t)
	store.AppendNameCreate("my-doc", "hash1")

	err := store.AppendNameCreate("my-doc", "hash2")
	if err == nil {
		t.Error("expected error creating duplicate name, got nil")
	}
}

func TestAppendNameUpdate_UpdatesPointer(t *testing.T) {
	store := newTestStore(t)
	store.AppendNameCreate("my-doc", "hash1")

	err := store.AppendNameUpdate("my-doc", "hash2")
	if err != nil {
		t.Fatalf("AppendNameUpdate() error: %v", err)
	}

	results := store.Query("name", "my-doc")
	if len(results) != 1 || results[0] != "hash2" {
		t.Errorf("expected name to point to hash2, got %v", results)
	}
}

func TestAppendNameUpdate_RejectsUnknownName(t *testing.T) {
	store := newTestStore(t)

	err := store.AppendNameUpdate("nonexistent", "hash1")
	if err == nil {
		t.Error("expected error updating nonexistent name, got nil")
	}
}

func TestAppendNameUpdate_HistoryPreservedInLog(t *testing.T) {
	store := newTestStore(t)
	store.AppendNameCreate("my-doc", "hash1")
	store.AppendNameUpdate("my-doc", "hash2")
	store.AppendNameUpdate("my-doc", "hash3")

	// Log should have all three entries.
	if len(store.Log) != 3 {
		t.Errorf("expected 3 log entries, got %d", len(store.Log))
	}

	// Index reflects most recent.
	results := store.Query("name", "my-doc")
	if len(results) != 1 || results[0] != "hash3" {
		t.Errorf("expected name to point to hash3, got %v", results)
	}
}

// --- Query ---

func TestQuery_ByTag(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "Note #ideas")
	store.AppendStash("hash2", 10, "Another #ideas note")
	store.AppendStash("hash3", 10, "Unrelated")

	results := store.Query("tag", "ideas")
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(results), results)
	}
}

func TestQuery_ByDate(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "Today")
	store.AppendStash("hash2", 10, "Also today")

	today := time.Now().UTC().Format("2006-01-02")
	results := store.Query("date", today)
	if len(results) != 2 {
		t.Errorf("expected 2 results for today, got %d", len(results))
	}
}

func TestQuery_ByName(t *testing.T) {
	store := newTestStore(t)
	store.AppendNameCreate("my-doc", "hash1")

	results := store.Query("name", "my-doc")
	if len(results) != 1 || results[0] != "hash1" {
		t.Errorf("expected [hash1], got %v", results)
	}
}

func TestQuery_UnknownIndex(t *testing.T) {
	store := newTestStore(t)
	results := store.Query("nonexistent", "key")
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown index, got %d", len(results))
	}
}

// --- TagsForHash ---

func TestTagsForHash_ReturnsMostRecent(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "First #draft")
	store.AppendStash("hash1", 10, "Second #draft #published")

	tags := store.TagsForHash("hash1")
	if len(tags) != 2 {
		t.Errorf("expected 2 tags from most recent entry, got %d: %v", len(tags), tags)
	}
}

func TestTagsForHash_UnknownHash(t *testing.T) {
	store := newTestStore(t)
	tags := store.TagsForHash("nonexistent")
	if len(tags) != 0 {
		t.Errorf("expected empty slice for unknown hash, got %v", tags)
	}
}

func TestTagsForHash_IgnoresNonStashEntries(t *testing.T) {
	store := newTestStore(t)
	store.AppendStash("hash1", 10, "Content #ideas")
	store.AppendNameCreate("my-doc", "hash1")

	// TagsForHash should still find the stash entry, not be confused by the name entry.
	tags := store.TagsForHash("hash1")
	if len(tags) != 1 || tags[0] != "ideas" {
		t.Errorf("expected [ideas], got %v", tags)
	}
}

// --- AppendBatch ---

func TestAppendBatch_AddsAllEntriesTogether(t *testing.T) {
	store := newTestStore(t)

	relPayload1, _ := json.Marshal(RelationPayload{Hash: "rel1", From: "item1", Rel: "is-context-for", To: "subject1"})
	relPayload2, _ := json.Marshal(RelationPayload{Hash: "rel2", From: "item2", Rel: "is-context-for", To: "subject1"})
	colPayload, _ := json.Marshal(CollectionPayload{Hash: "ctx1", Hashes: []string{"rel1", "rel2"}})

	entries := []Entry{
		{Op: OpRelation, Created: time.Now().UTC(), Payload: relPayload1},
		{Op: OpRelation, Created: time.Now().UTC(), Payload: relPayload2},
		{Op: OpCollection, Created: time.Now().UTC(), Payload: colPayload},
	}

	if err := store.AppendBatch(entries); err != nil {
		t.Fatalf("AppendBatch() error: %v", err)
	}
	if len(store.Log) != 3 {
		t.Fatalf("expected 3 log entries in memory, got %d", len(store.Log))
	}

	// Reload from disk to confirm the whole batch was actually persisted,
	// not just held in memory.
	reloaded, err := New(store.logPath[:len(store.logPath)-len("/log.json")], &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if len(reloaded.Log) != 3 {
		t.Fatalf("expected 3 entries after reload, got %d", len(reloaded.Log))
	}
	if reloaded.Log[2].Op != OpCollection {
		t.Errorf("expected last entry to be the collection, got %q", reloaded.Log[2].Op)
	}
}

func TestAppendBatch_EmptyIsNoOp(t *testing.T) {
	store := newTestStore(t)

	if err := store.AppendBatch(nil); err != nil {
		t.Fatalf("AppendBatch(nil) error: %v", err)
	}
	if len(store.Log) != 0 {
		t.Fatalf("expected no entries, got %d", len(store.Log))
	}
}

func TestAppendBatch_UpdatesIndexes(t *testing.T) {
	store := newTestStore(t)

	relPayload, _ := json.Marshal(RelationPayload{Hash: "rel1", From: "item1", Rel: "is-context-for", To: "subject1"})
	colPayload, _ := json.Marshal(CollectionPayload{Hash: "ctx1", Hashes: []string{"rel1"}})

	entries := []Entry{
		{Op: OpRelation, Created: time.Now().UTC(), Payload: relPayload},
		{Op: OpCollection, Created: time.Now().UTC(), Payload: colPayload},
	}
	if err := store.AppendBatch(entries); err != nil {
		t.Fatalf("AppendBatch() error: %v", err)
	}

	// RelationIndex should see the batched relation without a separate
	// AppendRelation call — confirms indexes are updated for every entry
	// in the batch, not just the last one.
	results := store.Query("relation", "to:subject1")
	if len(results) != 1 {
		t.Fatalf("expected 1 relation indexed for to:subject1, got %d", len(results))
	}
}

// --- NDJSON persistence ---

func TestStore_MigratesLegacyArrayFormatOnLoad(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	legacy := `[
  {"op":"stash","created":"2024-01-01T00:00:00Z","payload":{"hash":"aaa","size":3,"content":"x"}},
  {"op":"stash","created":"2024-01-02T00:00:00Z","payload":{"hash":"bbb","size":3,"content":"y"}}
]`
	if err := os.WriteFile(dir+"/log.json", []byte(legacy), 0644); err != nil {
		t.Fatalf("failed to seed legacy log: %v", err)
	}

	store, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if len(store.Log) != 2 {
		t.Fatalf("expected 2 entries loaded from legacy format, got %d", len(store.Log))
	}

	// The original should be preserved for inspection...
	if _, err := os.Stat(dir + "/log.json.pre-ndjson.bak"); err != nil {
		t.Errorf("expected pre-migration backup to exist: %v", err)
	}

	// ...and the live file should now be one compact JSON object per line.
	migrated, err := os.ReadFile(dir + "/log.json")
	if err != nil {
		t.Fatalf("failed to read migrated log: %v", err)
	}
	lines := bytes.Split(bytes.TrimRight(migrated, "\n"), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines after migration, got %d", len(lines))
	}
	for i, line := range lines {
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Errorf("migrated line %d is not valid standalone JSON: %v", i, err)
		}
	}

	// A subsequent append should be a real append, not another rewrite —
	// confirmed indirectly by re-loading and checking the entry landed.
	store.AppendStash("ccc", 3, "z")
	store2, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() reload error: %v", err)
	}
	if len(store2.Log) != 3 {
		t.Fatalf("expected 3 entries after append + reload, got %d", len(store2.Log))
	}
}

func TestStore_DropsUnparsableTrailingLine(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// A well-formed entry followed by a truncated one, as if the process
	// died mid-write of the second line.
	good := `{"op":"stash","created":"2024-01-01T00:00:00Z","payload":{"hash":"aaa","size":3,"content":"x"}}`
	truncated := `{"op":"stash","created":"2024-01-02T00:00:00Z","payl`
	content := good + "\n" + truncated
	if err := os.WriteFile(dir+"/log.json", []byte(content), 0644); err != nil {
		t.Fatalf("failed to seed log: %v", err)
	}

	store, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{})
	if err != nil {
		t.Fatalf("New() should not fail on a truncated trailing line, got: %v", err)
	}
	if len(store.Log) != 1 {
		t.Fatalf("expected the good entry to load and the truncated one to be dropped, got %d entries", len(store.Log))
	}
}

func TestStore_CorruptMiddleLineFailsToLoad(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	// Corruption in the middle of the file (not the last line) is real
	// data loss risk, not an interrupted write — it should fail loudly
	// rather than be silently dropped.
	corruptMiddle := `{"op":"stash","created":"2024-01-01T00:00:00Z","payl
{"op":"stash","created":"2024-01-02T00:00:00Z","payload":{"hash":"bbb","size":3,"content":"y"}}`
	if err := os.WriteFile(dir+"/log.json", []byte(corruptMiddle), 0644); err != nil {
		t.Fatalf("failed to seed log: %v", err)
	}

	if _, err := New(dir, &TagIndex{}, &DateIndex{}, &NameIndex{}, &RelationIndex{}); err == nil {
		t.Fatal("expected New() to fail on mid-file corruption, got nil error")
	}
}
