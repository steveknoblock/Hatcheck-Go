package auth

// Package auth provides Stytch-based authentication for Hatcheck.
// It wraps the Stytch Go SDK to provide magic link sending, token
// authentication, and session JWT validation.
//
// Configuration is loaded from environment variables:
//
//	STYTCH_PROJECT_ID   — Stytch project ID
//	STYTCH_SECRET       — Stytch project secret
//	STYTCH_REDIRECT_URL — Magic link redirect URL (e.g. http://localhost:8090/auth/authenticate)

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/stytchauth/stytch-go/v18/stytch/consumer/magiclinks"
	emailML "github.com/stytchauth/stytch-go/v18/stytch/consumer/magiclinks/email"
	"github.com/stytchauth/stytch-go/v18/stytch/consumer/sessions"
	"github.com/stytchauth/stytch-go/v18/stytch/consumer/stytchapi"
)

// SessionDurationMinutes is the lifetime of a Stytch session after magic link
// authentication. The session JWT itself has a fixed 5-minute lifetime and is
// refreshed by the client; the underlying session lasts this long.
const SessionDurationMinutes = 60 * 24 // 24 hours

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
// It returns the verified Identity and the session JWT to be returned to
// the client. The client stores the JWT and presents it on subsequent
// requests via the Authorization header.
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

	// Populate email from the first verified email on the user object if present.
	if resp.User != nil {
		for _, e := range resp.User.Emails {
			if e.Verified {
				identity.Email = e.Email
				break
			}
		}
	}

	return identity, resp.SessionJWT, nil
}

// ValidateSessionJWT validates a session JWT locally without a network call.
// It returns the verified Identity if the JWT is valid and the session has
// not expired. This is the hot path — called on every authenticated request.
//
// If local validation fails (e.g. JWT is near expiry), the caller should
// fall back to ValidateSessionJWTRemote for a fresh validation against Stytch.
func (c *Client) ValidateSessionJWT(ctx context.Context, sessionJWT string) (Identity, error) {
	resp, err := c.api.Sessions.AuthenticateJWT(
		ctx,
		&sessions.AuthenticateJWTParams{
			SessionJWT:             sessionJWT,
			SessionDurationMinutes: SessionDurationMinutes,
			MaxTokenAgeSeconds:     300, // 5 minutes — Stytch JWT fixed lifetime
		},
	)
	if err != nil {
		return Identity{}, fmt.Errorf("invalid session JWT: %w", err)
	}

	identity := Identity{
		UserID: resp.Session.UserID,
	}

	// Populate email from session user if available.
	if resp.User != nil {
		for _, e := range resp.User.Emails {
			if e.Verified {
				identity.Email = e.Email
				break
			}
		}
	}

	return identity, nil
}

// ValidateSessionJWTRemote validates a session JWT against the Stytch API.
// Use this as a fallback when local validation fails, or when a fresh
// authoritative check is required.
func (c *Client) ValidateSessionJWTRemote(ctx context.Context, sessionJWT string) (Identity, error) {
	resp, err := c.api.Sessions.Authenticate(
		ctx,
		&sessions.AuthenticateParams{
			SessionJWT:             sessionJWT,
			SessionDurationMinutes: SessionDurationMinutes,
		},
	)
	if err != nil {
		return Identity{}, fmt.Errorf("remote session validation failed: %w", err)
	}

	identity := Identity{
		UserID: resp.Session.UserID,
	}

	if resp.User != nil {
		for _, e := range resp.User.Emails {
			if e.Verified {
				identity.Email = e.Email
				break
			}
		}
	}

	return identity, nil
}
