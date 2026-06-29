package main

import (
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/auth"
	"github.com/steveknoblock/hatcheck-go/internal/cas"
	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// registerRoutes wires all HTTP handlers to their paths. It is the single
// place in the server where routes are defined, keeping main() focused on
// initialisation rather than routing.
func registerRoutes(
	store *cas.Store,
	meta *metadata.Store,
	am *AuthMiddleware,
	cm *CapabilityMiddleware,
	rl *RateLimiters,
	authClient *auth.Client,
	cfg Config,
) {
	// Auth routes — not capability protected, but also not JWT protected
	// since these are the routes that establish identity in the first place.
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		loginHandler(w, req, authClient)
	})
	http.HandleFunc("/auth/authenticate", func(w http.ResponseWriter, req *http.Request) {
		authenticateHandler(w, req, authClient, meta, cm.Key, cfg)
	})
	http.HandleFunc("/auth/logout", logoutHandler)

	// Stash is auth-only — no capability required. The server issues a
	// capability for the resulting hash automatically, making stash the
	// ownership creation point.
	http.HandleFunc("/stash", Adapt(am.RequireAuth(rl.Write.Limit(func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		stashHandler(w, req, store, meta, cm.Key, cfg, vr)
	}))))
	http.HandleFunc("/fetch", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		fetchHandler(w, req, store, vr)
	})))))
	http.HandleFunc("/list", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		listHandler(w, req, store, meta, vr)
	})))))
	http.HandleFunc("/query", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		queryHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/namespaces", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		namespacesHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/names", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		namesHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/name", Adapt(am.RequireAuth(rl.Write.Limit(cm.Protect(PermWrite, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		nameHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/collection", Adapt(am.RequireAuth(rl.Write.Limit(func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		collectionHandler(w, req, store, meta, cm.Key, cfg, vr)
	}))))
	http.HandleFunc("/relation", Adapt(am.RequireAuth(rl.Write.Limit(func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationHandler(w, req, store, meta, cm.Key, cfg, vr)
	}))))
	http.HandleFunc("/relations", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationsHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/tags", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		tagsHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/export", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		exportHandler(w, req, cfg.ObjPath, cfg.MetaPath, vr)
	})))))
	http.HandleFunc("/import", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		importHandler(w, req, cfg.ObjPath, cfg.MetaPath, vr)
	})))))
	// POST /capability issues a new capability. Other methods return 405.
	// GET /capability (capability lookup by ID) is not currently implemented.
	http.HandleFunc("/capability", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		issueHandler(w, req, cm.Key, meta, vr)
	})))))
	http.HandleFunc("/capability/revoke", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		revokeHandler(w, req, meta, cm.Revoked, vr)
	})))))

	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(cfg.UIPath))))
}
