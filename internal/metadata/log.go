package metadata

import (
	"encoding/json"
	"time"
)

// --- Log entry envelope ---

// Entry is the envelope for every log event.
type Entry struct {
	Op      string          `json:"op"`
	Created time.Time       `json:"created"`
	Payload json.RawMessage `json:"payload"`
}

// Op constants.
const (
	OpStash      = "stash"
	OpCollection = "collection"
	OpRelation   = "relation"
	OpNameCreate = "name-create"
	OpNameUpdate = "name-update"
)

// --- Payload structs ---

type StashPayload struct {
	Hash string   `json:"hash"`
	Size int      `json:"size"`
	Tags []string `json:"tags"`
}

type CollectionPayload struct {
	Hash   string   `json:"hash"`
	Hashes []string `json:"hashes"`
}

type RelationPayload struct {
	Hash string `json:"hash"`
	From string `json:"from"`
	Rel  string `json:"rel"`
	To   string `json:"to"`
}

type NameCreatePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

type NameUpdatePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}
