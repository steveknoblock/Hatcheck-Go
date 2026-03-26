# Hatcheck

The goal of Hatcheck is to enable the user to store and retrieve immutable data over HTTP to a web page or web application.

Simplicity, conciseness, and minimalism are goals Hatcheck tries to satisfy. The CAS is about as simple as you can get. It stores plain text. The text is used to generate a hash used to identify it in the store. There are four basic operations provided for creating and structuring objects. Tags are provided for organizing objects.

Data is stored in an Object, for which the value cannot be changed or removed from the store. A new “version” of the object is stored the same as any other object, and alongside any other objects. Immutable data has many advantages. A value returned from the store is independent of any other value. The CAS does not know about or store any kind of state. Hatcheck does allow storing some state, such as a Name, which can be changed. The CAS and the metadata are both immutable. The CAS by its nature, and the metadata by its log format. Every change leaves the previous data untouched and a history of changes exists in the metadata.

There are two important stores of data. The object store and the metadata store.

## Content Addressable Store

The Content Addressable Store provides persistence for objects. Objects are immutable. Each time you stash an object to the store, a new object is created by hashing the data. The hash is returned to the user.

For example, if you Stash some text and then it changes and you Stash the new text, a new object is created. It is up to the user of the application making requests to create a  document.

The CAS itself remains completely independent of the metadata system and the web server. It just stores and retrieves text by hash. The meaning of the content is interpreted by the metadata layer and the application.

## Data Types

The data types supported by Hatcheck are: Object, Collection, Relation, and Name. An object is plain text. A Collection or Relation is an object containing JSON specifying relationships between objects--each is a different interpretation of a plain text object.

## Metadata

The objects in the Content Addressable Store are plain text, addressed by hash, and do not contain metadata (except for tags).

Everything else — names, relations, collections, timestamps — lives entirely in the log. The objects in the CAS remain unaware of any of it.

## Event Log

Every Hatcheck operation is written to a log.

At runtime when the server starts, the metadata log is replayed and the NameIndex is built in memory alongside the TagIndex, DateIndex, and RelationIndex (and any other additional index defined by a plugin). From that point on all four indexes coexist in memory and can be queried together, even though their origins are different — tags and dates come from stash entries, names come from name entries, relations from relation entries.

Each log entry consists of an Envelope containing a typed operation.

### Tags

Tags are embedded metadata. They live in the text of plain text objects. Objects are organized by tags. Tags are an exception to the way metadata is stored in the Log because they are embedded metadata.

## Composability

I wanted to make Hatcheck as easy to use as possible and as flexible as possible for a document-leaning content store accessible over the web. I like the idea of composition or composability. 

The types of object and kinds of operations are not completely symmetrical. Object, Collection, and Relation live in the object store. The Name type and operation live in the metadata system.

## Operations

Hatcheck provides four basic operations to store and organize information. The operations give you the building blocks organize any kind of content.

* Stash (Object)
* Collection
* Relation
* Name

The first three basic operations store plain text in the Content Addressable Store and the fourth operation stores names for objects in the log. Names can be changed (are mutable, unlike objects, collections, and relations).

Objects, Collections, and Relations are data, and Names are metadata.

### Stash (Object)

A Stash creates a plain text object in the Object Store.

### Collection

An object containing a list of references to objects. Expects a JSON array of hashes. For example:

```
["a1b2c3", "d4e5f6"]
```

### Relation

An object expressing a relation between objects. Expects a JSON object. For example, an object serving as context for another object:

```
{
    "from": "a1b2c3", 
    "rel": "contextualizes",
    "to": "d4e5f6"
}
```

### Name

A Name creates a persistent, but changeable, human readable handle for an object in the Object Store. It contains a plain text label for the object and the hash of the object it labels. 

Names are mutable, meaning you can update a name by making another entry in the Event Log that will override any previous entries.

It is the only mutable value in Hatcheck (with the exception of tags, which are a different story TODO explain).

## Data Structures

### Object

An Object is the basic unit of storage in the CAS. It is defined by default to be plain text. Everything in the store (persistence layer) is an object (except for a Name, which is metadata and only stored in the Event Log).

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

_Important!_ Names are labels maintained by the metadata system in the Log. To preserve naming, the metadata must be exported along with the objects.

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

There are four built-in indexes: tag, date, name, and relation. Later, if you think of another useful projection, for example, an index by content size, or by which tags co-occur, you just add another index built from the same log. You don't change the log format or lose any history.
This is essentially what a database does internally when you add an index to a table — it builds a separate data structure optimized for a specific query pattern.