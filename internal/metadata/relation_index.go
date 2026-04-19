package metadata

import (
	"encoding/json"
	"strings"
)

// RelationIndex maps hashes to their relations.
// Query by "from:<hash>" or "to:<hash>" or "rel:<predicate>".
type RelationIndex struct {
	byFrom map[string][]RelationPayload
	byTo   map[string][]RelationPayload
	byRel  map[string][]RelationPayload
}

func (r *RelationIndex) Name() string { return "relation" }

func (r *RelationIndex) Add(entry Entry) {
	if entry.Op != OpRelation {
		return
	}
	if r.byFrom == nil {
		r.byFrom = make(map[string][]RelationPayload)
		r.byTo = make(map[string][]RelationPayload)
		r.byRel = make(map[string][]RelationPayload)
	}
	var p RelationPayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	r.byFrom[p.From] = append(r.byFrom[p.From], p)
	r.byTo[p.To] = append(r.byTo[p.To], p)
	r.byRel[p.Rel] = append(r.byRel[p.Rel], p)
}

// Query accepts keys in the form "from:<hash>", "to:<hash>", or "rel:<predicate>".
// Returns hashes of the relation objects matching the key.
func (r *RelationIndex) Query(key string) []string {
	relations := r.QueryRich(key)
	hashes := make([]string, len(relations))
	for i, rel := range relations {
		hashes[i] = rel.Hash
	}
	return hashes
}

// QueryRich accepts the same keys as Query but returns full RelationPayload structs
// rather than just hashes. Used by Store.RelationsForHash() to serve structured
// relation data to the application layer.
func (r *RelationIndex) QueryRich(key string) []RelationPayload {
	switch {
	case strings.HasPrefix(key, "from:"):
		return r.byFrom[strings.TrimPrefix(key, "from:")]
	case strings.HasPrefix(key, "to:"):
		return r.byTo[strings.TrimPrefix(key, "to:")]
	case strings.HasPrefix(key, "rel:"):
		return r.byRel[strings.TrimPrefix(key, "rel:")]
	}
	return []RelationPayload{}
}
