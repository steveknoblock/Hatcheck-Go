package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
	"github.com/steveknoblock/hatcheck-go/internal/share"
)

const usage = `Hatcheck - Content Addressable Store

Usage:
  hatcheck <command> [options]

Commands:
  stash     Store content in the CAS
  fetch     Retrieve content by hash
  list      List all objects in the store
  query     Query objects by index and key
  export    Export objects and metadata to a shareable archive
  import    Import objects and metadata from an archive

Options:
  -data     Path to objects directory (default: ./objects)
  -meta     Path to metadata directory (default: ./metadata)

Run 'hatcheck <command> -help' for command-specific options.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}

	// Global path flags — must come before the subcommand.
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
		// Content passed as argument.
		content = fs.Arg(0)
	} else {
		// Read from stdin.
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

	hash, err := cas.Stash(content, objPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Append to metadata log.
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

	hash := fs.Arg(0)
	data, err := cas.Fetch(hash, objPath)
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

	// Load metadata for tag annotation.
	meta, err := metadata.New(metaPath, &metadata.TagIndex{}, &metadata.DateIndex{}, &metadata.NameIndex{}, &metadata.RelationIndex{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load metadata store: %v\n", err)
	}

	type hashWithTags struct {
		Hash string   `json:"hash"`
		Tags []string `json:"tags"`
	}

	var results []hashWithTags

	err = filepath.Walk(objPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			shard := filepath.Base(filepath.Dir(path))
			file := filepath.Base(path)
			hash := shard + file
			var tags []string
			if meta != nil {
				tags = meta.TagsForHash(hash)
			}
			if tags == nil {
				tags = []string{}
			}
			results = append(results, hashWithTags{Hash: hash, Tags: tags})
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(results)
		return
	}

	// Plain text output.
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
	outFile := fs.String("o", "", "Output file (default: <source>.tar.gz)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: hatcheck export -source <name> [-o <file>]")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  hatcheck export -source bob")
		fmt.Fprintln(os.Stderr, "  hatcheck export -source bob -o my-export.tar.gz")
		fs.PrintDefaults()
	}
	fs.Parse(args)

	if *source == "" {
		fmt.Fprintln(os.Stderr, "error: -source is required")
		fs.Usage()
		os.Exit(1)
	}

	if err := share.Export(objPath, metaPath, *source, *outFile); err != nil {
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

	archivePath := fs.Arg(0)

	if err := share.Import(archivePath, objPath, metaPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("imported from %s\n", archivePath)
}
