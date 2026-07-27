all information about objects is stored in a metadata store using an append-only log

different types of metadata are stored

an index is built from the metadata log for each indexed type (a type does not have to be indexed, but without an index it cannot be queried)

indexes are rebuilt from the metadata log on application start up

That's the append-only-log-as-source-of-truth design paying off exactly as intended.

There are Objects containing data, and there are types of metadata, Capabilities, Principals, Date, Name, Tag, Relation.

## Metadata

Metadata about objects is accessed through indexes generated from the metadata log. An index makes the object associated with the metadata queryable. For example, you can ask for an object by name, or all the objects tagged with a particular tag.

Capability
Date
Principals
Name
Relation
Tag


Create and index for roles RoleIndex
Using the same interface as CapabilityIndex
Support queries principals for roles and roles for principals

Two new operations: OpRoleAssign and OpRoleRevoke

New Payloads for log: RoleAssignPayload and RoleRevokePayload

New methods on Store: AppendRoleAssign, AppendRoleRevoke, RolesForPrincipal, PrincipalsForRole.

You can see the pattern, create a new log entry for the new type, create operations for the new type, create methods on the store for appending and querying. Then construct an index for the new data type.


For each type the log handles there is an operation constant, and a payload structure. The payload is the actual data store in a log entry or envelope.

It is relativity simple to add a new data type to Hatcheck, it requires creating a data structure for the payload tailored to the specific type, any operations necessary to append the data to the log, and methods to append the data to the log. Then create queries necessary to access the data.


Metadata layer is in place; server is aware of it



How do I get changed code into the Ubuntu container?

Log in to container and git clone or pull from github.

git fetch
git checkout feature-branch



We discussed associating a role with a set of capabilities. A grant of a role to a user would create that set of capabilities for the user.



The access control systems defines Principals (users), Capabilities, Roles, and Grants.

Roles are simply containers for collections of Principals. A Principal is assigned a Role through making a Grant. The Grant generates or associates a set of Capabilities to the Principal.


Works out to:

A Role is a container for Principals and a definition of what Capabilities the container's members can hold.

Roles are store in the RolesIndex. Queries on the index are: byRole and byPrincipal.

A Grant is the definition of Capabilities for a Role.

Add this capability template to the role's definition

Assigning a Role reads the role's template (a set of role-grant pairs) and mints actual capabilities for a Principal.

This conflicts with the idea of a grant. Option to change to RoleTemplate

Perhaps it makes sense to Grant Capabilities to a Role?

Assign a Role to a Principal?

I looked in the Postgres documents for GRANT

"This variant of the GRANT command gives specific privileges on a database object to one or more roles. "

"This variant of the GRANT command grants membership in a role to one or more other roles"

GRANT INSERT ON films TO editors;

GRANT admins TO joe;

The first meaning of GRANT in PostgreSQL is the same as Claude wrote. It means granting capabilities to a role.

The second "grants" membership in a role. That sounds really weird.

So, the following fits with the idea that a GRANT giving privileges to a role:

A Grant is the definition of Capabilities for a Role.

Essentially "Add this capability template to the role's definition."

Assigning a Role reads the role's template (a set of role-grant pairs) and mints actual capabilities for a Principal.

So, I agree that "grant" means adding to the definition of capabilities for a role. And that "assign" means associate a principal with a role.
