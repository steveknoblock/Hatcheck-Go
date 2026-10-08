package metadata

// DefaultIndexes returns a fresh set of the pure data indexes — those
// derived from the log purely for lookup, with no part in authorization.
// It deliberately excludes the capability and role indexes: callers that
// need them (the server) add them explicitly, so a command that only reads
// or exports data can never end up depending on, or disturbing, access
// control state.
//
// Every call returns new instances; indexes hold state built from the log
// and must not be shared between stores.
func DefaultIndexes() []Index {
	return []Index{
		NewTagIndex(),
		NewDateIndex(),
		NewNameIndex(),
		NewRelationIndex(),
		NewKindIndex(),
		NewCreatedIndex(),
	}
}
