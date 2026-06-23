package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sethvargo/go-limiter/httplimit"
	"github.com/sethvargo/go-limiter/memorystore"
)

// Pool wraps httplimit.Middleware to add the Limit convenience method.
// We can't define methods directly on httplimit.Middleware since it belongs
// to an external package, so we own this type instead.
type Pool struct {
	mw *httplimit.Middleware
}

// Limit wraps a HandlerFunc with this pool's rate limiter.
// httplimit.Middleware.Handle accepts an http.Handler and returns an
// http.Handler; this method bridges that to the HandlerFunc pattern
// used throughout this server.
func (p *Pool) Limit(next http.HandlerFunc) http.HandlerFunc {
	return p.mw.Handle(next).ServeHTTP
}

// RateLimiters holds the three limiter pools used across the API.
// Each pool is keyed on the authenticated Stytch user ID (X-User-ID),
// which AuthMiddleware sets after validating the session JWT.
type RateLimiters struct {
	Read  *Pool // /fetch, /list, /query, /namespaces, /names, /relations, /tags
	Write *Pool // /stash, /collection, /relation, /name
	Admin *Pool // /export, /import, /capability, /capability/revoke
}

// userIDKeyFunc returns a KeyFunc that extracts the authenticated user ID
// from the X-User-ID header. AuthMiddleware sets this header after validating
// the session JWT, so it is always the verified Stytch user ID by the time
// any rate limiter sees the request.
func userIDKeyFunc() httplimit.KeyFunc {
	return func(r *http.Request) (string, error) {
		uid := r.Header.Get("X-User-ID")
		if uid == "" {
			return "", fmt.Errorf("missing X-User-ID header")
		}
		return uid, nil
	}
}

// newPool creates a single Pool with the given token budget and window interval.
func newPool(tokens uint64, interval time.Duration) (*Pool, error) {
	store, err := memorystore.New(&memorystore.Config{
		Tokens:   tokens,
		Interval: interval,
	})
	if err != nil {
		return nil, err
	}
	mw, err := httplimit.NewMiddleware(store, userIDKeyFunc())
	if err != nil {
		return nil, err
	}
	return &Pool{mw: mw}, nil
}

// NewRateLimiters constructs the three limiter pools. Call once from main()
// and pass the result to each route registration.
//
// Starting limits — adjust based on observed usage:
//
//	Read:  60 requests per minute per user
//	Write: 20 requests per minute per user
//	Admin:  5 requests per minute per user (export/import traverse the full graph)
func NewRateLimiters() *RateLimiters {
	read, err := newPool(60, time.Minute)
	if err != nil {
		log.Fatalf("failed to create read rate limiter: %v", err)
	}

	write, err := newPool(20, time.Minute)
	if err != nil {
		log.Fatalf("failed to create write rate limiter: %v", err)
	}

	admin, err := newPool(5, time.Minute)
	if err != nil {
		log.Fatalf("failed to create admin rate limiter: %v", err)
	}

	return &RateLimiters{
		Read:  read,
		Write: write,
		Admin: admin,
	}
}
