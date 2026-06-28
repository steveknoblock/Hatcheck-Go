package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
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
// It exchanges the Stytch token for a session JWT, issues a wildcard read
// capability for the authenticated user, and redirects to the UI with both.
//
// GET /auth/authenticate?token=<stytch_token>
func authenticateHandler(w http.ResponseWriter, req *http.Request, authClient *auth.Client, meta *metadata.Store, key []byte) {
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
		log.Printf("AuthenticateMagicLink error: %v", err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// Issue a wildcard read capability for this user. This allows them to
	// read any object in the CAS without needing a per-object capability.
	// Write and admin operations still require a specific capability.
	expires := time.Now().UTC().Add(capabilityExpiry)
	readCap := metadata.SignCapability(key, "*", PermRead, identity.UserID, identity.Email, expires)
	if err := meta.AppendCapability(readCap); err != nil {
		log.Printf("warning: failed to record read capability for %s: %v", identity.UserID, err)
	}
	readCapJSON, err := json.Marshal(readCap)
	if err != nil {
		log.Printf("warning: failed to marshal read capability for %s: %v", identity.UserID, err)
	}

	// Redirect to the UI with the session JWT, user identity, and read
	// capability as query parameters. handleMagicLinkCallback reads these,
	// stores them in sessionStorage, and cleans the URL.
	redirectURL := fmt.Sprintf("/ui/?session_jwt=%s&user_id=%s",
		url.QueryEscape(sessionJWT),
		url.QueryEscape(identity.UserID),
	)
	if identity.Email != "" {
		redirectURL += "&email=" + url.QueryEscape(identity.Email)
	}
	if readCapJSON != nil {
		redirectURL += "&read_cap=" + url.QueryEscape(string(readCapJSON))
	}
	http.Redirect(w, req, redirectURL, http.StatusFound)
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
