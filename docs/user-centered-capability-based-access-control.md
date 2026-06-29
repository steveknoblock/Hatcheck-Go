# User Centered Capability Access Control

## People have capabilities not objects

The guiding idea of this design is: People have capabilities not objects.

I have the capability to read this object.

I do not have the capability to write to this object.

{user} -has read capability-> {object}

Each capability contains one grant of permissions, such as Read or Write.

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

















