package metadata

// --- Index interface ---

// Index is implemented by any type that can be built from log entries and queried.
type Index interface {
	Name() string
	Add(entry Entry)
	Query(key string) []string
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
