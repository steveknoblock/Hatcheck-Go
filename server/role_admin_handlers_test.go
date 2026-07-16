package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

var roleAdminTestKey = []byte("role-admin-handler-test-key")

func newRoleAdminTestStore(t *testing.T) *metadata.Store {
	t.Helper()
	store, err := metadata.New(t.TempDir(), metadata.NewCapabilityIndex(), metadata.NewRoleIndex())
	if err != nil {
		t.Fatalf("failed to create metadata store: %v", err)
	}
	return store
}

func newRoleAdminTestMiddleware() *CapabilityMiddleware {
	return &CapabilityMiddleware{
		Key:     roleAdminTestKey,
		Revoked: metadata.NewRevokedSet(),
	}
}

func newRoleAdminTestConfig() Config {
	return Config{CapabilityExpiry: time.Hour}
}

func doRoleAssign(meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, principal, role, reason string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/role/assign?principal="+principal+"&role="+role+"&reason="+reason, nil)
	w := httptest.NewRecorder()
	roleAssignHandler(w, req, meta, cm, cfg, VerifiedRequest{Principal: "admin1"})
	return w
}

func doRoleRevoke(meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, principal, role, reason string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/role/revoke?principal="+principal+"&role="+role+"&reason="+reason, nil)
	w := httptest.NewRecorder()
	roleRevokeHandler(w, req, meta, cm, cfg, VerifiedRequest{Principal: "admin1"})
	return w
}

// --- principalHasRole ---

func TestPrincipalHasRole_FalseWhenNeverAssigned(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	if principalHasRole(meta, "alice", "editor") {
		t.Error("expected false for a principal who has never been assigned the role")
	}
}

func TestPrincipalHasRole_TrueAfterAssign(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	meta.AppendRoleAssign("alice", "editor", "admin1", "")
	if !principalHasRole(meta, "alice", "editor") {
		t.Error("expected true after assignment")
	}
}

func TestPrincipalHasRole_FalseAfterRevoke(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	meta.AppendRoleAssign("alice", "editor", "admin1", "")
	meta.AppendRoleRevoke("alice", "editor", "admin1", "")
	if principalHasRole(meta, "alice", "editor") {
		t.Error("expected false after revoke")
	}
}

// --- roleAssignHandler duplicate guard ---

func TestRoleAssignHandler_FirstAssignIssuesCapabilities(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	cm := newRoleAdminTestMiddleware()
	cfg := newRoleAdminTestConfig()
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	w := doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !principalHasRole(meta, "alice", "editor") {
		t.Error("expected alice to hold editor after first assign")
	}
	live := meta.CapabilitiesForPrincipalRole("alice", "editor", cm.Revoked)
	if len(live) != 1 {
		t.Errorf("expected 1 live capability issued, got %d", len(live))
	}
}

func TestRoleAssignHandler_DuplicateAssignIsNoop(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	cm := newRoleAdminTestMiddleware()
	cfg := newRoleAdminTestConfig()
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	before := meta.CapabilitiesForPrincipalRole("alice", "editor", cm.Revoked)

	// Assigning the same role to the same principal again should be a
	// clean no-op: no new log entry, no new capability, and a response
	// that says so rather than "0 capabilities issued" (which reads like
	// something went wrong rather than nothing needed to happen).
	w := doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, "already has role") {
		t.Errorf("expected 'already has role' message, got %q", got)
	}

	after := meta.CapabilitiesForPrincipalRole("alice", "editor", cm.Revoked)
	if len(after) != len(before) {
		t.Errorf("expected no new capabilities from duplicate assign, had %d now have %d", len(before), len(after))
	}
}

func TestRoleAssignHandler_ReassignAfterRevokeIsNotBlocked(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	cm := newRoleAdminTestMiddleware()
	cfg := newRoleAdminTestConfig()
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	doRoleRevoke(meta, cm, cfg, "alice", "editor", "")

	// The duplicate guard must only block re-assigning while already a
	// member — it must not block reassignment after a genuine revoke.
	w := doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); strings.Contains(got, "already has role") {
		t.Errorf("expected reassignment to proceed after revoke, got %q", got)
	}
	if !principalHasRole(meta, "alice", "editor") {
		t.Error("expected alice to hold editor again after reassignment")
	}
}

// --- roleRevokeHandler symmetric guard ---

func TestRoleRevokeHandler_RevokingUnheldRoleIsNoop(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	cm := newRoleAdminTestMiddleware()
	cfg := newRoleAdminTestConfig()

	w := doRoleRevoke(meta, cm, cfg, "alice", "editor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, "does not have role") {
		t.Errorf("expected 'does not have role' message, got %q", got)
	}
}

func TestRoleRevokeHandler_RevokingHeldRoleWorks(t *testing.T) {
	meta := newRoleAdminTestStore(t)
	cm := newRoleAdminTestMiddleware()
	cfg := newRoleAdminTestConfig()
	meta.AppendRoleGrantAdd("editor", "*", PermWrite, "admin1", "")

	doRoleAssign(meta, cm, cfg, "alice", "editor", "")
	w := doRoleRevoke(meta, cm, cfg, "alice", "editor", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if principalHasRole(meta, "alice", "editor") {
		t.Error("expected alice to no longer hold editor after revoke")
	}
	live := meta.CapabilitiesForPrincipalRole("alice", "editor", cm.Revoked)
	if len(live) != 0 {
		t.Errorf("expected 0 live capabilities after revoke, got %d", len(live))
	}
}
