package metadata

import (
	"testing"
	"time"
)

// --- Test helpers ---

var testKey = []byte("test-signing-key")

func futureTime() time.Time {
	return time.Now().UTC().Add(1 * time.Hour)
}

func expiredTime() time.Time {
	return time.Now().UTC().Add(-1 * time.Hour)
}

// --- CapabilityID ---

func TestCapabilityID_Deterministic(t *testing.T) {
	expires := futureTime().Truncate(time.Second)
	id1 := CapabilityID("hash1", "read", "alice", expires)
	id2 := CapabilityID("hash1", "read", "alice", expires)
	if id1 != id2 {
		t.Errorf("expected identical IDs, got %q and %q", id1, id2)
	}
}

func TestCapabilityID_DiffersOnHash(t *testing.T) {
	expires := futureTime()
	id1 := CapabilityID("hash1", "read", "alice", expires)
	id2 := CapabilityID("hash2", "read", "alice", expires)
	if id1 == id2 {
		t.Error("expected different IDs for different hashes")
	}
}

func TestCapabilityID_DiffersOnPerm(t *testing.T) {
	expires := futureTime()
	id1 := CapabilityID("hash1", "read", "alice", expires)
	id2 := CapabilityID("hash1", "write", "alice", expires)
	if id1 == id2 {
		t.Error("expected different IDs for different perms")
	}
}

func TestCapabilityID_DiffersOnPrincipal(t *testing.T) {
	expires := futureTime()
	id1 := CapabilityID("hash1", "read", "alice", expires)
	id2 := CapabilityID("hash1", "read", "bob", expires)
	if id1 == id2 {
		t.Error("expected different IDs for different principals")
	}
}

func TestCapabilityID_DiffersOnExpiry(t *testing.T) {
	expires1 := futureTime()
	expires2 := expires1.Add(1 * time.Minute)
	id1 := CapabilityID("hash1", "read", "alice", expires1)
	id2 := CapabilityID("hash1", "read", "alice", expires2)
	if id1 == id2 {
		t.Error("expected different IDs for different expiry times")
	}
}

// --- SignCapability ---

func TestSignCapability_FieldsPopulated(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)

	if cap.ID == "" {
		t.Error("expected non-empty ID")
	}
	if cap.Hash != "hash1" {
		t.Errorf("expected hash1, got %q", cap.Hash)
	}
	if cap.Perm != "read" {
		t.Errorf("expected read, got %q", cap.Perm)
	}
	if cap.Principal != "alice" {
		t.Errorf("expected alice, got %q", cap.Principal)
	}
	if cap.Sig == "" {
		t.Error("expected non-empty Sig")
	}
}

func TestSignCapability_IDMatchesCapabilityID(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	expected := CapabilityID("hash1", "read", "alice", expires)
	if cap.ID != expected {
		t.Errorf("expected ID %q, got %q", expected, cap.ID)
	}
}

func TestSignCapability_BearerToken(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "", "", expires)
	if cap.Principal != "" {
		t.Errorf("expected empty principal for bearer token, got %q", cap.Principal)
	}
}

func TestSignCapability_EmailOptIn(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "alice@example.com", expires)
	if cap.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %q", cap.Email)
	}
}

func TestSignCapability_EmailOptOut(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if cap.Email != "" {
		t.Errorf("expected empty email when not opted in, got %q", cap.Email)
	}
}

func TestVerifyCapability_EmailDoesNotAffectVerification(t *testing.T) {
	expires := futureTime()
	// Same capability issued with and without email should both verify identically.
	capWith := SignCapability(testKey, "hash1", "read", "alice", "alice@example.com", expires)
	capWithout := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if !VerifyCapability(testKey, capWith, "alice") {
		t.Error("expected capability with email to verify")
	}
	if !VerifyCapability(testKey, capWithout, "alice") {
		t.Error("expected capability without email to verify")
	}
	// IDs should be identical since email is not part of the signing message.
	if capWith.ID != capWithout.ID {
		t.Error("expected ID to be identical regardless of email")
	}
}

// --- VerifyCapability ---

func TestVerifyCapability_Valid(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if !VerifyCapability(testKey, cap, "alice") {
		t.Error("expected valid capability to verify")
	}
}

func TestVerifyCapability_WrongKey(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if VerifyCapability([]byte("wrong-key"), cap, "alice") {
		t.Error("expected verification to fail with wrong key")
	}
}

func TestVerifyCapability_Expired(t *testing.T) {
	expires := expiredTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if VerifyCapability(testKey, cap, "alice") {
		t.Error("expected expired capability to fail verification")
	}
}

func TestVerifyCapability_WrongPrincipal(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	if VerifyCapability(testKey, cap, "bob") {
		t.Error("expected verification to fail for wrong principal")
	}
}

func TestVerifyCapability_TamperedHash(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	cap.Hash = "hash2"
	if VerifyCapability(testKey, cap, "alice") {
		t.Error("expected verification to fail after tampering with hash")
	}
}

func TestVerifyCapability_TamperedPerm(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	cap.Perm = "write"
	if VerifyCapability(testKey, cap, "alice") {
		t.Error("expected verification to fail after tampering with perm")
	}
}

func TestVerifyCapability_TamperedExpiry(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	cap.Expires = cap.Expires.Add(24 * time.Hour)
	if VerifyCapability(testKey, cap, "alice") {
		t.Error("expected verification to fail after tampering with expiry")
	}
}

func TestVerifyCapability_BearerToken(t *testing.T) {
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "", "", expires)
	// Bearer tokens should verify regardless of the presented principal.
	if !VerifyCapability(testKey, cap, "anyone") {
		t.Error("expected bearer token to verify for any principal")
	}
}

// --- RevokedSet ---

func TestRevokedSet_AddAndCheck(t *testing.T) {
	r := NewRevokedSet()
	r.Add("cap-id-1")
	if !r.IsRevoked("cap-id-1") {
		t.Error("expected cap-id-1 to be revoked")
	}
}

func TestRevokedSet_NotRevoked(t *testing.T) {
	r := NewRevokedSet()
	if r.IsRevoked("cap-id-1") {
		t.Error("expected cap-id-1 to not be revoked")
	}
}

func TestRevokedSet_MultipleIDs(t *testing.T) {
	r := NewRevokedSet()
	r.Add("cap-id-1")
	r.Add("cap-id-2")
	if !r.IsRevoked("cap-id-1") {
		t.Error("expected cap-id-1 to be revoked")
	}
	if !r.IsRevoked("cap-id-2") {
		t.Error("expected cap-id-2 to be revoked")
	}
	if r.IsRevoked("cap-id-3") {
		t.Error("expected cap-id-3 to not be revoked")
	}
}

// --- AppendCapability / BuildRevokedSet ---

func TestAppendCapability_AddsToLog(t *testing.T) {
	store := newTestStore(t)
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)

	if err := store.AppendCapability(cap); err != nil {
		t.Fatalf("AppendCapability() error: %v", err)
	}

	if len(store.Log) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(store.Log))
	}
	if store.Log[0].Op != OpCapability {
		t.Errorf("expected op %q, got %q", OpCapability, store.Log[0].Op)
	}
}

func TestAppendCapabilityRevoke_AddsToLog(t *testing.T) {
	store := newTestStore(t)
	revoked := NewRevokedSet()
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	store.AppendCapability(cap)

	if err := store.AppendCapabilityRevoke(cap.ID, "test revocation", revoked); err != nil {
		t.Fatalf("AppendCapabilityRevoke() error: %v", err)
	}

	if len(store.Log) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(store.Log))
	}
	if store.Log[1].Op != OpCapabilityRevoke {
		t.Errorf("expected op %q, got %q", OpCapabilityRevoke, store.Log[1].Op)
	}
}

func TestAppendCapabilityRevoke_UpdatesLiveSet(t *testing.T) {
	store := newTestStore(t)
	revoked := NewRevokedSet()
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	store.AppendCapability(cap)

	store.AppendCapabilityRevoke(cap.ID, "", revoked)

	if !revoked.IsRevoked(cap.ID) {
		t.Error("expected live RevokedSet to be updated immediately")
	}
}

func TestBuildRevokedSet_PopulatesFromLog(t *testing.T) {
	store := newTestStore(t)
	revoked := NewRevokedSet()
	expires := futureTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	store.AppendCapability(cap)
	store.AppendCapabilityRevoke(cap.ID, "", revoked)

	// Build a fresh set from the log.
	fresh := NewRevokedSet()
	if err := store.BuildRevokedSet(fresh); err != nil {
		t.Fatalf("BuildRevokedSet() error: %v", err)
	}

	if !fresh.IsRevoked(cap.ID) {
		t.Error("expected revoked ID to appear in freshly built set")
	}
}

func TestBuildRevokedSet_SkipsExpired(t *testing.T) {
	store := newTestStore(t)
	revoked := NewRevokedSet()

	// Issue a capability that is already expired.
	expires := expiredTime()
	cap := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	store.AppendCapability(cap)
	store.AppendCapabilityRevoke(cap.ID, "", revoked)

	fresh := NewRevokedSet()
	store.BuildRevokedSet(fresh)

	if fresh.IsRevoked(cap.ID) {
		t.Error("expected expired capability revocation to be pruned from set")
	}
}

func TestBuildRevokedSet_OnlyRevoked(t *testing.T) {
	store := newTestStore(t)
	revoked := NewRevokedSet()
	expires := futureTime()

	cap1 := SignCapability(testKey, "hash1", "read", "alice", "", expires)
	cap2 := SignCapability(testKey, "hash2", "read", "bob", "", expires)
	store.AppendCapability(cap1)
	store.AppendCapability(cap2)
	// Only revoke cap1.
	store.AppendCapabilityRevoke(cap1.ID, "", revoked)

	fresh := NewRevokedSet()
	store.BuildRevokedSet(fresh)

	if !fresh.IsRevoked(cap1.ID) {
		t.Error("expected cap1 to be revoked")
	}
	if fresh.IsRevoked(cap2.ID) {
		t.Error("expected cap2 to not be revoked")
	}
}
