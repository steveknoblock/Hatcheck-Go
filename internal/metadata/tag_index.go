package metadata

import "strings"

// TagIndex maps tags to hashes from stash entries.
//
// It is a Projection[string] in AppendUnique mode, keyed by tag. Lookups
// are case-insensitive: Query lowercases the key it is given, while the
// tags themselves are stored exactly as the stash entry recorded them
// (ParseTags already lowercases them when the entry is written).
type TagIndex struct {
	*Projection[string]
}

func NewTagIndex() *TagIndex {
	return &TagIndex{NewProjection("tag", AppendUnique,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			pairs := make([]Pair[string], 0, len(p.Tags))
			for _, tag := range p.Tags {
				pairs = append(pairs, Pair[string]{Key: tag, Value: p.Hash})
			}
			return pairs
		}),
	).WithQueryKey(strings.ToLower)}
}

// Tags returns all known tag keys in the index, sorted.
// Used by Store.AllTags() to populate the relation type vocabulary.
func (t *TagIndex) Tags() []string {
	return t.Keys()
}
