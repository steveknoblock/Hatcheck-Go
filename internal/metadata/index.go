package metadata

// --- Index interface ---

// Index is implemented by any type that can be built from log entries and queried.
type Index interface {
	Name() string
	Add(entry Entry)
	Query(key string) []string
}

// --- Capability interfaces ---

// NameLister is implemented by indexes that support namespace and name listing.
type NameLister interface {
	ListNamespace(prefix string) []NameEntry
	Namespaces() []string
}

// RelationQuerier is implemented by indexes that support rich relation queries.
type RelationQuerier interface {
	QueryRich(key string) []RelationPayload
}

// TagLister is implemented by indexes that support listing all known tags.
type TagLister interface {
	Tags() []string
}

// CapabilityQuerier is implemented by indexes that support rich capability
// queries: lookup by principal, full enumeration, lookup by ID, and listing
// distinct principals. Bundled into one interface because Store's
// capability-listing methods always need the same underlying index.
type CapabilityQuerier interface {
	QueryRich(key string) []CapabilityPayload
	All() []CapabilityPayload
	ByID(id string) (CapabilityPayload, bool)
	Principals() []string
}

// --- Helpers ---

func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}
