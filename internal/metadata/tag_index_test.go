package metadata

import (
	"reflect"
	"testing"
)

// --- Add / Query ---

func TestTagIndex_QueryEmpty(t *testing.T) {
	idx := NewTagIndex()
	if got := idx.Query("go"); got != nil {
		t.Errorf("Query on empty index = %v, want nil", got)
	}
}

func TestTagIndex_AddAndQuery(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1", "go", "cas"))

	if got, want := idx.Query("go"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(go) = %v, want %v", got, want)
	}
	if got, want := idx.Query("cas"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(cas) = %v, want %v", got, want)
	}
}

func TestTagIndex_MultipleHashesSameTag(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1", "go"))
	idx.Add(stashEntry(t, "h2", "go"))

	if got, want := idx.Query("go"), []string{"h1", "h2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(go) = %v, want %v", got, want)
	}
}

func TestTagIndex_DuplicateStashSameHashIsDeduplicated(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1", "go"))
	idx.Add(stashEntry(t, "h1", "go"))

	if got, want := idx.Query("go"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(go) = %v, want %v", got, want)
	}
}

func TestTagIndex_QueryIsCaseInsensitive(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1", "go"))

	if got, want := idx.Query("GO"), []string{"h1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Query(GO) = %v, want %v", got, want)
	}
}

func TestTagIndex_IgnoresNonStashOps(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(relationEntry(t, "r1", "a", "cites", "b"))

	if tags := idx.Tags(); len(tags) != 0 {
		t.Errorf("Tags() = %v, want none after a non-stash entry", tags)
	}
}

func TestTagIndex_StashWithNoTagsAddsNothing(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1"))

	if tags := idx.Tags(); len(tags) != 0 {
		t.Errorf("Tags() = %v, want none", tags)
	}
}

// --- Tags ---

func TestTagIndex_TagsEmpty(t *testing.T) {
	idx := NewTagIndex()
	tags := idx.Tags()
	if tags == nil || len(tags) != 0 {
		t.Errorf("Tags() on empty index = %#v, want empty non-nil slice", tags)
	}
}

func TestTagIndex_TagsListsEveryTagOnceSorted(t *testing.T) {
	idx := NewTagIndex()
	idx.Add(stashEntry(t, "h1", "zebra", "apple"))
	idx.Add(stashEntry(t, "h2", "apple", "mango"))

	if got, want := idx.Tags(), []string{"apple", "mango", "zebra"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tags() = %v, want %v", got, want)
	}
}

// --- Interfaces / Name ---

func TestTagIndex_Name(t *testing.T) {
	if got := NewTagIndex().Name(); got != "tag" {
		t.Errorf("Name() = %q, want %q", got, "tag")
	}
}

func TestTagIndex_SatisfiesTagLister(t *testing.T) {
	var _ TagLister = NewTagIndex()
}
