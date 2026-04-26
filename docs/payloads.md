# Payloads

| Type      |
| Hash.    

Stash
----------------
Hash string
Size integer
Tags list

Name
----------------
Hash string
Label string
Hashes list


Collection
----------------
Hash string
Hashes list

Relation
----------------
Hash
From
Rel
To

Capability
---------------
Hash
Capability



// --- Payload structs ---

type StashPayload struct {
	Hash string   `json:"hash"`
	Size int      `json:"size"`
	Tags []string `json:"tags"`
}

type CollectionPayload struct {
	Hash   string   `json:"hash"`
	Hashes []string `json:"hashes"`
}

type RelationPayload struct {
	Hash string `json:"hash"`
	From string `json:"from"`
	Rel  string `json:"rel"`
	To   string `json:"to"`
}

type NamePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}