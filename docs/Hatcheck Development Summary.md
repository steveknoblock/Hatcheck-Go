# Hatcheck Development Summary

## Project Overview
Hatcheck is a content addressable store (CAS) accessible over HTTP, written in Go. It stores immutable text objects addressed by MD5 hash, with a metadata layer for organization and discovery.

**GitHub:** `https://github.com/steveknoblock/Hatcheck-Go`
**Working branch:** `develop-go`
**Local (Windows):** `C:\Users\User\Dropbox\Projects\Main\Hatcheck-Go`
**Local (Ubuntu/Docker):** `~/projects/Hatcheck-Go`

---

## Project Structure
```
Hatcheck-Go/
    cmd/hatcheck/       # CLI entry point
    internal/
        cas/            # Content addressable store
        metadata/       # Metadata log and indexes
        share/          # Export and import
    server/             # HTTP server
    ui/                 # Web interface
    objects/            # Runtime: CAS object store
    metadata/           # Runtime: metadata log
    go.mod
    SETUP.md
```

---

## Four Primitive Operations

Everything in Hatcheck is composed from four primitives:

**Stash** — stores any text content in the CAS, returns a hash. The fundamental storage operation.

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
- Completely independent of metadata and server

### Metadata (`internal/metadata`)
- **Envelope log format** — every entry has `op`, `created`, and `payload` (raw JSON)
- **Five operation types:** `stash`, `collection`, `relation`, `name-create`, `name-update`
- **Plugin index architecture** — `Index` interface with `Name()`, `Add(entry)`, `Query(key)` methods
- **Four built-in indexes:** `TagIndex`, `DateIndex`, `NameIndex`, `RelationIndex`
- Log is append-only, indexes rebuilt on startup by replaying the log
- `NamesInNamespace(namespace)` returns names with prefix stripped
- `Namespaces()` returns all unique namespace prefixes

### Tags
- Extracted from content using `#hashtag` convention on every stash
- Lowercase, alphanumeric and underscores, deduplicated
- Stored in the log, indexed by `TagIndex`

### Namespaces
- Names follow `namespace/label` convention
- Namespace is just a prefix on the label string
- Enables multi-user and multi-project organization
- No special implementation needed beyond prefix convention

### Metadata Log as Event Source
- Append-only log is the source of truth
- Indexes are projections built from the log
- Multiple indexes can be built from the same log
- Recovery possible by replaying log from scratch
- Log entries for the same name label accumulate — later entries win in the index but history is preserved

---

## HTTP API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/stash` | Store content, returns hash |
| GET | `/fetch?hash=` | Retrieve content by hash |
| GET | `/list` | List all objects with tags |
| GET | `/query?index=&key=` | Query by index and key |
| GET | `/namespaces` | List all namespace prefixes |
| GET | `/names?namespace=` | List names in namespace |
| POST | `/name?namespace=&label=&hash=` | Create or update a name |
| POST | `/collection` | Stash a collection, returns hash |
| GET | `/export?source=&name=` | Export tar.gz archive |
| POST | `/import` | Import tar.gz archive |
| GET | `/ui/` | Serve static UI files |

---

## CLI

```bash
hatcheck stash "content with #tags"
hatcheck fetch <hash>
hatcheck list [-json]
hatcheck query -index tag -key ideas [-json]
hatcheck export -source bob [-name namespace/label] [-o file.tar.gz]
hatcheck import archive.tar.gz
```

Environment variables: `HATCHECK_DATA`, `HATCHECK_META`, `HATCHECK_UI`

---

## Share Package (`internal/share`)

- **Full export** — bundles all CAS objects and metadata log
- **Partial export by name** — traverses reachable objects from a named root
- Reachability follows Collections and Relations recursively with **cycle detection** via visited set
- **Import** — unpacks archive, skips duplicate objects silently, prefixes name labels with source identifier
- Namespace collision handled by `source/label` prefixing — e.g. `bob/my-document`
- Manifest records source, date, version, object count, and name for partial exports

---

## UI

- Dark industrial aesthetic, JetBrains Mono + Syne fonts, acid green accent
- **Left panel** — namespace input with autocomplete at top, name/collection tree below
- **Right panel** — breadcrumb trail, editor header with Share and Save buttons, textarea editor
- **Draggable divider** between panels
- **Tree navigation** — Names expand into Collections, drill down to leaf text objects
- **Breadcrumb** — clickable path back up the navigation stack
- **Path copying on save** — creates new objects at each level of the stack, updates Name at root
- **Share button** — active only when a named document is open, downloads export
- **Import button** — file picker, posts to `/import`, switches namespace to imported source

---

## Path Copying

Key architectural decision for editing immutable content in a nested structure:

1. Edit leaf text → stash new object → new hash
2. Walk path stack in reverse, creating new Collections at each level with updated child hash
3. Update the Name at the root to point to the new top-level hash

The Name label stays stable. All intermediate versions preserved in CAS.

---

## Test Coverage
- `internal/cas` — 5 tests
- `internal/metadata` — 30 tests
- `internal/share` — 16 tests
- `server` — 26 tests
- **Total: 67 tests, all passing**

---

## Backlog

- **Relations UI** — create and browse relations between objects (graph view discussed, not implemented)
- **Markdown export** — leaf and document export to `.md` files, Hugo static site generator support
- **Document identity** — `prev` field linking versions of the same document (deferred)
- **SETUP.md** — needs updating to reflect all new endpoints and namespace concept
- **Hatcheck design document** — needs updating with four primitives, composability, syndetic web
- **Date query UI** — server supports it, UI doesn't expose it yet
- **Dead code** — `cas.go` has leftover debug `fmt.Printf` statements