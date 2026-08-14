package share

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest records provenance information for an export.
type Manifest struct {
	Source    string    `json:"source"`
	Exported  time.Time `json:"exported"`
	Version   string    `json:"version"`
	Objects   int       `json:"objects"`
	Name      string    `json:"name,omitempty"`      // set for single-name partial exports
	Namespace string    `json:"namespace,omitempty"` // set for namespace-scoped partial exports
}

const manifestVersion = "1"

// Export bundles the CAS objects and metadata log into a tar.gz archive.
// Exactly one of name or namespace may be set (or neither, for a full export):
//   - name and namespace both empty: full export of every object and the
//     entire log.
//   - name set: partial export of everything reachable from that single
//     name.
//   - namespace set: partial export of everything reachable from every
//     name currently defined within that namespace, unioned together.
//
// The output file is named <source>.tar.gz unless outPath is specified.
func Export(objPath, metaPath, source, name, namespace, outPath string) error {
	if name != "" && namespace != "" {
		return fmt.Errorf("name and namespace are mutually exclusive")
	}

	if outPath == "" {
		outPath = source + ".tar.gz"
	}

	// Determine which hashes to export, and which name labels the partial
	// export covers (used later to filter the log).
	var hashes map[string]bool
	var names []string
	switch {
	case name != "":
		var err error
		hashes, err = reachableHashes(name, objPath, metaPath)
		if err != nil {
			return fmt.Errorf("resolving name %q: %w", name, err)
		}
		if len(hashes) == 0 {
			return fmt.Errorf("name %q not found or has no reachable objects", name)
		}
		names = []string{name}

	case namespace != "":
		var err error
		hashes, names, err = reachableHashesForNamespace(namespace, objPath, metaPath)
		if err != nil {
			return fmt.Errorf("resolving namespace %q: %w", namespace, err)
		}
	}

	// Create the output file.
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer outFile.Close()

	gz := gzip.NewWriter(outFile)
	defer gz.Close()

	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Collect objects to export.
	type objectEntry struct {
		archivePath string
		diskPath    string
		info        os.FileInfo
	}
	var objects []objectEntry

	err = filepath.Walk(objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		shard := filepath.Base(filepath.Dir(path))
		file := filepath.Base(path)
		hash := shard + file

		if hashes != nil && !hashes[hash] {
			return nil // skip — not reachable from the named root
		}

		rel, err := filepath.Rel(objPath, path)
		if err != nil {
			return err
		}
		objects = append(objects, objectEntry{
			// Tar archives are always POSIX-style regardless of host OS.
			// filepath.Join would use '\' on Windows here, which Import's
			// strings.HasPrefix(hdr.Name, "objects/") check would then
			// never match — silently skipping every object on import.
			// filepath.ToSlash normalizes rel before joining so the
			// archive path is forward-slash on every platform.
			archivePath: "objects/" + filepath.ToSlash(rel),
			diskPath:    path,
			info:        info,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("walking objects: %w", err)
	}

	// Write manifest.
	manifest := Manifest{
		Source:    source,
		Exported:  time.Now().UTC(),
		Version:   manifestVersion,
		Objects:   len(objects),
		Name:      name,
		Namespace: namespace,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	if err := writeBytes(tw, "manifest.json", manifestData); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	// Write CAS objects.
	for _, obj := range objects {
		if err := writeFile(tw, obj.archivePath, obj.diskPath, obj.info); err != nil {
			return fmt.Errorf("writing object: %w", err)
		}
	}

	// Write metadata log — full log for full export, filtered for partial.
	logEntries, err := readLog(metaPath)
	if err != nil {
		return fmt.Errorf("reading log: %w", err)
	}

	if hashes != nil {
		logEntries = filterLog(logEntries, hashes, names)
	}

	if len(logEntries) > 0 {
		logData, err := json.MarshalIndent(logEntries, "", "  ")
		if err != nil {
			return fmt.Errorf("encoding log: %w", err)
		}
		if err := writeBytes(tw, "metadata/log.json", logData); err != nil {
			return fmt.Errorf("writing log: %w", err)
		}
	}

	return nil
}

// Import unpacks a tar.gz archive into the destination CAS and metadata store.
// The source from the manifest is used to prefix name labels.
// Existing CAS objects are silently skipped.
func Import(archivePath, objPath, metaPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("reading gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	var manifest *Manifest
	var logEntries []json.RawMessage

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading archive: %w", err)
		}

		switch hdr.Name {
		case "manifest.json":
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading manifest: %w", err)
			}
			manifest = &Manifest{}
			if err := json.Unmarshal(data, manifest); err != nil {
				return fmt.Errorf("parsing manifest: %w", err)
			}

		case "metadata/log.json":
			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading log: %w", err)
			}
			if err := json.Unmarshal(data, &logEntries); err != nil {
				return fmt.Errorf("parsing log: %w", err)
			}

		default:
			if !strings.HasPrefix(hdr.Name, "objects/") {
				continue
			}
			rel := strings.TrimPrefix(hdr.Name, "objects/")
			destPath := filepath.Join(objPath, rel)

			if _, err := os.Stat(destPath); err == nil {
				continue // already exists — skip
			}

			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}

			data, err := io.ReadAll(tr)
			if err != nil {
				return fmt.Errorf("reading object: %w", err)
			}
			if err := os.WriteFile(destPath, data, 0644); err != nil {
				return fmt.Errorf("writing object: %w", err)
			}
		}
	}

	if manifest == nil {
		return fmt.Errorf("archive missing manifest.json")
	}

	if len(logEntries) == 0 {
		return nil
	}

	prefixed, err := prefixNameLabels(logEntries, manifest.Source)
	if err != nil {
		return fmt.Errorf("prefixing name labels: %w", err)
	}

	if err := mergeLog(metaPath, prefixed); err != nil {
		return fmt.Errorf("merging log: %w", err)
	}

	return nil
}

// --- Reachability ---

// reachableHashes returns the set of all hashes reachable from a named root.
// It follows Collections and Relations recursively with cycle detection.
func reachableHashes(name, objPath, metaPath string) (map[string]bool, error) {
	// Resolve name to root hash via the name index in the log.
	rootHash, err := resolveNameFromLog(name, metaPath)
	if err != nil {
		return nil, err
	}

	visited := make(map[string]bool)
	if err := traverse(rootHash, objPath, visited); err != nil {
		return nil, err
	}
	return visited, nil
}

// namesInNamespace returns every distinct name label in the log whose
// label begins with "<namespace>/", in first-seen order. It scans the raw
// log directly (rather than going through metadata.Store) to stay
// consistent with resolveNameFromLog and the rest of this package, which
// operates on export/import archives independently of a running store.
func namesInNamespace(namespace, metaPath string) ([]string, error) {
	entries, err := readLog(metaPath)
	if err != nil {
		return nil, err
	}

	type envelope struct {
		Op      string          `json:"op"`
		Payload json.RawMessage `json:"payload"`
	}
	type namePayload struct {
		Label string `json:"label"`
		Hash  string `json:"hash"`
	}

	prefix := namespace + "/"
	seen := make(map[string]bool)
	var names []string

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
		if !seen[p.Label] {
			seen[p.Label] = true
			names = append(names, p.Label)
		}
	}

	return names, nil
}

// reachableHashesForNamespace returns the union of all hashes reachable
// from the current hash of every name defined in the given namespace,
// along with the list of names that were resolved (for later log
// filtering). A single visited set is shared across all names so
// overlapping subtrees — a Collection referenced by two documents in the
// same namespace, for example — are only walked once.
func reachableHashesForNamespace(namespace, objPath, metaPath string) (map[string]bool, []string, error) {
	names, err := namesInNamespace(namespace, metaPath)
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("namespace %q not found or has no names", namespace)
	}

	visited := make(map[string]bool)
	for _, name := range names {
		rootHash, err := resolveNameFromLog(name, metaPath)
		if err != nil {
			return nil, nil, fmt.Errorf("resolving name %q: %w", name, err)
		}
		if err := traverse(rootHash, objPath, visited); err != nil {
			return nil, nil, err
		}
	}

	return visited, names, nil
}

// traverse recursively visits a hash and all hashes reachable from it.
func traverse(hash, objPath string, visited map[string]bool) error {
	if visited[hash] {
		return nil // already visited — cycle detected, stop
	}
	visited[hash] = true

	// Read the object content.
	content, err := readObject(hash, objPath)
	if err != nil {
		return err
	}

	// Try to parse as a Collection — JSON array of strings.
	var collection []string
	if err := json.Unmarshal([]byte(content), &collection); err == nil {
		for _, h := range collection {
			if err := traverse(h, objPath, visited); err != nil {
				return err
			}
		}
		return nil
	}

	// Try to parse as a Relation — JSON object with from/rel/to.
	var relation struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.Unmarshal([]byte(content), &relation); err == nil {
		if relation.From != "" {
			if err := traverse(relation.From, objPath, visited); err != nil {
				return err
			}
		}
		if relation.To != "" {
			if err := traverse(relation.To, objPath, visited); err != nil {
				return err
			}
		}
		return nil
	}

	// Plain object — already added to visited, nothing to recurse into.
	return nil
}

// readObject reads the content of a CAS object by hash.
func readObject(hash, objPath string) (string, error) {
	shard := hash[0:2]
	file := hash[2:]
	path := filepath.Join(objPath, shard, file)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading object %s: %w", hash, err)
	}
	return string(data), nil
}

// resolveNameFromLog finds the most recent hash for a name label in the log.
func resolveNameFromLog(name, metaPath string) (string, error) {
	entries, err := readLog(metaPath)
	if err != nil {
		return "", err
	}

	type envelope struct {
		Op      string          `json:"op"`
		Payload json.RawMessage `json:"payload"`
	}
	type namePayload struct {
		Label string `json:"label"`
		Hash  string `json:"hash"`
	}

	// Walk in reverse to find most recent entry for this name.
	for i := len(entries) - 1; i >= 0; i-- {
		var env envelope
		if err := json.Unmarshal(entries[i], &env); err != nil {
			continue
		}
		if env.Op != "name-create" && env.Op != "name-update" {
			continue
		}
		var p namePayload
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			continue
		}
		if p.Label == name {
			return p.Hash, nil
		}
	}

	return "", fmt.Errorf("name %q not found", name)
}

// filterLog returns only log entries relevant to the given hash set and
// name labels. names may contain a single label (single-name export) or
// several (namespace export); every name-create/name-update entry whose
// label appears in names is retained, preserving each label's full history.
func filterLog(entries []json.RawMessage, hashes map[string]bool, names []string) []json.RawMessage {
	type envelope struct {
		Op      string          `json:"op"`
		Payload json.RawMessage `json:"payload"`
	}
	type hashPayload struct {
		Hash string `json:"hash"`
	}
	type namePayload struct {
		Label string `json:"label"`
		Hash  string `json:"hash"`
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var result []json.RawMessage

	for _, raw := range entries {
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue
		}

		switch env.Op {
		case "stash", "collection", "relation":
			var p hashPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			if hashes[p.Hash] {
				result = append(result, raw)
			}

		case "name-create", "name-update":
			var p namePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				continue
			}
			if nameSet[p.Label] {
				result = append(result, raw)
			}
		}
	}

	return result
}

// --- Log helpers ---

// readLog reads the live store's on-disk log, which is NDJSON (one
// compact JSON entry per line) — this package works directly off the raw
// log independent of a running Store (see the note below on why it
// doesn't just call into internal/metadata for this), so it needs its own
// understanding of that format rather than importing metadata's.
//
// A store that has never been opened via metadata.Store.load() could
// still be sitting in the older single-JSON-array format, so that shape
// is also accepted here for safety — but nothing in this package ever
// writes that format back out.
func readLog(metaPath string) ([]json.RawMessage, error) {
	logPath := filepath.Join(metaPath, "log.json")
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}

	if trimmed := bytes.TrimLeft(data, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
		var entries []json.RawMessage
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
		return entries, nil
	}

	var entries []json.RawMessage
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		entries = append(entries, append([]byte(nil), line...))
	}
	return entries, nil
}

// prefixNameLabels applies "<source>/" prefix to name-create and name-update labels.
func prefixNameLabels(entries []json.RawMessage, source string) ([]json.RawMessage, error) {
	type envelope struct {
		Op      string          `json:"op"`
		Created time.Time       `json:"created"`
		Payload json.RawMessage `json:"payload"`
	}
	type namePayload struct {
		Label string `json:"label"`
		Hash  string `json:"hash"`
	}

	result := make([]json.RawMessage, len(entries))

	for i, raw := range entries {
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, err
		}

		if env.Op == "name-create" || env.Op == "name-update" {
			var p namePayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return nil, err
			}
			p.Label = source + "/" + p.Label
			newPayload, err := json.Marshal(p)
			if err != nil {
				return nil, err
			}
			env.Payload = newPayload
		}

		modified, err := json.Marshal(env)
		if err != nil {
			return nil, err
		}
		result[i] = modified
	}

	return result, nil
}

// mergeLog appends entries to the destination log file as NDJSON lines —
// a real append (O_APPEND), not a read-modify-rewrite of the whole file,
// for the same reason Store.appendLine avoids it.
//
// If the destination log doesn't exist yet, it's created fresh. If it
// exists but is still in the legacy single-JSON-array format (a store
// that predates the NDJSON migration and has never been opened via
// metadata.Store.load()), it's migrated to NDJSON first so the append
// below lands on a consistent file rather than corrupting it by mixing
// formats.
func mergeLog(metaPath string, entries []json.RawMessage) error {
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(metaPath, "log.json")

	if data, err := os.ReadFile(logPath); err == nil {
		if trimmed := bytes.TrimLeft(data, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
			var existing []json.RawMessage
			if err := json.Unmarshal(data, &existing); err != nil {
				return fmt.Errorf("parsing legacy log for migration: %w", err)
			}
			var buf bytes.Buffer
			for _, raw := range existing {
				var compact bytes.Buffer
				if err := json.Compact(&compact, raw); err != nil {
					return fmt.Errorf("compacting entry during legacy migration: %w", err)
				}
				buf.Write(compact.Bytes())
				buf.WriteByte('\n')
			}
			if err := os.WriteFile(logPath, buf.Bytes(), 0644); err != nil {
				return fmt.Errorf("migrating legacy log before merge: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, raw := range entries {
		// Entries sourced from an import archive may still be
		// pretty-printed (Export writes the archive's log with
		// MarshalIndent for readability), which would embed literal
		// newlines and break the one-line-per-entry NDJSON invariant.
		// Compact defensively regardless of source.
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return fmt.Errorf("compacting log entry for merge: %w", err)
		}
		if _, err := f.Write(compact.Bytes()); err != nil {
			return err
		}
		if _, err := f.Write([]byte("\n")); err != nil {
			return err
		}
	}
	return nil
}

// --- Tar helpers ---

func writeFile(tw *tar.Writer, name, path string, info os.FileInfo) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(tw, f)
	return err
}

func writeBytes(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
