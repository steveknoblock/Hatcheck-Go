package metadata

import "time"

// CreatedIndex maps a hash to the timestamp of the log entry that created
// it — the same "avoid re-deriving something from the log every time it's
// needed" reasoning as KindIndex, this time for recency rather than kind.
// Used to power "how recently was this created" views (e.g. sizing a
// relations treemap by recency) without walking the log per hash.
//
// It is a Projection[time.Time] in Latest mode. As with KindIndex, a
// hash's creation entry is fixed forever once first recorded, so Latest
// never actually overwrites a differing value.
type CreatedIndex struct {
	*Projection[time.Time]
}

func NewCreatedIndex() *CreatedIndex {
	// An entry with a zero Created time records nothing: Created reports
	// the zero time for a hash it has never seen, so storing a zero time
	// would be indistinguishable from "unknown" anyway, and Query has
	// always returned nil for it.
	created := func(hash string, e Entry) []Pair[time.Time] {
		if e.Created.IsZero() {
			return nil
		}
		return []Pair[time.Time]{{Key: hash, Value: e.Created}}
	}

	return &CreatedIndex{NewProjection("created", Latest,
		On(OpStash, func(e Entry, p StashPayload) []Pair[time.Time] {
			return created(p.Hash, e)
		}),
		On(OpCollection, func(e Entry, p CollectionPayload) []Pair[time.Time] {
			return created(p.Hash, e)
		}),
		On(OpRelation, func(e Entry, p RelationPayload) []Pair[time.Time] {
			return created(p.Hash, e)
		}),
	).WithRender(func(t time.Time) string {
		return t.UTC().Format(time.RFC3339)
	})}
}

// Created returns the timestamp hash was created at, or the zero time.Time
// if hash has never appeared in a creation log entry.
func (c *CreatedIndex) Created(hash string) time.Time {
	t, _ := c.Value(hash)
	return t
}
