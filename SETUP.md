# Hatcheck Setup Guide

## Prerequisites

### Install Go

```bash
sudo apt update
sudo apt install golang-go
```

Verify the installation:

```bash
go version
```

Go 1.25.0 or later is required.

### Create a Stytch account

Hatcheck uses [Stytch](https://stytch.com) magic-link email for authentication — there are no passwords, and there is no local user table. You'll need:

1. A free Stytch account and a project (Test environment is fine for development).
2. The project's **Project ID** and **Secret**, from the Stytch dashboard.
3. A **redirect URL** registered in the Stytch dashboard's allowed redirect URLs, pointing at `/auth/authenticate` on wherever Hatcheck will run (e.g. `http://localhost:8090/auth/authenticate`).

These become the `STYTCH_PROJECT_ID`, `STYTCH_SECRET`, and `STYTCH_REDIRECT_URL` environment variables below.

---

## Get Hatcheck

Clone the repository from GitHub:

```bash
git clone https://github.com/steveknoblock/Hatcheck-Go.git
cd Hatcheck-Go
git checkout develop-go
```

---

## Build

### HTTP Server

```bash
go build -o hatcheck-server ./server/
```

### CLI

```bash
go build -o hatcheck ./cmd/hatcheck/
```

---

## Configuration

Hatcheck is configured entirely via environment variables (`server/config.go`). All paths are relative to the directory where you run the server.

### Required — the server calls `log.Fatal` at startup if any of these are missing

| Variable              | Description                                              |
|-----------------------|------------------------------------------------------------|
| `HATCHECK_SIGNING_KEY` | Secret key used to HMAC-sign every capability token       |
| `STYTCH_PROJECT_ID`    | Stytch project ID                                          |
| `STYTCH_SECRET`        | Stytch project secret                                       |
| `STYTCH_REDIRECT_URL`  | Magic-link redirect URL, e.g. `http://localhost:8090/auth/authenticate` |

### Optional — paths

| Variable        | Default       | Description                        |
|-----------------|---------------|-------------------------------------|
| `HATCHECK_DATA` | `./objects`   | Path to the CAS object store        |
| `HATCHECK_META` | `./metadata`  | Path to the metadata log directory  |
| `HATCHECK_UI`   | `./ui`        | Path to the UI static files         |

The `objects/` and `metadata/` directories are created automatically on first run.

### Optional — identity and authorization

| Variable                     | Default  | Description                                                      |
|-------------------------------|----------|-------------------------------------------------------------------|
| `HATCHECK_BOOTSTRAP_TOKEN`    | *(unset)* | Shared secret granting admin access without a signed capability — see [First run and bootstrap admin](#first-run-and-bootstrap-admin) |
| `HATCHECK_CAPABILITY_EXPIRY`  | `8760h` (1 year) | How long newly-issued capabilities remain valid, as a Go duration |

### Optional — rate limiting

Three independent token-bucket pools, keyed per authenticated user (see `server/ratelimit.go`):

| Pool    | Routes                                                                                          | Interval env var                | Burst env var                | Default |
|---------|--------------------------------------------------------------------------------------------------|----------------------------------|--------------------------------|---------|
| Read    | `/fetch`, `/list`, `/query`, `/namespaces`, `/names`, `/relations`, `/tags`, `/dates`, `/export` | `HATCHECK_RATE_READ_INTERVAL`   | `HATCHECK_RATE_READ_BURST`    | 1s / 30 |
| Write   | `/stash`, `/collection`, `/relation`, `/name`, `/import`                                        | `HATCHECK_RATE_WRITE_INTERVAL`  | `HATCHECK_RATE_WRITE_BURST`   | 5s / 4  |
| Admin   | `/capability`, `/capability/revoke`, `/capabilities`, `/principals`, `/config`, `/role/*`        | `HATCHECK_RATE_ADMIN_INTERVAL`  | `HATCHECK_RATE_ADMIN_BURST`   | 1s / 20 |

Interval values are Go durations (e.g. `1s`, `30s`, `500ms`); burst values are integers. All defaults are visible to an admin at runtime via `GET /config`.

### Example: run with required variables only, everything else defaulted

```bash
HATCHECK_SIGNING_KEY=some-long-random-secret \
STYTCH_PROJECT_ID=project-test-xxxxxxxx \
STYTCH_SECRET=secret-test-xxxxxxxx \
STYTCH_REDIRECT_URL=http://localhost:8090/auth/authenticate \
./hatcheck-server
```

### Example: custom paths and a wider Read pool

```bash
HATCHECK_SIGNING_KEY=some-long-random-secret \
STYTCH_PROJECT_ID=project-test-xxxxxxxx \
STYTCH_SECRET=secret-test-xxxxxxxx \
STYTCH_REDIRECT_URL=http://localhost:8090/auth/authenticate \
HATCHECK_DATA=/var/hatcheck/objects \
HATCHECK_META=/var/hatcheck/metadata \
HATCHECK_UI=/var/hatcheck/ui \
HATCHECK_RATE_READ_BURST=60 \
./hatcheck-server
```

---

## Docker

A `Dockerfile` and `docker-compose.yml` are provided for containerized deployment. Copy `.env.example` to `.env` and fill in the required variables — **use plain `KEY=VALUE` syntax, not `export KEY=VALUE`**; the latter is silently ignored by Docker Compose's `.env` parsing, which drops the corresponding environment variable entirely.

```bash
cp .env.example .env
# edit .env with your values
docker compose up --build
```

Named volumes persist the CAS object store and metadata log across container recreation — the metadata log is the source of truth for names, relations, tags, capabilities, and roles, so losing that volume loses all of it, not just convenience data.

---

## First run and bootstrap admin

On first run, no admin capabilities exist yet, so no one can call `/capability`, `/role/assign`, or any other admin-gated endpoint. `HATCHECK_BOOTSTRAP_TOKEN` solves this bootstrap problem: while set, presenting it in an `X-Bootstrap-Token` header grants admin access without a signed capability.

```bash
# with HATCHECK_BOOTSTRAP_TOKEN=my-bootstrap-secret set on the server
curl -X POST "http://localhost:8090/capability?hash=*&perm=admin&principal=<your-user-id>&expires=2027-01-01T00:00:00Z" \
     -H "Authorization: Bearer <your-session-jwt>" \
     -H "X-Bootstrap-Token: my-bootstrap-secret"
```

Once you hold a real admin capability, **unset `HATCHECK_BOOTSTRAP_TOKEN`** — it's meant for initial setup only, and leaving it set indefinitely is an open admin backdoor.

---

## Logging in

1. `POST /auth/login` with `{"email": "you@example.com"}` — triggers a Stytch magic-link email.
2. Click the link. It hits `GET /auth/authenticate?token=...`, which exchanges the Stytch token for a session JWT, issues you a wildcard read capability (`Hash: "*"`, `Perm: read`), and redirects to `/ui/` with `session_jwt`, `user_id`, `email`, `default_ns`, and `read_cap` as query parameters.
3. The UI reads those from the URL, stores them in `sessionStorage`, and cleans the URL.

Every subsequent request from the browser carries `Authorization: Bearer <session_jwt>`, plus `X-Capability-Token: <capability JSON>` for capability-protected routes.

`default_ns` is only a starting suggestion — derived from the local part of your email (or your Stytch user ID as a fallback), slugified. Nothing server-side enforces it; you're free to switch to or create any other namespace at any time.

---

## Core Concepts

Hatcheck is a content addressable store (CAS) accessible over HTTP. Everything is built from four primitives.

**Stash** stores any text content and returns an MD5 hash. The hash is the permanent address of that content. Content is immutable — the same content always produces the same hash.

**Collection** stores a JSON array of hashes as a CAS object. Collections are how ordered groups of objects are composed.

**Relation** stores a typed link between two hashes: `{"from": "...", "rel": "...", "to": "..."}`. Relations are CAS objects like any other — immutable, addressable by hash, and indexed for traversal.

**Name** is a human-readable label in the metadata log pointing to any hash. Names are the only mutable primitive — they can be updated to point to a new hash as content evolves. Names follow a `namespace/label` convention for organization.

### Namespaces

Names are organized by namespace using a `namespace/label` prefix convention. A namespace groups related names — for example, all documents belonging to a user or project. There is no special implementation: a namespace is simply the portion of a label before the first `/`.

### Tags

Tags are extracted automatically from stashed content using the `#hashtag` convention. Tags are lowercase, alphanumeric and underscores, and deduplicated. They are stored in the metadata log and indexed for query.

### Metadata Log

The metadata log is an append-only event log. Every stash, collection, relation, name, capability, and role operation is recorded as a log entry. Indexes are built by replaying the log on startup and kept live thereafter. The log is the source of truth — indexes are projections from it.

---

## Capability-based access control

Capabilities, not identity, are the sole authorization and ownership mechanism in Hatcheck. A capability is a signed, unforgeable token granting a specific permission (`read`, `write`, or `admin`) on a specific object hash (or `*` for all objects), optionally bound to a specific principal.

- **Every write creates ownership.** Stashing content, creating a collection, or creating a relation automatically issues the creator a `write` capability for the resulting hash.
- **Login grants a wildcard read capability.** Every authenticated user can read anything by default; write and admin operations still require a specific capability.
- **Capabilities are persistent**, not process-scoped — they're recorded in the metadata log and rebuilt into an in-memory set on startup, the same append-only pattern as every other index.
- **Revocation is immediate.** `POST /capability/revoke` appends a revocation entry and updates the live in-memory revoked set at once — no restart required, no window where a revoked capability still works.
- **Objects themselves carry no ACL.** There's no concept of an object "belonging" to a namespace or user beyond whatever capabilities happen to exist for it.

See `server/capability.md` for the full verification flow (`CapabilityMiddleware.Protect`), and `docs/capability_based_access_control*.md` for the underlying design discussion.

---

## Roles

Roles are a pure issuance-layer administrative convenience — named groups of principals that drive capability issuance and revocation in bulk. A role **never appears in a capability token and plays no part at verification time**; only individual capabilities are checked when a request comes in.

- **A role has a definition** — a set of (hash, perm) grants — and **membership** — the principals currently assigned to it. Both are tracked in `RoleIndex`, the same append-only, rebuilt-on-startup pattern as every other index.
- **Assigning a role** to a principal issues one capability per grant the role currently defines, skipping any the principal already holds live.
- **Removing a role** from a principal revokes every live capability that was issued under that role annotation — capabilities carry a `Role` field purely for this audit/bulk-revocation lookup, not for authorization.
- **Changing a role's definition** (adding or removing a grant) retroactively issues or revokes that grant for every existing member.

Manage roles via `PUT`-style `/role/*` endpoints below, or the **Roles** and **Assign** tabs in the admin UI.

---

## Admin UI

`ui/admin.html` provides a browser UI for the capability and role system, alongside the main document UI at `ui/index.html`. Six tabs:

| Tab                | Purpose                                                        |
|---------------------|-----------------------------------------------------------------|
| Principals          | Every principal that has been issued at least one bound capability |
| Capabilities        | Every capability ever issued, with live revocation status       |
| Issue capability    | Manually issue a new capability                                  |
| Roles               | Define and inspect roles and their grants                        |
| Assign              | Drag-and-drop role assignment between principals and roles       |
| Config              | Non-secret server configuration and store statistics (`GET /config`) |

All admin routes require a `PermAdmin` capability (or the bootstrap token during initial setup).

---

## CLI Usage

The CLI operates directly on the local CAS and metadata stores — no HTTP server, no auth, no capability tokens.

```bash
# Store content
./hatcheck stash "Some text with #tags"

# Store content from a file
cat myfile.txt | ./hatcheck stash

# Retrieve content by hash
./hatcheck fetch <hash>

# List all objects
./hatcheck list
./hatcheck list -json

# Query by tag / date / relation
./hatcheck query -index tag -key ideas
./hatcheck query -index date -key 2026-03-17
./hatcheck query -index relation -key from:<hash>
./hatcheck query -index relation -key to:<hash>
./hatcheck query -index relation -key rel:<predicate>
./hatcheck query -index tag -key ideas -json

# Export / import
./hatcheck export -source <namespace>
./hatcheck export -source <namespace> -name <namespace/label>
./hatcheck import archive.tar.gz

# Capability management (requires HATCHECK_SIGNING_KEY)
./hatcheck capability issue -hash <hash> -perm read -principal alice
./hatcheck capability issue -hash <hash> -perm write -principal bob -email bob@example.com -ttl 168h
./hatcheck capability revoke -id <capability-id> -reason "user offboarded"
./hatcheck capability list
./hatcheck capability list -principal alice
./hatcheck capability list -hash <hash> -json
```

---

## Run Tests

Run all tests from the project root:

```bash
go test -v ./...
```

Run tests for individual packages:

```bash
go test -v ./internal/cas/...
go test -v ./internal/metadata/...
go test -v ./internal/share/...
go test -v ./server/...
```

---

## API Endpoints

Every route below except `/auth/*` and `/ui/*` requires `Authorization: Bearer <session_jwt>`. Routes marked **capability** additionally require `X-Capability-Token: <capability JSON>` with sufficient permission; a `write` capability satisfies a `read` requirement, and `admin` satisfies any requirement.

### Auth

| Method | Endpoint             | Auth required | Description                                      |
|--------|-----------------------|----------------|----------------------------------------------------|
| POST   | `/auth/login`         | none           | Send a magic-link email. Body: `{"email": "..."}`  |
| GET    | `/auth/authenticate`  | none           | Magic-link callback; redirects to `/ui/` with session data |
| POST   | `/auth/logout`        | none           | No-op — JWTs are stateless, client just discards it |

### Content

| Method | Endpoint                     | Capability | Description                        |
|--------|-------------------------------|------------|--------------------------------------|
| POST   | `/stash`                     | —          | Store content, returns hash + issued write capability |
| GET    | `/fetch?hash=<hash>`         | read       | Retrieve content by hash             |
| GET    | `/list`                      | read       | List all objects with tags           |
| GET    | `/query?index=<n>&key=<k>`   | read       | Query by index and key               |

### Names and Namespaces

| Method | Endpoint                                          | Capability | Description                  |
|--------|-----------------------------------------------------|------------|--------------------------------|
| GET    | `/namespaces`                                       | read       | List all namespace prefixes     |
| GET    | `/names?namespace=<ns>`                             | read       | List names in a namespace       |
| POST   | `/name?namespace=<ns>&label=<label>&hash=<hash>`    | write      | Create or update a name          |

### Collections and Relations

| Method | Endpoint                                     | Capability | Description                                    |
|--------|------------------------------------------------|------------|---------------------------------------------------|
| POST   | `/collection`                                | —          | Stash a JSON array of hashes, returns hash + capability |
| POST   | `/relation?from=<hash>&rel=<pred>&to=<hash>` | —          | Store a relation, returns hash + capability       |
| GET    | `/relations?hash=<hash>`                     | read       | Get outgoing and incoming relations for a hash    |
| GET    | `/tags`                                      | read       | List all known tag keys                          |
| GET    | `/dates`                                     | read       | List all known stash dates                        |

### Share

| Method | Endpoint                                        | Capability | Description                                        |
|--------|----------------------------------------------------|------------|-------------------------------------------------------|
| GET    | `/export?source=<ns>`                             | read       | Export a namespace's reachable objects as tar.gz       |
| GET    | `/export?source=<ns>&name=<ns/label>`             | read       | Export a single named document (mutually exclusive with `namespace=`) |
| GET    | `/export?source=<ns>&namespace=<other-ns>`        | read       | Export a different namespace than `source` identifies (mutually exclusive with `name=`) |
| POST   | `/import`                                          | —          | Import a tar.gz archive; names are prefixed `source/label` |

### Capability admin

| Method | Endpoint                                                              | Capability | Description                                  |
|--------|--------------------------------------------------------------------------|------------|-------------------------------------------------|
| POST   | `/capability?hash=<hash>&perm=<perm>&principal=<id>&expires=<RFC3339>`   | admin      | Issue a new capability                          |
| POST   | `/capability/revoke?id=<capability-id>&reason=<text>`                    | admin      | Revoke a capability immediately                  |
| GET    | `/capabilities`                                                          | admin      | List every issued capability, with revocation status |
| GET    | `/capabilities?principal=<id>`                                           | admin      | List capabilities issued to one principal (`?principal=` with empty value lists bearer capabilities) |
| GET    | `/capabilities?id=<capability-id>`                                       | admin      | Look up a single capability by ID                |
| GET    | `/principals`                                                            | admin      | List every principal with at least one bound capability |

### Roles

| Method | Endpoint                                                        | Capability | Description                                          |
|--------|--------------------------------------------------------------------|------------|---------------------------------------------------------|
| POST   | `/role/assign?principal=<id>&role=<n>&reason=<text>`               | admin      | Assign a role, issuing capabilities for its current grants |
| POST   | `/role/revoke?principal=<id>&role=<n>&reason=<text>`               | admin      | Remove a role, revoking capabilities issued under it        |
| GET    | `/roles`                                                           | admin      | List all active role names                                  |
| GET    | `/roles?principal=<id>`                                            | admin      | List roles held by a principal                              |
| GET    | `/roles?role=<n>`                                                  | admin      | List principals holding a role                              |
| POST   | `/role/grant?role=<n>&hash=<hash>&perm=<read\|write>&reason=<text>`| admin      | Add a grant to a role's definition (retroactive to existing members) |
| POST   | `/role/grant/revoke?role=<n>&hash=<hash>&perm=<read\|write>&reason=<text>` | admin | Remove a grant from a role's definition (retroactive to existing members) |
| GET    | `/role/grants?role=<n>`                                            | admin      | List the grants currently defined for a role                |

### Config

| Method | Endpoint  | Capability | Description                                                       |
|--------|-----------|------------|-----------------------------------------------------------------------|
| GET    | `/config` | admin      | Non-secret config (paths, capability expiry, rate limits) and store stats — never the signing key or bootstrap token value |

### UI

| Method | Endpoint    | Description                                          |
|--------|-------------|---------------------------------------------------------|
| GET    | `/ui/`      | Serves `ui/index.html` (documents) and `ui/admin.html` (admin) |

---

## API Examples

All examples assume `SESSION_JWT` holds a valid session JWT from the login flow, and (where noted) `CAP` holds a JSON-encoded capability.

### Stash content

```bash
curl -X POST http://localhost:8090/stash \
     -H "Authorization: Bearer $SESSION_JWT" \
     -d "My content with #tags"
```

### Fetch content

```bash
curl "http://localhost:8090/fetch?hash=<hash>" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $CAP"
```

### Query by tag

```bash
curl "http://localhost:8090/query?index=tag&key=ideas" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $CAP"
```

### Create or update a name

```bash
curl -X POST "http://localhost:8090/name?namespace=bob&label=my-document&hash=<hash>" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $CAP"
```

### Create a relation

```bash
curl -X POST "http://localhost:8090/relation?from=<hash>&rel=contextualizes&to=<hash>" \
     -H "Authorization: Bearer $SESSION_JWT"
```

### Get relations for an object

```bash
curl "http://localhost:8090/relations?hash=<hash>" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $CAP"
```

Response shape:

```json
{
  "outgoing": [{"hash":"...","from":"...","rel":"...","to":"..."}],
  "incoming": [{"hash":"...","from":"...","rel":"...","to":"..."}]
}
```

### Export a namespace

```bash
curl "http://localhost:8090/export?source=bob" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $CAP" \
     -o bob.tar.gz
```

### Import an archive

```bash
curl -X POST http://localhost:8090/import \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "Content-Type: application/gzip" \
     --data-binary @bob.tar.gz
```

### Issue a capability (admin)

```bash
curl -X POST "http://localhost:8090/capability?hash=<hash>&perm=read&principal=alice&expires=2027-01-01T00:00:00Z" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $ADMIN_CAP"
```

### Assign a role (admin)

```bash
curl -X POST "http://localhost:8090/role/assign?principal=alice&role=editor&reason=onboarding" \
     -H "Authorization: Bearer $SESSION_JWT" \
     -H "X-Capability-Token: $ADMIN_CAP"
```

---

## Project Structure

```
Hatcheck-Go/
    cmd/
        hatcheck/               # CLI entry point
    internal/
        auth/                   # Stytch client wrapper
        cas/                    # Content addressable store
        metadata/                # Metadata log and indexes
        share/                   # Export and import
    server/                     # HTTP server, auth/capability/role middleware and handlers
    ui/
        index.html              # Main document UI
        admin.html               # Capability and role admin UI
        toast.js / toast.css     # Shared toast notification component
    objects/                    # Runtime: CAS object store (created on first run)
    metadata/                   # Runtime: metadata log (created on first run)
    Dockerfile
    docker-compose.yml
    .env.example
    go.mod
    SETUP.md
```
