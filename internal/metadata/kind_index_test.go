package metadata

import (
	"encoding/json"
	"testing"
)

// --- Test helpers ---

func entryForStash(t *testing.T, hash string) Entry {
	t.Helper()
	payload, err := json.Marshal(StashPayload{Hash: hash})
	if err != nil {
		t.Fatalf("failed to marshal StashPayload: %v", err)
	}
	return Entry{Op: OpStash, Payload: payload}
}

func entryForCollection(t *testing.T, hash string, hashes []string) Entry {
	t.Helper()
	payload, err := json.Marshal(CollectionPayload{Hash: hash, Hashes: hashes})
	if err != nil {
		t.Fatalf("failed to marshal CollectionPayload: %v", err)
	}
	return Entry{Op: OpCollection, Payload: payload}
}

func entryForRelation(t *testing.T, hash, from, rel, to string) Entry {
	t.Helper()
	payload, err := json.Marshal(RelationPayload{Hash: hash, From: from, Rel: rel, To: to})
	if err != nil {
		t.Fatalf("failed to marshal RelationPayload: %v", err)
	}
	return Entry{Op: OpRelation, Payload: payload}
}

// --- KindIndex ---

func TestKindIndex_UnknownHashReturnsEmpty(t *testing.T) {
	idx := NewKindIndex()
	if kind := idx.Kind("never-seen"); kind != "" {
		t.Errorf("expected empty string for an unknown hash, got %q", kind)
	}
}

func TestKindIndex_Stash(t *testing.T) {
	idx := NewKindIndex()
	idx.Add(entryForStash(t, "hash1"))
	if kind := idx.Kind("hash1"); kind != "stash" {
		t.Errorf("expected 'stash', got %q", kind)
	}
}

func TestKindIndex_Collection(t *testing.T) {
	idx := NewKindIndex()
	idx.Add(entryForCollection(t, "hash1", []string{"a", "b"}))
	if kind := idx.Kind("hash1"); kind != "collection" {
		t.Errorf("expected 'collection', got %q", kind)
	}
}

func TestKindIndex_Relation(t *testing.T) {
	idx := NewKindIndex()
	idx.Add(entryForRelation(t, "hash1", "a", "contextualizes", "b"))
	if kind := idx.Kind("hash1"); kind != "relation" {
		t.Errorf("expected 'relation', got %q", kind)
	}
}

func TestKindIndex_IgnoresUnrelatedOps(t *testing.T) {
	idx := NewKindIndex()
	payload, _ := json.Marshal(NameEntry{Label: "irrelevant", Hash: "hash1"})
	idx.Add(Entry{Op: OpNameCreate, Payload: payload})

	// NameCreate isn't a creation-of-content op — hash1 here is what a
	// Name points to, not something KindIndex should claim to know about.
	if kind := idx.Kind("hash1"); kind != "" {
		t.Errorf("expected empty string for a name-create entry, got %q", kind)
	}
}

func TestKindIndex_DifferentHashesIndependent(t *testing.T) {
	idx := NewKindIndex()
	idx.Add(entryForStash(t, "hash1"))
	idx.Add(entryForCollection(t, "hash2", []string{"hash1"}))
	idx.Add(entryForRelation(t, "hash3", "hash1", "contextualizes", "hash2"))

	if kind := idx.Kind("hash1"); kind != "stash" {
		t.Errorf("expected hash1 = 'stash', got %q", kind)
	}
	if kind := idx.Kind("hash2"); kind != "collection" {
		t.Errorf("expected hash2 = 'collection', got %q", kind)
	}
	if kind := idx.Kind("hash3"); kind != "relation" {
		t.Errorf("expected hash3 = 'relation', got %q", kind)
	}
}

func TestKindIndex_Name(t *testing.T) {
	idx := NewKindIndex()
	if idx.Name() != "kind" {
		t.Errorf("expected name 'kind', got %q", idx.Name())
	}
}

// --- Store.KindOf ---

func TestStore_KindOf_UnknownHash(t *testing.T) {
	store, err := New(t.TempDir(), NewKindIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	if kind := store.KindOf("never-seen"); kind != "" {
		t.Errorf("expected empty string, got %q", kind)
	}
}

func TestStore_KindOf_AfterAppendStash(t *testing.T) {
	store, err := New(t.TempDir(), NewKindIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.AppendStash("hash1", 10, "some content")

	if kind := store.KindOf("hash1"); kind != "stash" {
		t.Errorf("expected 'stash', got %q", kind)
	}
}

func TestStore_KindOf_AfterAppendCollection(t *testing.T) {
	store, err := New(t.TempDir(), NewKindIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.AppendCollection("hash1", []string{"a", "b"})

	if kind := store.KindOf("hash1"); kind != "collection" {
		t.Errorf("expected 'collection', got %q", kind)
	}
}

func TestStore_KindOf_SurvivesReplay(t *testing.T) {
	dir := t.TempDir()

	store1, err := New(dir, NewKindIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store1.AppendStash("hash1", 10, "leaf content")
	store1.AppendCollection("hash2", []string{"hash1"})

	// Rebuild from the log on disk, as would happen on server restart.
	store2, err := New(dir, NewKindIndex())
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	if kind := store2.KindOf("hash1"); kind != "stash" {
		t.Errorf("expected hash1 = 'stash' after replay, got %q", kind)
	}
	if kind := store2.KindOf("hash2"); kind != "collection" {
		t.Errorf("expected hash2 = 'collection' after replay, got %q", kind)
	}
}
