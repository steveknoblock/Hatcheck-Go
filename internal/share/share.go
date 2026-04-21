package share

import (
	"archive/tar"
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
	Source   string    `json:"source"`
	Exported time.Time `json:"exported"`
	Version  string    `json:"version"`
	Objects  int       `json:"objects"`
	Name     string    `json:"name,omitempty"` // set for partial exports
}

const manifestVersion = "1"

// Export bundles the CAS objects and metadata log into a tar.gz archive.
// If name is non-empty only the objects reachable from that name are exported.
// The output file is named <source>.tar.gz unless outPath is specified.
func Export(objPath, metaPath, source, name, outPath string) error {
	if outPath == "" {
		outPath = source + ".tar.gz"
	}

	// Determine which hashes to export.
	var hashes map[string]bool
	if name != "" {
		var err error
		hashes, err = reachableHashes(name, objPath, metaPath)
		if err != nil {
			return fmt.Errorf("resolving name %q: %w", name, err)
		}
		if len(hashes) == 0 {
			return fmt.Errorf("name %q not found or has no reachable objects", name)
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
			archivePath: filepath.Join("objects", rel),
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
		Source:   source,
		Exported: time.Now().UTC(),
		Version:  manifestVersion,
		Objects:  len(objects),
		Name:     name,
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
		logEntries = filterLog(logEntries, hashes, name)
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

// filterLog returns only log entries relevant to the given hash set and name.
func filterLog(entries []json.RawMessage, hashes map[string]bool, name string) []json.RawMessage {
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
			if p.Label == name {
				result = append(result, raw)
			}
		}
	}

	return result
}

// --- Log helpers ---

func readLog(metaPath string) ([]json.RawMessage, error) {
	logPath := filepath.Join(metaPath, "log.json")
	data, err := os.ReadFile(logPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
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

// mergeLog appends entries to the destination log file.
func mergeLog(metaPath string, entries []json.RawMessage) error {
	if err := os.MkdirAll(metaPath, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(metaPath, "log.json")

	var existing []json.RawMessage
	data, err := os.ReadFile(logPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(data, &existing); err != nil {
			return err
		}
	}

	merged := append(existing, entries...)

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(logPath, out, 0644)
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
