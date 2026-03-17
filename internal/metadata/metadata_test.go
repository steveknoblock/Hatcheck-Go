package metadata

import (
	"os"
	"testing"
	"time"
)

// newTestStore creates a Store backed by a temp directory, cleaned up after the test.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	store, err := New(dir)
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

// --- New / persistence ---

func TestNew_EmptyStore(t *testing.T) {
	store := newTestStore(t)
	if len(store.Log) != 0 {
		t.Errorf("expected empty log, got %d entries", len(store.Log))
	}
}

func TestNew_PersistsAcrossReload(t *testing.T) {
	dir, err := os.MkdirTemp("", "hatcheck-meta-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := New(dir)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := store.Append("abc123", 42, "Hello #world"); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	// Reload from the same directory.
	store2, err := New(dir)
	if err != nil {
		t.Fatalf("New() reload error: %v", err)
	}

	if len(store2.Log) != 1 {
		t.Fatalf("expected 1 log entry after reload, got %d", len(store2.Log))
	}
	if store2.Log[0].Hash != "abc123" {
		t.Errorf("expected hash abc123, got %s", store2.Log[0].Hash)
	}
}

// --- Append ---

func TestAppend_AddsToLog(t *testing.T) {
	store := newTestStore(t)

	if err := store.Append("hash1", 10, "Content with #ideas"); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Hash != "hash1" {
		t.Errorf("expected hash1, got %s", store.Log[0].Hash)
	}
	if store.Log[0].Size != 10 {
		t.Errorf("expected size 10, got %d", store.Log[0].Size)
	}
}

func TestAppend_ExtractsTags(t *testing.T) {
	store := newTestStore(t)

	if err := store.Append("hash1", 10, "Content with #ideas and #notes"); err != nil {
		t.Fatalf("Append() error: %v", err)
	}

	tags := store.Log[0].Tags
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d: %v", len(tags), tags)
	}
}

func TestAppend_SameHashTwice(t *testing.T) {
	store := newTestStore(t)

	store.Append("hash1", 10, "First stash #draft")
	store.Append("hash1", 10, "First stash #draft #published")

	if len(store.Log) != 2 {
		t.Errorf("expected 2 log entries for same hash stashed twice, got %d", len(store.Log))
	}
}

// --- QueryByTag ---

func TestQueryByTag_ReturnsMatches(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Note about #ideas")
	store.Append("hash2", 10, "Another #ideas note")
	store.Append("hash3", 10, "Unrelated content")

	results := store.QueryByTag("ideas")
	if len(results) != 2 {
		t.Errorf("expected 2 results for tag 'ideas', got %d: %v", len(results), results)
	}
}

func TestQueryByTag_CaseInsensitive(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Note about #Ideas")

	results := store.QueryByTag("ideas")
	if len(results) != 1 {
		t.Errorf("expected 1 result querying 'ideas' for content tagged #Ideas, got %d", len(results))
	}
}

func TestQueryByTag_NoMatches(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Note about #ideas")

	results := store.QueryByTag("nonexistent")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestQueryByTag_NoDuplicates(t *testing.T) {
	store := newTestStore(t)
	// Stash the same hash twice with the same tag.
	store.Append("hash1", 10, "Note #ideas")
	store.Append("hash1", 10, "Note #ideas again")

	results := store.QueryByTag("ideas")
	if len(results) != 1 {
		t.Errorf("expected 1 unique result, got %d: %v", len(results), results)
	}
}

// --- QueryByDate ---

func TestQueryByDate_ReturnsMatches(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Today's note")
	store.Append("hash2", 10, "Another note today")

	today := time.Now().UTC().Format("2006-01-02")
	results := store.QueryByDate(today)
	if len(results) != 2 {
		t.Errorf("expected 2 results for today, got %d", len(results))
	}
}

func TestQueryByDate_NoMatches(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Some note")

	results := store.QueryByDate("1999-01-01")
	if len(results) != 0 {
		t.Errorf("expected 0 results for past date, got %d", len(results))
	}
}

// --- QueryByTagAndDate ---

func TestQueryByTagAndDate_Intersection(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Note #ideas today")
	store.Append("hash2", 10, "Note #notes today")
	store.Append("hash3", 10, "Note #ideas today")

	today := time.Now().UTC().Format("2006-01-02")
	results := store.QueryByTagAndDate("ideas", today)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d: %v", len(results), results)
	}
}

func TestQueryByTagAndDate_NoIntersection(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "Note #ideas today")

	results := store.QueryByTagAndDate("ideas", "1999-01-01")
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- TagsForHash ---

func TestTagsForHash_ReturnsMostRecent(t *testing.T) {
	store := newTestStore(t)
	store.Append("hash1", 10, "First version #draft")
	store.Append("hash1", 10, "Second version #draft #published")

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
