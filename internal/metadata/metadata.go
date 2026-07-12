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

type Store struct {
	mu       sync.RWMutex
	logPath  string
	Log      []Entry
	indexes  []Index
	indexMap map[string]Index
}

func New(metaPath string, indexes ...Index) (*Store, error) {
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		return nil, err
	}

	s := &Store{
		logPath:  filepath.Join(metaPath, "log.json"),
		indexes:  indexes,
		indexMap: make(map[string]Index),
	}
	for _, idx := range indexes {
		s.indexMap[idx.Name()] = idx
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

	// Check for existing name
	if idx, ok := s.indexMap["name"]; ok {
		if results := idx.Query(label); len(results) > 0 {
			return errors.New("name already exists: " + label)
		}
	}

	payload, err := json.Marshal(NamePayload{
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

	// Check name exists
	found := false
	if idx, ok := s.indexMap["name"]; ok {
		if results := idx.Query(label); len(results) > 0 {
			found = true
		}
	}
	if !found {
		return errors.New("name does not exist: " + label)
	}

	payload, err := json.Marshal(NamePayload{
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

// AppendCapability records a newly issued capability in the log.
func (s *Store) AppendCapability(cap CapabilityPayload) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(cap)
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpCapability,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// AppendCapabilityRevoke records the revocation of a capability in the log
// and immediately updates the live RevokedSet so the change takes effect
// without requiring a server restart.
func (s *Store) AppendCapabilityRevoke(id, reason string, revoked *RevokedSet) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(CapabilityRevokePayload{
		ID:      id,
		Reason:  reason,
		Revoked: time.Now().UTC(),
	})
	if err != nil {
		return err
	}

	if err := s.append(Entry{
		Op:      OpCapabilityRevoke,
		Created: time.Now().UTC(),
		Payload: payload,
	}); err != nil {
		return err
	}

	// Update the live set only after the log is durably written.
	revoked.Add(id)
	return nil
}

// BuildRevokedSet walks the in-memory log and populates the provided RevokedSet
// with the ID of every capability that has been explicitly revoked.
// It also prunes IDs whose capabilities have already naturally expired,
// since they no longer need to be checked.
func (s *Store) BuildRevokedSet(revoked *RevokedSet) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build a map of capability expiry times keyed by ID so we can
	// skip revocations for already-expired capabilities.
	expiry := make(map[string]time.Time)
	for _, entry := range s.Log {
		if entry.Op != OpCapability {
			continue
		}
		var cap CapabilityPayload
		if err := json.Unmarshal(entry.Payload, &cap); err != nil {
			continue
		}
		expiry[cap.ID] = cap.Expires
	}

	now := time.Now().UTC()
	for _, entry := range s.Log {
		if entry.Op != OpCapabilityRevoke {
			continue
		}
		var p CapabilityRevokePayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			continue
		}
		// Only add to the revoked set if the capability has not already expired.
		if exp, ok := expiry[p.ID]; !ok || exp.UTC().After(now) {
			revoked.Add(p.ID)
		}
	}

	return nil
}

// --- Capability queries ---

// CapabilitiesForPrincipal returns every capability ever issued to a given
// principal, in issuance order. Pass "" to retrieve bearer-token
// capabilities (those issued with no bound principal). This does not filter
// out revoked or expired capabilities — callers that need current validity
// should join the result against a RevokedSet and check Expires themselves,
// the same way CapabilityMiddleware.Protect does at verification time.
func (s *Store) CapabilitiesForPrincipal(principal string) []CapabilityPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["capability"]; ok {
		if cq, ok := idx.(CapabilityQuerier); ok {
			return cq.QueryRich(principal)
		}
	}
	return []CapabilityPayload{}
}

// AllCapabilities returns every capability ever issued, in log order.
func (s *Store) AllCapabilities() []CapabilityPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["capability"]; ok {
		if cq, ok := idx.(CapabilityQuerier); ok {
			return cq.All()
		}
	}
	return []CapabilityPayload{}
}

// CapabilityByID returns a single capability by its ID, if one was ever
// issued with that ID.
func (s *Store) CapabilityByID(id string) (CapabilityPayload, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["capability"]; ok {
		if cq, ok := idx.(CapabilityQuerier); ok {
			return cq.ByID(id)
		}
	}
	return CapabilityPayload{}, false
}

// Principals returns all distinct principals that have been issued at least
// one bound capability. This is Hatcheck's closest thing to a user
// directory — there is no separate user table; a "user" is simply a
// principal string that has shown up on an issued capability.
func (s *Store) Principals() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["capability"]; ok {
		if cq, ok := idx.(CapabilityQuerier); ok {
			return cq.Principals()
		}
	}
	return []string{}
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

	if idx, ok := s.indexMap["tag"]; ok {
		if tl, ok := idx.(TagLister); ok {
			return tl.Tags()
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

	if idx, ok := s.indexMap["relation"]; ok {
		if rq, ok := idx.(RelationQuerier); ok {
			outgoing = rq.QueryRich("from:" + hash)
			incoming = rq.QueryRich("to:" + hash)
			return
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
	if idx, ok := s.indexMap["name"]; ok {
		if nl, ok := idx.(NameLister); ok {
			return nl.ListNamespace(prefix)
		}
	}
	return []NameEntry{}
}

// Namespaces returns all unique namespace prefixes across all Names.
func (s *Store) Namespaces() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["name"]; ok {
		if nl, ok := idx.(NameLister); ok {
			return nl.Namespaces()
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
