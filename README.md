# Hatcheck

A minimal content addressable storage system (CAS) exposed to a web interface.

Your content. On the web. As JSON objects. A simple content addressable storage system exposed to a web interface.

## Getting Started

See SETUP.md

Hatcheck is programmed in Go. The minimum version required to run Hatcheck is Go 1.21.6

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

An index can be added through a plugin architecture. 

Create a struct with the three interface methods defined by Index:

```
type MyIndex struct {
    data map[string][]string
}

func (m *MyIndex) Name() string { return "myindex" }

func (m *MyIndex) Add(entry Entry) {
    // extract whatever you want from entry and store it in data
}

func (m *MyIndex) Query(key string) []string {
    return m.data[key]
}
```

Register it when creating the store in both server/main.go and cmd/hatcheck/main.go:

```
metadata.New(metaPath,
    &metadata.TagIndex{},
    &metadata.DateIndex{},
    &metadata.MyIndex{},
)
```

**3. That's it.** The store automatically calls `Add` on every existing log entry at startup to build the index, and calls `Add` on every new stash going forward.

The `/query` endpoint and `hatcheck query` CLI command immediately support it with no other changes:
```
/query?index=myindex&key=somevalue
hatcheck query -index myindex -key somevalue
```


## Why Claude AI?

> The reason that tech generally--and coders in particular--see LLMs differently than everyone else is that in the creative disciplines, LLMs take away the most soulful human parts of the work and leave the drudgery to you," Dash says, "And in coding, LLMs take away the drudgery and leave the human soulful parts to you." -- Anil Dash

Another reason is that software is made using languages and ideas that are very mathematical, with logic governing decisions, and there is a long history of openness, sharing code, using libraries is encouraged in programming. Copying is good and takes away from no one. Automation has been essential to computer programming and use since the beginning. 



