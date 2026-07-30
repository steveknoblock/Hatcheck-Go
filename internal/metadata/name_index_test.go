package metadata

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Test helpers ---

func entryForName(t *testing.T, op, label, hash string) Entry {
	t.Helper()
	payload, err := json.Marshal(NamePayload{Label: label, Hash: hash})
	if err != nil {
		t.Fatalf("failed to marshal NamePayload: %v", err)
	}
	return Entry{Op: op, Created: time.Now().UTC(), Payload: payload}
}

// --- ListNamespace ---

func TestNameIndex_ListNamespace_FiltersByPrefix(t *testing.T) {
	idx := NewNameIndex()
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-one", "hash1"))
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-two", "hash2"))
	idx.Add(entryForName(t, OpNameCreate, "alice/other-doc", "hash3"))

	results := idx.ListNamespace("bob/")
	if len(results) != 2 {
		t.Fatalf("expected 2 names in 'bob' namespace, got %d: %v", len(results), results)
	}
}

func TestNameIndex_ListNamespace_StripsPrefix(t *testing.T) {
	idx := NewNameIndex()
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-one", "hash1"))

	results := idx.ListNamespace("bob/")
	if len(results) != 1 || results[0].Label != "doc-one" {
		t.Errorf("expected label 'doc-one' with prefix stripped, got %v", results)
	}
}

func TestNameIndex_ListNamespace_UpdateOverwrites(t *testing.T) {
	idx := NewNameIndex()
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-one", "hash1"))
	idx.Add(entryForName(t, OpNameUpdate, "bob/doc-one", "hash2"))

	results := idx.ListNamespace("bob/")
	if len(results) != 1 || results[0].Hash != "hash2" {
		t.Errorf("expected updated hash 'hash2', got %v", results)
	}
}

// TestNameIndex_ListNamespace_StableOrder guards against a real bug: Go
// deliberately randomizes map iteration order, so returning n.data's
// iteration order directly (as ListNamespace once did) produced a
// different label order on every call. ListNamespace must sort its
// results so repeated calls — and repeated /names requests — return a
// stable, alphabetically ordered list.
func TestNameIndex_ListNamespace_StableOrder(t *testing.T) {
	idx := NewNameIndex()
	labels := []string{
		"bob/zebra", "bob/apple", "bob/mango", "bob/banana", "bob/cherry",
		"bob/date", "bob/elder", "bob/fig", "bob/grape", "bob/honeydew",
	}
	for i, label := range labels {
		idx.Add(entryForName(t, OpNameCreate, label, string(rune('a'+i))))
	}

	want := []string{
		"apple", "banana", "cherry", "date", "elder",
		"fig", "grape", "honeydew", "mango", "zebra",
	}

	// Call repeatedly — since the underlying storage is a map, a single
	// passing call proves nothing about stability; only repeated calls
	// would have caught the original bug.
	for i := 0; i < 20; i++ {
		results := idx.ListNamespace("bob/")
		if len(results) != len(want) {
			t.Fatalf("call %d: expected %d results, got %d", i, len(want), len(results))
		}
		for j, entry := range results {
			if entry.Label != want[j] {
				t.Fatalf("call %d: expected sorted order %v, got order starting %v at index %d (full: %v)",
					i, want, entry.Label, j, results)
			}
		}
	}
}

func TestNameIndex_ListNamespace_Empty(t *testing.T) {
	idx := NewNameIndex()
	results := idx.ListNamespace("bob/")
	if len(results) != 0 {
		t.Errorf("expected no results for empty index, got %v", results)
	}
}

// --- Namespaces ---

func TestNameIndex_Namespaces_ReturnsUniquePrefixes(t *testing.T) {
	idx := NewNameIndex()
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-one", "hash1"))
	idx.Add(entryForName(t, OpNameCreate, "bob/doc-two", "hash2"))
	idx.Add(entryForName(t, OpNameCreate, "alice/other-doc", "hash3"))

	namespaces := idx.Namespaces()
	if len(namespaces) != 2 {
		t.Fatalf("expected 2 unique namespaces, got %d: %v", len(namespaces), namespaces)
	}
}

func TestNameIndex_Namespaces_LabelWithoutSlashIsOwnNamespace(t *testing.T) {
	idx := NewNameIndex()
	idx.Add(entryForName(t, OpNameCreate, "standalone-doc", "hash1"))

	namespaces := idx.Namespaces()
	if len(namespaces) != 1 || namespaces[0] != "standalone-doc" {
		t.Errorf("expected ['standalone-doc'], got %v", namespaces)
	}
}

// TestNameIndex_Namespaces_StableOrder guards against the same
// randomized-map-order bug as ListNamespace (see above), but for the
// /namespaces endpoint and the UI's namespace autocomplete dropdown.
func TestNameIndex_Namespaces_StableOrder(t *testing.T) {
	idx := NewNameIndex()
	prefixes := []string{"zebra", "apple", "mango", "banana", "cherry"}
	for i, ns := range prefixes {
		idx.Add(entryForName(t, OpNameCreate, ns+"/doc", string(rune('a'+i))))
	}

	want := []string{"apple", "banana", "cherry", "mango", "zebra"}

	for i := 0; i < 20; i++ {
		got := idx.Namespaces()
		if len(got) != len(want) {
			t.Fatalf("call %d: expected %d namespaces, got %d", i, len(want), len(got))
		}
		for j := range want {
			if got[j] != want[j] {
				t.Fatalf("call %d: expected sorted order %v, got %v", i, want, got)
			}
		}
	}
}

func TestNameIndex_Namespaces_Empty(t *testing.T) {
	idx := NewNameIndex()
	namespaces := idx.Namespaces()
	if len(namespaces) != 0 {
		t.Errorf("expected no namespaces for empty index, got %v", namespaces)
	}
}
