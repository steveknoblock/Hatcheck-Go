package main

import (
	"net/http"
	"strings"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
)

// AuthMiddleware wraps handlers with JWT validation.
// It extracts the Bearer token from the Authorization header, validates it
// against Stytch, builds a VerifiedRequest from the identity, and passes
// it to the next handler in the chain.
//
// The middleware tries local JWT validation first (no network call) and
// falls back to remote validation if local validation fails.
type AuthMiddleware struct {
	Client *auth.Client
}

// RequireAuth validates the session JWT and builds a VerifiedRequest from
// the verified identity. It is the single point where VerifiedRequest is
// constructed — all subsequent middleware and handlers receive it by value.
func (am *AuthMiddleware) RequireAuth(
	next func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest),
) func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
	return func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Authorization header must use Bearer scheme", http.StatusUnauthorized)
			return
		}

		sessionJWT := strings.TrimPrefix(authHeader, "Bearer ")
		if sessionJWT == "" {
			http.Error(w, "missing session JWT", http.StatusUnauthorized)
			return
		}

		identity, err := am.Client.ValidateSessionJWT(req.Context(), sessionJWT)
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}

		// Build the VerifiedRequest from the validated identity.
		// Capability fields are left zero — Protect fills them in if present.
		vr = VerifiedRequest{
			Principal: identity.UserID,
			Email:     identity.Email,
		}

		next(w, req, vr)
	}
}

// Adapt converts a func(w, req, vr VerifiedRequest) to http.HandlerFunc
// for use with http.HandleFunc. It is the only place in the server where
// this conversion happens — all middleware and handlers use the inner
// signature throughout the chain.
func Adapt(inner func(http.ResponseWriter, *http.Request, VerifiedRequest)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		inner(w, req, VerifiedRequest{})
	}
}
