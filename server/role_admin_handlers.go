package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// roleAssignHandler assigns a named role to a principal and records the
// assignment in the metadata log. The assigning principal is taken from the
// verified session (vr.Principal) for the audit trail.
//
// POST /role/assign?principal=<id>&role=<name>&reason=<text>
func roleAssignHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	principal := req.URL.Query().Get("principal")
	role := req.URL.Query().Get("role")
	reason := req.URL.Query().Get("reason")

	if principal == "" || role == "" {
		http.Error(w, "missing required parameter: principal, role", http.StatusBadRequest)
		return
	}

	if err := meta.AppendRoleAssign(principal, role, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role assignment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "role %s assigned to %s\n", role, principal)
}

// roleRevokeHandler removes a named role from a principal and records the
// removal in the metadata log. The revoking principal is taken from the
// verified session (vr.Principal) for the audit trail.
//
// POST /role/revoke?principal=<id>&role=<name>&reason=<text>
func roleRevokeHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	principal := req.URL.Query().Get("principal")
	role := req.URL.Query().Get("role")
	reason := req.URL.Query().Get("reason")

	if principal == "" || role == "" {
		http.Error(w, "missing required parameter: principal, role", http.StatusBadRequest)
		return
	}

	if err := meta.AppendRoleRevoke(principal, role, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role removal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "role %s removed from %s\n", role, principal)
}

// rolesHandler returns role assignment information.
//
// GET /roles                        — all distinct active role names
// GET /roles?principal=<id>         — roles currently held by a principal
// GET /roles?role=<name>            — principals currently holding a role
//
// principal and role are mutually exclusive; if both are present, principal wins.
func rolesHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := req.URL.Query()

	if principal := query.Get("principal"); principal != "" {
		roles := meta.RolesForPrincipal(principal)
		if roles == nil {
			roles = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(roles)
		return
	}

	if role := query.Get("role"); role != "" {
		principals := meta.PrincipalsForRole(role)
		if principals == nil {
			principals = []string{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(principals)
		return
	}

	// No filter — return all active role names.
	roles := meta.Roles()
	if roles == nil {
		roles = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roles)
}
