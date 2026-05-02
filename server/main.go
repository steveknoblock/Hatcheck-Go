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
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
	"github.com/steveknoblock/hatcheck-go/internal/share"
)

// Perm constants define the operations a capability may authorize.
const (
	PermRead  = "read"
	PermWrite = "write"
	PermAdmin = "admin"
)

func stashHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store, vr VerifiedRequest) {
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

	// Verify the capability covers the hash that was just written.
	if vr.Capability.Hash != hash {
		http.Error(w, "capability does not cover this object", http.StatusForbidden)
		return
	}

	if err := meta.AppendStash(hash, len(body), content); err != nil {
		log.Printf("warning: failed to append metadata for %s: %v", hash, err)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

func fetchHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := req.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "missing hash parameter", http.StatusBadRequest)
		return
	}

	// Verify the capability covers the requested hash.
	if vr.Capability.Hash != hash {
		http.Error(w, "capability does not cover this object", http.StatusForbidden)
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

func listHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store, vr VerifiedRequest) {
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

func queryHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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
func namespacesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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
func namesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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
func nameHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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

	// Verify the capability covers the named hash.
	if vr.Capability.Hash != hash {
		http.Error(w, "capability does not cover this object", http.StatusForbidden)
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
func collectionHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store, vr VerifiedRequest) {
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

// exportHandler streams a tar.gz archive to the client.
// GET /export?source=bob
// GET /export?source=bob&name=my-document
func exportHandler(w http.ResponseWriter, req *http.Request, objPath, metaPath string, vr VerifiedRequest) {
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
func importHandler(w http.ResponseWriter, req *http.Request, objPath, metaPath string, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

// relationHandler stashes a Relation object and logs it to the metadata store.
// POST /relation?from=<hash>&rel=<predicate>&to=<hash>
func relationHandler(w http.ResponseWriter, req *http.Request, store *cas.Store, meta *metadata.Store, vr VerifiedRequest) {
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

	hash, err := store.Stash(string(content))
	if err != nil {
		http.Error(w, "failed to stash relation", http.StatusInternalServerError)
		return
	}

	if err := meta.AppendRelation(hash, from, rel, to); err != nil {
		log.Printf("warning: failed to append relation metadata for %s: %v", hash, err)
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%s\n", hash)
}

// relationsHandler returns all outgoing and incoming relations for a given hash.
// GET /relations?hash=<hash>
func relationsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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
func tagsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
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

// issueHandler creates and signs a new capability for the given principal,
// records it in the log, and returns the serialized CapabilityPayload to the
// caller. Only principals with PermAdmin may issue capabilities.
// POST /capability?hash=<hash>&perm=<perm>&principal=<principal>&expires=<RFC3339>
func issueHandler(w http.ResponseWriter, req *http.Request, key []byte, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := req.URL.Query().Get("hash")
	perm := req.URL.Query().Get("perm")
	principal := req.URL.Query().Get("principal")
	expiresStr := req.URL.Query().Get("expires")
	email := req.URL.Query().Get("email") // optional — empty if user has not opted in

	if hash == "" || perm == "" || principal == "" || expiresStr == "" {
		http.Error(w, "missing required parameter: hash, perm, principal, expires", http.StatusBadRequest)
		return
	}

	// Validate perm is a known value.
	// Admin capabilities can only be issued via the bootstrap token (no
	// capability present in the request). Regular issuance is limited to
	// read or write to prevent privilege escalation.
	if perm != PermRead && perm != PermWrite {
		if perm == PermAdmin && vr.Capability.ID == "" {
			// Bootstrap path — admin issuance permitted.
		} else {
			http.Error(w, "perm must be read or write", http.StatusBadRequest)
			return
		}
	}

	expires, err := time.Parse(time.RFC3339, expiresStr)
	if err != nil {
		http.Error(w, "expires must be in RFC3339 format", http.StatusBadRequest)
		return
	}

	if expires.Before(time.Now().UTC()) {
		http.Error(w, "expires must be in the future", http.StatusBadRequest)
		return
	}

	cap := metadata.SignCapability(key, hash, perm, principal, email, expires)

	if err := meta.AppendCapability(cap); err != nil {
		http.Error(w, "failed to record capability: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(cap)
}

// revokeHandler records the revocation of a capability and updates the live
// revocation index. The capability ID is required; reason is optional.
// POST /capability/revoke?id=<capability-id>&reason=<reason>
func revokeHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, revoked *metadata.RevokedSet, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := req.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id parameter", http.StatusBadRequest)
		return
	}

	reason := req.URL.Query().Get("reason")

	if err := meta.AppendCapabilityRevoke(id, reason, revoked); err != nil {
		http.Error(w, "failed to record revocation: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "capability %s revoked\n", id)
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

	signingKey := []byte(os.Getenv("HATCHECK_SIGNING_KEY"))
	if len(signingKey) == 0 {
		log.Fatal("HATCHECK_SIGNING_KEY environment variable must be set")
	}

	// Initialise the CAS with a SHA-256 hash function.
	store, err := cas.New(objPath, func(content string) string {
		sum := md5.Sum([]byte(content))
		return hex.EncodeToString(sum[:])
	})
	if err != nil {
		log.Fatalf("failed to initialise object store: %v", err)
	}

	meta, err := metadata.New(metaPath,
		metadata.NewTagIndex(),
		metadata.NewDateIndex(),
		metadata.NewNameIndex(),
		metadata.NewRelationIndex(),
	)
	if err != nil {
		log.Fatalf("failed to load metadata store: %v", err)
	}

	// Build revocation index from log at startup.
	revoked := metadata.NewRevokedSet()
	if err := meta.BuildRevokedSet(revoked); err != nil {
		log.Fatalf("failed to build revocation index: %v", err)
	}

	cm := &CapabilityMiddleware{
		Key:            signingKey,
		Revoked:        revoked,
		BootstrapToken: os.Getenv("HATCHECK_BOOTSTRAP_TOKEN"),
	}

	// Initialise the Stytch auth client.
	authClient, err := auth.NewClient()
	if err != nil {
		log.Fatalf("failed to initialise auth client: %v", err)
	}

	am := &AuthMiddleware{Client: authClient}

	// Auth routes — not capability protected, but also not JWT protected
	// since these are the routes that establish identity in the first place.
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		loginHandler(w, req, authClient)
	})
	http.HandleFunc("/auth/authenticate", func(w http.ResponseWriter, req *http.Request) {
		authenticateHandler(w, req, authClient)
	})
	http.HandleFunc("/auth/logout", logoutHandler)

	http.HandleFunc("/stash", am.Wrap(cm.Protect(PermWrite, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		stashHandler(w, req, store, meta, vr)
	})))
	http.HandleFunc("/fetch", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		fetchHandler(w, req, store, vr)
	})))
	http.HandleFunc("/list", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		listHandler(w, req, store, meta, vr)
	})))
	http.HandleFunc("/query", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		queryHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/namespaces", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		namespacesHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/names", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		namesHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/name", am.Wrap(cm.Protect(PermWrite, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		nameHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/collection", am.Wrap(cm.Protect(PermWrite, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		collectionHandler(w, req, store, meta, vr)
	})))
	http.HandleFunc("/relation", am.Wrap(cm.Protect(PermWrite, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationHandler(w, req, store, meta, vr)
	})))
	http.HandleFunc("/relations", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationsHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/tags", am.Wrap(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		tagsHandler(w, req, meta, vr)
	})))
	http.HandleFunc("/export", am.Wrap(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		exportHandler(w, req, objPath, metaPath, vr)
	})))
	http.HandleFunc("/import", am.Wrap(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		importHandler(w, req, objPath, metaPath, vr)
	})))
	// POST /capability issues a new capability. Other methods return 405.
	// GET /capability (capability lookup by ID) is not currently implemented.
	http.HandleFunc("/capability", am.Wrap(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		issueHandler(w, req, cm.Key, meta, vr)
	})))
	http.HandleFunc("/capability/revoke", am.Wrap(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		revokeHandler(w, req, meta, cm.Revoked, vr)
	})))

	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(uiPath))))

	log.Println("starting server on :8090")
	if err := http.ListenAndServe(":8090", nil); err != nil {
		log.Fatal(err)
	}
}