package metadata

// DateIndex maps dates (YYYY-MM-DD) to hashes from stash entries.
//
// It is a Projection[string] in AppendUnique mode, keyed by the date of the
// entry's Created timestamp — the key comes from the log entry's envelope
// rather than its payload.
type DateIndex struct {
	*Projection[string]
}

func NewDateIndex() *DateIndex {
	return &DateIndex{NewProjection("date", AppendUnique,
		On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
			return []Pair[string]{{Key: e.Created.Format("2006-01-02"), Value: p.Hash}}
		}),
	)}
}

// Dates returns every date that has at least one stash entry, sorted
// most-recent-first — unlike TagIndex.Tags (alphabetical), chronological
// order is the whole point of browsing by date. YYYY-MM-DD sorts
// chronologically as a plain string, so this is Keys() reversed.
func (d *DateIndex) Dates() []string {
	dates := d.Keys()
	for i, j := 0, len(dates)-1; i < j; i, j = i+1, j-1 {
		dates[i], dates[j] = dates[j], dates[i]
	}
	return dates
}
