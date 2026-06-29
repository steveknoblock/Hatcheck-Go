# Capability-Based Access Control — Conversation Summary

---

## What is capability-based access control?

Capability-based access control is a security model where access rights are represented as unforgeable tokens called "capabilities," each of which grants the holder specific permissions to perform operations on a particular resource. Rather than checking a subject's identity against a central access control list (as in traditional access control), the system simply verifies possession of the appropriate capability — if you have the token, you have the right. This approach naturally enforces the principle of least privilege, simplifies delegation (you can pass a capability to another process without involving a central authority), and eliminates a class of confused deputy attacks since permissions travel with the caller rather than being looked up based on identity.

---

## Are capabilities associated with users or with objects?

Capabilities are associated with **objects** (resources). Each capability is essentially a reference to a specific object bundled together with the permissions allowed on that object (e.g., read, write, execute). A user or process collects whatever capabilities they've been granted, and those capabilities live in their "capability set" — but the capability itself is defined in terms of the object it points to, not the identity of who holds it. This is the key inversion from traditional access control: instead of asking "who is this user and what are they allowed to do?", the system asks "does this caller possess a valid capability for this object?"

---

## What fields are typically defined for a capability?

A capability typically includes these core fields:

- **Object reference** — a pointer or identifier to the specific resource being protected (a file, memory segment, service, etc.)
- **Rights/permissions mask** — a set of allowed operations on that object (read, write, execute, delete, etc.)
- **Unforgeable token** — some cryptographic or system-enforced guarantee that the capability can't be fabricated or tampered with (e.g., a cryptographic signature, or enforcement by a trusted kernel)

Some capability systems also include:

- **Expiry/validity** — a time limit or revocation flag indicating when the capability is no longer valid
- **Delegation rights** — whether the holder is allowed to pass the capability (or a restricted version of it) to another principal
- **Audit/provenance metadata** — tracking where the capability came from, useful for debugging and accountability

---

## How does a capability set work?

A capability set is simply the collection of capabilities held by a process or user at any given moment — it defines the complete set of things that process is currently allowed to do. Think of it like a keyring: each key (capability) opens a specific lock (resource) with specific operations permitted.

Key properties:

- **Scoped to a process** — each process has its own capability set, typically maintained by the OS kernel or runtime, so one process can't see or steal another's capabilities.
- **Inherited and delegated** — when a process spawns a child, it can pass along some or all of its capabilities. Delegation can be attenuating (giving a subset of rights) but never amplifying — you can't grant more than you yourself possess.
- **Dynamic** — capabilities can be added or dropped during a process's lifetime. A common security pattern is to drop capabilities as soon as they're no longer needed, reducing the attack surface.
- **Confinement** — a process is strictly confined to what's in its capability set. There's no ambient authority or implicit permissions based on identity; if you don't have the capability, the operation is simply not possible.

---

## How are users associated with capabilities?

Users are associated with capabilities indirectly, through the processes they run. When a user logs in and starts a session, the system bootstraps an initial capability set for their processes — typically derived from some policy. From that point on, the capabilities live with the processes, not the user identity itself.

Key points:

- **At login/spawn time** — a trusted authority consults policy to determine what capabilities a user's initial process should receive and hands them over.
- **Capabilities travel with processes, not users** — once granted, the user's identity becomes largely irrelevant to ongoing access decisions.
- **Delegation flows from there** — processes can pass along subsets of their capabilities without re-checking user identity.
- **Revocation is the hard problem** — because capabilities are distributed across many processes, revoking access is tricky. Solutions include expiring capabilities, indirection through a revocable reference, or capability revocation lists.

---

## What does the data structure look like for users and their capability sets?

```
User {
  user_id,
  credentials,
  policy_reference   // points to what capabilities they should receive
}

Process {
  pid,
  owner_user_id,     // traces back to the user, but not used for access checks
  capability_set     // the live authority this process holds
}

CapabilitySet {
  capabilities: [Capability, Capability, ...]
}

Capability {
  object_ref,        // pointer/identifier to the target resource
  rights_mask,       // bitmask of allowed operations
  token,             // unforgeable proof of validity
  delegatable,       // can this be passed to another process?
  expiry             // optional validity window
}
```

Relationship diagram:

```
User ──(at login)──> Process
                        │
                        └──> CapabilitySet
                                  │
                                  ├──> Capability --> Object A (read)
                                  ├──> Capability --> Object B (read, write)
                                  └──> Capability --> Object C (execute)
```

In many systems the capability set is a **capability table** — an indexed structure where capabilities are referenced by index (called a capability descriptor or c-list slot), similar to how Unix file descriptors work.

---

## Is there a concept of ownership in a capability-based access control system?

Ownership is not a core concept in pure capability-based access control. What matters is whether you hold the right capability, not whether you "own" the object. That said, ownership-like concepts tend to appear at the edges:

- **Creation** — the process that creates an object typically receives a full-rights capability for it, which looks like ownership in practice.
- **Revocation authority** — someone has to be able to revoke capabilities for an object. This role functions like an owner but is modeled as a capability with a special revocation right.
- **Resource accounting** — practical systems often tag objects with a creator for quota enforcement or billing, which resembles ownership even if it doesn't affect access control.
- **Administrative override** — real systems often have a privileged administrator role, modeled as capabilities rather than a built-in ownership field.

---

## Is there a one-to-one mapping between users and capabilities?

No — the mapping is **many-to-many**:

- **Many users → one capability**: Multiple users can hold capabilities pointing to the same object.
- **One user → many capabilities**: A single user's processes hold many capabilities simultaneously — one for each resource they're authorized to use.

The one invariant is that delegation is always **attenuating** — rights on a delegated capability are always a subset of the delegator's rights, even though the population of holders can grow arbitrarily.

---

## Can a user with only read rights give another user write rights?

No. This is the **"no amplification" rule** — you cannot grant more authority than you yourself possess. If you hold a read-only capability, you can only delegate a read capability (or a subset). Authority is strictly monotonically decreasing through delegation:

```
Original grant:  [read, write, execute]
You receive:     [read]
You can give:    [read]          ←  only this or a subset
You cannot give: [read, write]   ←  impossible, you don't have write
```

Privilege escalation can only happen when a trusted authority issues a new capability — a controlled, auditable event.

---

## Is there an admin user who can issue a new capability?

Yes, in practice all real capability systems have some form of privileged authority that can issue new capabilities:

- **Kernel as root authority** — in OS-level systems (like CHERI or L4), the kernel is the ultimate issuer. No userspace entity can forge a capability.
- **A privileged service or caretaker** — in distributed systems, a trusted "authority," "mint," or "capability server" holds the power to issue capabilities for resources it controls.
- **Bootstrap problem** — every capability system must solve this: someone has to issue the first capabilities at system startup, handled by the kernel or a trusted root process.
- **Key distinction from traditional admin** — even privileged administrators are modeled as holding special capabilities rather than having an identity-based override, avoiding ambient authority.

---

## Can the minting authority be restricted from reading or writing objects?

Yes. The minting authority and the access rights are **separate capabilities** that can be held independently. A capability to *create* capabilities for an object is a distinct right from the capabilities to *read* or *write* that object:

```
Mint service:    [mint]          // can issue capabilities, cannot read or write
User A:          [read]          // can read, cannot write or mint
User B:          [read, write]   // can read and write, cannot mint
```

This enforces **separation of duties** at a fundamental level — the entity responsible for access administration never needs to touch the actual data, and the entities that access the data never need minting authority. Compromise of one doesn't imply compromise of the other.

Administrative authority can be further decomposed into a tree of minting capabilities, where each node only controls what it has explicitly been delegated — without any node needing actual access to the underlying data it administers.
