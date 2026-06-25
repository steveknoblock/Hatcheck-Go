	rl := NewRateLimiters()

	// Auth routes — not JWT protected since these establish identity.
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		loginHandler(w, req, authClient)
	})
	http.HandleFunc("/auth/authenticate", func(w http.ResponseWriter, req *http.Request) {
		authenticateHandler(w, req, authClient, meta, cm.Key)
	})
	http.HandleFunc("/auth/logout", logoutHandler)

	// Stash is auth-only — no capability required. The server issues a
	// capability for the resulting hash automatically, making stash the
	// ownership creation point.
	http.HandleFunc("/stash", Adapt(am.RequireAuth(rl.Write.Limit(func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		stashHandler(w, req, store, meta, cm.Key, vr)
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
		collectionHandler(w, req, store, meta, cm.Key, vr)
	}))))
	http.HandleFunc("/relation", Adapt(am.RequireAuth(rl.Write.Limit(func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationHandler(w, req, store, meta, cm.Key, vr)
	}))))
	http.HandleFunc("/relations", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		relationsHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/tags", Adapt(am.RequireAuth(rl.Read.Limit(cm.Protect(PermRead, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		tagsHandler(w, req, meta, vr)
	})))))
	http.HandleFunc("/export", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		exportHandler(w, req, objPath, metaPath, vr)
	})))))
	http.HandleFunc("/import", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		importHandler(w, req, objPath, metaPath, vr)
	})))))
	// POST /capability issues a new capability. Other methods return 405.
	// GET /capability (capability lookup by ID) is not currently implemented.
	http.HandleFunc("/capability", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		issueHandler(w, req, cm.Key, meta, vr)
	})))))
	http.HandleFunc("/capability/revoke", Adapt(am.RequireAuth(rl.Admin.Limit(cm.Protect(PermAdmin, func(w http.ResponseWriter, req *http.Request, vr VerifiedRequest) {
		revokeHandler(w, req, meta, cm.Revoked, vr)
	})))))

	http.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(uiPath))))
