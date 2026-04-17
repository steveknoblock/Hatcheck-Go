package metadata

import (
	"encoding/json"
	"strings"
)

// TagIndex maps tags to hashes from stash entries.
type TagIndex struct {
	data map[string][]string
}

func (t *TagIndex) Name() string { return "tag" }

func (t *TagIndex) Add(entry Entry) {
	if entry.Op != OpStash {
		return
	}
	if t.data == nil {
		t.data = make(map[string][]string)
	}
	var p StashPayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	for _, tag := range p.Tags {
		t.data[tag] = appendUnique(t.data[tag], p.Hash)
	}
}

func (t *TagIndex) Query(key string) []string {
	return t.data[strings.ToLower(key)]
}

// Tags returns all known tag keys in the index.
// Used by Store.AllTags() to populate the relation type vocabulary.
func (t *TagIndex) Tags() []string {
	result := make([]string, 0, len(t.data))
	for tag := range t.data {
		result = append(result, tag)
	}
	return result
}
