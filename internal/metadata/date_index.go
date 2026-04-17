package metadata

import "encoding/json"

// DateIndex maps dates (YYYY-MM-DD) to hashes from stash entries.
type DateIndex struct {
	data map[string][]string
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
