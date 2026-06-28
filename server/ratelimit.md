# ratelimit.go

## Overview

`ratelimit.go` implements per-user rate limiting for the Hatcheck server using a token bucket algorithm. It follows the same struct-with-methods middleware pattern as `AuthMiddleware` and `CapabilityMiddleware`, using the same `func(w, req, vr VerifiedRequest)` signature throughout the chain.

The underlying token bucket implementation is provided by `golang.org/x/time/rate`, an official Go extended library. Rate limiting is keyed on `vr.Principal` — the authenticated Stytch user ID set by `RequireAuth` — so limits are enforced per user rather than per IP.

---

## Types

### `RateLimitMiddleware`

```go
type RateLimitMiddleware struct {
    mu       sync.Mutex
    limiters map[string]*rate.Limiter
    tokens   rate.Limit
    burst    int
}
```

Holds a per-user map of token bucket limiters and the configuration that applies to all limiters in this pool:

- **`mu`** — mutex protecting concurrent access to the `limiters` map.
- **`limiters`** — map from user ID to `rate.Limiter`. Entries are created lazily on first request.
- **`tokens`** — the steady-state refill rate expressed as a `rate.Limit` (requests per second). Set using `rate.Every(duration)` to express rates slower than one per second.
- **`burst`** — the maximum number of tokens the bucket can hold. A user can make up to `burst` requests instantly before the steady-state rate kicks in.

---

### `RateLimiters`

```go
type RateLimiters struct {
    Read  *RateLimitMiddleware
    Write *RateLimitMiddleware
    Admin *RateLimitMiddleware
}
```

Groups three pools matching the cost profile of the API routes. Constructed once in `main()` via `NewRateLimiters()` and passed to `registerRoutes()`.

| Pool    | Routes                                                             |
|---------|--------------------------------------------------------------------|
| `Read`  | `/fetch`, `/list`, `/query`, `/namespaces`, `/names`, `/relations`, `/tags` |
| `Write` | `/stash`, `/collection`, `/relation`, `/name`                      |
| `Admin` | `/export`, `/import`, `/capability`, `/capability/revoke`          |

---

## Functions

### `NewRateLimitMiddleware`

```go
func NewRateLimitMiddleware(tokens rate.Limit, burst int) *RateLimitMiddleware
```

Constructs a single pool with the given steady-state rate and burst size.

### `NewRateLimiters`

```go
func NewRateLimiters() *RateLimiters
```

Constructs all three pools with starting limits tuned to the cost profile of each route group:

| Pool    | Sustained rate          | Burst |
|---------|-------------------------|-------|
| `Read`  | 1 request/second        | 10    |
| `Write` | 1 request/5 seconds     | 4     |
| `Admin` | 1 request/30 seconds    | 2     |

These are starting values and should be adjusted based on observed usage.

---

## Methods

### `limiterFor` (private)

```go
func (rl *RateLimitMiddleware) limiterFor(userID string) *rate.Limiter
```

Returns the `rate.Limiter` for the given user ID, creating one if it does not already exist. Protected by `sync.Mutex` for safe concurrent access.

---

### `checkLimit` (private)

```go
func (rl *RateLimitMiddleware) checkLimit(w http.ResponseWriter, vr VerifiedRequest) bool
```

Performs the rate limit check and writes response headers. Called by `Limit` on every request. Returns `true` if the request is allowed, `false` if rejected — in which case the `429` response has already been written.

Uses `limiter.Reserve()` rather than `limiter.Allow()` in order to obtain the delay duration needed for the `Retry-After` header. If the reservation cannot be fulfilled immediately, it is cancelled so the token is returned to the bucket.

Sets the following headers on every response:

| Header                  | Value                                          |
|-------------------------|------------------------------------------------|
| `X-RateLimit-Limit`     | Configured burst size                          |
| `X-RateLimit-Remaining` | Tokens available after this request (floor 0)  |
| `Retry-After`           | Seconds until next token available (429 only)  |

---

### `Limit`

```go
func (rl *RateLimitMiddleware) Limit(
    next func(http.ResponseWriter, *http.Request, VerifiedRequest),
) func(http.ResponseWriter, *http.Request, VerifiedRequest)
```

Wraps an inner handler with rate limiting. Calls `checkLimit` and only calls through to `next` if the request is allowed. Uses the same `func(w, req, vr VerifiedRequest)` signature as the rest of the middleware chain, so no type bridging is needed.

---

## Middleware chain position

`Limit` sits between `RequireAuth` and `Protect` in the chain:

```
Adapt → RequireAuth → Limit → Protect → handler
```

`RequireAuth` must run first to populate `vr.Principal`, which `Limit` uses as the rate limiting key. `Protect` runs after so that rate limit tokens are not consumed by requests that would be rejected for capability reasons anyway.

---

## Token bucket behaviour

The token bucket refills at the configured steady-state rate up to the burst ceiling. A user who has been idle accumulates tokens up to `burst`, allowing a short burst of requests before the steady-state rate applies. Tokens are never accumulated beyond `burst` regardless of idle time.

```
Burst of 10, refill 1/second:

Tokens:  10  9  8  7  6  5  4  3  2  1  0  [429]  1  2  ...
Request:  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓  ✓   ✗    ✓  ✓
```
