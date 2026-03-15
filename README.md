# Hatcheck

A minimal content addressable storage system (CAS) exposed to a web interface.

Your content. On the web. As JSON objects. A simple content addressable storage system exposed to a web interface.

## Getting Started

Hatcheck is programmed in Go. The minimum version required to run Hatcheck is Go 1.21.6

## User Interface

Hatcheck comes with a user interface for adding content to the object store, including tags, which can be used to filter the list of objects by hash.

## Object Store API

Hatcheck exposes a RESTful, web API, enabling communication with the object store over HTTP requests.

On the first request to store an object the objects/ folder will be automatically created in the application root folder.

Endpoints

/stash          Create a new object in the object store

/fetch          Get the contents of an object from the object store.

/list route     Returns a JSON array of Object hashes.

/ui/ route — serves static files from the ui/ directory

## Why Claude AI?

The reason that tech generally--and coders in particular--see LLMs differently than everyone else is that in the creative disciplines, LLMs take away the most soulful human parts of the work and leave the drudgery to you," Dash says, "And in coding, LLMs take away the drudgery and leave the human soulful parts to you." -- Anil Dash

Another reason is that software is made using languages and ideas that are very mathematical, with logic governing decisions, and there is a long history of openness, sharing code, using libraries is encouraged in programming. Copying is good and takes away from no one. Automation has been essential to computer programming and use since the beginning. 



