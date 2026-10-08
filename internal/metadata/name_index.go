package metadata

import (
	"sort"
	"strings"
)

// NameIndex maps labels to their current hash.
//
// It is a Projection[string] in Latest mode, keyed by label: a name-create
// or name-update entry for a label replaces whatever hash it pointed at
// before. Both ops carry the same payload shape, so one handler serves
// both.
type NameIndex struct {
	*Projection[string]
}

func NewNameIndex() *NameIndex {
	setName := func(e Entry, p NamePayload) []Pair[string] {
		return []Pair[string]{{Key: p.Label, Value: p.Hash}}
	}

	return &NameIndex{NewProjection("name", Latest,
		On(OpNameCreate, setName),
		On(OpNameUpdate, setName),
	)}
}

var _ NameLister = (*NameIndex)(nil)

// ListNamespace returns all label/hash pairs whose label starts with prefix,
// sorted alphabetically (case-insensitively) by (stripped) label. Scan
// already returns keys in a fixed order, but that order is raw byte order,
// so the case-insensitive sort below is what gives callers (the /names
// endpoint, the UI's name list) the order they actually want.
func (n *NameIndex) ListNamespace(prefix string) []NameEntry {
	var results []NameEntry
	for _, kv := range n.Scan(prefix) {
		hash := kv.Value[len(kv.Value)-1] // Latest mode: exactly one value
		results = append(results, NameEntry{
			Label: strings.TrimPrefix(kv.Key, prefix),
			Hash:  hash,
		})
	}
	// Case-insensitive comparison: Go's plain string < compares raw bytes,
	// where uppercase ASCII (A-Z) sorts entirely before lowercase (a-z), so
	// capitalized labels would otherwise cluster together ahead of every
	// lowercase-starting label regardless of the letters that follow.
	// The sort is stable so that labels differing only in case (which tie
	// here) keep Scan's raw-byte order, making the result deterministic.
	sort.SliceStable(results, func(i, j int) bool {
		return strings.ToLower(results[i].Label) < strings.ToLower(results[j].Label)
	})
	return results
}

// Namespaces returns all unique namespace prefixes found in the name index,
// sorted alphabetically (case-insensitively) for a stable order (see
// ListNamespace). A namespace is the portion of a label before the first
// "/". Labels without a "/" are returned as-is as their own namespace.
func (n *NameIndex) Namespaces() []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, label := range n.Keys() {
		ns := label
		if slash := strings.Index(label, "/"); slash >= 0 {
			ns = label[:slash]
		}
		if !seen[ns] {
			seen[ns] = true
			result = append(result, ns)
		}
	}
	// Stable for the same reason as ListNamespace: namespaces differing
	// only in case tie, and keep the raw-byte order Keys() gave them.
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}
