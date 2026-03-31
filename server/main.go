package main

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
	"github.com/steveknoblock/hatcheck-go/internal/share"
)

func stashHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	content := string(body)

	hash, err := store.Stash(content)
	if err != nil {
		http.Error(w, "failed to stash content", http.StatusInternalServerError)
		return
	}

	if err := meta.AppendStash(hash, len(body), content); err != nil {
		log.Printf("warning: failed to append metadata for %s: %v", hash, err)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

func fetchHandler(w http.ResponseWriter, req *http.Request, store *cas.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := req.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "missing hash parameter", http.StatusBadRequest)
		return
	}

	data, err := store.Fetch(hash)
	if err != nil {
		http.Error(w, "content not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", data)
}

func listHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hashes, err := store.List()

	if err != nil {
		http.Error(w, "failed to list objects", http.StatusInternalServerError)
		return
	}

	if hashes == nil {
		hashes = []string{}
	}

	type hashWithTags struct {
		Hash string   `json:"hash"`
		Tags []string `json:"tags"`
	}

	result := make([]hashWithTags, len(hashes))
	for i, hash := range hashes {
		result[i] = hashWithTags{
			Hash: hash,
			Tags: meta.TagsForHash(hash),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func queryHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	indexName := req.URL.Query().Get("index")
	key := req.URL.Query().Get("key")

	if indexName == "" || key == "" {
		http.Error(w, "missing index or key parameter", http.StatusBadRequest)
		return
	}

	hashes := meta.Query(indexName, key)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hashes)
}

// namespacesHandler returns all unique namespace prefixes in the name index.
// GET /namespaces
func namespacesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespaces := meta.Namespaces()
	if namespaces == nil {
		namespaces = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(namespaces)
}

// namesHandler returns all Names in a namespace with the prefix stripped.
// GET /names?namespace=bob
func namesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := req.URL.Query().Get("namespace")
	if namespace == "" {
		http.Error(w, "missing namespace parameter", http.StatusBadRequest)
		return
	}

	names := meta.NamesInNamespace(namespace)
	if names == nil {
		names = []metadata.NameEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(names)
}

// nameHandler creates or updates a Name in the metadata store.
// POST /name?label=my-document&hash=a1b2c3&namespace=bob
func nameHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := req.URL.Query().Get("namespace")
	label := req.URL.Query().Get("label")
	hash := req.URL.Query().Get("hash")

	if label == "" || hash == "" {
		http.Error(w, "missing label or hash parameter", http.StatusBadRequest)
		return
	}

	// Prepend namespace if provided.
	fullLabel := label
	if namespace != "" {
		fullLabel = namespace + "/" + label
	}

	// Try create first, fall back to update.
	err := meta.AppendNameCreate(fullLabel, hash)
	if err != nil {
		// Name already exists — update it.
		if err := meta.AppendNameUpdate(fullLabel, hash); err != nil {
			http.Error(w, "failed to update name: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "%s\n", fullLabel)
}

// collectionHandler stashes a Collection object and returns its hash.
// POST /collection — body is a JSON array of hashes
func collectionHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Validate that the body is a JSON array of strings.
	var hashes []string
	if err := json.Unmarshal(body, &hashes); err != nil {
		http.Error(w, "body must be a JSON array of hashes", http.StatusBadRequest)
		return
	}

	content := string(body)
	hash, err := store.Stash(content)
	if err != nil {
		http.Error(w, "failed to stash collection", http.StatusInternalServerError)
		return
	}

	if err := meta.AppendCollection(hash, hashes); err != nil {
		log.Printf("warning: failed to append collection metadata for %s: %v", hash, err)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

// relationHandler stashes a Relation object and logs it to the metadata store.
// POST /relation?from=<hash>&rel=<predicate>&to=<hash>
//
// The relation JSON is stored as a CAS object (immutable, addressable by hash)
// and also appended to the metadata log for indexing. Both from and to must be
// non-empty; rel is the predicate drawn from the tag vocabulary.
func relationHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	from := req.URL.Query().Get("from")
	rel := req.URL.Query().Get("rel")
	to := req.URL.Query().Get("to")

	if from == "" || rel == "" || to == "" {
		http.Error(w, "missing from, rel, or to parameter", http.StatusBadRequest)
		return
	}

	// Build the relation JSON — this is the CAS object content.
	type relationObject struct {
		From string `json:"from"`
		Rel  string `json:"rel"`
		To   string `json:"to"`
	}
	content, err := json.Marshal(relationObject{From: from, Rel: rel, To: to})
	if err != nil {
		http.Error(w, "failed to marshal relation", http.StatusInternalServerError)
		return
	}

	// Stash as a CAS object — the relation is immutable content like any other.
	hash, err := store.Stash(string(content))
	if err != nil {
		http.Error(w, "failed to stash relation", http.StatusInternalServerError)
		return
	}

	// Log to metadata store for indexing by from, to, and rel.
	if err := meta.AppendRelation(hash, from, rel, to); err != nil {
		log.Printf("warning: failed to append relation metadata for %s: %v", hash, err)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

// relationsHandler returns all outgoing and incoming relations for a given hash.
// GET /relations?hash=<hash>
//
// Response shape:
//
//	{
//	  "outgoing": [{"hash":"...","from":"...","rel":"...","to":"..."}, ...],
//	  "incoming": [{"hash":"...","from":"...","rel":"...","to":"..."}, ...]
//	}
//
// Both arrays are always present, empty when no relations exist.
// This endpoint returns structured relation data rather than hashes alone,
// so the UI can display and navigate the syndetic web without additional fetches.
func relationsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := req.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "missing hash parameter", http.StatusBadRequest)
		return
	}

	outgoing, incoming := meta.RelationsForHash(hash)

	// Ensure non-nil slices so the JSON encodes as [] not null.
	if outgoing == nil {
		outgoing = []metadata.RelationPayload{}
	}
	if incoming == nil {
		incoming = []metadata.RelationPayload{}
	}

	type relationsResponse struct {
		Outgoing []metadata.RelationPayload `json:"outgoing"`
		Incoming []metadata.RelationPayload `json:"incoming"`
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(relationsResponse{
		Outgoing: outgoing,
		Incoming: incoming,
	})
}

// tagsHandler returns all known tag keys from the tag index.
// GET /tags
//
// Used by the UI to populate the relation type autocomplete picker.
// The tag vocabulary is derived entirely from #hashtags in stashed content,
// so it grows organically as the user creates documents.
func tagsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tags := meta.AllTags()
	if tags == nil {
		tags = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

// exportHandler streams a tar.gz archive to the client.
// GET /export?source=bob
// GET /export?source=bob&name=my-document
func exportHandler(w http.ResponseWriter, req *http.Request, objPath, metaPath string) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	source := req.URL.Query().Get("source")
	name := req.URL.Query().Get("name")

	if source == "" {
		http.Error(w, "missing source parameter", http.StatusBadRequest)
		return
	}

	// Write archive to a temp file then stream it.
	tmp, err := os.CreateTemp("", "hatcheck-export-*.tar.gz")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := share.Export(objPath, metaPath, source, name, tmp.Name()); err != nil {
		http.Error(w, "export failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	filename := source + ".tar.gz"
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	http.ServeFile(w, req, tmp.Name())
}

// importHandler accepts a tar.gz archive as the request body and imports it.
// POST /import
func importHandler(w http.ResponseWriter, req *http.Request, objPath, metaPath string) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Write body to a temp file.
	tmp, err := os.CreateTemp("", "hatcheck-import-*.tar.gz")
	if err != nil {
		http.Error(w, "failed to create temp file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, req.Body); err != nil {
		tmp.Close()
		http.Error(w, "failed to read upload", http.StatusBadRequest)
		return
	}
	tmp.Close()

	if err := share.Import(tmp.Name(), objPath, metaPath); err != nil {
		http.Error(w, "import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "import successful")
}

func main() {
	objPath := os.Getenv("HATCHECK_DATA")
	if objPath == "" {
		objPath = "./objects"
	}

	uiPath := os.Getenv("HATCHECK_UI")
	if uiPath == "" {
		uiPath = "./ui"
	}

	metaPath := os.Getenv("HATCHECK_META")
	if metaPath == "" {
		metaPath = "./metadata"
	}

	// Create the CAS store.
	store, err := cas.New(objPath,
		func(content string) string {
			sum := md5.Sum([]byte(content))
			return hex.EncodeToString(sum[:])
		},
	)
	if err != nil {
		log.Fatalf("failed to create CAS store: %v", err)
	}

	// Create the metadata store.
	meta, err := metadata.New(metaPath,
		&metadata.TagIndex{},
		&metadata.DateIndex{},
		&metadata.NameIndex{},
		&metadata.RelationIndex{},
	)
	if err != nil {
		log.Fatalf("failed to load metadata store: %v", err)
	}

	http.HandleFunc("/stash", func(w http.ResponseWriter, req *http.Request) {
		stashHandler(w, req, store, meta)
	})
	http.HandleFunc("/fetch", func(w http.ResponseWriter, req *http.Request) {
		fetchHandler(w, req, store)
	})
	http.HandleFunc("/list", func(w http.ResponseWriter, req *http.Request) {
		listHandler(w, req, store, meta)
	})
	http.HandleFunc("/query", func(w http.ResponseWriter, req *http.Request) {
		queryHandler(w, req, meta)
	})
	http.HandleFunc("/namespaces", func(w http.ResponseWriter, req *http.Request) {
		namespacesHandler(w, req, meta)
	})
	http.HandleFunc("/names", func(w http.ResponseWriter, req *http.Request) {
		namesHandler(w, req, meta)
	})
	http.HandleFunc("/name", func(w http.ResponseWriter, req *http.Request) {
		nameHandler(w, req, meta)
	})
	http.HandleFunc("/collection", func(w http.ResponseWriter, req *http.Request) {
		collectionHandler(w, req, store, meta)
	})
	http.HandleFunc("/relation", func(w http.ResponseWriter, req *http.Request) {
		relationHandler(w, req, store, meta)
	})
	http.HandleFunc("/relations", func(w http.ResponseWriter, req *http.Request) {
		relationsHandler(w, req, meta)
	})
	http.HandleFunc("/tags", func(w http.ResponseWriter, req *http.Request) {
		tagsHandler(w, req, meta)
	})
	http.HandleFunc("/export", func(w http.ResponseWriter, req *http.Request) {
		exportHandler(w, req, objPath, metaPath)
	})
	http.HandleFunc("/import", func(w http.ResponseWriter, req *http.Request) {
		importHandler(w, req, objPath, metaPath)
	})

	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(uiPath))))

	log.Println("starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatal(err)
	}
}
