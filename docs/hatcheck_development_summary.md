# Hatcheck Development Summary

## Project Overview
Hatcheck is a content addressable store (CAS) accessible over HTTP, written in Go. It stores immutable text objects addressed by MD5 hash, with a metadata layer for organization, discovery, and capability-based access control. Authentication is Stytch magic-link email.

**GitHub:** `https://github.com/steveknoblock/Hatcheck-Go`
**Working branch:** `develop-go`
**Local (Windows):** `C:\Users\User\Dropbox\Projects\Main\Hatcheck-Go`
**Local (Ubuntu/Docker):** `~/projects/Hatcheck-Go`

---

## Project Structure
```
Hatcheck-Go/
    cmd/
        hatcheck/       # CLI entry point
    internal/
        auth/           # Stytch client wrapper
        cas/            # Content addressable store
        metadata/       # Metadata log and indexes
        share/          # Export and import
    server/             # HTTP server: auth, capability, role, admin handlers
    ui/
        index.html      # Main document UI
        admin.html      # Capability and role admin UI
        toast.js/css    # Shared toast notification component
    objects/            # Runtime: CAS object store
    metadata/           # Runtime: metadata log
    Dockerfile
    docker-compose.yml
    go.mod
    SETUP.md
```

---

## Four Primitive Operations

Everything in Hatcheck is composed from four primitives:

**Stash** — stores any text content in the CAS, returns a hash. Automatically issues the creator a write capability for the resulting hash — this is the ownership creation point.

**Collection** — stores a JSON array of hashes as a CAS object. Anonymous, reachable via a Name.

**Relation** — stores a JSON object expressing a typed link between two hashes: `{"from": "...", "rel": "contextualizes", "to": "..."}`. Anonymous, part of the syndetic web concept.

**Name** — a human-readable label in the metadata log pointing to any hash. The only mutable primitive. Supports create and update. Lives only in metadata, not the CAS.

These four primitives compose into documents, collections, data graphs, structured documents, and blog-style time-structured documents.

---

## Architecture Decisions

### CAS (`internal/cas`)
- MD5 hashing, hex encoded
- Shard structure: first 2 hex chars = directory, remaining 30 = filename
- `Stash(content, objPath)` and `Fetch(hash, objPath)`
- Completely independent of metadata, auth, and server

### Metadata (`internal/metadata`)
- **Envelope log format** — every entry has `op`, `created`, and `payload` (raw JSON)
- **Plugin index architecture** — `Index` interface with `Name()`, `Add(entry)`, `Query(key)` methods
- **Built-in indexes:** `TagIndex`, `DateIndex`, `NameIndex`, `RelationIndex`, `KindIndex`, `CreatedIndex`, `CapabilityIndex`, `RoleIndex`
- Log is append-only, indexes rebuilt on startup by replaying the log
- `NamesInNamespace(namespace)` returns names with prefix stripped
- `Namespaces()` returns all unique namespace prefixes
- `RevokedSet` — a live, in-memory revocation index, updated immediately on `POST /capability/revoke` rather than requiring a restart

### Authentication (`internal/auth`)
- Stytch magic-link email — no passwords, no local user table
- `POST /auth/login` sends the link; `GET /auth/authenticate` exchanges the Stytch token for a session JWT
- On successful authentication, a wildcard read capability (`Hash: "*"`, `Perm: read`) is issued and recorded, so every logged-in user can read anything by default
- Redirect to `/ui/` carries `session_jwt`, `user_id`, `email`, `default_ns`, and `read_cap` as query params; the UI stores these in `sessionStorage` and cleans the URL
- Every subsequent request carries `Authorization: Bearer <session_jwt>`

### Capability-Based Access Control (`server/capability.go`, `internal/metadata`)
- Capabilities are the **sole** authorization and ownership mechanism — objects themselves carry no ACL
- A `CapabilityPayload` carries `ID`, `Hash`, `Perm` (a single string — separate capabilities for read and write allow independent revocation), `Expires`, optional `Principal`/`Email`, and `Sig` (HMAC-SHA256 over the authoritative fields)
- `Principal` is optional: present, it behaves as a bound credential; absent, the capability is a bearer token
- Capabilities are **persistent**, recorded in the metadata log, and rebuilt into an in-memory set on startup — the same pattern as every other index, not process-scoped
- Revocation is immediate: appends a log entry and updates the live `RevokedSet` at once
- `CapabilityMiddleware.Protect` verifies signature, expiry, principal binding, and revocation, then checks the capability's `Perm` satisfies the route's requirement (`admin` satisfies anything, `write` satisfies `read`)
- `HATCHECK_BOOTSTRAP_TOKEN` solves the bootstrap problem — while set, `X-Bootstrap-Token` grants admin access without a signed capability, for initial setup only

### Roles (`server/role_capability.go`, `internal/metadata/role_index.go`)
- Roles are a pure **issuance-layer** administrative concept — a named group of principals driving bulk capability issuance/revocation
- **Roles never appear in a capability token and play no part at verification time** — only the individual capability itself is checked on each request
- A role has a **definition** (a set of hash/perm grants) and **membership** (assigned principals), both tracked in `RoleIndex`
- Assigning a role issues one capability per grant the role currently defines; removing a role revokes every live capability issued under that role
- Capabilities carry a `Role` field purely for audit and bulk-revocation lookup
- Changing a role's definition (adding/removing a grant) retroactively issues or revokes that grant for every existing member

### Configuration (`server/config.go`)
- All server configuration resolved from environment variables in one place, logged at startup (secrets excluded)
- Three independent rate-limit pools (Read/Write/Admin), each a token-bucket keyed per authenticated principal, each independently tunable via `HATCHECK_RATE_*` env vars
- See `SETUP.md` for the full variable reference

### Tags
- Extracted from content using `#hashtag` convention on every stash
- Lowercase, alphanumeric and underscores, deduplicated
- Stored in the log, indexed by `TagIndex`

### Namespaces
- Names follow `namespace/label` convention
- Namespace is just a prefix on the label string
- Enables multi-user and multi-project organization
- No special implementation needed beyond prefix convention
- On login, `default_ns` suggests a starting namespace slugified from the user's email or Stytch user ID — a default, not a constraint; nothing server-side enforces it, and multiple namespaces per user are fully supported

### Metadata Log as Event Source
- Append-only log is the source of truth
- Indexes are projections built from the log
- Multiple indexes can be built from the same log
- Recovery possible by replaying log from scratch
- Log entries for the same name label accumulate — later entries win in the index but history is preserved

---

## HTTP API

See `SETUP.md` for the full endpoint reference including capability requirements and curl examples. Summary by area:

| Area                | Endpoints |
|----------------------|-------------|
| Auth                 | `/auth/login`, `/auth/authenticate`, `/auth/logout` |
| Content              | `/stash`, `/fetch`, `/list`, `/query` |
| Names/Namespaces     | `/namespaces`, `/names`, `/name` |
| Collections/Relations| `/collection`, `/relation`, `/relations`, `/tags`, `/dates` |
| Share                | `/export` (whole-namespace, single-document, or cross-namespace), `/import` (whole-namespace) |
| Capability admin     | `/capability`, `/capability/revoke`, `/capabilities`, `/principals` |
| Roles                | `/role/assign`, `/role/revoke`, `/roles`, `/role/grant`, `/role/grant/revoke`, `/role/grants` |
| Config               | `/config` |
| UI                   | `/ui/` |

---

## CLI

```bash
hatcheck stash "content with #tags"
hatcheck fetch <hash>
hatcheck list [-json]
hatcheck query -index tag -key ideas [-json]
hatcheck export -source bob [-name namespace/label] [-o file.tar.gz]
hatcheck import archive.tar.gz
hatcheck capability issue -hash <hash> -perm read -principal alice
hatcheck capability revoke -id <capability-id> -reason "..."
hatcheck capability list [-principal alice] [-hash <hash>] [-json]
```

The CLI operates directly on the local CAS and metadata stores — no HTTP, no auth, no capability tokens.

Environment variables: `HATCHECK_DATA`, `HATCHECK_META`, `HATCHECK_UI` (server also needs `HATCHECK_SIGNING_KEY` and the `STYTCH_*` vars — see `SETUP.md`).

---

## Share Package (`internal/share`)

- **Whole-namespace export** — bundles all objects reachable from a namespace's names
- **Single-document export** — `name=` traverses reachable objects from one named root instead
- **Cross-namespace export** — `namespace=` exports a different namespace than the one identified by `source`
- Reachability follows Collections and Relations recursively with **cycle detection** via visited set
- **Import** — unpacks archive, skips duplicate objects silently, prefixes name labels with source identifier
- Namespace collision handled by `source/label` prefixing — e.g. `bob/my-document`
- Manifest records source, date, version, object count, and name for partial exports
- Export and Import both live beside each other in the UI toolbar as whole-namespace-first operations (Export was formerly "Share," scoped to the single open document — this was changed and the button moved to sit next to Import)

---

## UI

- **Seseragi theme** — light background (`#eeeeee`), black text, blue accent (`#3377AA`), Lora (editor/content), Lato (UI chrome), Inconsolata (hashes/monospace) via Google Fonts
- Two pages: `ui/index.html` (main document UI) and `ui/admin.html` (capability/role admin), sharing `toast.js`/`toast.css`
- **Left panel** — namespace input at top, By Namespace / By Date toggle, name/collection tree below, relations list
- **Right panel** — breadcrumb trail, Editor / Relations tabs, editor header with Save button, textarea editor
- **Toolbar** — New, New Collection, Import, Export, connection status, Sign Out
- **Draggable divider** between panels
- **Tree navigation** — Names expand into Collections, drill down to leaf text objects
- **Breadcrumb** — clickable path back up the navigation stack
- **Path copying on save** — creates new objects at each level of the stack, updates Name at root
- **Admin UI (`admin.html`)** — six tabs: Principals, Capabilities, Issue capability, Roles, Assign (drag-and-drop role assignment), Config (non-secret config + store stats)

A Map tab (relations treemap, sized by fan-out/recency/tag-overlap) is in active development on `feature-treemap-navigator`, not yet on `develop-go` — see Backlog.

---

## Path Copying

Key architectural decision for editing immutable content in a nested structure:

1. Edit leaf text → stash new object → new hash
2. Walk path stack in reverse, creating new Collections at each level with updated child hash
3. Update the Name at the root to point to the new top-level hash

The Name label stays stable. All intermediate versions preserved in CAS.

---

## Test Coverage
- `internal/cas` — 9 tests
- `internal/metadata` — 155 tests (tag/date/name/relation/kind/created/capability/role indexes, plus log/metadata integration)
- `internal/share` — 23 tests
- `server` — 88 tests (auth, capability middleware, role admin, role capability, general server/handler tests)
- **Total: 275 tests, all passing**

---

## Backlog

- **`feature-treemap-navigator` merge** — Map tab (relations treemap) is functional on the branch: `CreatedIndex`, `/object-meta`, fan-out/recency/tag-overlap sizing, per-hash caching, and a widened Read rate-limit pool to support it. Still open: whether a single relation rendering as one full tile is worth extending to 2-hop neighbors for more visual information; general polish pass before merging to `develop-go`.
- **SETUP.md / design doc updates** — done this session; keep current as `feature-treemap-navigator` and future work land.
- **Document identity** — `prev` field linking versions of the same document (deferred, needs a design conversation).
- **Headless/API-key access mode** — programmatic CAS access via a static API key, no content-editing UI. Not yet designed.
- **"Context" primitive** — like a Collection but with a fixed item count, own dedicated interface. `feature-context` branch exists, no commits yet.
- **Markdown export** — leaf and document export to `.md` files, Hugo static site generator support. Not started.
- **Dead code** — check `cas.go` for leftover debug `fmt.Printf` statements from the original single-user version.
