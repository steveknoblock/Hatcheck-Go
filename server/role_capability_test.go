package main

import (
	"testing"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

var roleCapTestKey = []byte("role-capability-test-key")

func newRoleCapTestStore(t *testing.T) *metadata.Store {
	t.Helper()
	store, err := metadata.New(t.TempDir(), metadata.NewCapabilityIndex(), metadata.NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	return store
}

// --- issueCapabilitiesForRole ---

func TestIssueCapabilitiesForRole_NoGrantsIsNoop(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	issued, err := issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issued != 0 {
		t.Errorf("expected 0 issued for a role with no grants, got %d", issued)
	}
}

func TestIssueCapabilitiesForRole_IssuesOnePerGrant(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermRead, "admin1", "")
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	issued, err := issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issued != 2 {
		t.Fatalf("expected 2 capabilities issued, got %d", issued)
	}

	live := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked)
	if len(live) != 2 {
		t.Fatalf("expected 2 live role capabilities, got %d", len(live))
	}
	for _, cap := range live {
		if cap.Role != "editor" {
			t.Errorf("expected Role annotation 'editor', got %q", cap.Role)
		}
		if !metadata.VerifyCapability(roleCapTestKey, cap, "alice") {
			t.Errorf("issued capability failed verification: %+v", cap)
		}
	}
}

func TestIssueCapabilitiesForRole_SkipsAlreadyHeldGrant(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	// First assignment issues the capability.
	issued1, err := issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issued1 != 1 {
		t.Fatalf("expected 1 issued on first call, got %d", issued1)
	}

	// Calling it again for the same principal/role should be a no-op —
	// re-running an assignment must not pile up duplicate capabilities.
	issued2, err := issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issued2 != 0 {
		t.Errorf("expected 0 issued on repeat call, got %d", issued2)
	}

	live := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked)
	if len(live) != 1 {
		t.Errorf("expected exactly 1 live capability after repeat call, got %d", len(live))
	}
}

func TestIssueCapabilitiesForRole_ReissuesAfterRevocation(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")

	// Revoke the previously-issued capability out of band.
	live := meta.CapabilitiesForPrincipalRole("alice", "editor", nil)
	if len(live) != 1 {
		t.Fatalf("expected 1 capability before revocation, got %d", len(live))
	}
	meta.AppendCapabilityRevoke(live[0].ID, "test revocation", revoked)

	// Since the previous grant is no longer live, issuing again should
	// produce a fresh capability rather than staying silent.
	issued, err := issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issued != 1 {
		t.Errorf("expected reissuance after revocation, got %d issued", issued)
	}
}

func TestIssueCapabilitiesForRole_DifferentRolesDoNotInterfere(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")
	meta.AppendRoleGrantAdd("viewer", "*", PermRead, "admin1", "")

	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "viewer")

	editorCaps := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked)
	viewerCaps := meta.CapabilitiesForPrincipalRole("alice", "viewer", revoked)

	if len(editorCaps) != 1 || editorCaps[0].Perm != PermWrite {
		t.Errorf("expected 1 write capability under editor, got %v", editorCaps)
	}
	if len(viewerCaps) != 1 || viewerCaps[0].Perm != PermRead {
		t.Errorf("expected 1 read capability under viewer, got %v", viewerCaps)
	}
}

// --- revokeCapabilitiesForRole ---

func TestRevokeCapabilitiesForRole_RevokesAllUnderRole(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermRead, "admin1", "")
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")

	revokedCount, err := revokeCapabilitiesForRole(meta, revoked, "alice", "editor", "role removed", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedCount != 2 {
		t.Fatalf("expected 2 capabilities revoked, got %d", revokedCount)
	}

	if live := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked); len(live) != 0 {
		t.Errorf("expected no live capabilities remaining, got %v", live)
	}
}

func TestRevokeCapabilitiesForRole_LeavesOtherPrincipalsAlone(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "bob", "editor")

	revokeCapabilitiesForRole(meta, revoked, "alice", "editor", "role removed", nil)

	if live := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked); len(live) != 0 {
		t.Errorf("expected alice's capability revoked, got %v", live)
	}
	if live := meta.CapabilitiesForPrincipalRole("bob", "editor", revoked); len(live) != 1 {
		t.Errorf("expected bob's capability untouched, got %v", live)
	}
}

func TestRevokeCapabilitiesForRole_GrantFilterRevokesOnlyMatchingGrant(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	meta.AppendRoleGrantAdd("editor", "*", PermRead, "admin1", "")
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")
	issueCapabilitiesForRole(meta, roleCapTestKey, revoked, time.Hour, "alice", "editor")

	// Only the write grant is being removed from the role's definition —
	// the read grant should be left untouched.
	filter := &metadata.RoleGrant{Hash: "*", Perm: PermWrite}
	revokedCount, err := revokeCapabilitiesForRole(meta, revoked, "alice", "editor", "grant removed", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("expected 1 capability revoked, got %d", revokedCount)
	}

	live := meta.CapabilitiesForPrincipalRole("alice", "editor", revoked)
	if len(live) != 1 || live[0].Perm != PermRead {
		t.Errorf("expected only the read capability to remain, got %v", live)
	}
}

func TestRevokeCapabilitiesForRole_NoLiveCapabilitiesIsNoop(t *testing.T) {
	meta := newRoleCapTestStore(t)
	revoked := metadata.NewRevokedSet()

	revokedCount, err := revokeCapabilitiesForRole(meta, revoked, "alice", "editor", "role removed", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedCount != 0 {
		t.Errorf("expected 0 revoked when nothing was issued, got %d", revokedCount)
	}
}
