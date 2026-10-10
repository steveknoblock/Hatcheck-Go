package metadata

// RelationIndex maps hashes to their relations.
// Query by "from:<hash>" or "to:<hash>" or "rel:<predicate>".
//
// It is a single Projection[RelationPayload] in Append mode. Each relation
// entry emits three pairs, one per lookup direction, under keys prefixed
// "from:", "to:" and "rel:" — the same prefixes callers already query by,
// so the three former maps become one keyspace. Append (rather than
// AppendUnique) keeps every entry: posting an identical relation twice
// produces two log entries and has always produced two results here.
type RelationIndex struct {
	*Projection[RelationPayload]
}

func NewRelationIndex() *RelationIndex {
	return &RelationIndex{NewProjection("relation", Append,
		On(OpRelation, func(e Entry, p RelationPayload) []Pair[RelationPayload] {
			return []Pair[RelationPayload]{
				{Key: "from:" + p.From, Value: p},
				{Key: "to:" + p.To, Value: p},
				{Key: "rel:" + p.Rel, Value: p},
			}
		}),
	).WithRender(func(r RelationPayload) string { return r.Hash })}
}

var _ RelationQuerier = (*RelationIndex)(nil)

// Query accepts keys in the form "from:<hash>", "to:<hash>", or "rel:<predicate>".
// Returns hashes of the relation objects matching the key.
// (Provided by Projection, rendering each relation as its Hash.)

// QueryRich accepts the same keys as Query but returns full RelationPayload structs
// rather than just hashes. Used by Store.RelationsForHash() to serve structured
// relation data to the application layer.
func (r *RelationIndex) QueryRich(key string) []RelationPayload {
	return r.Get(key)
}
