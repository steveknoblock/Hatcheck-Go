## Capability Authorization System





| Route | Method | Required Perm | Hash Check | Description |
|---|---|---|---|---|
| `/fetch` | GET | read | yes | Retrieve a CAS object by hash |
| `/list` | GET | read | — | List all objects with tags |
| `/query` | GET | read | — | Query an index by key |
| `/namespaces` | GET | read | — | List all name namespaces |
| `/names` | GET | read | — | List names within a namespace |
| `/stash` | POST | write | yes | Store content in the CAS |
| `/name` | POST | write | yes | Create or update a named pointer |
| `/collection` | POST | write | — | Store a collection of hashes |
| `/export` | GET | admin | — | Export a namespace as tar.gz |
| `/import` | POST | admin | — | Import a tar.gz archive |
| `/capability` | POST | admin | — | Issue a signed capability |
| `/capability/revoke` | POST | admin | — | Revoke a capability by ID |

The hash check column indicates which routes verify that the capability's `Hash` field matches the specific object being accessed. Routes without a hash check operate on the store as a whole rather than a specific object.
