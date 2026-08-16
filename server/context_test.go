package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- contextHandler ---

func TestContextHandler_Success(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	subject := stashOne(t, store, meta, "subject object #main")
	item1 := stashOne(t, store, meta, "context item one")
	item2 := stashOne(t, store, meta, "context item two")

	body := `{"subject":"` + subject + `","items":["` + item1 + `","` + item2 + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(body))
	w := httptest.NewRecorder()
	contextHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var result stashResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response %q: %v", w.Body.String(), err)
	}
	if len(result.Hash) != 64 {
		t.Errorf("expected 64-char hash, got %q", result.Hash)
	}
	if result.Capability.Hash != result.Hash {
		t.Errorf("expected capability to cover %q, got %q", result.Hash, result.Capability.Hash)
	}

	// The Collection should be recorded with exactly 2 member hashes...
	if meta.KindOf(result.Hash) != "collection" {
		t.Errorf("expected context hash to be a collection, got kind %q", meta.KindOf(result.Hash))
	}

	// ...and RelationIndex.byTo should show both relations against the
	// subject — this is the actual "is this a Context" recognition path,
	// since nothing marks the Collection itself as a Context.
	_, incoming := meta.RelationsForHash(subject)
	if len(incoming) != 2 {
		t.Fatalf("expected 2 is-context-for relations pointing at subject, got %d", len(incoming))
	}
	for _, rel := range incoming {
		if rel.Rel != isContextFor {
			t.Errorf("expected rel %q, got %q", isContextFor, rel.Rel)
		}
		if rel.To != subject {
			t.Errorf("expected relation To to be subject %q, got %q", subject, rel.To)
		}
	}
}

func TestContextHandler_EmptyItemsAllowed(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	subject := stashOne(t, store, meta, "subject object #alone")

	body := `{"subject":"` + subject + `","items":[]}`
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(body))
	w := httptest.NewRecorder()
	contextHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	// No minimum-size enforcement is a deliberate backlog decision, not a
	// bug — a zero-item Context is just an empty Collection.
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for empty items, got %d: %s", w.Code, w.Body.String())
	}
}

func TestContextHandler_MissingSubject(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	body := `{"items":["aabbcc"]}`
	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader(body))
	w := httptest.NewRecorder()
	contextHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing subject, got %d", w.Code)
	}
}

func TestContextHandler_InvalidBody(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodPost, "/context", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	contextHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestContextHandler_WrongMethod(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/context", nil)
	w := httptest.NewRecorder()
	contextHandler(w, req, store, meta, serverTestKey, testConfig(), vrEmpty())

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// --- createContext ---

func TestCreateContext_AtomicBatch(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	subject := stashOne(t, store, meta, "subject for atomic test")
	item := stashOne(t, store, meta, "single context item")

	hash, cap, err := createContext(store, meta, serverTestKey, subject, []string{item}, "test-user", "", testConfig().CapabilityExpiry)
	if err != nil {
		t.Fatalf("createContext() error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty context hash")
	}
	if cap.Hash != hash {
		t.Errorf("expected capability to cover %q, got %q", hash, cap.Hash)
	}

	// Exactly 2 new log entries should exist for this call: 1 relation +
	// 1 collection — plus whatever stashOne's own /stash calls already
	// added, so check relative counts via RelationIndex instead of a raw
	// log length, which would be brittle against test setup order.
	results := meta.Query("relation", "to:"+subject)
	if len(results) != 1 {
		t.Fatalf("expected 1 relation indexed for subject, got %d", len(results))
	}
}

func TestCreateContext_RejectsEmptySubject(t *testing.T) {
	store, _, _, meta := newTestEnv(t)

	_, _, err := createContext(store, meta, serverTestKey, "", []string{"item1"}, "test-user", "", testConfig().CapabilityExpiry)
	if err == nil {
		t.Fatal("expected error for empty subject, got nil")
	}
}

func TestCreateContext_RejectsEmptyItemHash(t *testing.T) {
	store, _, _, meta := newTestEnv(t)
	subject := stashOne(t, store, meta, "subject for empty item test")

	_, _, err := createContext(store, meta, serverTestKey, subject, []string{"real-hash", ""}, "test-user", "", testConfig().CapabilityExpiry)
	if err == nil {
		t.Fatal("expected error for an empty item hash, got nil")
	}

	// Nothing from the rejected call should have landed — the first
	// (valid) item's relation must not be a partial write left behind.
	results := meta.Query("relation", "to:"+subject)
	if len(results) != 0 {
		t.Errorf("expected no relations recorded after a rejected call, got %d", len(results))
	}
}
