package metadata

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// --- Log entry envelope ---

// Entry is the envelope for every log event.
type Entry struct {
	Op      string          `json:"op"`
	Created time.Time       `json:"created"`
	Payload json.RawMessage `json:"payload"`
}

// Op constants.
const (
	OpStash       = "stash"
	OpCollection  = "collection"
	OpRelation    = "relation"
	OpNameCreate  = "name-create"
	OpNameUpdate  = "name-update"
)

// --- Payload structs ---

type StashPayload struct {
	Hash string   `json:"hash"`
	Size int      `json:"size"`
	Tags []string `json:"tags"`
}

type CollectionPayload struct {
	Hash   string   `json:"hash"`
	Hashes []string `json:"hashes"`
}

type RelationPayload struct {
	Hash string `json:"hash"`
	From string `json:"from"`
	Rel  string `json:"rel"`
	To   string `json:"to"`
}

type NameCreatePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

type NameUpdatePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

// --- Index interface ---

// Index is implemented by any type that can be built from log entries and queried.
type Index interface {
	Name() string
	Add(entry Entry)
	Query(key string) []string
}

// --- Store ---

// Store holds the log and all registered indexes.
type Store struct {
	mu      sync.RWMutex
	logPath string
	Log     []Entry
	indexes []Index
}

// New creates a Store, loads the log, and registers the provided indexes.
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

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.Log, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.logPath, data, 0644)
}

func (s *Store) buildIndexes() {
	for _, entry := range s.Log {
		for _, idx := range s.indexes {
			idx.Add(entry)
		}
	}
}

func (s *Store) append(entry Entry) error {
	s.Log = append(s.Log, entry)
	if err := s.save(); err != nil {
		return err
	}
	for _, idx := range s.indexes {
		idx.Add(entry)
	}
	return nil
}

// --- Append methods ---

func (s *Store) AppendStash(hash string, size int, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(StashPayload{
		Hash: hash,
		Size: size,
		Tags: ParseTags(content),
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpStash,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

func (s *Store) AppendCollection(hash string, hashes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(CollectionPayload{
		Hash:   hash,
		Hashes: hashes,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpCollection,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

func (s *Store) AppendRelation(hash, from, rel, to string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(RelationPayload{
		Hash: hash,
		From: from,
		Rel:  rel,
		To:   to,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpRelation,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

func (s *Store) AppendNameCreate(label, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for existing name via NameIndex.
	for _, idx := range s.indexes {
		if idx.Name() == "name" {
			if results := idx.Query(label); len(results) > 0 {
				return errors.New("name already exists: " + label)
			}
		}
	}

	payload, err := json.Marshal(NameCreatePayload{
		Label: label,
		Hash:  hash,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpNameCreate,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

func (s *Store) AppendNameUpdate(label, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check name exists via NameIndex.
	found := false
	for _, idx := range s.indexes {
		if idx.Name() == "name" {
			if results := idx.Query(label); len(results) > 0 {
				found = true
			}
		}
	}
	if !found {
		return errors.New("name does not exist: " + label)
	}

	payload, err := json.Marshal(NameUpdatePayload{
		Label: label,
		Hash:  hash,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpNameUpdate,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// --- Query ---

// Query returns results from the named index for the given key.
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

// TagsForHash returns tags from the most recent stash entry for a given hash.
func (s *Store) TagsForHash(hash string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(s.Log) - 1; i >= 0; i-- {
		if s.Log[i].Op != OpStash {
			continue
		}
		var p StashPayload
		if err := json.Unmarshal(s.Log[i].Payload, &p); err != nil {
			continue
		}
		if p.Hash == hash {
			return p.Tags
		}
	}
	return []string{}
}

// NameEntry holds a label and the hash it points to.
type NameEntry struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

// NamesInNamespace returns all Names whose label starts with namespace+"/".
// The namespace prefix is stripped from the returned labels.
func (s *Store) NamesInNamespace(namespace string) []NameEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := namespace + "/"
	for _, idx := range s.indexes {
		if idx.Name() == "name" {
			if ni, ok := idx.(*NameIndex); ok {
				return ni.ListNamespace(prefix)
			}
		}
	}
	return []NameEntry{}
}

// Namespaces returns all unique namespace prefixes across all Names.
func (s *Store) Namespaces() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, idx := range s.indexes {
		if idx.Name() == "name" {
			if ni, ok := idx.(*NameIndex); ok {
				return ni.Namespaces()
			}
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

// --- ParseTags ---

var hashtagRe = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

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

// --- Built-in indexes ---

// TagIndex maps tags to hashes from stash entries.
type TagIndex struct {
	data map[string][]string
}

func (t *TagIndex) Name() string { return "tag" }

func (t *TagIndex) Add(entry Entry) {
	if entry.Op != OpStash {
		return
	}
	if t.data == nil {
		t.data = make(map[string][]string)
	}
	var p StashPayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	for _, tag := range p.Tags {
		t.data[tag] = appendUnique(t.data[tag], p.Hash)
	}
}

func (t *TagIndex) Query(key string) []string {
	return t.data[strings.ToLower(key)]
}

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

// NameIndex maps labels to their current hash.
type NameIndex struct {
	data map[string]string // label -> current hash
}

func (n *NameIndex) Name() string { return "name" }

func (n *NameIndex) Add(entry Entry) {
	if entry.Op != OpNameCreate && entry.Op != OpNameUpdate {
		return
	}
	if n.data == nil {
		n.data = make(map[string]string)
	}
	// Both create and update payloads have the same shape.
	var p NameCreatePayload
	if err := json.Unmarshal(entry.Payload, &p); err != nil {
		return
	}
	n.data[p.Label] = p.Hash
}

func (n *NameIndex) Query(key string) []string {
	if hash, ok := n.data[key]; ok {
		return []string{hash}
	}
	return []string{}
}

// ListNamespace returns all label/hash pairs whose label starts with prefix.
// The prefix is stripped from the returned labels.
func (n *NameIndex) ListNamespace(prefix string) []NameEntry {
	var results []NameEntry
	for label, hash := range n.data {
		if strings.HasPrefix(label, prefix) {
			results = append(results, NameEntry{
				Label: strings.TrimPrefix(label, prefix),
				Hash:  hash,
			})
		}
	}
	return results
}

// Namespaces returns all unique namespace prefixes found in the name index.
// A namespace is the portion of a label before the first "/".
// Labels without a "/" are returned as-is as their own namespace.
func (n *NameIndex) Namespaces() []string {
	seen := make(map[string]bool)
	for label := range n.data {
		slash := strings.Index(label, "/")
		if slash >= 0 {
			seen[label[:slash]] = true
		} else {
			seen[label] = true
		}
	}
	result := make([]string, 0, len(seen))
	for ns := range seen {
		result = append(result, ns)
	}
	return result
}

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
	var relations []RelationPayload

	switch {
	case strings.HasPrefix(key, "from:"):
		relations = r.byFrom[strings.TrimPrefix(key, "from:")]
	case strings.HasPrefix(key, "to:"):
		relations = r.byTo[strings.TrimPrefix(key, "to:")]
	case strings.HasPrefix(key, "rel:"):
		relations = r.byRel[strings.TrimPrefix(key, "rel:")]
	}

	hashes := make([]string, len(relations))
	for i, rel := range relations {
		hashes[i] = rel.Hash
	}
	return hashes
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
