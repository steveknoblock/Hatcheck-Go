package main

import (
	"encoding/json"
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// CapabilityMiddleware holds the signing key and revocation index
// needed to verify capability tokens on every request.
type CapabilityMiddleware struct {
	Key     []byte
	Revoked *metadata.RevokedSet
}

// VerifiedRequest carries the verified capability and principal
// into the inner handler after the middleware has confirmed them.
type VerifiedRequest struct {
	Capability metadata.CapabilityPayload
	Principal  string
}

// Protect wraps a handler with capability verification. The inner handler
// receives a VerifiedRequest and is only called if verification passes.
// It does not check whether the capability's Hash or Perm match the
// specific object or operation — that is the responsibility of the
// inner handler.
func (cm *CapabilityMiddleware) Protect(
	perm string,
	inner func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest),
) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Extract principal from auth service header.
		principal := req.Header.Get("X-User-ID")
		if principal == "" {
			http.Error(w, "missing user identity", http.StatusUnauthorized)
			return
		}

		// Extract capability token from header.
		token := req.Header.Get("X-Capability-Token")
		if token == "" {
			http.Error(w, "missing capability token", http.StatusUnauthorized)
			return
		}

		// Decode the capability payload from the token.
		var cap metadata.CapabilityPayload
		if err := json.Unmarshal([]byte(token), &cap); err != nil {
			http.Error(w, "malformed capability token", http.StatusBadRequest)
			return
		}

		// Verify signature, expiry, and principal.
		if !metadata.VerifyCapability(cm.Key, cap, principal) {
			http.Error(w, "invalid or expired capability", http.StatusForbidden)
			return
		}

		// Check revocation index.
		if cm.Revoked.IsRevoked(cap.ID) {
			http.Error(w, "capability has been revoked", http.StatusForbidden)
			return
		}

		// Check the capability permits the required operation.
		if cap.Perm != perm {
			http.Error(w, "capability does not permit this operation", http.StatusForbidden)
			return
		}

		inner(w, req, VerifiedRequest{
			Capability: cap,
			Principal:  principal,
		})
	}
}
