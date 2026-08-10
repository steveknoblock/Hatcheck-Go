package share

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ExportMarkdown renders every name in namespace into outDir as markdown,
// suitable for use as a Hugo content directory (or any static site
// generator that reads a plain directory of .md files):
//
//   - A name resolving to a leaf (plain stashed text) becomes <slug>.md.
//   - A name resolving to a Collection becomes a directory <slug>/ with
//     an _index.md, one entry per member.
//   - A Collection member that is itself a named object in this namespace
//     is rendered the same way (file or subdirectory) using that name.
//     A member with no name of its own falls back to a shortened hash —
//     the same fallback the UI uses for unnamed collection members.
//   - A Relation encountered along the way gets a minimal descriptive
//     file rather than being silently dropped.
//
// Front matter (YAML title/date/tags) is included "when practical" — only
// where there's a real title to put in it. Top-level names and named
// collection members always have one (the log always records a label and
// a created/updated timestamp for a Name); a genuinely anonymous member
// gets plain content instead, with no front matter block at all.
//
// Like the rest of this package, ExportMarkdown reads the raw log and CAS
// directly rather than going through metadata.Store, so it works the same
// whether or not the server is running.
func ExportMarkdown(objPath, metaPath, namespace, outDir string) error {
	entries, err := namespaceEntries(namespace, metaPath)
	if err != nil {
		return fmt.Errorf("resolving namespace %q: %w", namespace, err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("namespace %q not found or has no names", namespace)
	}

	// hashToEntry lets a Collection member that happens to also be a
	// top-level Name in this namespace pick up its real title/date,
	// however deep it's nested.
	hashToEntry := make(map[string]namedEntry, len(entries))
	for _, e := range entries {
		hashToEntry[e.Hash] = e
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	used := make(map[string]int) // slug collisions among top-level names

	for _, e := range entries {
		destBase := uniquePath(outDir, slug(e.Label), used)
		// Each top-level name starts its own ancestor chain — content
		// reused across two different documents in the namespace should
		// render in both places, not be silently skipped the second time
		// (unlike the flat CAS archive in Export, where one physical copy
		// is correct regardless of how many things reference it, a
		// markdown file tree needs the same content to actually exist
		// wherever it's reachable from).
		if err := writeMarkdownEntry(e.Hash, destBase, e.Label, e.Updated, objPath, hashToEntry, map[string]bool{}); err != nil {
			return fmt.Errorf("exporting %q: %w", e.Label, err)
		}
	}

	return nil
}

// namedEntry is a name's current state as of the most recent log entry
// that set it: the (namespace-stripped) label, the hash it currently
// points to, and when that hash was last set.
type namedEntry struct {
	Label   string
	Hash    string
	Updated time.Time
}

// namespaceEntries resolves every name in namespace to its current hash
// and the timestamp of the log entry that last set it, in one pass over
// the log. This subsumes namesInNamespace + resolveNameFromLog for
// markdown export's purposes, but is kept as separate, independent logic
// rather than refactoring those — this package has no tests exercising a
// shared code path between namespace .tar.gz export and markdown export,
// and the two have different failure-mode consequences (a bug here can't
// silently affect the already-shipped archive export).
func namespaceEntries(namespace, metaPath string) ([]namedEntry, error) {
	entries, err := readLog(metaPath)
	if err != nil {
		return nil, err
	}

	type envelope struct {
		Op      string          `json:"op"`
		Created time.Time       `json:"created"`
		Payload json.RawMessage `json:"payload"`
	}
	type namePayload struct {
		Label string `json:"label"`
		Hash  string `json:"hash"`
	}

	prefix := namespace + "/"
	latest := make(map[string]namedEntry)
	var order []string

	for _, raw := range entries {
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}
		if env.Op != "name-create" && env.Op != "name-update" {
			continue
		}
		var p namePayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			continue
		}
		if !strings.HasPrefix(p.Label, prefix) {
			continue
		}
		if _, seen := latest[p.Label]; !seen {
			order = append(order, p.Label)
		}
		latest[p.Label] = namedEntry{
			Label:   strings.TrimPrefix(p.Label, prefix),
			Hash:    p.Hash,
			Updated: env.Created,
		}
	}

	result := make([]namedEntry, 0, len(order))
	for _, label := range order {
		result = append(result, latest[label])
	}
	return result, nil
}

// writeMarkdownEntry renders the object at hash into destPath — a single
// "<destPath>.md" file for a leaf or relation, a directory at destPath
// (with an _index.md) for a Collection — recursing into Collection
// members. title/updated drive front matter and are empty/zero for an
// anonymous member with no Name of its own.
//
// ancestors tracks hashes on the current root-to-node path only (not a
// global "already written" set): the same content reachable from two
// different, non-overlapping branches is written in both places, since a
// markdown file tree needs real content wherever it's reachable, unlike
// the flat CAS archive in Export where one physical copy is correct
// regardless of reference count. A hash reappearing as its own ancestor
// would mean an actual cycle — structurally not supposed to be possible
// given CAS objects can only reference hashes that already existed before
// they were created, but guarded against defensively rather than
// recursing forever if the data is ever corrupt.
func writeMarkdownEntry(hash, destPath, title string, updated time.Time, objPath string, hashToEntry map[string]namedEntry, ancestors map[string]bool) error {
	if ancestors[hash] {
		return fmt.Errorf("cycle detected at %s", refString(hash, hashToEntry))
	}

	content, err := readObject(hash, objPath)
	if err != nil {
		return err
	}

	// Try Collection: JSON array of hashes.
	var members []string
	if err := json.Unmarshal([]byte(content), &members); err == nil {
		if err := os.MkdirAll(destPath, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", destPath, err)
		}
		// _index.md always exists so the directory is navigable, but only
		// carries front matter when this collection itself has a title —
		// an untitled nested collection just gets an empty index.
		indexPath := filepath.Join(destPath, "_index.md")
		if err := writeMarkdownFile(indexPath, title, updated, nil, ""); err != nil {
			return err
		}

		childAncestors := make(map[string]bool, len(ancestors)+1)
		for h := range ancestors {
			childAncestors[h] = true
		}
		childAncestors[hash] = true

		used := make(map[string]int) // slug collisions among siblings in this directory
		for i, memberHash := range members {
			memberTitle, memberUpdated := "", time.Time{}
			if e, ok := hashToEntry[memberHash]; ok {
				memberTitle, memberUpdated = e.Label, e.Updated
			}

			var base string
			if memberTitle != "" {
				base = slug(memberTitle)
			} else {
				base = memberHash[:8] // anonymous member — same fallback as the UI
			}
			memberPath := uniquePath(destPath, base, used)

			if err := writeMarkdownEntry(memberHash, memberPath, memberTitle, memberUpdated, objPath, hashToEntry, childAncestors); err != nil {
				return fmt.Errorf("member %d (%s): %w", i, memberHash, err)
			}
		}
		return nil
	}

	// Try Relation: JSON object with from/rel/to.
	var relation struct {
		From string `json:"from"`
		Rel  string `json:"rel"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal([]byte(content), &relation); err == nil && relation.From != "" && relation.To != "" {
		body := fmt.Sprintf(
			"Relation: %s\n\nFrom: %s\nTo: %s\n",
			relation.Rel, refString(relation.From, hashToEntry), refString(relation.To, hashToEntry),
		)
		return writeMarkdownFile(destPath+".md", title, updated, nil, body)
	}

	// Plain leaf — the common case. Tags only get parsed (and therefore
	// only ever appear) alongside a title: an anonymous member gets no
	// front matter block at all, so there's nowhere for tags to go either.
	var tags []string
	if title != "" {
		tags = parseTagsForExport(content)
	}
	return writeMarkdownFile(destPath+".md", title, updated, tags, content)
}

// refString renders a hash reference for the body of a Relation's
// exported file — the referenced object's name if it has one in this
// namespace, otherwise a shortened hash.
func refString(hash string, hashToEntry map[string]namedEntry) string {
	if e, ok := hashToEntry[hash]; ok {
		return e.Label
	}
	if len(hash) > 8 {
		return hash[:8] + "…"
	}
	return hash
}

// writeMarkdownFile writes a single markdown file. A YAML front matter
// block is included only when title is non-empty — front matter "when
// practical" means when there's real metadata to put in it; a title-less
// file (an anonymous collection member) is just plain content.
func writeMarkdownFile(path, title string, updated time.Time, tags []string, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var b strings.Builder
	if title != "" {
		b.WriteString("---\n")
		b.WriteString("title: " + yamlQuote(title) + "\n")
		if !updated.IsZero() {
			b.WriteString("date: " + updated.UTC().Format(time.RFC3339) + "\n")
		}
		if len(tags) > 0 {
			quoted := make([]string, len(tags))
			for i, t := range tags {
				quoted[i] = yamlQuote(t)
			}
			b.WriteString("tags: [" + strings.Join(quoted, ", ") + "]\n")
		}
		b.WriteString("---\n\n")
	}
	b.WriteString(body)
	if body != "" && !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
}

// uniquePath joins base onto dir, appending "-2", "-3", ... on collision
// so two sibling names that slugify to the same string don't overwrite
// each other. used is scoped to a single directory's siblings — the
// caller creates a fresh map per directory (top-level names in outDir,
// or a Collection's own members).
func uniquePath(dir, base string, used map[string]int) string {
	used[base]++
	if used[base] == 1 {
		return filepath.Join(dir, base)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d", base, used[base]))
}

// --- Local, self-contained helpers ---
//
// slug and parseTagsForExport intentionally duplicate logic that already
// exists elsewhere (server/auth_handlers.go's slugify, metadata.ParseTags)
// rather than importing across packages for it. This package works
// directly off the raw log and CAS specifically so it can operate
// independently of a running Store (see Export/Import above) — pulling in
// internal/metadata for two small pure functions would work today (no
// import cycle), but would quietly attach this package's independence to
// metadata's, which is exactly the coupling the rest of this file avoids.

var hashtagRe = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

// parseTagsForExport extracts #hashtags from content the same way
// metadata.ParseTags does: lowercased, deduplicated, in first-seen order.
func parseTagsForExport(content string) []string {
	matches := hashtagRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	var tags []string
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}

// slug lowercases s and replaces every run of characters other than
// a-z, 0-9, '-', and '_' with a single '-', trimming leading/trailing '-'.
// Mirrors server/auth_handlers.go's slugify exactly, for the same reason
// documented above.
func slug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// yamlQuote double-quotes s for use as a YAML scalar, escaping backslashes
// and double quotes so titles/tags containing them can't break the front
// matter block.
func yamlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
