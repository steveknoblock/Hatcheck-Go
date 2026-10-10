# Hatcheck

Your content. On the web. As Markdown and JSON objects.

Hatcheck is a minimal content addressable object store for documents with a metadata system to enable finding and connecting objects through semantic relationships. The default content type for Hatcheck is plain text, with the default formats being Markdown and JSON. The content store is exposed through a web applications interface.

## Getting Started

See SETUP.md

Hatcheck is programmed in Go. The minimum version required to run Hatcheck is Go 1.25.0

## User Interface

Hatcheck comes with a user interface for adding content to the object store, including tags, which can be used to filter the list of objects by hash.

## Object Store API

Hatcheck exposes a RESTful, web API, enabling communication with the object store over HTTP requests.

On the first request to store an object the objects/ folder will be automatically created in the application root folder.

### Endpoints

| Endpoint | Description |
| -------- | ----------- |
| /stash          | Create a new object in the object store
| /fetch          | Get the contents of an object from the object store.
| /list           | Returns a JSON array of Object hashes.

## Hatcheck CLI

## Hatcheck Indexing

An index is a read model built from the metadata log: every log entry is offered to every index at startup (to rebuild it) and again as each new entry is appended. Most indexes are a `Projection`, which does the decoding and storage for you. You only say which log entries you care about and what keys and values they produce.

```go
type SizeIndex struct{ *Projection[int] }

func NewSizeIndex() *SizeIndex {
    return &SizeIndex{NewProjection("size", Latest,
        On(OpStash, func(e Entry, p StashPayload) []Pair[int] {
            return []Pair[int]{{Key: p.Hash, Value: p.Size}}
        }),
    )}
}

// Size returns the byte size recorded for hash, or 0 if it is unknown.
func (s *SizeIndex) Size(hash string) int {
    v, _ := s.Value(hash)
    return v
}
```

Register it by adding it to `DefaultIndexes()` in `internal/metadata/defaults.go`. The server and the CLI both build their data indexes from that list, so there is nowhere else to add it. Indexes are always built with a constructor; there is no usable zero value.

```go
func DefaultIndexes() []Index {
    return []Index{
        NewTagIndex(),
        // ...
        NewSizeIndex(),
    }
}
```

The `/query` endpoint and the `hatcheck query` command then support it with no other changes:

```
/query?index=size&key=<hash>
hatcheck query -index size -key <hash>
```

Indexes that need to undo earlier entries or join several kinds of state (capabilities and roles) are not projections; they implement the `Index` interface by hand and are registered separately. See [docs/indexing.md](docs/indexing.md) for the full guide.


## Why Claude AI?

> The reason that tech generally--and coders in particular--see LLMs differently than everyone else is that in the creative disciplines, LLMs take away the most soulful human parts of the work and leave the drudgery to you," Dash says, "And in coding, LLMs take away the drudgery and leave the human soulful parts to you." -- Anil Dash

Another reason is that software is made using languages and ideas that are very mathematical, with logic governing decisions, and there is a long history of openness, sharing code, using libraries is encouraged in programming. Copying is good and takes away from no one. Automation has been essential to computer programming and use since the beginning. 



