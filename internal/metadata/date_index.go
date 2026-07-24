package metadata

import (
	"encoding/json"
	"sort"
)

// DateIndex maps dates (YYYY-MM-DD) to hashes from stash entries.
type DateIndex struct {
	data map[string][]string
}

func NewDateIndex() *DateIndex {
	return &DateIndex{
		data: make(map[string][]string),
	}
}

func (d *DateIndex) Name() string { return "date" }

func (d *DateIndex) Add(entry Entry) {
	if entry.Op != OpStash {
		return
	}
	if d.data == nil {
		d.data = make(map[string][]string)
	}
	var p StashPayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	date := entry.Created.Format("2006-01-02")
	d.data[date] = appendUnique(d.data[date], p.Hash)
}

func (d *DateIndex) Query(key string) []string {
	return d.data[key]
}

// Dates returns every date that has at least one stash entry, sorted
// most-recent-first — unlike TagIndex.Tags (unordered), chronological
// order is the whole point of browsing by date, so this sorts rather than
// just ranging over the map.
func (d *DateIndex) Dates() []string {
	result := make([]string, 0, len(d.data))
	for date := range d.data {
		result = append(result, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(result)))
	return result
}
