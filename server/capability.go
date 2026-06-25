package main

import (
	"crypto/hmac"
	"encoding/json"
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// CapabilityMiddleware holds the signing key and revocation index
// needed to verify capability tokens on every request.
// BootstrapToken, if set, is a shared secret that grants PermAdmin access
// without a signed capability. It is intended for first-time setup only
// and should be rotated or unset once real admin capabilities are issued.
type CapabilityMiddleware struct {
	Key            []byte
	Revoked        *metadata.RevokedSet
	BootstrapToken string
}

// VerifiedRequest carries the verified identity and capability
// through the middleware chain into the inner handler.
// Principal and Email are always set by RequireAuth.
// Capability is set by Protect if a capability token is present.
type VerifiedRequest struct {
	Capability metadata.CapabilityPayload
	Principal  string
	Email      string
}

// Protect wraps a handler with capability verification. It receives a
// VerifiedRequest already populated with identity by RequireAuth, adds the
// verified capability, and passes the complete VerifiedRequest to the inner
// handler. The inner handler is only called if verification passes.
func (cm *CapabilityMiddleware) Protect(
	perm string,
	inner func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest),
) func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
	return func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		// Bootstrap token check — grants PermAdmin without a signed capability.
		// Only active when HATCHECK_BOOTSTRAP_TOKEN is set. Should be unset
		// once real admin capabilities have been issued.
		if cm.BootstrapToken != "" {
			token := req.Header.Get("X-Bootstrap-Token")
			if token != "" {
				if !hmac.Equal([]byte(token), []byte(cm.BootstrapToken)) {
					http.Error(w, "invalid bootstrap token", http.StatusForbidden)
					return
				}
				if perm != PermAdmin {
					http.Error(w, "bootstrap token only grants admin access", http.StatusForbidden)
					return
				}
				inner(w, req, vr)
				return
			}
		}

		// Extract capability token from header.
		capToken := req.Header.Get("X-Capability-Token")
		if capToken == "" {
			http.Error(w, "missing capability token", http.StatusForbidden)
			return
		}

		// Decode the capability payload from the token.
		var cap metadata.CapabilityPayload
		if err := json.Unmarshal([]byte(capToken), &cap); err != nil {
			http.Error(w, "malformed capability token", http.StatusBadRequest)
			return
		}

		// Verify signature, expiry, and principal.
		if !metadata.VerifyCapability(cm.Key, cap, vr.Principal) {
			http.Error(w, "invalid or expired capability", http.StatusForbidden)
			return
		}

		// Check revocation index.
		if cm.Revoked.IsRevoked(cap.ID) {
			http.Error(w, "capability has been revoked", http.StatusForbidden)
			return
		}

		// Check the capability permits the required operation.
		// Admin capabilities satisfy any permission check.
		// Write capabilities also satisfy read permission checks.
		if cap.Perm != PermAdmin && cap.Perm != perm {
			if !(cap.Perm == PermWrite && perm == PermRead) {
				http.Error(w, "capability does not permit this operation", http.StatusForbidden)
				return
			}
		}

		// Add the verified capability to the VerifiedRequest and call through.
		vr.Capability = cap
		inner(w, req, vr)
	}
}
