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
}

const manifestVersion = "1"

// Export bundles the CAS objects and metadata log into a tar.gz archive.
// The output file is named <source>.tar.gz unless outPath is specified.
func Export(objPath, metaPath, source, outPath string) error {
	if outPath == "" {
		outPath = source + ".tar.gz"
	}

	// Count objects for the manifest.
	objectCount := 0
	err := filepath.Walk(objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			objectCount++
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("counting objects: %w", err)
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

	// Write manifest.
	manifest := Manifest{
		Source:   source,
		Exported: time.Now().UTC(),
		Version:  manifestVersion,
		Objects:  objectCount,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding manifest: %w", err)
	}
	if err := writeBytes(tw, "manifest.json", manifestData); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	// Write CAS objects preserving shard structure.
	err = filepath.Walk(objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Preserve relative path within the archive as objects/<shard>/<file>
		rel, err := filepath.Rel(objPath, path)
		if err != nil {
			return err
		}
		return writeFile(tw, filepath.Join("objects", rel), path, info)
	})
	if err != nil {
		return fmt.Errorf("writing objects: %w", err)
	}

	// Write metadata log.
	logPath := filepath.Join(metaPath, "log.json")
	logInfo, err := os.Stat(logPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("checking log: %w", err)
	}
	if err == nil {
		if err := writeFile(tw, "metadata/log.json", logPath, logInfo); err != nil {
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

	// First pass — read manifest and log, write objects.
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
			// CAS object — write only if it doesn't already exist.
			if !strings.HasPrefix(hdr.Name, "objects/") {
				continue
			}
			rel := strings.TrimPrefix(hdr.Name, "objects/")
			destPath := filepath.Join(objPath, rel)

			if _, err := os.Stat(destPath); err == nil {
				// Already exists — skip.
				continue
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

	// Apply namespace prefix to name labels and merge into destination log.
	prefixed, err := prefixNameLabels(logEntries, manifest.Source)
	if err != nil {
		return fmt.Errorf("prefixing name labels: %w", err)
	}

	if err := mergeLog(metaPath, prefixed); err != nil {
		return fmt.Errorf("merging log: %w", err)
	}

	return nil
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

	// Load existing log.
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

	// Append new entries.
	merged := append(existing, entries...)

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(logPath, out, 0644)
}

// writeFile adds a file from disk to the tar archive.
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

// writeBytes adds raw bytes as a file in the tar archive.
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
