package metadata

// KindIndex maps a hash to the kind of object that created it — "stash",
// "collection", or "relation" — so callers can tell what a hash is
// without fetching and parsing its content. The alternative
// (content-sniffing: fetch the object, try json.Unmarshal, check its
// shape) works — descend() in the UI already does exactly this — but
// costs a full fetch per object. That's fine for one object at a time,
// but too expensive to do for every entry just to pick an icon when
// rendering a whole namespace's worth of names in one list. This makes
// that lookup O(1) instead.
//
// It is a Projection[string] in Latest mode. A hash's kind is fixed
// forever once first recorded — content is addressed by its own hash, so
// the same hash could never later be produced by a different kind of
// operation — so Latest never actually overwrites a differing value.
type KindIndex struct {
	*Projection[string]
}

func NewKindIndex() *KindIndex {
	return &KindIndex{NewProjection("kind", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "stash"}}
		}),
		On(OpCollection, func(e Entry, p CollectionPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "collection"}}
		}),
		On(OpRelation, func(e Entry, p RelationPayload) []Pair[string] {
			return []Pair[string]{{Key: p.Hash, Value: "relation"}}
		}),
	)}
}

// Kind returns the kind of object hash is ("stash", "collection", or
// "relation"), or "" if hash has never appeared in a creation log entry.
func (k *KindIndex) Kind(hash string) string {
	kind, _ := k.Value(hash)
	return kind
}
