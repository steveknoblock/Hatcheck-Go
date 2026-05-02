// Package auth provides Stytch-based authentication for Hatcheck.
// It wraps the Stytch Go SDK to provide magic link sending, token
// authentication, and session JWT validation.
//
// Configuration is loaded from environment variables:
//
//	STYTCH_PROJECT_ID   — Stytch project ID
//	STYTCH_SECRET       — Stytch project secret
//	STYTCH_REDIRECT_URL — Magic link redirect URL (e.g. http://localhost:8090/auth/authenticate)
package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/stytchauth/stytch-go/v18/stytch/consumer/magiclinks"
	emailML "github.com/stytchauth/stytch-go/v18/stytch/consumer/magiclinks/email"
	"github.com/stytchauth/stytch-go/v18/stytch/consumer/sessions"
	"github.com/stytchauth/stytch-go/v18/stytch/consumer/stytchapi"
)

// SessionDurationMinutes is the lifetime of a Stytch session after magic link
// authentication.
const SessionDurationMinutes = 60 * 24 // 24 hours

// maxTokenAge is the maximum age of a JWT before local validation falls back
// to a remote Stytch API call. Stytch JWTs have a fixed 5-minute lifetime.
const maxTokenAge = 5 * time.Minute

// Identity holds the verified identity of an authenticated user extracted
// from a Stytch session JWT. It is passed into the capability middleware
// as the trusted user identity.
type Identity struct {
	// UserID is the Stytch user ID (e.g. "user-live-abc123"). This is the
	// stable, unique identifier used as Principal in capabilities.
	UserID string

	// Email is the user's email address, populated only if available from
	// the session. Used for display purposes only — UserID is authoritative.
	Email string
}

// Client wraps the Stytch API client and exposes the authentication
// operations needed by the Hatcheck server.
type Client struct {
	api         *stytchapi.API
	redirectURL string
}

// NewClient creates a new auth Client from environment variables.
// Returns an error if required environment variables are missing or
// if the Stytch client cannot be initialised.
func NewClient() (*Client, error) {
	projectID := os.Getenv("STYTCH_PROJECT_ID")
	secret := os.Getenv("STYTCH_SECRET")
	redirectURL := os.Getenv("STYTCH_REDIRECT_URL")

	if projectID == "" {
		return nil, errors.New("STYTCH_PROJECT_ID environment variable must be set")
	}
	if secret == "" {
		return nil, errors.New("STYTCH_SECRET environment variable must be set")
	}
	if redirectURL == "" {
		return nil, errors.New("STYTCH_REDIRECT_URL environment variable must be set")
	}

	api, err := stytchapi.NewClient(projectID, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to create Stytch client: %w", err)
	}

	return &Client{
		api:         api,
		redirectURL: redirectURL,
	}, nil
}

// SendMagicLink sends a magic link email to the given address. If the user
// does not exist in Stytch they are created automatically. The magic link
// redirects to the configured STYTCH_REDIRECT_URL on click.
func (c *Client) SendMagicLink(ctx context.Context, email string) error {
	_, err := c.api.MagicLinks.Email.LoginOrCreate(
		ctx,
		&emailML.LoginOrCreateParams{
			Email:              email,
			LoginMagicLinkURL:  c.redirectURL,
			SignupMagicLinkURL: c.redirectURL,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to send magic link: %w", err)
	}
	return nil
}

// AuthenticateMagicLink exchanges a magic link token for a session JWT.
// Returns the verified Identity and the session JWT to be returned to
// the client. The client stores the JWT and presents it as a Bearer token
// on subsequent requests.
func (c *Client) AuthenticateMagicLink(ctx context.Context, token string) (Identity, string, error) {
	resp, err := c.api.MagicLinks.Authenticate(
		ctx,
		&magiclinks.AuthenticateParams{
			Token:                  token,
			SessionDurationMinutes: SessionDurationMinutes,
		},
	)
	if err != nil {
		return Identity{}, "", fmt.Errorf("failed to authenticate magic link: %w", err)
	}

	identity := Identity{
		UserID: resp.UserID,
	}

	// Populate email from the first verified email on the user object.
	// User is a value type in v18 — check UserID rather than nil.
	if resp.User.UserID != "" {
		for _, e := range resp.User.Emails {
			if e.Verified {
				identity.Email = e.Email
				break
			}
		}
	}

	return identity, resp.SessionJWT, nil
}

// ValidateSessionJWT validates a session JWT, trying local validation first
// and falling back to a remote Stytch API call if the JWT is older than
// maxTokenAge. This is the hot path called on every authenticated request.
//
// In v18, AuthenticateJWT handles both local and remote validation automatically:
// it validates locally if the JWT is fresh, and falls back to remote if not.
func (c *Client) ValidateSessionJWT(ctx context.Context, sessionJWT string) (Identity, error) {
	resp, err := c.api.Sessions.AuthenticateJWT(
		ctx,
		maxTokenAge,
		&sessions.AuthenticateParams{
			SessionJWT:             sessionJWT,
			SessionDurationMinutes: SessionDurationMinutes,
		},
	)
	if err != nil {
		return Identity{}, fmt.Errorf("invalid or expired session: %w", err)
	}

	identity := Identity{
		UserID: resp.Session.UserID,
	}

	// Populate email from user object if returned (only on remote validation).
	if resp.User.UserID != "" {
		for _, e := range resp.User.Emails {
			if e.Verified {
				identity.Email = e.Email
				break
			}
		}
	}

	return identity, nil
}
