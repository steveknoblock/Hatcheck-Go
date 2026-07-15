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

// --- Role methods ---

// AppendRoleAssign records the assignment of a role to a principal in the log
// and updates the live RoleIndex immediately.
func (s *Store) AppendRoleAssign(principal, role, assignedBy, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(RoleAssignPayload{
		Principal:  principal,
		Role:       role,
		AssignedBy: assignedBy,
		Reason:     reason,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpRoleAssign,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// AppendRoleRevoke records the removal of a role from a principal in the log
// and updates the live RoleIndex immediately.
func (s *Store) AppendRoleRevoke(principal, role, revokedBy, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(RoleRevokePayload{
		Principal: principal,
		Role:      role,
		RevokedBy: revokedBy,
		Reason:    reason,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpRoleRevoke,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// RolesForPrincipal returns the current role names held by the given principal.
func (s *Store) RolesForPrincipal(principal string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["role"]; ok {
		if rq, ok := idx.(RoleQuerier); ok {
			return rq.RolesForPrincipal(principal)
		}
	}
	return []string{}
}

// PrincipalsForRole returns the principals currently holding the given role.
func (s *Store) PrincipalsForRole(role string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["role"]; ok {
		if rq, ok := idx.(RoleQuerier); ok {
			return rq.PrincipalsForRole(role)
		}
	}
	return []string{}
}

// Roles returns all distinct role names that currently have at least one
// active assignment.
func (s *Store) Roles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["role"]; ok {
		if rq, ok := idx.(RoleQuerier); ok {
			return rq.Roles()
		}
	}
	return []string{}
}

// AppendRoleGrantAdd records that a role's definition grants a capability
// template (hash+perm) and updates the live RoleIndex immediately. This
// only defines what the role *means* — it does not by itself issue any
// capabilities. Retroactively issuing this grant to existing role members
// is the caller's responsibility (see server/role_capability.go).
func (s *Store) AppendRoleGrantAdd(role, hash, perm, addedBy, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(RoleGrantPayload{
		Role:    role,
		Hash:    hash,
		Perm:    perm,
		AddedBy: addedBy,
		Reason:  reason,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpRoleGrantAdd,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// AppendRoleGrantRemove records the removal of a capability template from a
// role's definition and updates the live RoleIndex immediately. This does
// not by itself revoke anything already issued — the caller is responsible
// for revoking any live capabilities that were issued under this grant (see
// server/role_capability.go).
func (s *Store) AppendRoleGrantRemove(role, hash, perm, removedBy, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(RoleGrantRemovePayload{
		Role:      role,
		Hash:      hash,
		Perm:      perm,
		RemovedBy: removedBy,
		Reason:    reason,
	})
	if err != nil {
		return err
	}

	return s.append(Entry{
		Op:      OpRoleGrantRemove,
		Created: time.Now().UTC(),
		Payload: payload,
	})
}

// GrantsForRole returns the capability templates currently defined for the
// given role — what a principal receives when assigned it.
func (s *Store) GrantsForRole(role string) []RoleGrant {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx, ok := s.indexMap["role"]; ok {
		if rq, ok := idx.(RoleQuerier); ok {
			return rq.GrantsForRole(role)
		}
	}
	return []RoleGrant{}
}

// CapabilitiesForPrincipalRole returns the capabilities issued to principal
// that are annotated with the given role and are still live — not expired,
// and not present in revoked (pass nil to skip the revocation check, e.g.
// when the caller only cares about which grants are already represented
// regardless of subsequent revocation). Used both to avoid re-issuing a
// grant a principal already holds, and to find what to revoke when a role
// or a role's grant is removed.
//
// This delegates to CapabilitiesForPrincipal, which takes its own read lock,
// so it must not be called while s.mu is already held.
func (s *Store) CapabilitiesForPrincipalRole(principal, role string, revoked *RevokedSet) []CapabilityPayload {
	all := s.CapabilitiesForPrincipal(principal)

	now := time.Now().UTC()
	result := make([]CapabilityPayload, 0, len(all))
	for _, cap := range all {
		if cap.Role != role {
			continue
		}
		if revoked != nil && revoked.IsRevoked(cap.ID) {
			continue
		}
		if cap.Expires.UTC().Before(now) {
			continue
		}
		result = append(result, cap)
	}
	return result
}

// IndexNames returns the names of all registered indexes.
func (s *Store) IndexNames() []string {
	names := make([]string, len(s.indexes))
	for i, idx := range s.indexes {
		names[i] = idx.Name()
	}
	return names
}
