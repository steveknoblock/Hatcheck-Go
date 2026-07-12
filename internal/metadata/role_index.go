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
// Two projections are maintained:
//
//	byPrincipal — maps a principal to its current set of role names
//	byRole      — maps a role name to the set of principals holding it
//
// Both are updated together on every Add call so either query is O(1).
type RoleIndex struct {
	byPrincipal map[string]map[string]struct{} // principal -> set of role names
	byRole      map[string]map[string]struct{} // role -> set of principals
}

// NewRoleIndex returns an initialised RoleIndex ready for use.
func NewRoleIndex() *RoleIndex {
	return &RoleIndex{
		byPrincipal: make(map[string]map[string]struct{}),
		byRole:      make(map[string]map[string]struct{}),
	}
}

// Name satisfies the Index interface.
func (r *RoleIndex) Name() string { return "role" }

// Add processes a log entry. Only OpRoleAssign and OpRoleRevoke entries
// are handled; all others are silently ignored.
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
