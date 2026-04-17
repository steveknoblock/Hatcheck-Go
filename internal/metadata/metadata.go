package metadata

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

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

// AllTags returns all known tag keys from the TagIndex.
// Used to populate the relation type autocomplete vocabulary.
func (s *Store) AllTags() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, idx := range s.indexes {
		if idx.Name() == "tag" {
			if ti, ok := idx.(*TagIndex); ok {
				return ti.Tags()
			}
		}
	}
	return []string{}
}

// RelationsForHash returns all outgoing and incoming relations for a given hash.
// Outgoing: relations where hash is the From end.
// Incoming: relations where hash is the To end.
func (s *Store) RelationsForHash(hash string) (outgoing []RelationPayload, incoming []RelationPayload) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, idx := range s.indexes {
		if idx.Name() == "relation" {
			if ri, ok := idx.(*RelationIndex); ok {
				outgoing = ri.QueryRich("from:" + hash)
				incoming = ri.QueryRich("to:" + hash)
				return
			}
		}
	}
	return []RelationPayload{}, []RelationPayload{}
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
