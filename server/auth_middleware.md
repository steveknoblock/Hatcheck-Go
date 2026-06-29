# auth_middleware.go

## Overview

`auth_middleware.go` implements the JWT authentication layer of the Hatcheck server middleware chain. It defines `AuthMiddleware` with a single method `RequireAuth`, and the package-level `Adapt` function that bridges the internal middleware signature to the standard library's `http.HandlerFunc`.

This file is the single point where `VerifiedRequest` is constructed. All subsequent middleware and handlers receive identity through `vr.Principal` and `vr.Email` rather than reading request headers directly.

---

## Types

### `AuthMiddleware`

```go
type AuthMiddleware struct {
    Client *auth.Client
}
```

Holds the Stytch auth client used for JWT validation. Constructed once in `main()` and shared across all routes.

- **`Client`** — the Stytch client wrapping both local JWKS-based validation and remote fallback validation. Local validation requires no network call; remote validation is used when local validation fails.

---

## Methods

### `RequireAuth`

```go
func (am *AuthMiddleware) RequireAuth(
    next func(http.ResponseWriter, *http.Request, VerifiedRequest),
) func(http.ResponseWriter, *http.Request, VerifiedRequest)
```

Wraps an inner handler with JWT validation. Extracts the Bearer token from the `Authorization` header, validates it against Stytch, builds a `VerifiedRequest` from the verified identity, and calls through to `next`.

#### Validation sequence

1. **Header presence** — checks that the `Authorization` header is present. Returns `401` if absent.
2. **Bearer scheme** — checks that the header value begins with `Bearer `. Returns `401` if not.
3. **Token extraction** — strips the `Bearer ` prefix to obtain the session JWT. Returns `401` if the result is empty.
4. **JWT validation** — calls `am.Client.ValidateSessionJWT`. Returns `401` if validation fails or the session has expired.
5. **VerifiedRequest construction** — builds a `VerifiedRequest` with `Principal` set to the Stytch user ID and `Email` set to the verified email address. `Capability` is left zero-valued — `Protect` fills it in for routes that require a capability.
6. **Call through** — passes the populated `VerifiedRequest` to `next`.

---

## Functions

### `Adapt`

```go
func Adapt(inner func(http.ResponseWriter, *http.Request, VerifiedRequest)) http.HandlerFunc
```

Converts the internal middleware signature `func(w, req, vr VerifiedRequest)` to `http.HandlerFunc` for use with `http.HandleFunc`. This is the only place in the server where this conversion happens.

`Adapt` passes a zero-valued `VerifiedRequest` as the starting value. `RequireAuth` immediately replaces it with the verified identity, so the zero value is never visible to any handler.

---

## Middleware chain position

`RequireAuth` is the outermost middleware layer after `Adapt`:

```
Adapt → RequireAuth → Limit → Protect → handler
```

Every protected route in the server passes through `RequireAuth`. The three auth routes (`/auth/login`, `/auth/authenticate`, `/auth/logout`) are the only routes that do not — they establish identity and therefore cannot require it as a precondition.

---

## Relationship to other middleware

| File                | Struct                  | Concern                          | Reads from `vr`       | Writes to `vr`              |
|---------------------|-------------------------|----------------------------------|-----------------------|-----------------------------|
| `auth_middleware.go` | `AuthMiddleware`        | JWT validation, identity         | —                     | `Principal`, `Email`        |
| `ratelimit.go`      | `RateLimitMiddleware`   | Per-user rate limiting           | `vr.Principal`        | —                           |
| `capability.go`     | `CapabilityMiddleware`  | Capability verification          | `vr.Principal`        | `vr.Capability`             |
