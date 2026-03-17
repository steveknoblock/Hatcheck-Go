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

# Query as JSON
./hatcheck query -index tag -key ideas -json
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
```

---

## API Endpoints

| Method | Endpoint                        | Description                  |
|--------|---------------------------------|------------------------------|
| POST   | `/stash`                        | Store content, returns hash  |
| GET    | `/fetch?hash=<hash>`            | Retrieve content by hash     |
| GET    | `/list`                         | List all objects with tags   |
| GET    | `/query?index=<n>&key=<k>`      | Query by index and key       |

### Example: stash content with curl

```bash
curl -X POST http://localhost:8090/stash \
     -d "My content with #tags"
```

### Example: fetch content

```bash
curl "http://localhost:8090/fetch?hash=<hash>"
```

### Example: query by tag

```bash
curl "http://localhost:8090/query?index=tag&key=ideas"
```

### Example: query by date

```bash
curl "http://localhost:8090/query?index=date&key=2026-03-17"
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
    server/             # HTTP server
    ui/                 # Web interface
    objects/            # Runtime: CAS object store (created on first run)
    metadata/           # Runtime: metadata log (created on first run)
    go.mod
```
