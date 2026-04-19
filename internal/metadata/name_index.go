package metadata

import (
	"encoding/json"
	"strings"
)

// NameIndex maps labels to their current hash.
type NameIndex struct {
	data map[string]string // label -> current hash
}

func (n *NameIndex) Name() string { return "name" }

func (n *NameIndex) Add(entry Entry) {
	if entry.Op != OpNameCreate && entry.Op != OpNameUpdate {
		return
	}
	if n.data == nil {
		n.data = make(map[string]string)
	}
	// Both create and update payloads have the same shape.
	var p NamePayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	n.data[p.Label] = p.Hash
}

func (n *NameIndex) Query(key string) []string {
	if hash, ok := n.data[key]; ok {
		return []string{hash}
	}
	return []string{}
}

// ListNamespace returns all label/hash pairs whose label starts with prefix.
// The prefix is stripped from the returned labels.
func (n *NameIndex) ListNamespace(prefix string) []NameEntry {
	var results []NameEntry
	for label, hash := range n.data {
		if strings.HasPrefix(label, prefix) {
			results = append(results, NameEntry{
				Label: strings.TrimPrefix(label, prefix),
				Hash:  hash,
			})
		}
	}
	return results
}

// Namespaces returns all unique namespace prefixes found in the name index.
// A namespace is the portion of a label before the first "/".
// Labels without a "/" are returned as-is as their own namespace.
func (n *NameIndex) Namespaces() []string {
	seen := make(map[string]bool)
	for label := range n.data {
		slash := strings.Index(label, "/")
		if slash >= 0 {
			seen[label[:slash]] = true
		} else {
			seen[label] = true
		}
	}
	result := make([]string, 0, len(seen))
	for ns := range seen {
		result = append(result, ns)
	}
	return result
}
