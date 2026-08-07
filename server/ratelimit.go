package main

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitMiddleware holds a per-user map of token bucket limiters.
// Each limiter is keyed on the Principal field of the VerifiedRequest,
// which RequireAuth sets after validating the session JWT.
type RateLimitMiddleware struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	tokens   rate.Limit // steady-state rate in requests per second
	burst    int        // maximum burst size
}

// NewRateLimitMiddleware creates a RateLimitMiddleware with the given
// steady-state rate and burst size.
func NewRateLimitMiddleware(tokens rate.Limit, burst int) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiters: make(map[string]*rate.Limiter),
		tokens:   tokens,
		burst:    burst,
	}
}

// limiterFor returns the rate.Limiter for the given user ID, creating
// one if it does not already exist.
func (rl *RateLimitMiddleware) limiterFor(userID string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	l, ok := rl.limiters[userID]
	if !ok {
		l = rate.NewLimiter(rl.tokens, rl.burst)
		rl.limiters[userID] = l
	}
	return l
}

// checkLimit performs the rate limit check and sets response headers.
// Returns true if the request is allowed, false if it was rejected.
// On rejection the 429 response is written before returning false.
func (rl *RateLimitMiddleware) checkLimit(w http.ResponseWriter, vr VerifiedRequest) bool {
	if vr.Principal == "" {
		http.Error(w, "missing user identity", http.StatusUnauthorized)
		return false
	}

	limiter := rl.limiterFor(vr.Principal)
	reservation := limiter.Reserve()

	// Always set the limit and remaining headers.
	w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.burst))
	remaining := int(math.Max(0, limiter.Tokens()))
	w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

	if !reservation.OK() || reservation.Delay() > 0 {
		reservation.Cancel()
		retryAfter := int(math.Ceil(reservation.Delay().Seconds()))
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return false
	}

	return true
}

// Limit wraps a handler with rate limiting keyed on vr.Principal.
// It sits between RequireAuth and Protect in the chain, using the
// same func(w, req, vr VerifiedRequest) signature throughout.
//
// Sets the following headers on every response:
//
//	X-RateLimit-Limit     — configured burst size
//	X-RateLimit-Remaining — tokens available after this request
//	Retry-After           — seconds until next token available (429 only)
func (rl *RateLimitMiddleware) Limit(
	next func(http.ResponseWriter, *http.Request, VerifiedRequest),
) func(http.ResponseWriter, *http.Request, VerifiedRequest) {
	return func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		if rl.checkLimit(w, vr) {
			next(w, req, vr)
		}
	}
}

// RateLimiters holds three pools matching the cost profile of the API routes.
type RateLimiters struct {
	Read  *RateLimitMiddleware // /fetch, /list, /query, /namespaces, /names, /relations, /tags, /object-meta
	Write *RateLimitMiddleware // /stash, /collection, /relation, /name
	Admin *RateLimitMiddleware // /export, /import, /capability, /capability/revoke
}

// NewRateLimiters constructs the three pools. Call once from main().
//
// Starting limits — adjust based on observed usage:
//
//	Read:  3 requests/second sustained, burst of 30
//	Write: 1 request/5 seconds sustained, burst of 4
//	Admin: 1 request/30 seconds sustained, burst of 2
//
// Read was bumped from (1/s, burst 10) after the Map tab's treemap started
// tripping 429s on the second click: loadMapPanel fires up to
// 2 × (1 + neighbor count) GET requests concurrently via Promise.all for
// every navigation (relations + object-meta for the center, plus the same
// pair per unique neighbor). A node with even a handful of relations can
// burn through the old burst of 10 in one click, leaving too little
// headroom to immediately click again. The UI-side fix (caching
// object-meta and neighbor fan-out per hash, see loadMapPanel in
// ui/index.html) cuts repeat-visit request volume close to zero, but a
// first-time visit to a densely-connected object still needs real
// headroom here rather than relying on caching alone.
func NewRateLimiters() *RateLimiters {
	return &RateLimiters{
		Read:  NewRateLimitMiddleware(rate.Limit(3), 30),
		Write: NewRateLimitMiddleware(rate.Every(5*time.Second), 4),
		Admin: NewRateLimitMiddleware(rate.Every(30*time.Second), 2),
	}
}
