package metadata

import (
	"encoding/json"
	"testing"
)

// --- Test helpers ---

func entryForCapability(t *testing.T, cap CapabilityPayload) Entry {
	t.Helper()
	payload, err := json.Marshal(cap)
	if err != nil {
		t.Fatalf("failed to marshal capability: %v", err)
	}
	return Entry{Op: OpCapability, Payload: payload}
}

// --- CapabilityIndex ---

func TestCapabilityIndex_Name(t *testing.T) {
	idx := NewCapabilityIndex()
	if idx.Name() != "capability" {
		t.Errorf("expected name 'capability', got %q", idx.Name())
	}
}

func TestCapabilityIndex_IgnoresOtherOps(t *testing.T) {
	idx := NewCapabilityIndex()
	idx.Add(Entry{Op: OpStash, Payload: json.RawMessage(`{}`)})
	if len(idx.All()) != 0 {
		t.Errorf("expected no capabilities indexed, got %d", len(idx.All()))
	}
}

func TestCapabilityIndex_QueryByPrincipal(t *testing.T) {
	idx := NewCapabilityIndex()
	alice := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	bob := SignCapability(testKey, "hash2", "read", "bob", "", futureTime())

	idx.Add(entryForCapability(t, alice))
	idx.Add(entryForCapability(t, bob))

	got := idx.QueryRich("alice")
	if len(got) != 1 || got[0].ID != alice.ID {
		t.Errorf("expected alice's capability, got %v", got)
	}

	got = idx.QueryRich("bob")
	if len(got) != 1 || got[0].ID != bob.ID {
		t.Errorf("expected bob's capability, got %v", got)
	}
}

func TestCapabilityIndex_QueryReturnsIDs(t *testing.T) {
	idx := NewCapabilityIndex()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	idx.Add(entryForCapability(t, cap))

	ids := idx.Query("alice")
	if len(ids) != 1 || ids[0] != cap.ID {
		t.Errorf("expected [%s], got %v", cap.ID, ids)
	}
}

func TestCapabilityIndex_BearerCapabilitiesGroupedUnderEmptyPrincipal(t *testing.T) {
	idx := NewCapabilityIndex()
	bearer := SignCapability(testKey, "hash1", "read", "", "", futureTime())
	idx.Add(entryForCapability(t, bearer))

	got := idx.QueryRich("")
	if len(got) != 1 || got[0].ID != bearer.ID {
		t.Errorf("expected bearer capability under empty key, got %v", got)
	}
}

func TestCapabilityIndex_MultipleCapabilitiesSamePrincipal(t *testing.T) {
	idx := NewCapabilityIndex()
	cap1 := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	cap2 := SignCapability(testKey, "hash2", "read", "alice", "", futureTime())
	idx.Add(entryForCapability(t, cap1))
	idx.Add(entryForCapability(t, cap2))

	got := idx.QueryRich("alice")
	if len(got) != 2 {
		t.Errorf("expected 2 capabilities for alice, got %d", len(got))
	}
}

func TestCapabilityIndex_All(t *testing.T) {
	idx := NewCapabilityIndex()
	cap1 := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	cap2 := SignCapability(testKey, "hash2", "read", "bob", "", futureTime())
	idx.Add(entryForCapability(t, cap1))
	idx.Add(entryForCapability(t, cap2))

	all := idx.All()
	if len(all) != 2 {
		t.Errorf("expected 2 total capabilities, got %d", len(all))
	}
}

func TestCapabilityIndex_ByID(t *testing.T) {
	idx := NewCapabilityIndex()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	idx.Add(entryForCapability(t, cap))

	got, ok := idx.ByID(cap.ID)
	if !ok {
		t.Fatal("expected capability to be found by ID")
	}
	if got.Hash != "hash1" {
		t.Errorf("expected hash1, got %q", got.Hash)
	}

	_, ok = idx.ByID("nonexistent-id")
	if ok {
		t.Error("expected nonexistent ID to not be found")
	}
}

func TestCapabilityIndex_Principals(t *testing.T) {
	idx := NewCapabilityIndex()
	alice := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	bob := SignCapability(testKey, "hash2", "read", "bob", "", futureTime())
	bearer := SignCapability(testKey, "hash3", "read", "", "", futureTime())
	idx.Add(entryForCapability(t, alice))
	idx.Add(entryForCapability(t, bob))
	idx.Add(entryForCapability(t, bearer))

	principals := idx.Principals()
	if len(principals) != 2 {
		t.Errorf("expected 2 principals (bearer excluded), got %v", principals)
	}
	seen := map[string]bool{}
	for _, p := range principals {
		seen[p] = true
	}
	if !seen["alice"] || !seen["bob"] {
		t.Errorf("expected alice and bob, got %v", principals)
	}
}

func TestCapabilityIndex_PrincipalsExcludesEmptyKeyWhenNoOthers(t *testing.T) {
	idx := NewCapabilityIndex()
	bearer := SignCapability(testKey, "hash1", "read", "", "", futureTime())
	idx.Add(entryForCapability(t, bearer))

	principals := idx.Principals()
	if len(principals) != 0 {
		t.Errorf("expected no principals, got %v", principals)
	}
}

// --- Store integration ---

func TestStore_CapabilitiesForPrincipal(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewCapabilityIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cap := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	if err := s.AppendCapability(cap); err != nil {
		t.Fatalf("failed to append capability: %v", err)
	}

	got := s.CapabilitiesForPrincipal("alice")
	if len(got) != 1 || got[0].ID != cap.ID {
		t.Errorf("expected alice's capability, got %v", got)
	}

	if got := s.CapabilitiesForPrincipal("bob"); len(got) != 0 {
		t.Errorf("expected no capabilities for bob, got %v", got)
	}
}

func TestStore_AllCapabilities(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewCapabilityIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	s.AppendCapability(SignCapability(testKey, "hash1", "read", "alice", "", futureTime()))
	s.AppendCapability(SignCapability(testKey, "hash2", "read", "bob", "", futureTime()))

	all := s.AllCapabilities()
	if len(all) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(all))
	}
}

func TestStore_CapabilityByID(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewCapabilityIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cap := SignCapability(testKey, "hash1", "read", "alice", "", futureTime())
	s.AppendCapability(cap)

	got, ok := s.CapabilityByID(cap.ID)
	if !ok || got.Hash != "hash1" {
		t.Errorf("expected to find capability by ID, got %v, ok=%v", got, ok)
	}
}

func TestStore_Principals(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewCapabilityIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	s.AppendCapability(SignCapability(testKey, "hash1", "read", "alice", "", futureTime()))
	s.AppendCapability(SignCapability(testKey, "hash2", "read", "bob", "", futureTime()))

	principals := s.Principals()
	if len(principals) != 2 {
		t.Errorf("expected 2 principals, got %v", principals)
	}
}

func TestStore_CapabilityMethodsWithoutIndexRegistered(t *testing.T) {
	// If CapabilityIndex isn't registered on the store, methods should
	// degrade gracefully to empty results rather than panic.
	dir := t.TempDir()
	s, err := New(dir, NewTagIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if got := s.CapabilitiesForPrincipal("alice"); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if got := s.AllCapabilities(); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if got := s.Principals(); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if _, ok := s.CapabilityByID("anything"); ok {
		t.Error("expected not found")
	}
}
