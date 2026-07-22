package main

import (
	"testing"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
)

// --- slugify ---

func TestSlugify_LowercasesAndKeepsSafeChars(t *testing.T) {
	got := slugify("Steve_Knoblock-99")
	want := "steve_knoblock-99"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSlugify_ReplacesUnsafeCharsWithSingleDash(t *testing.T) {
	got := slugify("steve.knoblock+test@work")
	want := "steve-knoblock-test-work"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSlugify_CollapsesConsecutiveUnsafeRuns(t *testing.T) {
	got := slugify("a!!!b   c")
	want := "a-b-c"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSlugify_TrimsLeadingTrailingDashes(t *testing.T) {
	got := slugify("  .steve.  ")
	want := "steve"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestSlugify_EmptyInputGivesEmptyOutput(t *testing.T) {
	if got := slugify(""); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestSlugify_AllUnsafeCharsGivesEmptyOutput(t *testing.T) {
	if got := slugify("!!!@@@..."); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// --- defaultNamespace ---

func TestDefaultNamespace_PrefersEmailLocalPart(t *testing.T) {
	identity := auth.Identity{UserID: "user-live-abc123", Email: "steve@example.com"}
	got := defaultNamespace(identity)
	want := "steve"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDefaultNamespace_FallsBackToUserIDWhenNoEmail(t *testing.T) {
	identity := auth.Identity{UserID: "user-live-abc123", Email: ""}
	got := defaultNamespace(identity)
	want := "user-live-abc123"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDefaultNamespace_FallsBackToUserIDWhenEmailLocalPartIsUnsafe(t *testing.T) {
	// Local part is entirely punctuation and slugifies to empty — must not
	// hand back an empty namespace just because the email was unusual.
	identity := auth.Identity{UserID: "user-live-abc123", Email: "...@example.com"}
	got := defaultNamespace(identity)
	want := "user-live-abc123"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDefaultNamespace_NeverReturnsEmpty(t *testing.T) {
	// No email, no UserID — should still return something rather than "".
	identity := auth.Identity{}
	if got := defaultNamespace(identity); got == "" {
		t.Error("expected a non-empty fallback namespace, got empty string")
	}
}

func TestDefaultNamespace_EmailWithoutAtSignFallsBackToUserID(t *testing.T) {
	// Malformed email with no '@' — strings.Cut's ok will be false.
	identity := auth.Identity{UserID: "user-live-abc123", Email: "not-an-email"}
	got := defaultNamespace(identity)
	want := "user-live-abc123"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
