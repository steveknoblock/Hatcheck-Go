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
// Each limiter is keyed on the authenticated Stytch user ID, which
// AuthMiddleware sets as X-User-ID after validating the session JWT.
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

// Limit wraps a HandlerFunc with rate limiting keyed on the authenticated
// user ID. It sits inside am.RequireAuth so that X-User-ID is always set
// before the limiter sees the request.
//
// Sets the following headers on every response:
//
//	X-RateLimit-Limit     — configured burst size
//	X-RateLimit-Remaining — tokens available after this request
//	Retry-After           — seconds until next token available (429 only)
func (rl *RateLimitMiddleware) Limit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		userID := req.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "missing user identity", http.StatusUnauthorized)
			return
		}

		limiter := rl.limiterFor(userID)
		reservation := limiter.Reserve()

		// Always set the limit and remaining headers.
		w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", rl.burst))
		remaining := int(math.Max(0, limiter.Tokens()))
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))

		if !reservation.OK() || reservation.Delay() > 0 {
			// Cancel the reservation so the token is returned to the bucket.
			reservation.Cancel()
			retryAfter := int(math.Ceil(reservation.Delay().Seconds()))
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next(w, req)
	}
}

// LimitFunc wraps a func(w, req, vr VerifiedRequest) with rate limiting keyed
// on the authenticated user ID. Use this for routes that go through
// RequireAuthWithIdentity instead of RequireAuth, since those routes use a
// different inner function signature.
func (rl *RateLimitMiddleware) LimitFunc(next func(http.ResponseWriter, *http.Request, VerifiedRequest)) func(http.ResponseWriter, *http.Request, VerifiedRequest) {
	return func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		userID := req.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "missing user identity", http.StatusUnauthorized)
			return
		}

		limiter := rl.limiterFor(userID)
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
			return
		}

		next(w, req, vr)
	}
}

// RateLimiters holds three pools matching the cost profile of the API routes.
type RateLimiters struct {
	Read  *RateLimitMiddleware // /fetch, /list, /query, /namespaces, /names, /relations, /tags
	Write *RateLimitMiddleware // /stash, /collection, /relation, /name
	Admin *RateLimitMiddleware // /export, /import, /capability, /capability/revoke
}

// NewRateLimiters constructs the three pools. Call once from main().
//
// Starting limits — adjust based on observed usage:
//
//	Read:  1 request/second sustained, burst of 10
//	Write: 1 request/5 seconds sustained, burst of 4
//	Admin: 1 request/30 seconds sustained, burst of 2
func NewRateLimiters() *RateLimiters {
	return &RateLimiters{
		Read:  NewRateLimitMiddleware(rate.Every(time.Second), 10),
		Write: NewRateLimitMiddleware(rate.Every(5*time.Second), 4),
		Admin: NewRateLimitMiddleware(rate.Every(30*time.Second), 2),
	}
}
