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
