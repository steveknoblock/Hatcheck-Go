# Hatcheck

The goal of Hatcheck is to enable the user to store and retrieve immutable data over HTTP to a web page or web application.

Simplicity, conciseness, and minimalism are goals Hatcheck tries to satisfy. The CAS is about as simple as you can get. It stores plain text. The text is used to generate a hash used to identify it in the store. There are four basic operations provided for creating and structuring objects. Tags are provided for organizing objects.

Data is stored in an Object, for which the value cannot be changed or removed from the store. A new “version” of the object is stored the same as any other object, and alongside any other objects. Immutable data has many advantages. A value returned from the store is independent of any other value. The CAS does not know about or store any kind of state. Hatcheck does allow storing some state, such as a Name, which can be changed. The CAS and the metadata are both immutable. The CAS by its nature, and the metadata by its log format. Every change leaves the previous data untouched and a history of changes exists in the metadata.

## Metadata

### Tags


## Composability

I wanted to make Hatcheck as easy to use as possible and as flexible as possible for a document-leaning content store accessible over the web. I like the idea of composition or composability. 

The types of object and kinds of operations are not completely symmetrical. Object, Collection, and Relation live in the object store. The Name type and operation live in the metadata system.

## Operations

Access to Hatcheck data is made through four basic operations. All four operations store plain text in the CAS.

### Name

A metadata label giving a name to another object containing a reference to the named object. A name is mutable, meaning you can update it making another entry that will override any previous entries. It is the only mutable value in Hatcheck (with the exception of tags, which are a different story TODO explain).

A name gives a persistent human readable handle for referencing an object in the metadata store.

### Object

An object. Expects plain text by default, but can be JSON.

### Collection

An object containing a list of references to objects. Expects JSON.

### Relation

An object expressing a relation between objects. Expects JSON.

## Common Data Structures

Common data structures can be composed from a sequence of basic operations.

### Text Document

Create Name
Returns hash identifier
Create Object
Stash text

### Collection

Create Name
Returns hash identifier 
Create Collection with hashes of all objects in collection.

No append. To append you must create a new collection object.

### Data Graph

Create Name
Returns hash identifier 
Create Object
Stash text
Create Object
Stash text
Create Relation
“Is context for” hashA hashB

### Structured Document

Create Object
Returns hash identifier

Create Object
Returns hash identifier

Create Collection with hashes of all objects in collection.

Create Name
Returns hash identifier to point to the collection.

You could also create a named collection of names that reference objects. 

### Blog/Time Structured Document

Create Object
Returns hash identifier

Create Object
Returns hash identifier

Create Collection with hashes of all objects in collection.

Create Name
Returns hash identifier to point to the collection.


## Data Structures

### Object

An Object is the basic unit of storage in the CAS. It is assumed to be a text object. Everything in the store (persistence layer) is an object (except for a Name).

The other data types are Objects containing different kinds of JSON data that help with organizing objects in the store.

### Collection

A Collection is a JSON array of object identifiers (hashes). 

```
["a1b2c3", "d4e5f6"]
```

### Relation

A Relation is a JSON object containing identifiers for two related objects and a text label for describing the kind of relationship.
```
{
    "from": "a1b2c3",
    "rel": "contextualizes",
    "to": "d4e5f6"
}
```
### Name

A Name is a label that provides a human-readable label for any kind of object and is not stored in the CAS, but exists only in the metadata log.

_Important!_ Names are labels maintained by the metadata system. To preserve naming the metadata would have to be exported with the objects.

## Tags

Objects are organized in metadata by tags.

## Content Addressable Store

The Content Addressable Store provides persistence for objects. Objects are immutable. Each time you stash an object to the store, a new object is created by hashing the data. The hash is returned to the user.

For example, if you Stash some text and then it changes and you Stash the new text, a new object is created. It is up to the user of the application making requests to create a  document.

The CAS itself remains completely independent of the metadata system and the web server. It just stores and retrieves text by hash. The meaning of the content is interpreted by the metadata layer and the application.

The data types supported by Hatcheck are different interpretations of JSON data. Currently supported types are: Object, Collection, and Relation.

## Metadata

## Event Sourcing

Event sourcing is a pattern from software architecture where instead of storing the current state of something, the sequence of events that led to that state are stored. The current state is derived by replaying the events.

The name comes from the idea that events are the source of truth — not the current state. The state is just a projection, a convenience computed from the event log.

An example is a bank account. Instead of storing a single balance:

balance: $450

You store every transaction:

opened:     $0
deposited:  $500
withdrew:   $50

The balance of $450 is derived by replaying the events. If you need the balance at any point in the past you just replay up to that point.
It has a long history — accounting ledgers have worked this way for centuries. You never erase an entry, you only add new ones. An audit trail is a natural byproduct.

For Hatcheck it maps naturally — each stash is an event, the metadata index is the current state derived from replaying those events. The log is the single source of truth.

## Index Plugins

That's one of the powerful properties of event sourcing — you can build as many projections as you need from the same log. The log is the source of truth and the indexes are just different views over it.

Two indexes built on startup by replaying the log:
Tags to objects — for querying by tag:
gotagIndex map[string][]string
// tagIndex["ideas"] = ["a1b2c3", "d4e5f6", "g7h8i9"]
Date to objects — for querying by when tags were assigned:
godateIndex map[string][]string
// dateIndex["2026-03-12"] = ["a1b2c3", "d4e5f6"]
You could query either independently or combine them:

All objects tagged #ideas — use tagIndex
All objects stashed today — use dateIndex
All objects tagged #ideas stashed this week — intersect both results

And later if you think of another useful projection — say an index by content size, or by which tags co-occur — you just add another index built from the same log. You don't change the log format or lose any history.
This is essentially what a database does internally when you add an index to a table — it builds a separate data structure optimized for a specific query pattern.