package metadata

import (
	"encoding/json"
	"testing"
)

// --- Test helpers ---

func entryForRoleAssign(t *testing.T, principal, role, assignedBy string) Entry {
	t.Helper()
	payload, err := json.Marshal(RoleAssignPayload{
		Principal:  principal,
		Role:       role,
		AssignedBy: assignedBy,
	})
	if err != nil {
		t.Fatalf("failed to marshal RoleAssignPayload: %v", err)
	}
	return Entry{Op: OpRoleAssign, Payload: payload}
}

func entryForRoleRevoke(t *testing.T, principal, role, revokedBy string) Entry {
	t.Helper()
	payload, err := json.Marshal(RoleRevokePayload{
		Principal: principal,
		Role:      role,
		RevokedBy: revokedBy,
	})
	if err != nil {
		t.Fatalf("failed to marshal RoleRevokePayload: %v", err)
	}
	return Entry{Op: OpRoleRevoke, Payload: payload}
}

// --- RoleIndex unit tests ---

func TestRoleIndex_Name(t *testing.T) {
	idx := NewRoleIndex()
	if idx.Name() != "role" {
		t.Errorf("expected name 'role', got %q", idx.Name())
	}
}

func TestRoleIndex_IgnoresOtherOps(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(Entry{Op: OpStash, Payload: json.RawMessage(`{}`)})
	idx.Add(Entry{Op: OpCapability, Payload: json.RawMessage(`{}`)})
	if roles := idx.RolesForPrincipal("alice"); len(roles) != 0 {
		t.Errorf("expected no roles, got %v", roles)
	}
}

func TestRoleIndex_AssignRole(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor], got %v", roles)
	}
}

func TestRoleIndex_AssignMultipleRolesToOnePrincipal(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "alice", "viewer", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 2 {
		t.Errorf("expected 2 roles for alice, got %v", roles)
	}
}

func TestRoleIndex_AssignSameRoleToMultiplePrincipals(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "bob", "editor", "admin"))

	principals := idx.PrincipalsForRole("editor")
	if len(principals) != 2 {
		t.Errorf("expected 2 principals for editor, got %v", principals)
	}
	seen := map[string]bool{}
	for _, p := range principals {
		seen[p] = true
	}
	if !seen["alice"] || !seen["bob"] {
		t.Errorf("expected alice and bob, got %v", principals)
	}
}

func TestRoleIndex_RevokeRole(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleRevoke(t, "alice", "editor", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 0 {
		t.Errorf("expected no roles after revoke, got %v", roles)
	}

	principals := idx.PrincipalsForRole("editor")
	if len(principals) != 0 {
		t.Errorf("expected no principals for editor after revoke, got %v", principals)
	}
}

func TestRoleIndex_RevokeOneRoleLeaveOther(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "alice", "viewer", "admin"))
	idx.Add(entryForRoleRevoke(t, "alice", "editor", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 1 || roles[0] != "viewer" {
		t.Errorf("expected [viewer], got %v", roles)
	}
}

func TestRoleIndex_RevokeNonexistentIsNoop(t *testing.T) {
	idx := NewRoleIndex()
	// Revoking a role that was never assigned should not panic.
	idx.Add(entryForRoleRevoke(t, "alice", "editor", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 0 {
		t.Errorf("expected no roles, got %v", roles)
	}
}

func TestRoleIndex_QueryByPrincipal(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))

	results := idx.Query("alice")
	if len(results) != 1 || results[0] != "editor" {
		t.Errorf("expected [editor], got %v", results)
	}
}

func TestRoleIndex_QueryByRole(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "bob", "editor", "admin"))

	results := idx.Query("role:editor")
	if len(results) != 2 {
		t.Errorf("expected 2 results for role:editor, got %v", results)
	}
}

func TestRoleIndex_Roles(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "bob", "viewer", "admin"))

	roles := idx.Roles()
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %v", roles)
	}
}

func TestRoleIndex_RolesExcludesEmptyAfterRevoke(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleRevoke(t, "alice", "editor", "admin"))

	roles := idx.Roles()
	if len(roles) != 0 {
		t.Errorf("expected no active roles after all principals revoked, got %v", roles)
	}
}

func TestRoleIndex_DuplicateAssignIsIdempotent(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin"))

	roles := idx.RolesForPrincipal("alice")
	if len(roles) != 1 {
		t.Errorf("expected 1 role (deduped), got %v", roles)
	}

	principals := idx.PrincipalsForRole("editor")
	if len(principals) != 1 {
		t.Errorf("expected 1 principal (deduped), got %v", principals)
	}
}

// --- Store integration tests ---

func TestStore_AppendRoleAssign(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if err := s.AppendRoleAssign("alice", "editor", "admin", "initial setup"); err != nil {
		t.Fatalf("AppendRoleAssign failed: %v", err)
	}

	roles := s.RolesForPrincipal("alice")
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor], got %v", roles)
	}
}

func TestStore_AppendRoleRevoke(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	s.AppendRoleAssign("alice", "editor", "admin", "")
	if err := s.AppendRoleRevoke("alice", "editor", "admin", "leaving project"); err != nil {
		t.Fatalf("AppendRoleRevoke failed: %v", err)
	}

	roles := s.RolesForPrincipal("alice")
	if len(roles) != 0 {
		t.Errorf("expected no roles after revoke, got %v", roles)
	}
}

func TestStore_PrincipalsForRole(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	s.AppendRoleAssign("alice", "editor", "admin", "")
	s.AppendRoleAssign("bob", "editor", "admin", "")

	principals := s.PrincipalsForRole("editor")
	if len(principals) != 2 {
		t.Errorf("expected 2 principals for editor, got %v", principals)
	}
}

func TestStore_Roles(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	s.AppendRoleAssign("alice", "editor", "admin", "")
	s.AppendRoleAssign("bob", "viewer", "admin", "")

	roles := s.Roles()
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %v", roles)
	}
}

func TestStore_RoleMethodsWithoutIndexRegistered(t *testing.T) {
	// If RoleIndex isn't registered, methods should degrade gracefully.
	dir := t.TempDir()
	s, err := New(dir, NewTagIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if got := s.RolesForPrincipal("alice"); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if got := s.PrincipalsForRole("editor"); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
	if got := s.Roles(); len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestStore_RolesPersistAcrossRestart(t *testing.T) {
	dir := t.TempDir()

	s1, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	s1.AppendRoleAssign("alice", "editor", "admin", "")
	s1.AppendRoleAssign("alice", "viewer", "admin", "")
	s1.AppendRoleRevoke("alice", "viewer", "admin", "")

	// Simulate restart by creating a new store from the same directory.
	s2, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	roles := s2.RolesForPrincipal("alice")
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor] after restart, got %v", roles)
	}
}
