# Indexing

An index is a read model derived from the metadata log. The log is the only source of truth; every index is rebuilt from it at startup and updated as each new entry is appended. Nothing is stored in an index that cannot be reproduced by replaying the log.

## The `Index` interface

Every index implements three methods (`internal/metadata/index.go`):

```go
type Index interface {
    Name() string
    Add(entry Entry)
    Query(key string) []string
}
```

`Query` is the string-only view used by the `/query` endpoint and `Store.Query`. Typed access goes through optional interfaces (`TagLister`, `DateLister`, `NameLister`, `KindQuerier`, `CreatedQuerier`, `RelationQuerier`, and so on) that the `Store` finds by type assertion.

## Writing an index with `Projection`

`Projection[V]` (`internal/metadata/projection.go`) implements `Index` for you. You supply a name, a storage mode, and one handler per log op:

```go
NewProjection(name, mode, handlers...)
On(op, func(e Entry, p PayloadType) []Pair[V])
```

`On` decodes the entry's payload into `PayloadType` before calling your function, so handlers never call `json.Unmarshal` themselves. An entry whose payload does not decode is skipped. A handler returns zero or more `Pair{Key, Value}`; returning several lets one entry be looked up several ways (Relation emits three).

### Storage modes

| Mode | Behavior | Used by |
|---|---|---|
| `AppendUnique` | list of values per key, arrival order, duplicates skipped | Tag, Date |
| `Latest` | one value per key; a later value replaces an earlier one | Kind, Created, Name |
| `Append` | list of values per key, arrival order, duplicates kept | Relation |

### Options

- `.WithQueryKey(fn)` transforms the key passed to `Get`, `Value` and `Query` before lookup (Tag uses `strings.ToLower`). Stored keys are exactly what the handlers emitted.
- `.WithRender(fn)` controls how a value becomes a string for `Query`. The default is the string itself, or `fmt.Sprint` for other types.

### Reading an index

| Method | Returns |
|---|---|
| `Get(key)` | all values under the key (`nil` if none) |
| `Value(key)` | one value and whether it exists; the latest value in list modes |
| `Keys()` | every key, sorted |
| `Scan(prefix)` | every key with that prefix, sorted, each with its values |

The returned slices are the index's own storage and must not be modified.

### Typed wrappers

Embed the projection in a struct and add the typed methods your callers need. The embedded projection supplies `Name`, `Add` and `Query`:

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

If the `Store` needs to reach the typed methods, define a small interface for them in `index.go` and look the index up by name with a type assertion, as `Store.KindOf` does with `KindQuerier`.

## Registering an index

Add the constructor to `DefaultIndexes()` in `internal/metadata/defaults.go`. The server and the CLI both build their data indexes from that list. The server then adds the capability and role indexes itself (`server/main.go`), because those take part in authorization and the CLI should never depend on them.

Indexes always come from a constructor. A `Projection` has no usable zero value: `&metadata.TagIndex{}` has a nil embedded projection and will panic.

## Things to know

- **No locking.** An index does none of its own. `Store` serializes `Add` and `Query` under its mutex.
- **Keys are plain strings.** To look something up more than one way, emit several pairs with distinct key prefixes in one handler, as the Relation index does (`from:`, `to:`, `rel:`).
- **Derived keys are fine.** A key may come from the entry envelope rather than the payload, as the Date index does with `e.Created`.
- **Order is log order.** In list modes values come back in the order the entries were logged. `Keys()` and `Scan()` are sorted; if a caller needs a different order (the Name index sorts case-insensitively), sort in your typed method.
- **Handlers must be deterministic.** An index must come out the same every time the log is replayed, so do not read the clock, the filesystem or the network from a handler.

## When not to use `Projection`

A projection only ever adds. Write the `Index` interface by hand when an index must:

- undo an earlier entry (revocations, removing a role assignment), or
- keep several linked structures consistent with each other (principal to roles, role to principals, role to grants).

`CapabilityIndex` and `RoleIndex` are written this way.

## Testing an index

Test handlers by calling `Add` with hand-built entries and checking `Query` or your typed methods. `projection_test.go` has helpers for building stash and relation entries. Also run one test through a real `Store`: append an entry, query it, then open a second `Store` on the same directory to check that replaying the log rebuilds the index.
