package metadata

import "encoding/json"

// KindIndex maps a hash to the kind of object that created it — "stash",
// "collection", or "relation" — so callers can tell what a hash is
// without fetching and parsing its content. The alternative
// (content-sniffing: fetch the object, try json.Unmarshal, check its
// shape) works — descend() in the UI already does exactly this — but
// costs a full fetch per object. That's fine for one object at a time,
// but too expensive to do for every entry just to pick an icon when
// rendering a whole namespace's worth of names in one list. This makes
// that lookup O(1) instead.
type KindIndex struct {
	data map[string]string
}

func NewKindIndex() *KindIndex {
	return &KindIndex{data: make(map[string]string)}
}

func (k *KindIndex) Name() string { return "kind" }

func (k *KindIndex) Add(entry Entry) {
	if k.data == nil {
		k.data = make(map[string]string)
	}

	var hash, kind string
	switch entry.Op {
	case OpStash:
		var p StashPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash, kind = p.Hash, "stash"
	case OpCollection:
		var p CollectionPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash, kind = p.Hash, "collection"
	case OpRelation:
		var p RelationPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash, kind = p.Hash, "relation"
	default:
		return
	}

	// A hash's kind is fixed forever once first recorded — content is
	// addressed by its own hash, so the same hash could never later be
	// produced by a different kind of operation. No need to guard against
	// overwriting a different value; it would always be identical anyway.
	k.data[hash] = kind
}

// Kind returns the kind of object hash is ("stash", "collection", or
// "relation"), or "" if hash has never appeared in a creation log entry.
func (k *KindIndex) Kind(hash string) string {
	return k.data[hash]
}

// Query satisfies the base Index interface. Kind is the real API for this
// index (a hash maps to exactly one kind, not a list) — this just wraps it
// in a single-element slice, or returns nil if the hash is unknown.
func (k *KindIndex) Query(hash string) []string {
	if kind := k.data[hash]; kind != "" {
		return []string{kind}
	}
	return nil
}
