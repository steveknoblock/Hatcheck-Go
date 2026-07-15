package metadata

import "encoding/json"

// RoleIndex tracks role assignments and removals for principals.
//
// A role is an administrative label — it records why capabilities were
// issued and drives bulk issuance/revocation decisions, but never appears
// in a capability token itself. At verification time the system checks
// individual capabilities only; RoleIndex plays no part.
//
// The index is built from OpRoleAssign and OpRoleRevoke log entries on
// startup, following the same append-only pattern as all other indexes.
// The effective role set for a principal is the set of roles that have
// been assigned but not subsequently revoked.
//
// Two projections are maintained for assignments:
//
//	byPrincipal — maps a principal to its current set of role names
//	byRole      — maps a role name to the set of principals holding it
//
// A third projection, grants, maps a role name to the set of capability
// templates (Hash, Perm pairs) that role currently grants. This is the
// role's *definition* — built from OpRoleGrantAdd/OpRoleGrantRemove entries
// — as distinct from byRole, which is *membership*. Neither projection
// issues or revokes capabilities itself; RoleIndex only answers "what does
// this role currently mean" and "who currently holds it." The server layer
// (see role_capability.go) is what turns those answers into actual
// capability issuance and revocation.
//
// All three are updated together on every Add call so any query is O(1).
type RoleIndex struct {
	byPrincipal map[string]map[string]struct{}    // principal -> set of role names
	byRole      map[string]map[string]struct{}    // role -> set of principals
	grants      map[string]map[RoleGrant]struct{} // role -> set of capability templates
}

// NewRoleIndex returns an initialised RoleIndex ready for use.
func NewRoleIndex() *RoleIndex {
	return &RoleIndex{
		byPrincipal: make(map[string]map[string]struct{}),
		byRole:      make(map[string]map[string]struct{}),
		grants:      make(map[string]map[RoleGrant]struct{}),
	}
}

// Name satisfies the Index interface.
func (r *RoleIndex) Name() string { return "role" }

// Add processes a log entry. Only role-related entries are handled; all
// others are silently ignored.
func (r *RoleIndex) Add(entry Entry) {
	switch entry.Op {
	case OpRoleAssign:
		var p RoleAssignPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		r.assign(p.Principal, p.Role)
	case OpRoleRevoke:
		var p RoleRevokePayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		r.revoke(p.Principal, p.Role)
	case OpRoleGrantAdd:
		var p RoleGrantPayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		r.addGrant(p.Role, p.Hash, p.Perm)
	case OpRoleGrantRemove:
		var p RoleGrantRemovePayload
		if err := json.Unmarshal(entry.Payload, &p); err != nil {
			return
		}
		r.removeGrant(p.Role, p.Hash, p.Perm)
	}
}

// Query satisfies the Index interface. It returns the role names held by
// the given principal, for use via the generic query endpoint.
// Pass a role name prefixed with "role:" to query by role instead:
// e.g. key="role:editor" returns the principals holding that role.
func (r *RoleIndex) Query(key string) []string {
	const rolePrefix = "role:"
	if len(key) > len(rolePrefix) && key[:len(rolePrefix)] == rolePrefix {
		return r.PrincipalsForRole(key[len(rolePrefix):])
	}
	return r.RolesForPrincipal(key)
}

// RolesForPrincipal returns the current role names held by the given principal.
func (r *RoleIndex) RolesForPrincipal(principal string) []string {
	roles := r.byPrincipal[principal]
	result := make([]string, 0, len(roles))
	for role := range roles {
		result = append(result, role)
	}
	return result
}

// PrincipalsForRole returns the principals currently holding the given role.
func (r *RoleIndex) PrincipalsForRole(role string) []string {
	principals := r.byRole[role]
	result := make([]string, 0, len(principals))
	for p := range principals {
		result = append(result, p)
	}
	return result
}

// Roles returns all distinct role names that have ever had at least one
// active assignment.
func (r *RoleIndex) Roles() []string {
	result := make([]string, 0, len(r.byRole))
	for role := range r.byRole {
		if len(r.byRole[role]) > 0 {
			result = append(result, role)
		}
	}
	return result
}

// GrantsForRole returns the capability templates currently defined for the
// given role — i.e. what a principal receives when assigned this role.
func (r *RoleIndex) GrantsForRole(role string) []RoleGrant {
	g := r.grants[role]
	result := make([]RoleGrant, 0, len(g))
	for grant := range g {
		result = append(result, grant)
	}
	return result
}

// assign adds a role to a principal's set, creating the inner maps as needed.
func (r *RoleIndex) assign(principal, role string) {
	if r.byPrincipal[principal] == nil {
		r.byPrincipal[principal] = make(map[string]struct{})
	}
	r.byPrincipal[principal][role] = struct{}{}

	if r.byRole[role] == nil {
		r.byRole[role] = make(map[string]struct{})
	}
	r.byRole[role][principal] = struct{}{}
}

// revoke removes a role from a principal's set. A no-op if the assignment
// does not exist, which can happen if log entries are replayed out of order
// or a revoke entry was written without a matching assign entry.
func (r *RoleIndex) revoke(principal, role string) {
	if r.byPrincipal[principal] != nil {
		delete(r.byPrincipal[principal], role)
	}
	if r.byRole[role] != nil {
		delete(r.byRole[role], principal)
	}
}

// addGrant adds a capability template to a role's definition, creating the
// inner map as needed. A no-op if the exact (hash, perm) pair is already
// present — RoleGrant is a plain comparable struct, so duplicates collapse
// naturally via the set.
func (r *RoleIndex) addGrant(role, hash, perm string) {
	if r.grants[role] == nil {
		r.grants[role] = make(map[RoleGrant]struct{})
	}
	r.grants[role][RoleGrant{Hash: hash, Perm: perm}] = struct{}{}
}

// removeGrant removes a capability template from a role's definition. A
// no-op if the grant does not exist, which can happen on out-of-order replay
// or a removal written without a matching add.
func (r *RoleIndex) removeGrant(role, hash, perm string) {
	if r.grants[role] != nil {
		delete(r.grants[role], RoleGrant{Hash: hash, Perm: perm})
	}
}
