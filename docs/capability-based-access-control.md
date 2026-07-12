# Capability Based Access Control

## User Centered Capability Access Control

An access control list (ACL) defines who can access an object. Access is organized from the object's perspective. An example of this organization comes from the Linux access control list. A set of access rules can be associated with a file. Here, Alice is given read and write access and Bob is given read access to a file:

financial.md
u:alice:rw
u:bob:r

Capability based access control is defined by what you *can* do.

```
{User A}
    -has Read capability-> {Object 123}
    -has Read capability-> {Object 124}
    -has Read capability-> {Object 125}
    -has Write capability-> {Object 125}
```

Specifying what user A *cannot* access is unnecessary.

More than one user can have a capability for the same object.

```
{User A}
    -has Read capability-> {Object 123}
{User B}
    -has Read capability-> {Object 123}
                                ↑
                          same object, two capability holders
```

## The absence of a capability is the statement of not having a capability

Exactly. And this is actually one of the elegant simplifications of capability-based thinking — you never need to reason about denials, exceptions, or negative rules. The access control question reduces to a single check: does this user hold a valid capability for this object? If yes, access is granted. If no capability exists, access is simply not possible. There's no deny list to maintain, no conflict resolution between allow and deny rules, and no edge cases around rule ordering.

This is a meaningful contrast with ACL-based systems, which often end up with both allow and deny entries and then need rules to determine which takes precedence when they conflict. That complexity is entirely absent from a pure capability model.


## To grant read and write permission two capabilities can be issued

Yes, exactly. One capability granting read, another granting write — each a distinct token with its own `ID` and `Sig`. This fits naturally with the existing `CapabilityPayload` design where `Perm` is a single string.

It also has a nice operational consequence: you can revoke write access independently of read access. If you want to downgrade a user from read/write to read-only, you simply revoke the write capability and leave the read capability intact. No need to modify or reissue anything — just a revocation log entry for the one token.

This is the capability model working as intended — fine-grained, independently revocable grants rather than a monolithic permission set attached to a user.






### People have capabilities not objects

The guiding idea of this design is: People have capabilities not objects.

I have the capability to read this object.

I do not have the capability to write to this object.

{user} -has read capability-> {object}

Each capability contains one grant of permissions, such as Read or Write.

## Capabilities

s
More than one user has access to the same object:

{User A} -> {Capability: read} -> {Object 123}
{User B} -> {Capability: read} -> {Object 123}
                                       ↑
                          same object, two capability holders

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
