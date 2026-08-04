package metadata

import (
	"encoding/json"
	"time"
)

// CreatedIndex maps a hash to the timestamp of the log entry that created
// it — the same "avoid re-deriving something from the log every time it's
// needed" reasoning as KindIndex, this time for recency rather than kind.
// Used to power "how recently was this created" views (e.g. sizing a
// relations treemap by recency) without walking the log per hash.
type CreatedIndex struct {
	data map[string]time.Time
}

func NewCreatedIndex() *CreatedIndex {
	return &CreatedIndex{data: make(map[string]time.Time)}
}

func (c *CreatedIndex) Name() string { return "created" }

func (c *CreatedIndex) Add(entry Entry) {
	if c.data == nil {
		c.data = make(map[string]time.Time)
	}

	var hash string
	switch entry.Op {
	case OpStash:
		var p StashPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash = p.Hash
	case OpCollection:
		var p CollectionPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash = p.Hash
	case OpRelation:
		var p RelationPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		hash = p.Hash
	default:
		return
	}

	// Same reasoning as KindIndex: a hash's creation entry is fixed
	// forever once first recorded, since content is addressed by its own
	// hash — the same hash could never later be produced by a second,
	// different creation entry. No need to guard against overwriting a
	// different value; it would always be identical anyway.
	c.data[hash] = entry.Created
}

// Created returns the timestamp hash was created at, or the zero time.Time
// if hash has never appeared in a creation log entry.
func (c *CreatedIndex) Created(hash string) time.Time {
	return c.data[hash]
}

// Query satisfies the base Index interface. Created is the real API for
// this index (a hash maps to exactly one timestamp, not a list) — this
// just wraps it in a single-element slice (RFC3339), or returns nil if the
// hash is unknown.
func (c *CreatedIndex) Query(hash string) []string {
	t, ok := c.data[hash]
	if !ok || t.IsZero() {
		return nil
	}
	return []string{t.UTC().Format(time.RFC3339)}
}
