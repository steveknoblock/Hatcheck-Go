# Capability-Based Access Control — Conversation Part 2

---

## What if a capability-based access control system assigned capabilities to users?

That's a valid design choice and some systems do work this way. Instead of capabilities living purely with processes, they'd be stored and managed at the user level — essentially a persistent capability set attached to the user account rather than (or in addition to) the ephemeral process-level set.

The tradeoffs compared to the pure process-centric model:

**Advantages**
- **Simpler mental model** — administrators think in terms of users, which maps more naturally to organizational concepts like roles and responsibilities.
- **Persistence** — capabilities survive process termination and re-login without needing to be re-issued each time.
- **Easier auditing** — you can look up what a user is authorized to do without having to inspect all their running processes.

**Disadvantages**
- **Weaker confinement** — if capabilities are tied to the user, every process that user runs potentially has access to all their capabilities, which undermines least privilege.
- **Revocation scope** — revoking a capability from a user is straightforward, but you'd also need to ensure any running processes that inherited it are also affected.
- **Blurs the identity/authority separation** — one of the philosophical strengths of capability systems is decoupling "who you are" from "what you can do." Attaching capabilities to users re-entangles those two concerns.

In practice this hybrid approach resembles **role-based access control (RBAC)** fairly closely — users are assigned roles, roles carry permissions, which is structurally similar to users holding capability sets. The difference is mainly in whether the rights are enforced by possession of an unforgeable token (capability-style) or by identity lookup at access time (RBAC-style).

---

## How do access control lists fit into this discussion?

Access control lists (ACLs) are essentially the **dual** of capabilities — they represent the same underlying access control matrix but from the opposite perspective.

The access control matrix is a conceptual table where:
- **Rows are subjects** (users or processes)
- **Columns are objects** (resources)
- **Cells contain rights** (read, write, execute, etc.)

```
              File A        File B        Service C
User 1        read          read/write    —
User 2        read/write    —             execute
User 3        —             read          execute
```

**ACLs and capabilities are two ways to slice this matrix:**

- **ACL** — store the matrix column by column. Each object carries a list of who can access it and with what rights.
- **Capability set** — store the matrix row by row. Each subject carries a list of what objects they can access and with what rights.

**The practical differences:**

- **ACLs make it easy to ask "who can access this object?"** — just read the list. Capabilities make this hard because authority is distributed across many subjects.
- **Capabilities make it easy to ask "what can this process do?"** — just inspect its capability set. ACLs make this hard because you'd have to scan every object's list.
- **Revocation is easier with ACLs** — remove an entry from the object's list and access is immediately denied. With capabilities, the token is already distributed and harder to claw back.
- **Delegation is easier with capabilities** — just pass the token. With ACLs, delegation requires modifying the object's list, which requires administrative access to the object itself.
- **ACLs rely on identity verification** — the system must authenticate who is making the request before checking the list. Capabilities avoid this — possession of the token is sufficient.

Most real systems use a hybrid of both. Unix/Linux filesystems use ACLs, while also having process-level capability sets for system privileges.

---

## What other kinds of access control systems exist?

**Discretionary Access Control (DAC)**
The resource owner decides who gets access. Unix file permissions are the classic example. Flexible but relies on users making good security decisions.

**Mandatory Access Control (MAC)**
Access policy is set by a central authority and cannot be overridden by individual users. SELinux and military classification systems are examples. Much stronger than DAC but more rigid and complex to administer.

**Role-Based Access Control (RBAC)**
Permissions are assigned to roles, and users are assigned to roles. The most widely deployed model in enterprise software today. Maps well to organizational structures but can become unwieldy with many fine-grained roles.

**Attribute-Based Access Control (ABAC)**
Access decisions are made by evaluating policies against attributes of the subject, the object, and the environment. Very expressive and flexible but complex to manage and reason about.

**Rule-Based Access Control**
Access is governed by a fixed set of rules, often time- or context-dependent. Often used as a layer on top of other models.

**Bell-LaPadula and Biba Models**
Formal mathematical models designed for specific security properties. Bell-LaPadula enforces confidentiality ("no read up, no write down"). Biba enforces integrity ("no read down, no write up"). These underpin many MAC implementations.

**Graph-Based / Relationship-Based Access Control (ReBAC)**
Access is determined by the relationship between the subject and the object in a graph. Google Zanzibar (which powers Google Drive sharing) is the famous example.

These models aren't mutually exclusive — most real systems layer several of them together.

---

## Is an ACL a model or a tool?

**ACL is primarily a tool (mechanism)**, not a model (policy).

An ACL is a data structure and enforcement mechanism — it's the implementation technique of storing a list of subjects and their rights on each object. It doesn't by itself say anything about how those rights should be assigned, who should be able to change them, or what security properties the overall system should have.

The **models** are the policies that sit above the mechanism:

- **DAC** is a model — ACLs are the typical tool used to implement it.
- **MAC** is a model — ACLs can also implement MAC, but with different rules about who can modify them.
- **RBAC** is a model — often implemented using ACL-like structures under the hood.

The same ACL mechanism can appear in multiple different models depending on the policy governing it. Capabilities sit in an interesting middle ground — the capability token is a mechanism, but the capability model also carries implicit policy (no amplification, confinement, delegation rules), blurring the line between tool and model.

The broader point: access control systems have two distinct layers — the **policy** (model, defines the rules) and the **mechanism** (tool, enforces them). Keeping those layers clearly separated is considered good security design.

---

## Which system is the least dependent on application logic to control access?

**Mandatory Access Control (MAC)** is the least dependent on application logic. MAC enforcement happens **below the application layer** — in the OS kernel, hypervisor, or hardware — so the application simply cannot override or bypass it.

Every other model has more application dependency by comparison:

- **DAC** — the kernel enforces file permissions, but applications often implement additional access logic on top.
- **RBAC** — almost always implemented inside the application itself. Nothing stops a buggy application from skipping the check.
- **ABAC** — usually evaluated by application logic or a middleware policy engine.
- **ACLs** — can be enforced at the OS level (trustworthy) or inside the application (depends entirely on the application).
- **Capabilities** — hardware or kernel-enforced capabilities (like CHERI) are very strong. Software capability systems implemented in userspace depend on the application or runtime to honor them.

The general principle: **the lower in the stack enforcement happens, the less it depends on application logic.** MAC pushed to the kernel or hardware is the extreme end of that spectrum.

---

## Comparing the canonical capability data structure to CapabilityPayload

The canonical capability fields mapped against the application struct:

```go
// CapabilityPayload represents a capability granting a principal permission
// to perform an operation on a specific object in the CAS.
type CapabilityPayload struct {
    ID        string    `json:"id"`
    Hash      string    `json:"hash"`
    Perm      string    `json:"perm"`
    Expires   time.Time `json:"expires"`
    Principal string    `json:"principal,omitempty"`
    Email     string    `json:"email,omitempty"`
    Sig       string    `json:"sig"`
}
```

**Object reference → `Hash`**
Clean direct mapping. The capability points to an immutable, content-addressed object, so there's no ambiguity about what is being protected.

**Rights/permissions mask → `Perm`**
Present, as a string rather than a bitmask. More readable and flexible, at the cost of needing careful validation.

**Unforgeable token → `Sig` + `ID`**
Two distinct cryptographic fields:
- `Sig` is the HMAC-SHA256 that makes the capability unforgeable — covers all authoritative fields together.
- `ID` is a SHA-256 hash of the signing message, giving a deterministic self-verifying identifier — a useful addition beyond the canonical model for referencing, revoking, or auditing a specific capability.

**Expiry/validity → `Expires`**
Present and strongly typed as `time.Time`, better than many implementations that use a raw Unix timestamp or omit it entirely.

**Delegation rights → absent**
Not present, a reasonable simplification if capabilities are only issued by the server.

**Principal → `Principal` (optional)**
Making Principal optional turns the capability into a **bearer token** when omitted — whoever holds it can use it, with no identity check required. The two modes give flexibility between bound credentials and bearer tokens.

**Email → `Email`**
Thoughtfully scoped — explicitly excluded from the signing message and documented as carrying no authority. A convenience annotation that avoids the common mistake of letting human-readable annotations accidentally become part of the security boundary.

**Overall assessment:** A solid, practical capability implementation covering all essential fields, with a clean self-verifying ID, proper expiry handling, and a deliberate and well-documented choice about the identity/bearer token tradeoff.

---

## Does including Principal conflict with multiple users holding capabilities to the same object?

The concern is well-founded but the design handles it correctly. The many-to-one relationship is fully preserved. Nothing prevents multiple capability entries pointing to the same `Hash` with different `Principal` values:

```
Capability 1: { Hash: "abc123", Perm: "read", Principal: "user-A" }
Capability 2: { Hash: "abc123", Perm: "read", Principal: "user-B" }
Capability 3: { Hash: "abc123", Perm: "read", Principal: "" }  // bearer
```

Each is a distinct capability with its own `ID` and `Sig`. The many-to-one relationship lives at the **issuance level** — the mint authority issues separate capabilities per principal — not at the struct level.

**What Principal actually does**

Principal doesn't change the capability into an ACL entry. It adds an optional **binding constraint** — when present, the server can verify that the presenter is who the capability was issued to. The enforcement mode matters:

- If the server verifies Principal against an authenticated session → behaves like a **bound credential**.
- If the server just checks the `Sig` → behaves like a **bearer token** regardless of the field being set.

**The genuine tension**

The only real conflict with pure capability theory is that including Principal at all reintroduces identity as a concern at verification time. A pure capability system would say possession of a valid `Sig` is sufficient. The optional Principal is a pragmatic compromise that lets you choose per-capability how much identity binding you want.

---

## The capabilities are persistent — stored in metadata and loaded into memory on startup

This is an important architectural detail with several implications:

**It resolves the user vs. process question**
Since capabilities survive process restarts, they're clearly not ephemeral process-level capabilities — they're persistent grants that exist independently of any running process. This puts the design firmly in the "capabilities assigned to users" camp, a deliberate and practical choice for an application-level system.

**It mirrors the metadata log architecture**
Hatcheck already uses an append-only log with indexes rebuilt on startup. If capabilities follow the same pattern, you get the same benefits — the log is the source of truth, the in-memory structure is a projection, and history is preserved. Replay the log and you reconstruct the exact capability set.

**Revocation becomes straightforward**
Since capabilities live in the metadata store rather than being handed out as free-floating tokens, revocation is just a log entry — append a revocation event, rebuild the index, and the capability is gone from the in-memory set. You don't have to chase down distributed copies.

**The in-memory set is your enforcement surface**
At verification time the server checks the presented capability against what's loaded in memory:
- The `Sig` guards against forgery of capabilities that were never issued.
- The in-memory set guards against use of capabilities that were revoked after issuance.
- Together they cover both attack surfaces cleanly.

**One consideration**
Since capabilities are loaded on startup, there's a window between when a revocation is written to the log and when it takes effect if the server doesn't reload or invalidate its in-memory set immediately. Revocation should be a live operation that updates the in-memory index directly rather than waiting for the next restart.
