package metadata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// --- Projection ---

// Mode controls how a Projection stores values that arrive for a key it
// has already seen.
type Mode int

const (
	// AppendUnique keeps a list of values per key, in arrival order,
	// skipping any value already present under that key. Tag, Date and
	// Relation-style indexes use this.
	AppendUnique Mode = iota

	// Latest keeps exactly one value per key; a later value replaces an
	// earlier one. Kind, Created and Name-style indexes use this.
	Latest
)

// Pair is one key/value mutation produced by a handler for a log entry.
type Pair[V any] struct {
	Key   string
	Value V
}

// Handler turns one kind of log entry into zero or more Pairs. Build one
// with On rather than constructing it directly.
type Handler[V any] struct {
	op string
	fn func(Entry) []Pair[V]
}

// On declares how a Projection reacts to entries with the given op. P is
// the payload type for that op; the entry's payload is decoded into P
// before fn is called, so handlers never repeat the json.Unmarshal
// boilerplate. An entry whose payload fails to decode is skipped, which
// matches the behaviour of the hand-written indexes.
//
//	On(OpStash, func(e Entry, p StashPayload) []Pair[string] {
//	    return []Pair[string]{{Key: p.Hash, Value: "stash"}}
//	})
func On[P, V any](op string, fn func(e Entry, p P) []Pair[V]) Handler[V] {
	return Handler[V]{
		op: op,
		fn: func(e Entry) []Pair[V] {
			var p P
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				return nil
			}
			return fn(e, p)
		},
	}
}

// Projection is a read model built by folding over the metadata log: each
// log entry is offered to the handlers registered for its op, and the
// key/value pairs they emit are stored according to the Projection's Mode.
// It is derived entirely from the log and holds no state of its own, so it
// can always be rebuilt by replaying the log.
//
// Projection satisfies Index. Like the hand-written indexes it does no
// locking of its own; Store serialises Add and Query under its own mutex.
// Unlike them it has no usable zero value — construct it with
// NewProjection.
type Projection[V comparable] struct {
	name     string
	mode     Mode
	handlers map[string][]func(Entry) []Pair[V]
	data     map[string][]V

	queryKey func(string) string
	render   func(V) string
}

// NewProjection returns an empty Projection with the given index name,
// storage mode, and handlers. Several handlers may be registered for the
// same op; they run in the order given.
func NewProjection[V comparable](name string, mode Mode, handlers ...Handler[V]) *Projection[V] {
	p := &Projection[V]{
		name:     name,
		mode:     mode,
		handlers: make(map[string][]func(Entry) []Pair[V]),
		data:     make(map[string][]V),
	}
	for _, h := range handlers {
		p.handlers[h.op] = append(p.handlers[h.op], h.fn)
	}
	return p
}

// WithQueryKey sets a function applied to the key passed to Get and Query
// before lookup (for example strings.ToLower, so that queries are
// case-insensitive). It is applied at lookup only; stored keys are exactly
// what the handlers emitted. Returns p for chaining.
func (p *Projection[V]) WithQueryKey(fn func(string) string) *Projection[V] {
	p.queryKey = fn
	return p
}

// WithRender sets how a value is rendered to a string for Query, which is
// the string-only view required by the Index interface. The default
// renders a string value as itself and anything else with fmt.Sprint.
// Returns p for chaining.
func (p *Projection[V]) WithRender(fn func(V) string) *Projection[V] {
	p.render = fn
	return p
}

// Name satisfies Index.
func (p *Projection[V]) Name() string { return p.name }

// Add satisfies Index. It offers the entry to the handlers registered for
// its op and stores whatever pairs they emit. Entries with no registered
// handler are ignored.
func (p *Projection[V]) Add(entry Entry) {
	fns := p.handlers[entry.Op]
	if len(fns) == 0 {
		return
	}
	if p.data == nil {
		p.data = make(map[string][]V)
	}
	for _, fn := range fns {
		for _, pair := range fn(entry) {
			p.put(pair)
		}
	}
}

func (p *Projection[V]) put(pair Pair[V]) {
	switch p.mode {
	case Latest:
		p.data[pair.Key] = []V{pair.Value}
	default: // AppendUnique
		for _, existing := range p.data[pair.Key] {
			if existing == pair.Value {
				return
			}
		}
		p.data[pair.Key] = append(p.data[pair.Key], pair.Value)
	}
}

func (p *Projection[V]) lookupKey(key string) string {
	if p.queryKey != nil {
		return p.queryKey(key)
	}
	return key
}

// Get returns the values stored under key, or nil if there are none. In
// Latest mode the result has at most one element. The returned slice is
// the Projection's own storage and must not be modified.
func (p *Projection[V]) Get(key string) []V {
	return p.data[p.lookupKey(key)]
}

// Value returns the single value stored under key. It is the natural
// accessor for Latest mode; in AppendUnique mode it returns the most
// recently added value.
func (p *Projection[V]) Value(key string) (V, bool) {
	vals := p.data[p.lookupKey(key)]
	if len(vals) == 0 {
		var zero V
		return zero, false
	}
	return vals[len(vals)-1], true
}

// Keys returns every key in the Projection, sorted so that callers get a
// deterministic order.
func (p *Projection[V]) Keys() []string {
	keys := make([]string, 0, len(p.data))
	for k := range p.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Scan returns every key with the given prefix, in sorted key order, each
// paired with its values. An empty prefix returns everything. The
// returned value slices are the Projection's own storage and must not be
// modified.
func (p *Projection[V]) Scan(prefix string) []Pair[[]V] {
	var out []Pair[[]V]
	for _, k := range p.Keys() {
		if strings.HasPrefix(k, prefix) {
			out = append(out, Pair[[]V]{Key: k, Value: p.data[k]})
		}
	}
	return out
}

// Query satisfies Index. It returns the values stored under key rendered
// as strings, or nil if there are none.
func (p *Projection[V]) Query(key string) []string {
	vals := p.Get(key)
	if len(vals) == 0 {
		return nil
	}
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = p.renderValue(v)
	}
	return out
}

func (p *Projection[V]) renderValue(v V) string {
	if p.render != nil {
		return p.render(v)
	}
	if s, ok := any(v).(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

var _ Index = (*Projection[string])(nil)
