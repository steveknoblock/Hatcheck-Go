# Metadata Log

## Append Only Log

This could live in the metadata log as a new operation type, consistent with the existing append-only architecture. Hatcheck already uses an append-only log with indexes rebuilt on startup for its content metadata. 

## Operations

## Indexing and Indexes

An index is a projection from the log. Indexes only live in memory.

That's one of the powerful properties of event sourcing — you can build as many projections as you need from the same log. The log is the source of truth and the indexes are just different views over it.

There are four built-in indexes: tag, date, name, and relation.

And index is a data structure that allows for efficient retrieval of information based on specific keys or attributes. In this context, the `TagIndex`, `RoleIndex`, `RelationIndex`, and `DateIndex` structs are designed to facilitate quick lookups and associations between different entities in a system.	

Each index type serves a specific purpose:

`TagIndex`: This index maps tags (as strings) to a list of associated objects (also as strings). It allows for efficient retrieval of all objects associated with a particular tag.

`{name} --> {hash}`

A name is a text string giving a stored object a name.

`{kind} --> {hash}`

The kind of object the hash stores.

`{created} --> {hash}`

The date the hash object was created.

`{date} --> {hash}`

I don't know what this is.

`{tag} --> {hash}`

A tag name for the object.

## Queries

A RoleIndex following the same pattern as your existing indexes — built from the log on startup, queryable by principal or by role. Two useful queries: