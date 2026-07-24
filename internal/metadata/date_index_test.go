package metadata

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Test helpers ---

func entryForStashOnDate(t *testing.T, hash string, created time.Time) Entry {
	t.Helper()
	payload, err := json.Marshal(StashPayload{Hash: hash})
	if err != nil {
		t.Fatalf("failed to marshal StashPayload: %v", err)
	}
	return Entry{Op: OpStash, Created: created, Payload: payload}
}

func mustParseDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("failed to parse date %q: %v", s, err)
	}
	return tm
}

// --- Add / Query ---

func TestDateIndex_QueryEmpty(t *testing.T) {
	idx := NewDateIndex()
	if hashes := idx.Query("2026-01-15"); len(hashes) != 0 {
		t.Errorf("expected no hashes, got %v", hashes)
	}
}

func TestDateIndex_AddAndQuery(t *testing.T) {
	idx := NewDateIndex()
	idx.Add(entryForStashOnDate(t, "hash1", mustParseDate(t, "2026-01-15")))

	hashes := idx.Query("2026-01-15")
	if len(hashes) != 1 || hashes[0] != "hash1" {
		t.Errorf("expected [hash1], got %v", hashes)
	}
}

func TestDateIndex_MultipleHashesSameDate(t *testing.T) {
	idx := NewDateIndex()
	day := mustParseDate(t, "2026-01-15")
	idx.Add(entryForStashOnDate(t, "hash1", day))
	idx.Add(entryForStashOnDate(t, "hash2", day))

	hashes := idx.Query("2026-01-15")
	if len(hashes) != 2 {
		t.Errorf("expected 2 hashes, got %v", hashes)
	}
}

func TestDateIndex_DuplicateStashSameHashIsDeduplicated(t *testing.T) {
	idx := NewDateIndex()
	day := mustParseDate(t, "2026-01-15")
	idx.Add(entryForStashOnDate(t, "hash1", day))
	idx.Add(entryForStashOnDate(t, "hash1", day))

	hashes := idx.Query("2026-01-15")
	if len(hashes) != 1 {
		t.Errorf("expected duplicate stash to collapse to 1 hash, got %v", hashes)
	}
}

func TestDateIndex_DifferentDatesAreIndependent(t *testing.T) {
	idx := NewDateIndex()
	idx.Add(entryForStashOnDate(t, "hash1", mustParseDate(t, "2026-01-15")))
	idx.Add(entryForStashOnDate(t, "hash2", mustParseDate(t, "2026-01-16")))

	if hashes := idx.Query("2026-01-15"); len(hashes) != 1 || hashes[0] != "hash1" {
		t.Errorf("expected [hash1] on 2026-01-15, got %v", hashes)
	}
	if hashes := idx.Query("2026-01-16"); len(hashes) != 1 || hashes[0] != "hash2" {
		t.Errorf("expected [hash2] on 2026-01-16, got %v", hashes)
	}
}

func TestDateIndex_IgnoresNonStashOps(t *testing.T) {
	idx := NewDateIndex()
	payload, _ := json.Marshal(NameEntry{Label: "irrelevant"})
	idx.Add(Entry{Op: OpNameCreate, Created: mustParseDate(t, "2026-01-15"), Payload: payload})

	if dates := idx.Dates(); len(dates) != 0 {
		t.Errorf("expected non-stash ops to be ignored, got dates %v", dates)
	}
}

// --- Dates() ---

func TestDateIndex_DatesEmpty(t *testing.T) {
	idx := NewDateIndex()
	if dates := idx.Dates(); len(dates) != 0 {
		t.Errorf("expected no dates, got %v", dates)
	}
}

func TestDateIndex_DatesSortedMostRecentFirst(t *testing.T) {
	idx := NewDateIndex()
	idx.Add(entryForStashOnDate(t, "hash1", mustParseDate(t, "2026-01-15")))
	idx.Add(entryForStashOnDate(t, "hash2", mustParseDate(t, "2026-03-01")))
	idx.Add(entryForStashOnDate(t, "hash3", mustParseDate(t, "2025-12-25")))

	dates := idx.Dates()
	want := []string{"2026-03-01", "2026-01-15", "2025-12-25"}
	if len(dates) != len(want) {
		t.Fatalf("expected %d dates, got %d: %v", len(want), len(dates), dates)
	}
	for i := range want {
		if dates[i] != want[i] {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, want[i], dates[i], dates)
		}
	}
}

func TestDateIndex_DatesOneEntryPerDateRegardlessOfHashCount(t *testing.T) {
	idx := NewDateIndex()
	day := mustParseDate(t, "2026-01-15")
	idx.Add(entryForStashOnDate(t, "hash1", day))
	idx.Add(entryForStashOnDate(t, "hash2", day))
	idx.Add(entryForStashOnDate(t, "hash3", day))

	dates := idx.Dates()
	if len(dates) != 1 {
		t.Errorf("expected 1 date entry regardless of hash count, got %v", dates)
	}
}

// --- Name() ---

func TestDateIndex_Name(t *testing.T) {
	idx := NewDateIndex()
	if idx.Name() != "date" {
		t.Errorf("expected name 'date', got %q", idx.Name())
	}
}
