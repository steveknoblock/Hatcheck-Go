package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
)

// loginHandler accepts a POST with an email address and sends a magic link.
// POST /auth/login
// Body: {"email": "user@example.com"}
func loginHandler(w http.ResponseWriter, req *http.Request, authClient *auth.Client) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	if body.Email == "" {
		http.Error(w, "email is required", http.StatusBadRequest)
		return
	}

	if err := authClient.SendMagicLink(req.Context(), body.Email); err != nil {
		log.Printf("SendMagicLink error: %v", err)
		http.Error(w, "failed to send magic link", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "magic link sent")
}

// authenticateHandler handles the magic link callback from Stytch.
// The user clicks the magic link in their email and is redirected here
// with a token query parameter. The server exchanges the token for a
// session JWT and returns it to the client.
//
// GET /auth/authenticate?token=<stytch_token>&stytch_token_type=magic_links
func authenticateHandler(w http.ResponseWriter, req *http.Request, authClient *auth.Client) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := req.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token parameter", http.StatusBadRequest)
		return
	}

	identity, sessionJWT, err := authClient.AuthenticateMagicLink(req.Context(), token)
	if err != nil {
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Return the session JWT and identity to the client as JSON.
	// The client stores the JWT and presents it as a Bearer token on
	// subsequent requests.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(struct {
		SessionJWT string `json:"session_jwt"`
		UserID     string `json:"user_id"`
		Email      string `json:"email,omitempty"`
	}{
		SessionJWT: sessionJWT,
		UserID:     identity.UserID,
		Email:      identity.Email,
	})
}

// logoutHandler instructs the client to discard its session JWT.
// Since JWTs are stateless there is no server-side session to invalidate —
// the client simply stops presenting the token.
//
// POST /auth/logout
func logoutHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "logged out")
}

// --- JWT middleware ---

// AuthMiddleware wraps a capability-protected handler with JWT validation.
// It extracts the Bearer token from the Authorization header, validates it
// against Stytch, and sets the verified user ID on the request via the
// X-User-ID header before passing to the next handler.
//
// The middleware tries local JWT validation first (no network call) and
// falls back to remote validation if local validation fails.
type AuthMiddleware struct {
	Client *auth.Client
}

// Wrap adds JWT validation in front of a handler that expects X-User-ID to
// be set. It is designed to sit before CapabilityMiddleware in the chain.
func (am *AuthMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
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

		// AuthenticateJWT tries local validation first and falls back to
		// remote automatically — no separate fallback needed.
		identity, err := am.Client.ValidateSessionJWT(req.Context(), sessionJWT)
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}

		// Set the verified user ID so the capability middleware can read it.
		// This replaces the manual X-User-ID header that would otherwise be
		// set by an upstream auth proxy.
		req.Header.Set("X-User-ID", identity.UserID)

		// Optionally propagate email for capability issuance opt-in.
		if identity.Email != "" {
			req.Header.Set("X-User-Email", identity.Email)
		}

		next(w, req)
	}
}
