package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// A Context is deliberately not a new primitive: it's a Collection whose
// members are exactly the "is-context-for" Relation hashes pointing at one
// subject. There is no new Op, Payload, or Kind — recognizing a Context is
// done by querying RelationIndex.byTo for the subject, not by reading any
// marker on the Collection itself. See the is-context-for constant below;
// callers that build a Collection of relations any other way (or with
// mismatched To values) simply haven't made a Context, by convention —
// nothing here validates that at creation.
const isContextFor = "is-context-for"

// relationContent is the CAS-stored shape of a Relation, matching what
// relationHandler stores for a client-submitted relation body — no Hash
// field, since the hash is derived from this content, not embedded in it.
type relationContent struct {
	From string `json:"from"`
	Rel  string `json:"rel"`
	To   string `json:"to"`
}

// createContext builds a Context: one "is-context-for" Relation per item
// (each pointing at subject), plus one Collection wrapping all of their
// hashes. All of it — the CAS objects and the metadata log entries alike —
// is written before this function returns anything but an error, so a
// caller that gets a hash back can trust the whole structure exists.
//
// items may be empty; a Context with no context items yet is just an
// empty Collection, same as any other Collection. Enforcing a minimum is
// a deliberate backlog item, not an oversight.
func createContext(
	store *cas.Store,
	meta *metadata.Store,
	key []byte,
	subject string,
	items []string,
	principal, email string,
	expiry time.Duration,
) (string, metadata.CapabilityPayload, error) {
	if subject == "" {
		return "", metadata.CapabilityPayload{}, fmt.Errorf("subject is required")
	}

	now := time.Now().UTC()
	relHashes := make([]string, len(items))
	entries := make([]metadata.Entry, 0, len(items)+1)

	// Stash every relation's CAS content first. CAS writes are
	// independent and idempotent (content-addressed — restashing
	// identical content on retry reproduces the same hash), so if this
	// loop fails partway through, whatever was already stashed is just
	// harmless orphan content, not corruption. Nothing durable is
	// recorded yet — that only happens in the AppendBatch call below.
	for i, item := range items {
		if item == "" {
			return "", metadata.CapabilityPayload{}, fmt.Errorf("context item %d is empty", i)
		}

		content, err := json.Marshal(relationContent{From: item, Rel: isContextFor, To: subject})
		if err != nil {
			return "", metadata.CapabilityPayload{}, err
		}
		hash, err := store.Stash(string(content))
		if err != nil {
			return "", metadata.CapabilityPayload{}, fmt.Errorf("failed to stash context relation for %s: %w", item, err)
		}
		relHashes[i] = hash

		payload, err := json.Marshal(metadata.RelationPayload{Hash: hash, From: item, Rel: isContextFor, To: subject})
		if err != nil {
			return "", metadata.CapabilityPayload{}, err
		}
		entries = append(entries, metadata.Entry{Op: metadata.OpRelation, Created: now, Payload: payload})
	}

	colContent, err := json.Marshal(relHashes)
	if err != nil {
		return "", metadata.CapabilityPayload{}, err
	}
	colHash, err := store.Stash(string(colContent))
	if err != nil {
		return "", metadata.CapabilityPayload{}, fmt.Errorf("failed to stash context collection: %w", err)
	}

	colPayload, err := json.Marshal(metadata.CollectionPayload{Hash: colHash, Hashes: relHashes})
	if err != nil {
		return "", metadata.CapabilityPayload{}, err
	}
	entries = append(entries, metadata.Entry{Op: metadata.OpCollection, Created: now, Payload: colPayload})

	// Unlike stashAndIssue's warn-and-continue on metadata failures, a
	// half-written Context is a corrupt structure with no clean recovery
	// — so a batch failure here is returned to the caller as a real
	// error rather than logged and swallowed. AppendBatch guarantees
	// this is all-or-nothing: either every relation and the collection
	// landed, or none of them did.
	if err := meta.AppendBatch(entries); err != nil {
		return "", metadata.CapabilityPayload{}, fmt.Errorf("failed to record context: %w", err)
	}

	// Only the Collection — the nameable, shareable handle for the whole
	// Context — gets a capability issued back to the caller. The
	// relations underneath it are implementation detail; nothing about
	// having a Context implies independent access to each relation.
	email = resolveEmail(meta, principal, email)
	expires := time.Now().UTC().Add(expiry)
	cap := metadata.SignCapability(key, colHash, PermWrite, principal, email, expires)
	if err := meta.AppendCapability(cap); err != nil {
		log.Printf("warning: failed to record capability for context %s: %v", colHash, err)
	}

	return colHash, cap, nil
}

// contextRequest is the JSON body for POST /context.
type contextRequest struct {
	Subject string   `json:"subject"`
	Items   []string `json:"items"`
}

// contextHandler creates a Context for a subject hash: one is-context-for
// Relation per item, wrapped in a Collection, written atomically.
// POST /context — body: {"subject":"<hash>","items":["<hash>",...]}
func contextHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store, key []byte, cfg Config, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	var creq contextRequest
	if err := json.Unmarshal(body, &creq); err != nil {
		http.Error(w, `body must be JSON: {"subject":"<hash>","items":["<hash>",...]}`, http.StatusBadRequest)
		return
	}

	if creq.Subject == "" {
		http.Error(w, "subject is required", http.StatusBadRequest)
		return
	}

	hash, cap, err := createContext(store, meta, key, creq.Subject, creq.Items, vr.Principal, vr.Email, cfg.CapabilityExpiry)
	if err != nil {
		log.Printf("context error: %v", err)
		http.Error(w, "failed to create context", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(stashResponse{Hash: hash, Capability: cap})
}
