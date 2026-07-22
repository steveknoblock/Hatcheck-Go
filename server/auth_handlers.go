package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
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

// defaultNamespace derives a starting namespace for a newly-authenticated
// user, analogous to a Unix home directory: somewhere sensible to land by
// default, not a constraint on where they're allowed to work. Nothing on
// the server enforces this — namespace is purely a prefix convention on
// Name labels (see internal/metadata), so a user remains free to create or
// switch to any other namespace at any time. This only affects what the
// client pre-selects right after login.
//
// Prefers the local part of the email (before '@'), since that's usually
// the most recognizable to the user themselves; falls back to the Stytch
// user ID if no email is available. Both are slugified since namespace
// becomes a literal path segment in Name labels ("namespace/label") — it
// needs to be predictable and free of characters that would be awkward in
// a label or a URL query parameter.
func defaultNamespace(identity auth.Identity) string {
	if identity.Email != "" {
		if local, _, ok := strings.Cut(identity.Email, "@"); ok {
			if slug := slugify(local); slug != "" {
				return slug
			}
		}
	}
	if slug := slugify(identity.UserID); slug != "" {
		return slug
	}
	// Should be unreachable in practice — Stytch always provides a UserID —
	// but avoid ever handing back an empty namespace, which would just
	// look like "no default" on the client rather than "no identity".
	return "user"
}

// slugify lowercases s and replaces every run of characters other than
// a-z, 0-9, '-', and '_' with a single '-', then trims leading/trailing
// '-'. Used to turn free-form identity fields (email, user ID) into
// something safe to embed as a namespace prefix in a Name label and as a
// URL query parameter value.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// authenticateHandler handles the magic link callback from Stytch.
// It exchanges the Stytch token for a session JWT, issues a wildcard read
// capability for the authenticated user, and redirects to the UI with both.
//
// GET /auth/authenticate?token=<stytch_token>
func authenticateHandler(w http.ResponseWriter, req *http.Request, authClient *auth.Client, meta *metadata.Store, key []byte, cfg Config) {
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
	expires := time.Now().UTC().Add(cfg.CapabilityExpiry)
	readCap := metadata.SignCapability(key, "*", PermRead, identity.UserID, identity.Email, expires)
	if err := meta.AppendCapability(readCap); err != nil {
		log.Printf("warning: failed to record read capability for %s: %v", identity.UserID, err)
	}
	readCapJSON, err := json.Marshal(readCap)
	if err != nil {
		log.Printf("warning: failed to marshal read capability for %s: %v", identity.UserID, err)
	}

	// Redirect to the UI with the session JWT, user identity, read
	// capability, and a default namespace as query parameters.
	// handleMagicLinkCallback reads these, stores them in sessionStorage,
	// and cleans the URL. default_ns is only ever a starting suggestion —
	// see defaultNamespace's doc comment — the client is free to ignore or
	// override it, and nothing server-side enforces it.
	redirectURL := fmt.Sprintf("/ui/?session_jwt=%s&user_id=%s&default_ns=%s",
		url.QueryEscape(sessionJWT),
		url.QueryEscape(identity.UserID),
		url.QueryEscape(defaultNamespace(identity)),
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
