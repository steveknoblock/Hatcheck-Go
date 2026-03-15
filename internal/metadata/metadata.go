package metadata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Entry represents a single stash event in the log.
type Entry struct {
	Hash    string    `json:"hash"`
	Created time.Time `json:"created"`
	Size    int       `json:"size"`
	Tags    []string  `json:"tags"`
}

// Index is the interface that all indexes must implement.
// Any struct with these three methods is automatically an Index.
type Index interface {
	// Name returns the identifier used to select this index in a query.
	Name() string
	// Add is called for each entry when building or updating the index.
	Add(entry Entry)
	// Query returns hashes matching the given key.
	Query(key string) []string
}

// Store holds the log and a set of registered indexes.
type Store struct {
	mu      sync.RWMutex
	logPath string
	Log     []Entry
	indexes []Index
}

var hashtagRe = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

// ParseTags extracts hashtags from content, lowercased and deduplicated.
func ParseTags(content string) []string {
	matches := hashtagRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	tags := []string{}
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// New creates a Store, loads the log, and registers the provided indexes.
// The metadata directory is created if it does not exist.
func New(metaPath string, indexes ...Index) (*Store, error) {
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		logPath: filepath.Join(metaPath, "log.json"),
		indexes: indexes,
	}

	if err := s.load(); err != nil {
		return nil, err
	}

	s.buildIndexes()
	return s, nil
}

// load reads the log file into memory. If the file does not exist it is a no-op.
func (s *Store) load() error {
	data, err := os.ReadFile(s.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.Log)
}

// save writes the in-memory log to disk.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.Log, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.logPath, data, 0644)
}

// buildIndexes replays the full log through all registered indexes.
func (s *Store) buildIndexes() {
	for _, entry := range s.Log {
		for _, idx := range s.indexes {
			idx.Add(entry)
		}
	}
}

// Append adds a new entry to the log, saves it, and updates all indexes.
func (s *Store) Append(hash string, size int, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := Entry{
		Hash:    hash,
		Created: time.Now().UTC(),
		Size:    size,
		Tags:    ParseTags(content),
	}

	s.Log = append(s.Log, entry)

	if err := s.save(); err != nil {
		return err
	}

	for _, idx := range s.indexes {
		idx.Add(entry)
	}

	return nil
}

// Query returns hashes from the named index matching the given key.
// Returns an empty slice if the index is not found or has no matches.
func (s *Store) Query(indexName, key string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, idx := range s.indexes {
		if idx.Name() == indexName {
			results := idx.Query(key)
			if results == nil {
				return []string{}
			}
			return results
		}
	}
	return []string{}
}

// TagsForHash returns the tags from the most recent log entry for a given hash.
func (s *Store) TagsForHash(hash string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := len(s.Log) - 1; i >= 0; i-- {
		if s.Log[i].Hash == hash {
			return s.Log[i].Tags
		}
	}
	return []string{}
}

// IndexNames returns the names of all registered indexes.
func (s *Store) IndexNames() []string {
	names := make([]string, len(s.indexes))
	for i, idx := range s.indexes {
		names[i] = idx.Name()
	}
	return names
}

// --- Built-in indexes ---

// TagIndex maps tags to hashes.
type TagIndex struct {
	data map[string][]string
}

func (t *TagIndex) Name() string { return "tag" }

func (t *TagIndex) Add(entry Entry) {
	if t.data == nil {
		t.data = make(map[string][]string)
	}
	for _, tag := range entry.Tags {
		t.data[tag] = appendUnique(t.data[tag], entry.Hash)
	}
}

func (t *TagIndex) Query(key string) []string {
	return t.data[strings.ToLower(key)]
}

// DateIndex maps dates (YYYY-MM-DD) to hashes.
type DateIndex struct {
	data map[string][]string
}

func (d *DateIndex) Name() string { return "date" }

func (d *DateIndex) Add(entry Entry) {
	if d.data == nil {
		d.data = make(map[string][]string)
	}
	date := entry.Created.Format("2006-01-02")
	d.data[date] = appendUnique(d.data[date], entry.Hash)
}

func (d *DateIndex) Query(key string) []string {
	return d.data[key]
}

// --- Helpers ---

// appendUnique appends a value to a slice only if not already present.
func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// intersect returns elements present in both slices.
func intersect(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, v := range b {
		set[v] = true
	}
	result := []string{}
	for _, v := range a {
		if set[v] {
			result = append(result, v)
		}
	}
	return result
}
