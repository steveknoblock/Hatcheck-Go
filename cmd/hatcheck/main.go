package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
	"github.com/steveknoblock/hatcheck-go/internal/share"
)

const usage = `Hatcheck - Content Addressable Store

Usage:
  hatcheck <command> [options]

Commands:
  stash       Store content in the CAS
  fetch       Retrieve content by hash
  list        List all objects in the store
  query       Query objects by index and key
  export      Export objects and metadata to a shareable archive
  import      Import objects and metadata from an archive
  capability  Manage capabilities (issue, revoke, list)

Options:
  -data     Path to objects directory (default: ./objects)
  -meta     Path to metadata directory (default: ./metadata)

Run 'hatcheck <command> -help' for command-specific options.
`

// newStore creates a CAS store with the placeholder MD5 hash function.
// Replace with the production hash function when the dev branch is merged.
func newStore(objPath string) (*cas.Store, error) {
	return cas.New(objPath, func(content string) string {
		sum := md5.Sum([]byte(content))
		return hex.EncodeToString(sum[:])
	})
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	objPath := os.Getenv("HATCHECK_DATA")
	if objPath == "" {
		objPath = "./objects"
	}
	metaPath := os.Getenv("HATCHECK_META")
	if metaPath == "" {
		metaPath = "./metadata"
	}

	switch os.Args[1] {
	case "stash":
		runStash(os.Args[2:], objPath, metaPath)
	case "fetch":
		runFetch(os.Args[2:], objPath)
	case "list":
		runList(os.Args[2:], objPath, metaPath)
	case "query":
		runQuery(os.Args[2:], metaPath)
	case "export":
		runExport(os.Args[2:], objPath, metaPath)
	case "import":
		runImport(os.Args[2:], objPath, metaPath)
	case "capability":
		runCapability(os.Args[2:], metaPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
}

// --- stash ---

func runStash(args []string, objPath, metaPath string) {
	fs := flag.NewFlagSet("stash", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck stash <content>")
		fmt.Fprintln(os.Stderr, "       echo 'content' | hatcheck stash")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	var content string
	if fs.NArg() > 0 {
		content = fs.Arg(0)
	} else {
		buf, err := os.ReadFile("/dev/stdin")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading stdin: %v\n", err)
			os.Exit(1)
		}
		content = string(buf)
	}

	if content == "" {
		fmt.Fprintln(os.Stderr, "error: no content provided")
		os.Exit(1)
	}

	store, err := newStore(objPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open object store: %v\n", err)
		os.Exit(1)
	}

	hash, err := store.Stash(content)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load metadata store: %v\n", err)
	} else {
		meta.AppendStash(hash, len(content), content)
	}

	fmt.Println(hash)
}

// --- fetch ---

func runFetch(args []string, objPath string) {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck fetch <hash>")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	store, err := newStore(objPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open object store: %v\n", err)
		os.Exit(1)
	}

	data, err := store.Fetch(fs.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(data)
}

// --- list ---

func runList(args []string, objPath, metaPath string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck list [-json]")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	store, err := newStore(objPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not open object store: %v\n", err)
		os.Exit(1)
	}

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load metadata store: %v\n", err)
	}

	hashes, err := store.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	type hashWithTags struct {
		Hash string   `json:"hash"`
		Tags []string `json:"tags"`
	}

	results := make([]hashWithTags, len(hashes))
	for i, hash := range hashes {
		var tags []string
		if meta != nil {
			tags = meta.TagsForHash(hash)
		}
		if tags == nil {
			tags = []string{}
		}
		results[i] = hashWithTags{Hash: hash, Tags: tags}
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	if len(results) == 0 {
		fmt.Println("No objects in store.")
		return
	}
	for _, r := range results {
		if len(r.Tags) > 0 {
			fmt.Printf("%s  %v\n", r.Hash, r.Tags)
		} else {
			fmt.Println(r.Hash)
		}
	}
}

// --- query ---

func runQuery(args []string, metaPath string) {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	indexName := fs.String("index", "tag", "Index to query (tag, date)")
	key := fs.String("key", "", "Key to look up in the index")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck query -index <name> -key <value> [-json]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck query -index tag -key ideas")
		fmt.Fprintln(os.Stderr, "  hatcheck query -index date -key 2026-03-14")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *key == "" {
		fs.Usage()
		os.Exit(1)
	}

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not load metadata store: %v\n", err)
		os.Exit(1)
	}

	hashes := meta.Query(*indexName, *key)

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(hashes)
		return
	}

	if len(hashes) == 0 {
		fmt.Printf("No objects found for %s=%s\n", *indexName, *key)
		return
	}
	for _, h := range hashes {
		fmt.Println(h)
	}
}

// --- export ---

func runExport(args []string, objPath, metaPath string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	source := fs.String("source", "", "Source identifier (required)")
	name := fs.String("name", "", "Export only objects reachable from this name (optional)")
	outFile := fs.String("o", "", "Output file (default: <source>.tar.gz)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck export -source <name> [-o <file>]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck export -source bob")
		fmt.Fprintln(os.Stderr, "  hatcheck export -source bob -name my-document")
		fmt.Fprintln(os.Stderr, "  hatcheck export -source bob -o my-export.tar.gz")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *source == "" {
		fmt.Fprintln(os.Stderr, "error: -source is required")
		fs.Usage()
		os.Exit(1)
	}

	if err := share.Export(objPath, metaPath, *source, *name, *outFile); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	outPath := *outFile
	if outPath == "" {
		outPath = *source + ".tar.gz"
	}
	fmt.Printf("exported to %s\n", outPath)
}

// --- import ---

func runImport(args []string, objPath, metaPath string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck import <archive>")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck import bob.tar.gz")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if fs.NArg() < 1 {
		fs.Usage()
		os.Exit(1)
	}

	if err := share.Import(fs.Arg(0), objPath, metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("imported from %s\n", fs.Arg(0))
}

// --- capability ---

const capabilityUsage = `Usage: hatcheck capability <subcommand> [options]

Subcommands:
  issue   Issue a new signed capability for a principal
  revoke  Revoke a capability by ID
  list    List capabilities recorded in the log

Requires HATCHECK_SIGNING_KEY to be set in the environment.
`

func runCapability(args []string, metaPath string) {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, capabilityUsage)
		os.Exit(1)
	}

	signingKey := []byte(os.Getenv("HATCHECK_SIGNING_KEY"))
	if len(signingKey) == 0 {
		fmt.Fprintln(os.Stderr, "error: HATCHECK_SIGNING_KEY environment variable must be set")
		os.Exit(1)
	}

	switch args[0] {
	case "issue":
		runCapabilityIssue(args[1:], metaPath, signingKey)
	case "revoke":
		runCapabilityRevoke(args[1:], metaPath, signingKey)
	case "list":
		runCapabilityList(args[1:], metaPath)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", args[0])
		fmt.Fprint(os.Stderr, capabilityUsage)
		os.Exit(1)
	}
}

// --- capability issue ---

func runCapabilityIssue(args []string, metaPath string, signingKey []byte) {
	fs := flag.NewFlagSet("capability issue", flag.ExitOnError)
	hash := fs.String("hash", "", "Object hash to grant access to (required)")
	perm := fs.String("perm", "", "Permission to grant: read or write (required)")
	principal := fs.String("principal", "", "User ID to grant the capability to (required)")
	ttl := fs.Duration("ttl", 24*time.Hour, "How long the capability is valid (e.g. 24h, 7*24h)")
	jsonOut := fs.Bool("json", false, "Output the full capability payload as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck capability issue -hash <hash> -perm <read|write> -principal <user-id> [-ttl <duration>] [-json]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck capability issue -hash abc123 -perm read -principal alice")
		fmt.Fprintln(os.Stderr, "  hatcheck capability issue -hash abc123 -perm write -principal bob -ttl 168h")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *hash == "" || *perm == "" || *principal == "" {
		fmt.Fprintln(os.Stderr, "error: -hash, -perm, and -principal are required")
		fs.Usage()
		os.Exit(1)
	}
	if *perm != "read" && *perm != "write" {
		fmt.Fprintln(os.Stderr, "error: -perm must be read or write")
		fs.Usage()
		os.Exit(1)
	}

	expires := time.Now().UTC().Add(*ttl)
	cap := metadata.SignCapability(signingKey, *hash, *perm, *principal, expires)

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not load metadata store: %v\n", err)
		os.Exit(1)
	}
	if err := meta.AppendCapability(cap); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not record capability: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(cap)
		return
	}

	fmt.Printf("capability issued\n")
	fmt.Printf("  id:        %s\n", cap.ID)
	fmt.Printf("  hash:      %s\n", cap.Hash)
	fmt.Printf("  perm:      %s\n", cap.Perm)
	fmt.Printf("  principal: %s\n", cap.Principal)
	fmt.Printf("  expires:   %s\n", cap.Expires.Format(time.RFC3339))
}

// --- capability revoke ---

func runCapabilityRevoke(args []string, metaPath string, signingKey []byte) {
	fs := flag.NewFlagSet("capability revoke", flag.ExitOnError)
	id := fs.String("id", "", "Capability ID to revoke (required)")
	reason := fs.String("reason", "", "Reason for revocation (optional)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck capability revoke -id <capability-id> [-reason <text>]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck capability revoke -id abc123")
		fmt.Fprintln(os.Stderr, "  hatcheck capability revoke -id abc123 -reason \"user offboarded\"")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *id == "" {
		fmt.Fprintln(os.Stderr, "error: -id is required")
		fs.Usage()
		os.Exit(1)
	}

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not load metadata store: %v\n", err)
		os.Exit(1)
	}

	// The CLI operates directly on the store so no live RevokedSet is needed —
	// pass a fresh one that is discarded after the call.
	if err := meta.AppendCapabilityRevoke(*id, *reason, metadata.NewRevokedSet()); err != nil {
		fmt.Fprintf(os.Stderr, "error: could not record revocation: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("capability %s revoked\n", *id)
	if *reason != "" {
		fmt.Printf("  reason: %s\n", *reason)
	}
}

// --- capability list ---

func runCapabilityList(args []string, metaPath string) {
	fs := flag.NewFlagSet("capability list", flag.ExitOnError)
	principal := fs.String("principal", "", "Filter by principal (optional)")
	hash := fs.String("hash", "", "Filter by object hash (optional)")
	jsonOut := fs.Bool("json", false, "Output as JSON")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck capability list [-principal <user-id>] [-hash <hash>] [-json]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck capability list")
		fmt.Fprintln(os.Stderr, "  hatcheck capability list -principal alice")
		fmt.Fprintln(os.Stderr, "  hatcheck capability list -hash abc123")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not load metadata store: %v\n", err)
		os.Exit(1)
	}

	// Build revoked set for status annotation.
	revoked := metadata.NewRevokedSet()
	meta.BuildRevokedSet(revoked)

	type capEntry struct {
		ID        string `json:"id"`
		Hash      string `json:"hash"`
		Perm      string `json:"perm"`
		Principal string `json:"principal,omitempty"`
		Expires   string `json:"expires"`
		Status    string `json:"status"`
	}

	var entries []capEntry
	now := time.Now().UTC()

	for _, entry := range meta.Log {
		if entry.Op != metadata.OpCapability {
			continue
		}
		var cap metadata.CapabilityPayload
		if err := json.Unmarshal(entry.Payload, &cap); err != nil {
			continue
		}

		// Apply filters.
		if *principal != "" && cap.Principal != *principal {
			continue
		}
		if *hash != "" && cap.Hash != *hash {
			continue
		}

		status := "active"
		if revoked.IsRevoked(cap.ID) {
			status = "revoked"
		} else if cap.Expires.UTC().Before(now) {
			status = "expired"
		}

		entries = append(entries, capEntry{
			ID:        cap.ID,
			Hash:      cap.Hash,
			Perm:      cap.Perm,
			Principal: cap.Principal,
			Expires:   cap.Expires.Format(time.RFC3339),
			Status:    status,
		})
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(entries)
		return
	}

	if len(entries) == 0 {
		fmt.Println("No capabilities found.")
		return
	}

	fmt.Printf("%-12s  %-8s  %-12s  %-20s  %-8s  %s\n",
		"ID (prefix)", "PERM", "PRINCIPAL", "EXPIRES", "STATUS", "HASH")
	fmt.Println(fmt.Sprintf("%s", "────────────────────────────────────────────────────────────────────────────────"))
	for _, e := range entries {
		idPrefix := e.ID
		if len(idPrefix) > 12 {
			idPrefix = idPrefix[:12]
		}
		fmt.Printf("%-12s  %-8s  %-12s  %-20s  %-8s  %s\n",
			idPrefix, e.Perm, e.Principal, e.Expires, e.Status, e.Hash)
	}
}
