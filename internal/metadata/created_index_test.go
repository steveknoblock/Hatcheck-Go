package metadata

import (
	"encoding/json"
	"testing"
	"time"
)

// --- CreatedIndex ---

func TestCreatedIndex_UnknownHashReturnsZero(t *testing.T) {
	idx := NewCreatedIndex()
	if created := idx.Created("never-seen"); !created.IsZero() {
		t.Errorf("expected zero time for an unknown hash, got %v", created)
	}
}

func TestCreatedIndex_Stash(t *testing.T) {
	idx := NewCreatedIndex()
	when := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	entry := entryForStash(t, "hash1")
	entry.Created = when
	idx.Add(entry)

	if created := idx.Created("hash1"); !created.Equal(when) {
		t.Errorf("expected %v, got %v", when, created)
	}
}

func TestCreatedIndex_Collection(t *testing.T) {
	idx := NewCreatedIndex()
	when := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	entry := entryForCollection(t, "hash1", []string{"a", "b"})
	entry.Created = when
	idx.Add(entry)

	if created := idx.Created("hash1"); !created.Equal(when) {
		t.Errorf("expected %v, got %v", when, created)
	}
}

func TestCreatedIndex_Relation(t *testing.T) {
	idx := NewCreatedIndex()
	when := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	entry := entryForRelation(t, "hash1", "a", "contextualizes", "b")
	entry.Created = when
	idx.Add(entry)

	if created := idx.Created("hash1"); !created.Equal(when) {
		t.Errorf("expected %v, got %v", when, created)
	}
}

func TestCreatedIndex_IgnoresUnrelatedOps(t *testing.T) {
	idx := NewCreatedIndex()
	payload, err := json.Marshal(NameEntry{Label: "irrelevant", Hash: "hash1"})
	if err != nil {
		t.Fatalf("failed to marshal NameEntry: %v", err)
	}
	idx.Add(Entry{Op: OpNameCreate, Payload: payload, Created: time.Now().UTC()})

	if created := idx.Created("hash1"); !created.IsZero() {
		t.Errorf("expected zero time for a name-create entry, got %v", created)
	}
}

func TestCreatedIndex_DifferentHashesIndependent(t *testing.T) {
	idx := NewCreatedIndex()
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	e1 := entryForStash(t, "hash1")
	e1.Created = t1
	e2 := entryForCollection(t, "hash2", []string{"hash1"})
	e2.Created = t2
	e3 := entryForRelation(t, "hash3", "hash1", "contextualizes", "hash2")
	e3.Created = t3

	idx.Add(e1)
	idx.Add(e2)
	idx.Add(e3)

	if created := idx.Created("hash1"); !created.Equal(t1) {
		t.Errorf("expected hash1 = %v, got %v", t1, created)
	}
	if created := idx.Created("hash2"); !created.Equal(t2) {
		t.Errorf("expected hash2 = %v, got %v", t2, created)
	}
	if created := idx.Created("hash3"); !created.Equal(t3) {
		t.Errorf("expected hash3 = %v, got %v", t3, created)
	}
}

func TestCreatedIndex_Name(t *testing.T) {
	idx := NewCreatedIndex()
	if idx.Name() != "created" {
		t.Errorf("expected name 'created', got %q", idx.Name())
	}
}

func TestCreatedIndex_QueryWrapsInSlice(t *testing.T) {
	idx := NewCreatedIndex()
	when := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	entry := entryForStash(t, "hash1")
	entry.Created = when
	idx.Add(entry)

	got := idx.Query("hash1")
	want := "2026-03-15T12:00:00Z"
	if len(got) != 1 || got[0] != want {
		t.Errorf("expected [%q], got %v", want, got)
	}
}

func TestCreatedIndex_QueryUnknownHash(t *testing.T) {
	idx := NewCreatedIndex()
	if got := idx.Query("never-seen"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// --- Store.CreatedAt ---

func TestStore_CreatedAt_UnknownHash(t *testing.T) {
	store, err := New(t.TempDir(), NewCreatedIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if created := store.CreatedAt("never-seen"); !created.IsZero() {
		t.Errorf("expected zero time, got %v", created)
	}
}

func TestStore_CreatedAt_AfterAppendStash(t *testing.T) {
	store, err := New(t.TempDir(), NewCreatedIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	before := time.Now().UTC()
	store.AppendStash("hash1", 10, "some content")
	after := time.Now().UTC()

	created := store.CreatedAt("hash1")
	if created.Before(before) || created.After(after) {
		t.Errorf("expected created time between %v and %v, got %v", before, after, created)
	}
}

func TestStore_CreatedAt_SurvivesReplay(t *testing.T) {
	dir := t.TempDir()

	store1, err := New(dir, NewCreatedIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store1.AppendStash("hash1", 10, "leaf content")
	want := store1.CreatedAt("hash1")

	// Rebuild from the log on disk, as would happen on server restart.
	store2, err := New(dir, NewCreatedIndex())
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	got := store2.CreatedAt("hash1")
	if !got.Equal(want) {
		t.Errorf("expected %v after replay, got %v", want, got)
	}
}
