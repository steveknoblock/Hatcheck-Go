package metadata

import (
	"encoding/json"
	"testing"
	"time"
)

// --- Test helpers ---

// newRoleTestStore creates a Store backed only by a RoleIndex — enough for
// role assignment and grant-definition tests without pulling in the other
// indexes' setup.
func newRoleTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir(), NewRoleIndex())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return store
}

// newCapabilityRoleTestStore creates a Store backed by both a CapabilityIndex
// and a RoleIndex — needed for tests that issue capabilities and then query
// them back by principal/role.
func newCapabilityRoleTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := New(t.TempDir(), NewCapabilityIndex(), NewRoleIndex())
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	return store
}

func futureExpiryForTest() time.Time {
	return time.Now().UTC().Add(1 * time.Hour)
}

func pastExpiryForTest() time.Time {
	return time.Now().UTC().Add(-1 * time.Hour)
}

func entryForRoleGrantAdd(t *testing.T, role, hash, perm, addedBy string) Entry {
	t.Helper()
	payload, err := json.Marshal(RoleGrantPayload{
		Role:    role,
		Hash:    hash,
		Perm:    perm,
		AddedBy: addedBy,
	})
	if err != nil {
		t.Fatalf("failed to marshal RoleGrantPayload: %v", err)
	}
	return Entry{Op: OpRoleGrantAdd, Payload: payload}
}

func entryForRoleGrantRemove(t *testing.T, role, hash, perm, removedBy string) Entry {
	t.Helper()
	payload, err := json.Marshal(RoleGrantRemovePayload{
		Role:      role,
		Hash:      hash,
		Perm:      perm,
		RemovedBy: removedBy,
	})
	if err != nil {
		t.Fatalf("failed to marshal RoleGrantRemovePayload: %v", err)
	}
	return Entry{Op: OpRoleGrantRemove, Payload: payload}
}

// containsGrant reports whether grants contains a RoleGrant with the given
// hash and perm, regardless of slice order (map iteration order is random).
func containsGrant(grants []RoleGrant, hash, perm string) bool {
	for _, g := range grants {
		if g.Hash == hash && g.Perm == perm {
			return true
		}
	}
	return false
}

// --- RoleIndex grant unit tests ---

func TestRoleIndex_GrantsForRole_Empty(t *testing.T) {
	idx := NewRoleIndex()
	if grants := idx.GrantsForRole("editor"); len(grants) != 0 {
		t.Errorf("expected no grants, got %v", grants)
	}
}

func TestRoleIndex_GrantAdd(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))

	grants := idx.GrantsForRole("editor")
	if len(grants) != 1 {
		t.Fatalf("expected 1 grant, got %d", len(grants))
	}
	if !containsGrant(grants, "*", "write") {
		t.Errorf("expected grant {*, write}, got %v", grants)
	}
}

func TestRoleIndex_GrantAdd_Duplicate(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))

	grants := idx.GrantsForRole("editor")
	if len(grants) != 1 {
		t.Errorf("expected duplicate grant to collapse to 1, got %d: %v", len(grants), grants)
	}
}

func TestRoleIndex_GrantAdd_MultiplePerRole(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "read", "admin1"))
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))

	grants := idx.GrantsForRole("editor")
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants, got %d: %v", len(grants), grants)
	}
	if !containsGrant(grants, "*", "read") || !containsGrant(grants, "*", "write") {
		t.Errorf("expected both read and write grants, got %v", grants)
	}
}

func TestRoleIndex_GrantRemove(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleGrantRemove(t, "editor", "*", "write", "admin1"))

	if grants := idx.GrantsForRole("editor"); len(grants) != 0 {
		t.Errorf("expected grant removed, got %v", grants)
	}
}

func TestRoleIndex_GrantRemove_NoMatchingAdd(t *testing.T) {
	idx := NewRoleIndex()
	// Removing a grant that was never added should be a harmless no-op.
	idx.Add(entryForRoleGrantRemove(t, "editor", "*", "write", "admin1"))

	if grants := idx.GrantsForRole("editor"); len(grants) != 0 {
		t.Errorf("expected no grants, got %v", grants)
	}
}

func TestRoleIndex_GrantsIndependentAcrossRoles(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleGrantAdd(t, "viewer", "*", "read", "admin1"))

	editorGrants := idx.GrantsForRole("editor")
	viewerGrants := idx.GrantsForRole("viewer")

	if len(editorGrants) != 1 || !containsGrant(editorGrants, "*", "write") {
		t.Errorf("expected editor to have only write grant, got %v", editorGrants)
	}
	if len(viewerGrants) != 1 || !containsGrant(viewerGrants, "*", "read") {
		t.Errorf("expected viewer to have only read grant, got %v", viewerGrants)
	}
}

func TestRoleIndex_IgnoresGrantOpsForUnrelatedQueries(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))

	// Grant definitions must not leak into membership queries.
	if roles := idx.RolesForPrincipal("alice"); len(roles) != 0 {
		t.Errorf("expected no role membership from a grant-only entry, got %v", roles)
	}
	if principals := idx.PrincipalsForRole("editor"); len(principals) != 0 {
		t.Errorf("expected no members from a grant-only entry, got %v", principals)
	}
}

// --- Roles() must include defined-but-unassigned roles ---

func TestRoleIndex_Roles_IncludesDefinedButUnassignedRole(t *testing.T) {
	idx := NewRoleIndex()
	// A grant with no assignments at all — this role has never had a member.
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))

	roles := idx.Roles()
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor] to be visible from its grant alone, got %v", roles)
	}
}

func TestRoleIndex_Roles_UnionsMembershipAndGrants(t *testing.T) {
	idx := NewRoleIndex()
	// "editor" has both a grant and a member; "viewer" has only a member
	// (no grant defined yet); "archivist" has only a grant (defined but
	// nobody assigned yet). All three should be visible.
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin1"))
	idx.Add(entryForRoleAssign(t, "bob", "viewer", "admin1"))
	idx.Add(entryForRoleGrantAdd(t, "archivist", "*", "read", "admin1"))

	roles := idx.Roles()
	if len(roles) != 3 {
		t.Fatalf("expected 3 roles, got %v", roles)
	}
	for _, want := range []string{"editor", "viewer", "archivist"} {
		found := false
		for _, r := range roles {
			if r == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in roles list, got %v", want, roles)
		}
	}
}

func TestRoleIndex_Roles_ExcludesRoleWithNeitherGrantsNorMembers(t *testing.T) {
	idx := NewRoleIndex()
	// Defined, then un-defined again, with no assignment ever made —
	// should not linger in the list.
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleGrantRemove(t, "editor", "*", "write", "admin1"))

	if roles := idx.Roles(); len(roles) != 0 {
		t.Errorf("expected no roles once grants and members are both empty, got %v", roles)
	}
}

func TestRoleIndex_Roles_StaysVisibleAfterLastMemberRemovedIfGrantRemains(t *testing.T) {
	idx := NewRoleIndex()
	idx.Add(entryForRoleGrantAdd(t, "editor", "*", "write", "admin1"))
	idx.Add(entryForRoleAssign(t, "alice", "editor", "admin1"))
	idx.Add(entryForRoleRevoke(t, "alice", "editor", "admin1"))

	// Alice was the only member and has been removed, but the role's
	// definition (its grant) is still there — it should stay visible so an
	// admin can still find it to assign to someone else.
	roles := idx.Roles()
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor] to remain visible via its grant, got %v", roles)
	}
}

// --- Store-level Roles() ---

func TestStore_Roles_IncludesDefinedButUnassignedRole(t *testing.T) {
	store := newRoleTestStore(t)

	store.AppendRoleGrantAdd("editor", "*", "write", "admin1", "")

	roles := store.Roles()
	if len(roles) != 1 || roles[0] != "editor" {
		t.Errorf("expected [editor], got %v", roles)
	}
}

// --- Store-level tests ---

func TestStore_AppendRoleGrantAdd_And_GrantsForRole(t *testing.T) {
	store := newRoleTestStore(t)

	if err := store.AppendRoleGrantAdd("editor", "*", "write", "admin1", "initial grant"); err != nil {
		t.Fatalf("AppendRoleGrantAdd failed: %v", err)
	}

	grants := store.GrantsForRole("editor")
	if len(grants) != 1 || !containsGrant(grants, "*", "write") {
		t.Errorf("expected grant {*, write}, got %v", grants)
	}
}

func TestStore_AppendRoleGrantRemove(t *testing.T) {
	store := newRoleTestStore(t)

	store.AppendRoleGrantAdd("editor", "*", "write", "admin1", "")
	if err := store.AppendRoleGrantRemove("editor", "*", "write", "admin1", "no longer needed"); err != nil {
		t.Fatalf("AppendRoleGrantRemove failed: %v", err)
	}

	if grants := store.GrantsForRole("editor"); len(grants) != 0 {
		t.Errorf("expected grant removed, got %v", grants)
	}
}

func TestStore_GrantsSurviveReplay(t *testing.T) {
	dir := t.TempDir()

	store1, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store1.AppendRoleGrantAdd("editor", "*", "write", "admin1", "")
	store1.AppendRoleGrantAdd("editor", "*", "read", "admin1", "")
	store1.AppendRoleGrantRemove("editor", "*", "read", "admin1", "")

	// Rebuild from the log on disk, as would happen on server restart.
	store2, err := New(dir, NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	grants := store2.GrantsForRole("editor")
	if len(grants) != 1 || !containsGrant(grants, "*", "write") {
		t.Errorf("expected only write grant to survive replay, got %v", grants)
	}
}

// --- CapabilityPayload.Role field ---

func TestCapabilityPayload_RoleExcludedFromSignature(t *testing.T) {
	key := []byte("test-key")
	expires := futureExpiryForTest()

	cap := SignCapability(key, "abc123", "write", "alice", "", expires)
	cap.Role = "editor" // annotate after signing, as role_capability.go does

	// Verification must still succeed — Role must not be part of the signed message.
	if !VerifyCapability(key, cap, "alice") {
		t.Error("expected capability to still verify after annotating Role")
	}
}

// --- CapabilitiesForPrincipalRole ---

func TestStore_CapabilitiesForPrincipalRole_FiltersByRoleAndLiveness(t *testing.T) {
	store := newCapabilityRoleTestStore(t)
	key := []byte("test-key")
	expires := futureExpiryForTest()
	expired := pastExpiryForTest()

	live := SignCapability(key, "hashA", "write", "alice", "", expires)
	live.Role = "editor"
	store.AppendCapability(live)

	wrongRole := SignCapability(key, "hashB", "write", "alice", "", expires)
	wrongRole.Role = "viewer"
	store.AppendCapability(wrongRole)

	expiredCap := SignCapability(key, "hashC", "write", "alice", "", expired)
	expiredCap.Role = "editor"
	store.AppendCapability(expiredCap)

	revokedCap := SignCapability(key, "hashD", "write", "alice", "", expires)
	revokedCap.Role = "editor"
	store.AppendCapability(revokedCap)
	revoked := NewRevokedSet()
	revoked.Add(revokedCap.ID)

	result := store.CapabilitiesForPrincipalRole("alice", "editor", revoked)
	if len(result) != 1 || result[0].Hash != "hashA" {
		t.Errorf("expected only the live editor capability for hashA, got %v", result)
	}
}

func TestStore_CapabilitiesForPrincipalRole_NilRevokedSkipsRevocationCheck(t *testing.T) {
	store := newCapabilityRoleTestStore(t)
	key := []byte("test-key")
	expires := futureExpiryForTest()

	cap := SignCapability(key, "hashA", "write", "alice", "", expires)
	cap.Role = "editor"
	store.AppendCapability(cap)

	// Passing nil for revoked means the revocation check is skipped entirely.
	result := store.CapabilitiesForPrincipalRole("alice", "editor", nil)
	if len(result) != 1 {
		t.Errorf("expected 1 capability with nil revoked set, got %d", len(result))
	}
}
