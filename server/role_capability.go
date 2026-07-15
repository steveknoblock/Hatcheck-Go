package main

import (
	"log"
	"time"

	"github.com/steveknoblock/hatcheck-go/internal/metadata"
)

// This file is the bridge between role bookkeeping (internal/metadata —
// RoleIndex, which only knows "who holds what role" and "what does a role
// grant") and actual capability issuance/revocation (which needs the
// signing key and the live RevokedSet, both server-layer concerns). Roles
// never appear in capability tokens and play no part at verification time
// — the functions here are the only place a role assignment or a role's
// grant definition is translated into real capabilities.

// issueCapabilitiesForRole issues one capability to principal for every
// capability template currently granted by role, skipping any grant the
// principal already holds live (so calling this again for an unchanged
// role/principal pair is a no-op — safe to call on every assignment, and
// safe to call for every existing member when a role's grant changes).
//
// Called from:
//   - roleAssignHandler, when a role is newly assigned to a principal
//   - roleGrantAddHandler, once per existing member, when a grant is added
//     to a role that already has members
func issueCapabilitiesForRole(
	meta *metadata.Store,
	key []byte,
	revoked *metadata.RevokedSet,
	expiry time.Duration,
	principal string,
	role string,
) (issued int, err error) {
	grants := meta.GrantsForRole(role)
	if len(grants) == 0 {
		return 0, nil
	}

	live := meta.CapabilitiesForPrincipalRole(principal, role, revoked)
	have := make(map[metadata.RoleGrant]bool, len(live))
	for _, cap := range live {
		have[metadata.RoleGrant{Hash: cap.Hash, Perm: cap.Perm}] = true
	}

	email := resolveEmail(meta, principal, "")

	for _, grant := range grants {
		if have[grant] {
			continue
		}

		expires := time.Now().UTC().Add(expiry)
		cap := metadata.SignCapability(key, grant.Hash, grant.Perm, principal, email, expires)
		cap.Role = role
		if err := meta.AppendCapability(cap); err != nil {
			log.Printf("warning: failed to issue role capability (%s/%s) for %s under role %s: %v",
				grant.Hash, grant.Perm, principal, role, err)
			continue
		}
		issued++
	}

	return issued, nil
}

// revokeCapabilitiesForRole revokes every live capability issued to
// principal under the given role annotation. If grantFilter is non-nil,
// only capabilities matching that exact (hash, perm) grant are revoked —
// used when a single grant is removed from a role's definition rather than
// the whole role being removed from the principal.
//
// Called from:
//   - roleRevokeHandler, when a role is removed from a principal (grantFilter nil)
//   - roleGrantRemoveHandler, once per existing member, when a grant is
//     removed from a role's definition (grantFilter set)
func revokeCapabilitiesForRole(
	meta *metadata.Store,
	revoked *metadata.RevokedSet,
	principal string,
	role string,
	reason string,
	grantFilter *metadata.RoleGrant,
) (revokedCount int, err error) {
	live := meta.CapabilitiesForPrincipalRole(principal, role, revoked)

	for _, cap := range live {
		if grantFilter != nil && (cap.Hash != grantFilter.Hash || cap.Perm != grantFilter.Perm) {
			continue
		}
		if err := meta.AppendCapabilityRevoke(cap.ID, reason, revoked); err != nil {
			log.Printf("warning: failed to revoke role capability %s for %s under role %s: %v",
				cap.ID, principal, role, err)
			continue
		}
		revokedCount++
	}

	return revokedCount, nil
}
