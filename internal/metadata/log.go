package metadata

import (
	"encoding/json"
	"regexp"
	"strings"
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

type NamePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

// --- ParseTags ---

var hashtagRe = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

func ParseTags(content string) []string {
	matches := hashtagRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	tags := []string{}
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}
