package metadata

import (
	"reflect"
	"testing"
)

// --- Add / Query ---

func TestRelationIndex_QueryEmpty(t *testing.T) {
	idx := NewRelationIndex()
	for _, key := range []string{"from:a", "to:a", "rel:cites", "bogus"} {
		if got := idx.Query(key); len(got) != 0 {
			t.Errorf("Query(%q) on empty index = %v, want none", key, got)
		}
		if got := idx.QueryRich(key); len(got) != 0 {
			t.Errorf("QueryRich(%q) on empty index = %v, want none", key, got)
		}
	}
}

func TestRelationIndex_QueryByFromToAndRel(t *testing.T) {
	idx := NewRelationIndex()
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))
	idx.Add(relationEntry(t, "r2", "a", "extends", "c"))

	cases := map[string][]string{
		"from:a":       {"r1", "r2"},
		"to:b":         {"r1"},
		"to:c":         {"r2"},
		"rel:cites":    {"r1"},
		"rel:extends":  {"r2"},
		"from:b":       nil,
		"rel:unknown":  nil,
		"no-such-kind": nil,
	}
	for key, want := range cases {
		got := idx.Query(key)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Query(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestRelationIndex_QueryRichReturnsFullPayloads(t *testing.T) {
	idx := NewRelationIndex()
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))

	want := []RelationPayload{{Hash: "r1", From: "a", Rel: "cites", To: "b"}}
	for _, key := range []string{"from:a", "to:b", "rel:cites"} {
		if got := idx.QueryRich(key); !reflect.DeepEqual(got, want) {
			t.Errorf("QueryRich(%q) = %+v, want %+v", key, got, want)
		}
	}
}

func TestRelationIndex_PrefixesDoNotCollide(t *testing.T) {
	// The same string used as a from-hash, a to-hash and a predicate must
	// stay in three separate lookups.
	idx := NewRelationIndex()
	idx.Add(relationEntry(t, "r1", "x", "y", "z"))
	idx.Add(relationEntry(t, "r2", "y", "z", "x"))
	idx.Add(relationEntry(t, "r3", "z", "x", "y"))

	if got, want := idx.Query("from:x"), []string{"r1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(from:x) = %v, want %v", got, want)
	}
	if got, want := idx.Query("to:x"), []string{"r2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(to:x) = %v, want %v", got, want)
	}
	if got, want := idx.Query("rel:x"), []string{"r3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(rel:x) = %v, want %v", got, want)
	}
}

func TestRelationIndex_DuplicateEntriesAreKept(t *testing.T) {
	// Posting an identical relation twice yields the same hash and two log
	// entries; the index has always reported both.
	idx := NewRelationIndex()
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))

	if got, want := idx.Query("from:a"), []string{"r1", "r1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(from:a) = %v, want %v", got, want)
	}
}

func TestRelationIndex_ArrivalOrderIsPreserved(t *testing.T) {
	idx := NewRelationIndex()
	idx.Add(relationEntry(t, "r2", "a", "cites", "c"))
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))

	if got, want := idx.Query("from:a"), []string{"r2", "r1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(from:a) = %v, want %v (log order, not sorted)", got, want)
	}
}

func TestRelationIndex_IgnoresOtherOps(t *testing.T) {
	idx := NewRelationIndex()
	idx.Add(stashEntry(t, "h1", "go"))

	if keys := idx.Keys(); len(keys) != 0 {
		t.Errorf("Keys() = %v, want none after a non-relation op", keys)
	}
}

func TestRelationIndex_Name(t *testing.T) {
	if got := NewRelationIndex().Name(); got != "relation" {
		t.Errorf("Name() = %q, want %q", got, "relation")
	}
}

// --- Through the Store ---

func TestRelationIndex_RelationsForHashThroughStore(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, NewRelationIndex())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.AppendRelation("r1", "a", "cites", "b"); err != nil {
		t.Fatalf("AppendRelation: %v", err)
	}

	out, in := store.RelationsForHash("a")
	if len(out) != 1 || out[0].Hash != "r1" || len(in) != 0 {
		t.Errorf("RelationsForHash(a) = %+v, %+v; want one outgoing r1, no incoming", out, in)
	}
	out, in = store.RelationsForHash("b")
	if len(out) != 0 || len(in) != 1 || in[0].Hash != "r1" {
		t.Errorf("RelationsForHash(b) = %+v, %+v; want no outgoing, one incoming r1", out, in)
	}

	// Reopening replays the log and must rebuild the index.
	reopened, err := New(dir, NewRelationIndex())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if out, _ := reopened.RelationsForHash("a"); len(out) != 1 {
		t.Errorf("after reload RelationsForHash(a) outgoing = %+v, want one", out)
	}
}
