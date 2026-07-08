package main

import (
	"fmt"
	"math"
	"net/http"
	"sync"
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
	Read  *RateLimitMiddleware // /fetch, /list, /query, /namespaces, /names, /relations, /tags
	Write *RateLimitMiddleware // /stash, /collection, /relation, /name
	Admin *RateLimitMiddleware // /export, /import, /capability, /capability/revoke
}

// NewRateLimiters constructs the three pools from the provided Config.
// Call once from main() after LoadConfig().
func NewRateLimiters(cfg Config) *RateLimiters {
	return &RateLimiters{
		Read:  NewRateLimitMiddleware(cfg.RateReadTokens, cfg.RateReadBurst),
		Write: NewRateLimitMiddleware(cfg.RateWriteTokens, cfg.RateWriteBurst),
		Admin: NewRateLimitMiddleware(cfg.RateAdminTokens, cfg.RateAdminBurst),
	}
}
