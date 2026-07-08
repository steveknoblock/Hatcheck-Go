package main

import (
	"encoding/json"
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// capabilityListEntry is a CapabilityPayload annotated with its current
// revocation status. Sig is still included — it's already public knowledge
// to whoever holds the capability itself, and an admin viewing the full
// list needs it to cross-reference against issued tokens.
type capabilityListEntry struct {
	metadata.CapabilityPayload
	Revoked bool `json:"revoked"`
}

// capabilitiesHandler returns capability records for admin visibility.
// Only principals with PermAdmin may call this — enforced by the same
// cm.Protect(PermAdmin, ...) wrapping used for /capability and
// /capability/revoke in routes.go.
//
// GET /capabilities                     — every capability ever issued
// GET /capabilities?principal=<id>       — capabilities issued to one principal
//
//	(pass principal= with an empty value, i.e. "?principal=", to list
//	bearer-token capabilities, which are stored under the empty-string key)
//
// GET /capabilities?id=<capability-id>   — a single capability by ID
//
// id and principal are mutually exclusive; if both are present, id wins.
func capabilitiesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, revoked *metadata.RevokedSet, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := req.URL.Query()

	if id := query.Get("id"); id != "" {
		cap, ok := meta.CapabilityByID(id)
		if !ok {
			http.Error(w, "capability not found", http.StatusNotFound)
			return
		}
		entry := capabilityListEntry{
			CapabilityPayload: cap,
			Revoked:           revoked.IsRevoked(cap.ID),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
		return
	}

	var caps []metadata.CapabilityPayload
	if principal, present := query["principal"]; present {
		caps = meta.CapabilitiesForPrincipal(firstOrEmpty(principal))
	} else {
		caps = meta.AllCapabilities()
	}

	result := make([]capabilityListEntry, len(caps))
	for i, cap := range caps {
		result[i] = capabilityListEntry{
			CapabilityPayload: cap,
			Revoked:           revoked.IsRevoked(cap.ID),
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// principalsHandler returns every distinct principal that has been issued
// at least one bound (non-bearer) capability. This is Hatcheck's closest
// equivalent to a user directory — there is no separate user table;
// "users" are derived entirely from the capability log.
//
// GET /principals
func principalsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	principals := meta.Principals()
	if principals == nil {
		principals = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(principals)
}

// firstOrEmpty returns the first element of a query param slice, or "" if
// empty. Used so that "?principal=" (present, empty value) is distinguished
// from the principal param being absent entirely — the former means "list
// bearer capabilities," the latter means "list everything."
func firstOrEmpty(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
