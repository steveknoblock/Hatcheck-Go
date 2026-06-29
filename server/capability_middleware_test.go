package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// --- Test helpers ---

var middlewareKey = []byte("middleware-test-key")

func newTestMiddleware(t *testing.T) *CapabilityMiddleware {
	t.Helper()
	return &CapabilityMiddleware{
		Key:     middlewareKey,
		Revoked: metadata.NewRevokedSet(),
	}
}

func newTestMiddlewareWithBootstrap(t *testing.T, token string) *CapabilityMiddleware {
	t.Helper()
	return &CapabilityMiddleware{
		Key:            middlewareKey,
		Revoked:        metadata.NewRevokedSet(),
		BootstrapToken: token,
	}
}

// signedCapToken returns a JSON-encoded capability token for use in requests.
func signedCapToken(t *testing.T, hash, perm, principal string, expires time.Time) string {
	t.Helper()
	cap := metadata.SignCapability(middlewareKey, hash, perm, principal, "", expires)
	b, err := json.Marshal(cap)
	if err != nil {
		t.Fatalf("failed to marshal capability: %v", err)
	}
	return string(b)
}

func futureExpiry() time.Time {
	return time.Now().UTC().Add(1 * time.Hour)
}

func expiredExpiry() time.Time {
	return time.Now().UTC().Add(-1 * time.Hour)
}

// innerOK is a simple inner handler that records it was called.
func innerOK(called *bool) func(http.ResponseWriter, *http.Request, VerifiedRequest) {
	return func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}
}

// vrWithPrincipalFor returns a VerifiedRequest with the given principal.
// Used to simulate what RequireAuth sets before Protect runs.
func vrWithPrincipalFor(principal string) VerifiedRequest {
	return VerifiedRequest{Principal: principal}
}

// --- Protect: missing capability token ---

func TestProtect_MissingCapabilityToken(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=abc", nil)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: malformed token ---

func TestProtect_MalformedToken(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	req := httptest.NewRequest(http.MethodGet, "/fetch", nil)
	req.Header.Set("X-Capability-Token", "not-valid-json")
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: valid capability ---

func TestProtect_ValidCapability(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	token := signedCapToken(t, "hash1", PermRead, "alice", futureExpiry())
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=hash1", nil)
	req.Header.Set("X-Capability-Token", token)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("expected inner handler to be called")
	}
}

// --- Protect: expired capability ---

func TestProtect_ExpiredCapability(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	token := signedCapToken(t, "hash1", PermRead, "alice", expiredExpiry())
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=hash1", nil)
	req.Header.Set("X-Capability-Token", token)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: wrong principal ---

func TestProtect_WrongPrincipal(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	// Capability was issued to alice but bob is presenting it.
	token := signedCapToken(t, "hash1", PermRead, "alice", futureExpiry())
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=hash1", nil)
	req.Header.Set("X-Capability-Token", token)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("bob"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: wrong perm ---

func TestProtect_WrongPerm(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	// Route requires PermWrite but token grants PermRead.
	handler := cm.Protect(PermWrite, innerOK(&called))

	token := signedCapToken(t, "hash1", PermRead, "alice", futureExpiry())
	req := httptest.NewRequest(http.MethodPost, "/stash", nil)
	req.Header.Set("X-Capability-Token", token)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: revoked capability ---

func TestProtect_RevokedCapability(t *testing.T) {
	cm := newTestMiddleware(t)
	called := false
	handler := cm.Protect(PermRead, innerOK(&called))

	expires := futureExpiry()
	cap := metadata.SignCapability(middlewareKey, "hash1", PermRead, "alice", "", expires)
	cm.Revoked.Add(cap.ID)

	b, _ := json.Marshal(cap)
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=hash1", nil)
	req.Header.Set("X-Capability-Token", string(b))
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

// --- Protect: VerifiedRequest passed to inner handler ---

func TestProtect_VerifiedRequestPopulated(t *testing.T) {
	cm := newTestMiddleware(t)
	var gotVR VerifiedRequest
	handler := cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		gotVR = vr
		w.WriteHeader(http.StatusOK)
	})

	token := signedCapToken(t, "hash1", PermRead, "alice", futureExpiry())
	req := httptest.NewRequest(http.MethodGet, "/fetch?hash=hash1", nil)
	req.Header.Set("X-Capability-Token", token)
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if gotVR.Principal != "alice" {
		t.Errorf("expected principal alice, got %q", gotVR.Principal)
	}
	if gotVR.Capability.Hash != "hash1" {
		t.Errorf("expected hash1, got %q", gotVR.Capability.Hash)
	}
}

// --- Bootstrap token ---

func TestProtect_BootstrapTokenGrantsAdmin(t *testing.T) {
	cm := newTestMiddlewareWithBootstrap(t, "secret-bootstrap")
	called := false
	handler := cm.Protect(PermAdmin, innerOK(&called))

	req := httptest.NewRequest(http.MethodPost, "/capability", nil)
	req.Header.Set("X-Bootstrap-Token", "secret-bootstrap")
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("expected inner handler to be called")
	}
}

func TestProtect_BootstrapTokenWrongValue(t *testing.T) {
	cm := newTestMiddlewareWithBootstrap(t, "secret-bootstrap")
	called := false
	handler := cm.Protect(PermAdmin, innerOK(&called))

	req := httptest.NewRequest(http.MethodPost, "/capability", nil)
	req.Header.Set("X-Bootstrap-Token", "wrong-token")
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

func TestProtect_BootstrapTokenOnlyGrantsAdmin(t *testing.T) {
	cm := newTestMiddlewareWithBootstrap(t, "secret-bootstrap")
	called := false
	// Bootstrap token presented on a PermRead route — should be rejected.
	handler := cm.Protect(PermRead, innerOK(&called))

	req := httptest.NewRequest(http.MethodGet, "/fetch", nil)
	req.Header.Set("X-Bootstrap-Token", "secret-bootstrap")
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}

func TestProtect_BootstrapTokenDisabledWhenEmpty(t *testing.T) {
	// BootstrapToken is empty — presenting X-Bootstrap-Token should have no effect.
	cm := newTestMiddleware(t) // no bootstrap token set
	called := false
	handler := cm.Protect(PermAdmin, innerOK(&called))

	req := httptest.NewRequest(http.MethodPost, "/capability", nil)
	req.Header.Set("X-Bootstrap-Token", "any-value")
	w := httptest.NewRecorder()
	handler(w, req, vrWithPrincipalFor("alice"))

	// Should fall through to capability token check and fail with 403.
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("expected inner handler not to be called")
	}
}
