# capability.go

## Overview

`capability.go` implements the capability verification layer of the Hatcheck server middleware chain. It defines two types — `CapabilityMiddleware` and `VerifiedRequest` — and a single method, `Protect`, which enforces capability-based access control on protected routes.

---

## Types

### `CapabilityMiddleware`

```go
type CapabilityMiddleware struct {
    Key            []byte
    Revoked        *metadata.RevokedSet
    BootstrapToken string
}
```

Holds the server-side state needed to verify capability tokens:

- **`Key`** — the HMAC-SHA256 signing key used to verify capability signatures. Set from the `HATCHECK_SIGNING_KEY` environment variable.
- **`Revoked`** — the in-memory revocation index, rebuilt from the metadata log on startup and updated live when a capability is revoked via `/capability/revoke`.
- **`BootstrapToken`** — an optional shared secret set via `HATCHECK_BOOTSTRAP_TOKEN`. When present, it allows a first-time admin to access admin routes without a signed capability. Should be unset once real admin capabilities have been issued.

---

### `VerifiedRequest`

```go
type VerifiedRequest struct {
    Capability metadata.CapabilityPayload
    Principal  string
    Email      string
}
```

Carries verified identity and capability through the middleware chain into the inner handler. It is passed by value so each handler receives its own copy.

- **`Principal`** and **`Email`** are always set by `RequireAuth` after JWT validation.
- **`Capability`** is set by `Protect` after the capability token is verified. It is zero-valued for routes that do not go through `Protect`.

---

## Method

### `Protect`

```go
func (cm *CapabilityMiddleware) Protect(
    perm string,
    inner func(http.ResponseWriter, *http.Request, VerifiedRequest),
) func(http.ResponseWriter, *http.Request, VerifiedRequest)
```

Wraps an inner handler with capability verification. Takes the required permission level (`PermRead`, `PermWrite`, or `PermAdmin`) and the next handler in the chain. Returns a handler with the same signature, fitting naturally into the middleware chain between `RateLimitMiddleware.Limit` and the inner handler.

#### Verification sequence

1. **Bootstrap token check** — if `BootstrapToken` is set and the request presents `X-Bootstrap-Token`, the token is compared using `hmac.Equal` to prevent timing attacks. A valid bootstrap token grants `PermAdmin` only; it is rejected on any other route. If `BootstrapToken` is empty, this check is skipped entirely.

2. **Capability token extraction** — reads `X-Capability-Token` from the request header. Returns `403` if absent.

3. **Deserialization** — unmarshals the JSON capability payload into a `CapabilityPayload`. Returns `400` if malformed.

4. **Signature and expiry verification** — calls `metadata.VerifyCapability` with the signing key, the payload, and `vr.Principal`. This checks the HMAC-SHA256 signature covers all authoritative fields and that the capability has not expired. Returns `403` if verification fails.

5. **Revocation check** — looks up the capability ID in the in-memory `RevokedSet`. Returns `403` if revoked.

6. **Permission check** — verifies the capability's `Perm` field satisfies the required permission. The permission hierarchy is:
   - `PermAdmin` satisfies any check.
   - `PermWrite` satisfies `PermRead` checks (write implies read).
   - `PermRead` satisfies only `PermRead` checks.
   Returns `403` if the capability does not satisfy the required permission.

7. **Call through** — attaches the verified capability to `vr.Capability` and calls the inner handler.

---

## Middleware chain position

`Protect` sits inside `RequireAuth` and `RateLimitMiddleware.Limit` in the chain:

```
Adapt → RequireAuth → Limit → Protect → handler
```

`RequireAuth` constructs the `VerifiedRequest` with identity. `Limit` enforces the rate budget. `Protect` adds the verified capability. The inner handler receives a fully populated `VerifiedRequest`.

Routes that do not require a capability (`/stash`, `/collection`, `/relation`) skip `Protect` entirely and receive a `VerifiedRequest` with a zero-valued `Capability`.

---

## Permission constants

Defined in `main.go` and used by `Protect`:

| Constant    | Value     | Used by                                              |
|-------------|-----------|------------------------------------------------------|
| `PermRead`  | `"read"`  | `/fetch`, `/list`, `/query`, `/namespaces`, `/names`, `/relations`, `/tags` |
| `PermWrite` | `"write"` | `/name`                                              |
| `PermAdmin` | `"admin"` | `/export`, `/import`, `/capability`, `/capability/revoke` |
