# Capability-Based Access Control — Conversation Part 3

---

## Review of draft document: User Centered Capability Access Control

### Draft document

```
# User Centered Capability Access Control
## People have capabilities not objects
The guiding idea of this design is: People have capabilities not objects.
I have the capability to read this object.
I do not have the capability to write to this object.
{user} -has read capability-> {object}
Each capability contains one or more grants of permissions, such as Read or Write.
More than one user has access to the same object:
 {object}
    -has read capability-> User A
    -does not have write capability-> User B
Note: This can be modelled as a network graph of the relationships between objects, users, and capabilities (Google uses this for Drive).
In a capability based access system leaning in to users, multiple users can hold capabilities pointing to the same object.
{User A}
    -> {Capability} -> {Object 123}
    -> {Capability} -> {Object 255}
{User B}
    -> {Capability} -> {Object 123}
    -> {Capability} -> {Object 512}
A one-to-many relationship can map one user to many capabilities.
It should be possible to list all capabilities associated with a user.
A many-to-many relationship can map many users to many capabilities.
```

### Assessment

The document captures the core ideas correctly, but there are a few places where the framing is imprecise or could be tightened.

**What's working well**

The central inversion — "people have capabilities, not objects" — is a clear and memorable statement of the design philosophy. The graph diagrams are useful and the mention of Google Zanzibar as a reference point is appropriate.

**A few issues worth addressing**

**The second diagram has the arrows backwards.** This section:

```
{object}
    -has read capability-> User A
    -does not have write capability-> User B
```

describes an ACL, not a capability system — it's organized from the object's perspective, listing who can access it. That's precisely the dual you're moving away from. The arrows should flow from users to capabilities to objects, consistently with the diagrams below it.

**"Each capability contains one or more grants of permissions"** is slightly ambiguous. In the actual implementation, each `CapabilityPayload` represents a single permission (`Perm` is a single string). If that's the design, the document should say so. If a capability is intended to carry multiple permissions (read AND write in one token), that's a different design decision worth making explicit.

**The "does not have write capability" notation** is odd in a capability document. Capability systems are defined by what you *can* do — the absence of a capability is just absence, not a thing you record or represent. Stating a negative grant implies an ACL-style deny list, which is a different model. Better to simply omit it.

**"It should be possible to list all capabilities associated with a user"** is tucked in at the end almost as an afterthought, but it's actually one of the key operational advantages of the user-centric design. It deserves more prominence — this is exactly what makes auditing and revocation tractable.

**Overall**

Solid first draft that communicates the intent clearly. The main thing to fix is the backwards diagram that accidentally describes an ACL, and tightening the language around what a single capability represents. The user-centric framing is well chosen and worth keeping.

---

## On the second diagram — intent was to show how the capability relates back to the object

That makes sense as intent, but the way it's written still reads as an ACL because the object is the subject of the sentence and users are listed under it. The structure implies the object is tracking who has access to it, which is exactly how an ACL works.

The fix is straightforward — keep the same information but reverse the perspective so the capability is the organizing concept rather than the object:

```
{User A} -> {Capability: read} -> {Object 123}
{User B} -> {Capability: read} -> {Object 123}
```

This shows the same fact — two users have read access to the same object — but from the capability holder's perspective rather than the object's perspective. The object appears at the end of the arrow as the target, not at the top as the subject organizing the list.

If you specifically want to illustrate that multiple users can reach the same object, you could add a note like:

```
{User A} -> {Capability: read} -> {Object 123}
{User B} -> {Capability: read} -> {Object 123}
                                       ↑
                          same object, two capability holders
```

That preserves the intent — showing the relationship back to the object — without accidentally implying the object maintains a list of who can access it.

---

## On the remaining issues — clarifying single permission per capability and removing negative grants

Also worth addressing in the revision:

1. **Clarify whether a capability carries a single permission or multiple** — the actual `CapabilityPayload` uses a single `Perm` string, so the document should reflect that if that's the intended design.

2. **Remove the "does not have write capability" notation** — in a capability system, absence of a capability is just absence. Recording a negative grant implies a deny list, which pulls the design toward ACL thinking.

Once those are cleaned up the document will be consistent with both the conceptual model and the actual implementation.

---

## The absence of a capability is the statement of not having a capability

Exactly. And this is actually one of the elegant simplifications of capability-based thinking — you never need to reason about denials, exceptions, or negative rules. The access control question reduces to a single check: does this user hold a valid capability for this object? If yes, access is granted. If no capability exists, access is simply not possible. There's no deny list to maintain, no conflict resolution between allow and deny rules, and no edge cases around rule ordering.

This is a meaningful contrast with ACL-based systems, which often end up with both allow and deny entries and then need rules to determine which takes precedence when they conflict. That complexity is entirely absent from a pure capability model.

---

## To grant read and write permission two capabilities can be issued

Yes, exactly. One capability granting read, another granting write — each a distinct token with its own `ID` and `Sig`. This fits naturally with the existing `CapabilityPayload` design where `Perm` is a single string.

It also has a nice operational consequence: you can revoke write access independently of read access. If you want to downgrade a user from read/write to read-only, you simply revoke the write capability and leave the read capability intact. No need to modify or reissue anything — just a revocation log entry for the one token.

This is the capability model working as intended — fine-grained, independently revocable grants rather than a monolithic permission set attached to a user.

How could a capability access system support roles for users?

Roles can be layered on top of a capability system in a few different ways, each with different tradeoffs:

**Role as a capability bundle**
The simplest approach — a role is just a named collection of capabilities that get issued together. When a user is assigned the "editor" role, the mint authority issues them the corresponding set of capabilities automatically. The role itself is just an administrative shorthand for "issue these capabilities to this user." At verification time, the system still checks individual capabilities — it never checks the role name directly.

**Role as a capability**
A role can itself be represented as a capability. The user holds a "role: editor" capability, and when they present it, the server resolves it to the set of permissions that role implies. This adds an indirection layer — changing what the editor role can do automatically affects all users holding that role capability, without needing to reissue individual capabilities to each user.

**Role as a principal group**
Rather than issuing capabilities to individual users, you issue them to a role identifier as the principal. Any user who is a member of that role inherits its capabilities. This is closer to how RBAC works, but enforced via capability tokens rather than identity lookup.

**The key design question**
The tradeoff is between flexibility and simplicity:

- Issuing individual capabilities gives maximum flexibility and independent revocability but more administrative overhead.
- Role-as-bundle reduces overhead but means revoking a role requires revoking all the individual capabilities that were issued under it.
- Role-as-capability makes role changes instant and atomic but reintroduces some indirection at verification time.

For Hatcheck specifically, given that capabilities are singular per `Perm` and independently revocable, the role-as-bundle approach would fit most naturally — roles live at the issuance layer as an administrative concept, while the underlying capability model stays clean and unchanged.

So roles are like a group or filter on capabilities by principal?

Yes, that's a good way to frame it. A role is essentially a query over the capability space — "give me all capabilities where the principal belongs to group X" — rather than a first-class concept in the access control model itself.

This maps cleanly onto what you already have. Your `CapabilityIndex` (from the conversation history) already supports querying capabilities by principal. A role would just be one level of indirection above that:

```
Role: editor
    → Principal: user-A
    → Principal: user-B

Query: "what can editors do?"
    → find all principals with role: editor
    → find all capabilities for those principals
```

Or from the other direction:

```
Query: "what can user-A do?"
    → find all capabilities for user-A
    → role is just a label that explains why those capabilities were issued
```

The role itself never needs to appear in the capability token at all. It lives in the administrative layer — a record of why capabilities were issued — while the capability token remains a clean, self-contained grant that the verification system checks without needing to know anything about roles.

This also means role changes are straightforward to reason about: assigning a user to a role issues them the corresponding capabilities, and removing them from a role revokes those capabilities. The role is just an organizational concept that drives issuance and revocation decisions, not something the access control enforcement layer needs to understand.

Sounds good

It's a clean design because it keeps the layers separate — roles are an administrative and organizational concept, while capabilities are the enforcement mechanism. The two don't need to know about each other at runtime.

It also means you get the auditability benefit naturally. If you ever need to answer "why does user-A have write access to object 123?" the answer is traceable: user-A has a write capability for object 123 because they were assigned the editor role, which triggered issuance of that capability. The capability log gives you the what, and the role assignment gives you the why.

What is required to implement the role based organization on principal?

Given what's already in place in Hatcheck, the implementation is relatively lightweight because the capability infrastructure already exists. Here's what would be needed:

**Data**
A role assignment record associating principals with roles:
```
RoleAssignment {
    principal,   // the user ID
    role,        // e.g. "editor", "viewer", "admin"
    assigned_by, // audit trail
    assigned_at, // audit trail
}
```
This could live in the metadata log as a new operation type, consistent with the existing append-only architecture.

**Storage and indexing**
A `RoleIndex` following the same pattern as your existing indexes — built from the log on startup, queryable by principal or by role. Two useful queries:
- "what roles does this principal have?" — needed at capability issuance time
- "what principals have this role?" — needed for bulk operations like role changes

**Issuance logic**
When a role is assigned to a principal, the mint authority issues the corresponding capabilities automatically. This is just a loop over the capability set defined for that role, calling the existing capability issuance logic for each one.

**Revocation logic**
When a role is removed from a principal, the capabilities that were issued under that role assignment get revoked. This requires tracking which capabilities were issued as a result of a role assignment — a `role` annotation on the capability record would support this, even if it plays no part in verification.

**Admin UI**
A role management tab alongside the existing capability UI — assign/remove roles for principals, define what capabilities each role implies.

The core verification path doesn't change at all. Roles live entirely in the issuance and revocation layer, leaving the runtime capability check untouched.

