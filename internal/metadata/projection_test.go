package metadata

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// --- Test projections ---
//
// These mirror the shapes of the hand-written indexes (Tag, Kind, Created,
// Relation) so the tests double as a check that Projection can express
// them.

func newTestTagProjection() *Projection[string] {
	return NewProjection("tag", AppendUnique,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			pairs := make([]Pair[string], 0, len(p.Tags))
			for _, tag := range p.Tags {
				pairs = append(pairs, Pair[string]{Key: tag, Value: p.Hash})
			}
			return pairs
		}),
	).WithQueryKey(strings.ToLower)
}

func newTestKindProjection() *Projection[string] {
	return NewProjection("kind", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "stash"}}
		}),
		On(OpCollection, func(e Entry, p CollectionPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "collection"}}
		}),
		On(OpRelation, func(e Entry, p RelationPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "relation"}}
		}),
	)
}

func newTestRelationProjection() *Projection[RelationPayload] {
	return NewProjection("relation", Append,
		On(OpRelation, func(e Entry, p RelationPayload) []Pair[RelationPayload] {
			return []Pair[RelationPayload]{
				{Key: "from:" + p.From, Value: p},
				{Key: "to:" + p.To, Value: p},
				{Key: "rel:" + p.Rel, Value: p},
			}
		}),
	).WithRender(func(r RelationPayload) string { return r.Hash })
}

func stashEntry(t *testing.T, hash string, tags ...string) Entry {
	t.Helper()
	payload, err := json.Marshal(StashPayload{Hash: hash, Tags: tags})
	if err != nil {
		t.Fatalf("failed to marshal StashPayload: %v", err)
	}
	return Entry{Op: OpStash, Payload: payload}
}

func relationEntry(t *testing.T, hash, from, rel, to string) Entry {
	t.Helper()
	payload, err := json.Marshal(RelationPayload{Hash: hash, From: from, Rel: rel, To: to})
	if err != nil {
		t.Fatalf("failed to marshal RelationPayload: %v", err)
	}
	return Entry{Op: OpRelation, Payload: payload}
}

// --- Basics ---

func TestProjection_NameAndIndexInterface(t *testing.T) {
	var idx Index = newTestTagProjection()
	if idx.Name() != "tag" {
		t.Errorf("Name() = %q, want %q", idx.Name(), "tag")
	}
}

func TestProjection_UnknownKeyReturnsNil(t *testing.T) {
	p := newTestTagProjection()
	if got := p.Query("nope"); got != nil {
		t.Errorf("Query(unknown) = %v, want nil", got)
	}
	if got := p.Get("nope"); got != nil {
		t.Errorf("Get(unknown) = %v, want nil", got)
	}
	if _, ok := p.Value("nope"); ok {
		t.Error("Value(unknown) reported ok")
	}
}

func TestProjection_EmptyProjectionIsSafe(t *testing.T) {
	p := newTestTagProjection()
	if keys := p.Keys(); len(keys) != 0 {
		t.Errorf("Keys() on empty projection = %v, want empty", keys)
	}
	if got := p.Scan(""); len(got) != 0 {
		t.Errorf("Scan on empty projection = %v, want empty", got)
	}
}

// --- AppendUnique mode ---

func TestProjection_AppendUnique_CollectsHashesPerKey(t *testing.T) {
	p := newTestTagProjection()
	p.Add(stashEntry(t, "h1", "go", "cas"))
	p.Add(stashEntry(t, "h2", "go"))

	if got, want := p.Query("go"), []string{"h1", "h2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(go) = %v, want %v", got, want)
	}
	if got, want := p.Query("cas"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(cas) = %v, want %v", got, want)
	}
}

func TestProjection_AppendUnique_SkipsDuplicateValues(t *testing.T) {
	p := newTestTagProjection()
	p.Add(stashEntry(t, "h1", "go"))
	p.Add(stashEntry(t, "h1", "go"))

	if got, want := p.Query("go"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(go) = %v, want %v (duplicate should be skipped)", got, want)
	}
}

func TestProjection_QueryKeyNormalizesLookupOnly(t *testing.T) {
	p := newTestTagProjection()
	p.Add(stashEntry(t, "h1", "go"))

	if got := p.Query("GO"); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Errorf("Query(GO) = %v, want [h1]", got)
	}
	// Stored keys are exactly what the handler emitted.
	if got := p.Keys(); !reflect.DeepEqual(got, []string{"go"}) {
		t.Errorf("Keys() = %v, want [go]", got)
	}
}

// --- Append mode ---

func TestProjection_Append_KeepsDuplicateValuesInOrder(t *testing.T) {
	p := NewProjection("append", Append,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: "all", Value: p.Hash}}
		}),
	)
	p.Add(stashEntry(t, "h1"))
	p.Add(stashEntry(t, "h2"))
	p.Add(stashEntry(t, "h1"))

	if got, want := p.Query("all"), []string{"h1", "h2", "h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(all) = %v, want %v (Append must not dedupe)", got, want)
	}
}

// --- Latest mode ---

func TestProjection_Latest_LaterValueReplacesEarlier(t *testing.T) {
	p := NewProjection("latest", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: e.Created.Format(time.RFC3339)}}
		}),
	)
	first := stashEntry(t, "h1")
	first.Created = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := stashEntry(t, "h1")
	second.Created = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	p.Add(first)
	p.Add(second)

	if got, want := p.Query("h1"), []string{"2026-02-01T00:00:00Z"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(h1) = %v, want %v", got, want)
	}
	if v, ok := p.Value("h1"); !ok || v != "2026-02-01T00:00:00Z" {
		t.Errorf("Value(h1) = %q, %v; want the later value", v, ok)
	}
}

func TestProjection_Latest_MultipleOpsShareOneProjection(t *testing.T) {
	p := newTestKindProjection()
	p.Add(stashEntry(t, "hs"))
	payload, _ := json.Marshal(CollectionPayload{Hash: "hc", Hashes: []string{"hs"}})
	p.Add(Entry{Op: OpCollection, Payload: payload})
	p.Add(relationEntry(t, "hr", "hs", "cites", "hc"))

	for hash, want := range map[string]string{"hs": "stash", "hc": "collection", "hr": "relation"} {
		if got, ok := p.Value(hash); !ok || got != want {
			t.Errorf("Value(%s) = %q, %v; want %q", hash, got, ok, want)
		}
	}
}

// --- Multiple keys per entry (Relation shape) ---

func TestProjection_MultipleKeysPerEntry(t *testing.T) {
	p := newTestRelationProjection()
	p.Add(relationEntry(t, "r1", "a", "cites", "b"))
	p.Add(relationEntry(t, "r2", "a", "cites", "c"))

	cases := map[string][]string{
		"from:a":     {"r1", "r2"},
		"to:b":       {"r1"},
		"to:c":       {"r2"},
		"rel:cites":  {"r1", "r2"},
		"from:b":     nil,
		"rel:absent": nil,
	}
	for key, want := range cases {
		if got := p.Query(key); !reflect.DeepEqual(got, want) {
			t.Errorf("Query(%q) = %v, want %v", key, got, want)
		}
	}

	rich := p.Get("from:a")
	if len(rich) != 2 || rich[0].To != "b" || rich[1].To != "c" {
		t.Errorf("Get(from:a) = %+v, want the two full payloads", rich)
	}
}

// --- Handler behaviour ---

func TestProjection_IgnoresOpsWithoutHandler(t *testing.T) {
	p := newTestTagProjection()
	p.Add(relationEntry(t, "r1", "a", "cites", "b"))
	if keys := p.Keys(); len(keys) != 0 {
		t.Errorf("Keys() = %v, want none for an unhandled op", keys)
	}
}

func TestProjection_SkipsMalformedPayload(t *testing.T) {
	p := newTestTagProjection()
	p.Add(Entry{Op: OpStash, Payload: json.RawMessage(`{not json`)})
	p.Add(stashEntry(t, "h1", "go"))

	if got := p.Query("go"); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Errorf("Query(go) = %v, want [h1]; a bad entry must not break later ones", got)
	}
}

func TestProjection_MultipleHandlersForSameOpRunInOrder(t *testing.T) {
	p := NewProjection("multi", AppendUnique,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: "all", Value: p.Hash}}
		}),
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: "all", Value: p.Hash + "-b"}}
		}),
	)
	p.Add(stashEntry(t, "h1"))

	if got, want := p.Query("all"), []string{"h1", "h1-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(all) = %v, want %v", got, want)
	}
}

func TestProjection_HandlerCanEmitNothing(t *testing.T) {
	p := newTestTagProjection()
	p.Add(stashEntry(t, "h1")) // no tags
	if keys := p.Keys(); len(keys) != 0 {
		t.Errorf("Keys() = %v, want none", keys)
	}
}

func TestProjection_HandlerCanDeriveKeyFromEntryEnvelope(t *testing.T) {
	// Date-index shape: the key comes from the envelope, not the payload.
	p := NewProjection("date", AppendUnique,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: e.Created.UTC().Format("2006-01-02"), Value: p.Hash}}
		}),
	)
	e := stashEntry(t, "h1")
	e.Created = time.Date(2026, 3, 4, 23, 30, 0, 0, time.UTC)
	p.Add(e)

	if got := p.Query("2026-03-04"); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Errorf("Query(2026-03-04) = %v, want [h1]", got)
	}
}

// --- Listing ---

func TestProjection_KeysAreSorted(t *testing.T) {
	p := newTestTagProjection()
	p.Add(stashEntry(t, "h1", "zebra", "apple", "mango"))

	if got, want := p.Keys(), []string{"apple", "mango", "zebra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys() = %v, want %v", got, want)
	}
}

func TestProjection_ScanByPrefix(t *testing.T) {
	p := newTestRelationProjection()
	p.Add(relationEntry(t, "r1", "a", "cites", "b"))
	p.Add(relationEntry(t, "r2", "c", "cites", "b"))

	got := p.Scan("from:")
	if len(got) != 2 || got[0].Key != "from:a" || got[1].Key != "from:c" {
		t.Errorf("Scan(from:) keys = %+v, want [from:a from:c]", got)
	}
	if all := p.Scan(""); len(all) != len(p.Keys()) {
		t.Errorf("Scan(\"\") returned %d entries, want %d", len(all), len(p.Keys()))
	}
	if none := p.Scan("zzz"); len(none) != 0 {
		t.Errorf("Scan(zzz) = %v, want empty", none)
	}
}

// --- Rendering ---

func TestProjection_DefaultRenderForNonStringValues(t *testing.T) {
	p := NewProjection("size", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[int] {
			return []Pair[int]{{Key: p.Hash, Value: p.Size}}
		}),
	)
	payload, _ := json.Marshal(StashPayload{Hash: "h1", Size: 42})
	p.Add(Entry{Op: OpStash, Payload: payload})

	if got := p.Query("h1"); !reflect.DeepEqual(got, []string{"42"}) {
		t.Errorf("Query(h1) = %v, want [42]", got)
	}
}

func TestProjection_CustomRender(t *testing.T) {
	p := NewProjection("created", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[time.Time] {
			return []Pair[time.Time]{{Key: p.Hash, Value: e.Created}}
		}),
	).WithRender(func(t time.Time) string { return t.UTC().Format(time.RFC3339) })

	e := stashEntry(t, "h1")
	e.Created = time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	p.Add(e)

	if got := p.Query("h1"); !reflect.DeepEqual(got, []string{"2026-05-06T07:08:09Z"}) {
		t.Errorf("Query(h1) = %v, want [2026-05-06T07:08:09Z]", got)
	}
}

// --- Integration with Store ---

func TestProjection_WorksAsStoreIndex(t *testing.T) {
	dir := t.TempDir()
	store, err := New(dir, newTestTagProjection())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.AppendStash("h1", 10, "hello #Go #cas"); err != nil {
		t.Fatalf("AppendStash: %v", err)
	}

	if got := store.Query("tag", "go"); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Errorf("Query(tag, go) = %v, want [h1]", got)
	}

	// Reopening replays the log, which must rebuild the projection.
	reopened, err := New(dir, newTestTagProjection())
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.Query("tag", "cas"); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Errorf("after reload Query(tag, cas) = %v, want [h1]", got)
	}
}
