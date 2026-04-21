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

Go 1.21 or later is required.

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

Hatcheck is configured via environment variables. All paths are relative to the directory where you run the server or CLI.

| Variable        | Default       | Description                        |
|-----------------|---------------|------------------------------------|
| `HATCHECK_DATA` | `./objects`   | Path to the CAS object store       |
| `HATCHECK_META` | `./metadata`  | Path to the metadata log directory |
| `HATCHECK_UI`   | `./ui`        | Path to the UI static files        |

The `objects/` and `metadata/` directories are created automatically on first run.

### Example: run with default paths from the project root

```bash
./hatcheck-server
```

### Example: run with custom paths

```bash
HATCHECK_DATA=/var/hatcheck/objects \
HATCHECK_META=/var/hatcheck/metadata \
HATCHECK_UI=/var/hatcheck/ui \
./hatcheck-server
```

---

## Run the Server

```bash
./hatcheck-server
```

The server starts on port 8090. Open the UI in your browser:

```
http://localhost:8090/ui/index.html
```

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

The metadata log is an append-only event log. Every stash, collection, relation, and name operation is recorded as a log entry. Indexes (tag, date, name, relation) are built by replaying the log on startup. The log is the source of truth — indexes are projections from it.

---

## CLI Usage

```bash
# Store content
./hatcheck stash "Some text with #tags"

# Store content from a file
cat myfile.txt | ./hatcheck stash

# Retrieve content by hash
./hatcheck fetch <hash>

# List all objects
./hatcheck list

# List all objects as JSON
./hatcheck list -json

# Query by tag
./hatcheck query -index tag -key ideas

# Query by date
./hatcheck query -index date -key 2026-03-17

# Query by relation (returns relation object hashes)
./hatcheck query -index relation -key from:<hash>
./hatcheck query -index relation -key to:<hash>
./hatcheck query -index relation -key rel:<predicate>

# Query as JSON
./hatcheck query -index tag -key ideas -json

# Export a namespace as a tar.gz archive
./hatcheck export -source <namespace>

# Export a single named document
./hatcheck export -source <namespace> -name <namespace/label>

# Import an archive
./hatcheck import archive.tar.gz
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

### Content

| Method | Endpoint               | Description                        |
|--------|------------------------|------------------------------------|
| POST   | `/stash`               | Store content, returns hash        |
| GET    | `/fetch?hash=<hash>`   | Retrieve content by hash           |
| GET    | `/list`                | List all objects with tags         |
| GET    | `/query?index=<n>&key=<k>` | Query by index and key         |

### Names and Namespaces

| Method | Endpoint                                           | Description                          |
|--------|----------------------------------------------------|--------------------------------------|
| GET    | `/namespaces`                                      | List all namespace prefixes          |
| GET    | `/names?namespace=<ns>`                            | List names in a namespace            |
| POST   | `/name?namespace=<ns>&label=<label>&hash=<hash>`   | Create or update a name              |

### Collections

| Method | Endpoint       | Description                                      |
|--------|----------------|--------------------------------------------------|
| POST   | `/collection`  | Stash a JSON array of hashes, returns hash       |

### Relations

| Method | Endpoint                                     | Description                                      |
|--------|----------------------------------------------|--------------------------------------------------|
| POST   | `/relation?from=<hash>&rel=<pred>&to=<hash>` | Store a relation, returns hash                   |
| GET    | `/relations?hash=<hash>`                     | Get outgoing and incoming relations for a hash   |
| GET    | `/tags`                                      | List all known tag keys                          |

### Share

| Method | Endpoint                              | Description                          |
|--------|---------------------------------------|--------------------------------------|
| GET    | `/export?source=<ns>`                 | Export full namespace as tar.gz      |
| GET    | `/export?source=<ns>&name=<ns/label>` | Export a single named document       |
| POST   | `/import`                             | Import a tar.gz archive              |

---

## API Examples

### Stash content

```bash
curl -X POST http://localhost:8090/stash \
     -d "My content with #tags"
```

### Fetch content

```bash
curl "http://localhost:8090/fetch?hash=<hash>"
```

### Query by tag

```bash
curl "http://localhost:8090/query?index=tag&key=ideas"
```

### Query by date

```bash
curl "http://localhost:8090/query?index=date&key=2026-03-17"
```

### List namespaces

```bash
curl http://localhost:8090/namespaces
```

### List names in a namespace

```bash
curl "http://localhost:8090/names?namespace=bob"
```

### Create or update a name

```bash
curl -X POST "http://localhost:8090/name?namespace=bob&label=my-document&hash=<hash>"
```

### Stash a collection

```bash
curl -X POST http://localhost:8090/collection \
     -H "Content-Type: application/json" \
     -d '["<hash1>","<hash2>","<hash3>"]'
```

### Create a relation

```bash
curl -X POST "http://localhost:8090/relation?from=<hash>&rel=contextualizes&to=<hash>"
```

### Get relations for an object

```bash
curl "http://localhost:8090/relations?hash=<hash>"
```

Response shape:

```json
{
  "outgoing": [{"hash":"...","from":"...","rel":"...","to":"..."}],
  "incoming": [{"hash":"...","from":"...","rel":"...","to":"..."}]
}
```

### List all tags

```bash
curl http://localhost:8090/tags
```

### Export a namespace

```bash
curl "http://localhost:8090/export?source=bob" -o bob.tar.gz
```

### Import an archive

```bash
curl -X POST http://localhost:8090/import \
     -H "Content-Type: application/gzip" \
     --data-binary @bob.tar.gz
```

---

## Project Structure

```
Hatcheck-Go/
    cmd/
        hatcheck/       # CLI entry point
    internal/
        cas/            # Content addressable store
        metadata/       # Metadata log and indexes
        share/          # Export and import
    server/             # HTTP server
    ui/                 # Web interface
    objects/            # Runtime: CAS object store (created on first run)
    metadata/           # Runtime: metadata log (created on first run)
    go.mod
    SETUP.md
```
