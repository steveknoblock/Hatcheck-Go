package metadata

import "testing"

func TestDefaultIndexes_NamesAreUniqueAndComplete(t *testing.T) {
	want := map[string]bool{
		"tag": true, "date": true, "name": true,
		"relation": true, "kind": true, "created": true,
	}
	got := map[string]bool{}
	for _, idx := range DefaultIndexes() {
		if got[idx.Name()] {
			t.Errorf("duplicate index name %q", idx.Name())
		}
		got[idx.Name()] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("DefaultIndexes missing %q", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("DefaultIndexes unexpectedly includes %q", name)
		}
	}
}

func TestDefaultIndexes_ExcludesAccessControlIndexes(t *testing.T) {
	for _, idx := range DefaultIndexes() {
		switch idx.Name() {
		case "capability", "role":
			t.Errorf("DefaultIndexes must not include %q", idx.Name())
		}
	}
}

func TestDefaultIndexes_ReturnsFreshInstances(t *testing.T) {
	a, b := DefaultIndexes(), DefaultIndexes()
	for i := range a {
		if a[i] == b[i] {
			t.Errorf("index %q shared between calls", a[i].Name())
		}
	}
}
