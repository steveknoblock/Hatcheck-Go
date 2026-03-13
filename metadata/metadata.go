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

// Store holds the log and the indexes built from it.
type Store struct {
	mu        sync.RWMutex
	logPath   string
	Log       []Entry
	TagIndex  map[string][]string // tag -> []hash
	DateIndex map[string][]string // YYYY-MM-DD -> []hash
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

// New creates a Store and loads the log from the metadata directory.
// The metadata directory is created if it does not exist.
func New(metaPath string) (*Store, error) {
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		logPath:   filepath.Join(metaPath, "log.json"),
		TagIndex:  make(map[string][]string),
		DateIndex: make(map[string][]string),
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

// buildIndexes rebuilds TagIndex and DateIndex from the log.
func (s *Store) buildIndexes() {
	s.TagIndex = make(map[string][]string)
	s.DateIndex = make(map[string][]string)

	for _, entry := range s.Log {
		date := entry.Created.Format("2006-01-02")
		s.DateIndex[date] = appendUnique(s.DateIndex[date], entry.Hash)
		for _, tag := range entry.Tags {
			s.TagIndex[tag] = appendUnique(s.TagIndex[tag], entry.Hash)
		}
	}
}

// Append adds a new entry to the log, saves it, and updates the indexes.
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

	// Update indexes incrementally rather than full rebuild.
	date := entry.Created.Format("2006-01-02")
	s.DateIndex[date] = appendUnique(s.DateIndex[date], entry.Hash)
	for _, tag := range entry.Tags {
		s.TagIndex[tag] = appendUnique(s.TagIndex[tag], entry.Hash)
	}

	return nil
}

// QueryByTag returns all hashes tagged with the given tag.
func (s *Store) QueryByTag(tag string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tag = strings.ToLower(tag)
	return s.TagIndex[tag]
}

// QueryByDate returns all hashes stashed on the given date (YYYY-MM-DD).
func (s *Store) QueryByDate(date string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DateIndex[date]
}

// QueryByTagAndDate returns hashes matching both tag and date.
func (s *Store) QueryByTagAndDate(tag, date string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tag = strings.ToLower(tag)
	tagHashes := s.TagIndex[tag]
	dateHashes := s.DateIndex[date]
	return intersect(tagHashes, dateHashes)
}

// TagsForHash returns the tags from the most recent log entry for a given hash.
func (s *Store) TagsForHash(hash string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Walk log in reverse to find the most recent entry.
	for i := len(s.Log) - 1; i >= 0; i-- {
		if s.Log[i].Hash == hash {
			return s.Log[i].Tags
		}
	}
	return []string{}
}

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
