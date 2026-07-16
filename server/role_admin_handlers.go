package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// roleAssignHandler assigns a named role to a principal, records the
// assignment in the metadata log, and issues a capability for every grant
// currently defined on that role that the principal does not already hold
// live. The assigning principal is taken from the verified session
// (vr.Principal) for the audit trail.
//
// If the principal already holds the role, this is a no-op: no new
// role-assign log entry is written (RoleIndex membership is already a set,
// so a duplicate entry would change nothing, only add log noise), and
// issueCapabilitiesForRole is skipped entirely rather than relying on it to
// discover there's nothing to do. This is what makes it safe to drag a
// principal onto a role they already hold in the Assign tab — nothing
// happens beyond a confirmation message.
//
// POST /role/assign?principal=<id>&role=<n>&reason=<text>
func roleAssignHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, vr VerifiedRequest) {
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

	if principalHasRole(meta, principal, role) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%s already has role %s — no change made\n", principal, role)
		return
	}

	if err := meta.AppendRoleAssign(principal, role, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role assignment: "+err.Error(), http.StatusInternalServerError)
		return
	}

	issued, err := issueCapabilitiesForRole(meta, cm.Key, cm.Revoked, cfg.CapabilityExpiry, principal, role)
	if err != nil {
		log.Printf("warning: capability issuance for role %s assigned to %s: %v", role, principal, err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "role %s assigned to %s (%d capability(ies) issued)\n", role, principal, issued)
}

// roleRevokeHandler removes a named role from a principal, records the
// removal in the metadata log, and revokes every live capability that was
// issued to the principal under that role. The revoking principal is taken
// from the verified session (vr.Principal) for the audit trail.
//
// Symmetric to roleAssignHandler: if the principal doesn't currently hold
// the role, this is a no-op rather than writing a meaningless revoke entry.
//
// POST /role/revoke?principal=<id>&role=<n>&reason=<text>
func roleRevokeHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, vr VerifiedRequest) {
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

	if !principalHasRole(meta, principal, role) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "%s does not have role %s — no change made\n", principal, role)
		return
	}

	if err := meta.AppendRoleRevoke(principal, role, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role removal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	revokeReason := "role removed"
	if reason != "" {
		revokeReason = "role removed: " + reason
	}
	revoked, err := revokeCapabilitiesForRole(meta, cm.Revoked, principal, role, revokeReason, nil)
	if err != nil {
		log.Printf("warning: capability revocation for role %s removed from %s: %v", role, principal, err)
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "role %s removed from %s (%d capability(ies) revoked)\n", role, principal, revoked)
}

// rolesHandler returns role assignment information.
//
// GET /roles                     — all distinct active role names
// GET /roles?principal=<id>      — roles currently held by a principal
// GET /roles?role=<n>            — principals currently holding a role
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

// roleGrantAddHandler adds a capability template (hash+perm) to a role's
// definition and records it in the metadata log. Every principal currently
// holding the role is then issued that grant retroactively (existing grants
// they already hold are left untouched — issueCapabilitiesForRole only
// issues what's missing). The admin principal is taken from the verified
// session (vr.Principal) for the audit trail.
//
// POST /role/grant?role=<n>&hash=<hash>&perm=<read|write>&reason=<text>
func roleGrantAddHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	role := req.URL.Query().Get("role")
	hash := req.URL.Query().Get("hash")
	perm := req.URL.Query().Get("perm")
	reason := req.URL.Query().Get("reason")

	if role == "" || hash == "" || perm == "" {
		http.Error(w, "missing required parameter: role, hash, perm", http.StatusBadRequest)
		return
	}
	if perm != PermRead && perm != PermWrite {
		http.Error(w, "perm must be read or write", http.StatusBadRequest)
		return
	}

	if err := meta.AppendRoleGrantAdd(role, hash, perm, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role grant: "+err.Error(), http.StatusInternalServerError)
		return
	}

	issuedTotal := 0
	for _, principal := range meta.PrincipalsForRole(role) {
		issued, err := issueCapabilitiesForRole(meta, cm.Key, cm.Revoked, cfg.CapabilityExpiry, principal, role)
		if err != nil {
			log.Printf("warning: retroactive issuance of grant %s/%s (role %s) to %s: %v", hash, perm, role, principal, err)
			continue
		}
		issuedTotal += issued
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "grant %s/%s added to role %s (%d capability(ies) issued to existing members)\n", hash, perm, role, issuedTotal)
}

// roleGrantRemoveHandler removes a capability template (hash+perm) from a
// role's definition and records it in the metadata log. Every principal
// currently holding the role then has just that matching grant revoked —
// any other capabilities they hold under the role are left alone. The admin
// principal is taken from the verified session (vr.Principal) for the audit
// trail.
//
// POST /role/grant/revoke?role=<n>&hash=<hash>&perm=<read|write>&reason=<text>
func roleGrantRemoveHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, cm *CapabilityMiddleware, cfg Config, vr VerifiedRequest) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	role := req.URL.Query().Get("role")
	hash := req.URL.Query().Get("hash")
	perm := req.URL.Query().Get("perm")
	reason := req.URL.Query().Get("reason")

	if role == "" || hash == "" || perm == "" {
		http.Error(w, "missing required parameter: role, hash, perm", http.StatusBadRequest)
		return
	}

	if err := meta.AppendRoleGrantRemove(role, hash, perm, vr.Principal, reason); err != nil {
		http.Error(w, "failed to record role grant removal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	revokeReason := "role grant removed"
	if reason != "" {
		revokeReason = "role grant removed: " + reason
	}
	grantFilter := &metadata.RoleGrant{Hash: hash, Perm: perm}

	revokedTotal := 0
	for _, principal := range meta.PrincipalsForRole(role) {
		revoked, err := revokeCapabilitiesForRole(meta, cm.Revoked, principal, role, revokeReason, grantFilter)
		if err != nil {
			log.Printf("warning: retroactive revocation of grant %s/%s (role %s) from %s: %v", hash, perm, role, principal, err)
			continue
		}
		revokedTotal += revoked
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "grant %s/%s removed from role %s (%d capability(ies) revoked from existing members)\n", hash, perm, role, revokedTotal)
}

// roleGrantsHandler returns the capability templates currently defined for
// a role — i.e. what a principal receives when assigned it.
//
// GET /role/grants?role=<n>
func roleGrantsHandler(w http.ResponseWriter, req *http.Request, meta *metadata.Store, vr VerifiedRequest) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	role := req.URL.Query().Get("role")
	if role == "" {
		http.Error(w, "missing role parameter", http.StatusBadRequest)
		return
	}

	grants := meta.GrantsForRole(role)
	if grants == nil {
		grants = []metadata.RoleGrant{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(grants)
}

// principalHasRole reports whether principal currently holds role, per
// RoleIndex membership (not capability state). Used by roleAssignHandler and
// roleRevokeHandler to make repeated assign/revoke calls no-ops rather than
// writing redundant log entries — in particular, this is what makes it safe
// to drop a principal onto a role they already hold in the Assign tab.
func principalHasRole(meta *metadata.Store, principal, role string) bool {
	for _, r := range meta.RolesForPrincipal(principal) {
		if r == role {
			return true
		}
	}
	return false
}
