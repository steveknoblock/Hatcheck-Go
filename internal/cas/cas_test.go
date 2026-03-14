package cas

import (
	"os"
	"testing"
)

func TestStash(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	hash, err := Stash("Hello World", objPath)
	if err != nil {
		t.Fatalf("Stash failed: %v", err)
	}
	if len(hash) != 32 {
		t.Errorf("expected 32 char hash, got %d", len(hash))
	}
}

func TestFetch(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	data := "Hello World"
	hash, err := Stash(data, objPath)
	if err != nil {
		t.Fatalf("Stash failed: %v", err)
	}

	result, err := Fetch(hash, objPath)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}
	if result != data {
		t.Errorf("expected %q, got %q", data, result)
	}
}

func TestFetchInvalidHash(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	_, err = Fetch("invalidhash", objPath)
	if err == nil {
		t.Error("expected error for invalid hash, got nil")
	}
}

func TestRoundTrip(t *testing.T) {
	objPath, err := os.MkdirTemp("", "hatcheck-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(objPath)

	tests := []string{
		"Hello World",
		"Hello World 255",
		"The quick brown fox jumps over the lazy dog",
		"1234567890",
		"",
	}

	for _, data := range tests {
		hash, err := Stash(data, objPath)
		if err != nil {
			t.Fatalf("Stash failed for %q: %v", data, err)
		}

		result, err := Fetch(hash, objPath)
		if err != nil {
			t.Fatalf("Fetch failed for %q: %v", data, err)
		}

		if result != data {
			t.Errorf("round trip failed: expected %q, got %q", data, result)
		}
	}
}
