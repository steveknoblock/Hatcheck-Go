package cas

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"testing"
)

// testStore creates a CAS Store with an MD5 hash function and a temp directory.
// The caller is responsible for removing the directory via the returned cleanup func.
func testStore(t *testing.T) (*Store, func()) {
	t.Helper()
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(objPath, func(content string) string {
		sum := md5.Sum([]byte(content))
		return hex.EncodeToString(sum[:])
	})
	if err != nil {
		os.RemoveAll(objPath)
		t.Fatalf("failed to create store: %v", err)
	}
	return store, func() { os.RemoveAll(objPath) }
}

func TestNew(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewNilHashFunc(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	_, err = New(objPath, nil)
	if err == nil {
		t.Error("expected error for nil hashFunc, got nil")
	}
}

func TestStash(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	hash, err := store.Stash("Hello World")
	if err != nil {
		t.Fatalf("Stash failed: %v", err)
	}
	if len(hash) != 32 {
		t.Errorf("expected 32 char hash, got %d", len(hash))
	}
}

func TestStashShortHash(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	// Hash function that returns a string too short to shard.
	store, err := New(objPath, func(content string) string {
		return "x"
	})
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	_, err = store.Stash("Hello World")
	if err == nil {
		t.Error("expected error for short hash, got nil")
	}
}

func TestFetch(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	data := "Hello World"
	hash, err := store.Stash(data)
	if err != nil {
		t.Fatalf("Stash failed: %v", err)
	}

	result, err := store.Fetch(hash)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if result != data {
		t.Errorf("expected %q, got %q", data, result)
	}
}

func TestFetchInvalidHash(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	_, err := store.Fetch("invalidhash")
	if err == nil {
		t.Error("expected error for invalid hash, got nil")
	}
}

func TestList(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	// Empty store.
	hashes, err := store.List()
	if err != nil {
		t.Fatalf("List failed on empty store: %v", err)
	}
	if len(hashes) != 0 {
		t.Errorf("expected 0 hashes, got %d", len(hashes))
	}

	// Stash some objects.
	contents := []string{"alpha", "beta", "gamma"}
	for _, c := range contents {
		if _, err := store.Stash(c); err != nil {
			t.Fatalf("Stash failed for %q: %v", c, err)
		}
	}

	hashes, err = store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(hashes) != len(contents) {
		t.Errorf("expected %d hashes, got %d", len(contents), len(hashes))
	}
}

func TestRoundTrip(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	tests := []string{
		"Hello World",
		"Hello World 255",
		"The quick brown fox jumps over the lazy dog",
		"1234567890",
		"",
	}

	for _, data := range tests {
		hash, err := store.Stash(data)
		if err != nil {
			t.Fatalf("Stash failed for %q: %v", data, err)
		}

		result, err := store.Fetch(hash)
		if err != nil {
			t.Fatalf("Fetch failed for %q: %v", data, err)
		}

		if result != data {
			t.Errorf("round trip failed: expected %q, got %q", data, result)
		}
	}
}

func TestDeduplication(t *testing.T) {
	store, cleanup := testStore(t)
	defer cleanup()

	// Stashing the same content twice should return the same hash.
	hash1, err := store.Stash("same content")
	if err != nil {
		t.Fatalf("first Stash failed: %v", err)
	}
	hash2, err := store.Stash("same content")
	if err != nil {
		t.Fatalf("second Stash failed: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("expected same hash for same content, got %q and %q", hash1, hash2)
	}

	// And only one object should exist in the store.
	hashes, err := store.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(hashes) != 1 {
		t.Errorf("expected 1 object after deduplication, got %d", len(hashes))
	}
}
