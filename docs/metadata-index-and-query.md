Metadata Indexes

Index data structures have a varying number of fields or properties. This suggests that a single index data type cannot cover all index types. If each index had one map that might be possible. The solutions are 

1. pass the index data structure to the index module or define index fields as array elements.

There are index map types for different types of relationships.

1 field
```
go
type NameIndex struct {
	data map[string]string // label -> current hash
}
```

3 fields
```
go
type CapabilityIndex struct {
	byPrincipal map[string][]CapabilityPayload
	byID        map[string]CapabilityPayload
	all         []CapabilityPayload
}
```

1 field
```
type TagIndex struct {
	data map[string][]string
}
```

3 fields
```
type RoleIndex struct {
	byPrincipal map[string]map[string]struct{}    // principal -> set of role names
	byRole      map[string]map[string]struct{}    // role -> set of principals
	grants      map[string]map[RoleGrant]struct{} // role -> set of capability templates
}
```
3 fields
```
type RelationIndex struct {
	byFrom map[string][]RelationPayload
	byTo   map[string][]RelationPayload
	byRel  map[string][]RelationPayload
}
```

1 field
```
type DateIndex struct {
	data map[string][]string
}
```

And index is a data structure that allows for efficient retrieval of information based on specific keys or attributes. In this context, the `TagIndex`, `RoleIndex`, `RelationIndex`, and `DateIndex` structs are designed to facilitate quick lookups and associations between different entities in a system.	


Each index type serves a specific purpose:
- `TagIndex`: This index maps tags (as strings) to a list of associated entities (also as strings). It allows for efficient retrieval of all entities associated with a particular tag.

Each index type has a constructor function that initializes the underlying data structures, ensuring they are ready for use. The `NewTagIndex`, `NewRoleIndex`, `NewRelationIndex`, and `NewDateIndex` functions create new instances of their respective index types with properly initialized maps.


func NewTagIndex() *TagIndex {
	return &TagIndex{
		data: make(map[string][]string),
	}
}

func NewRoleIndex() *RoleIndex {
	return &RoleIndex{
		byPrincipal: make(map[string]map[string]struct{}),
		byRole:      make(map[string]map[string]struct{}),
		grants:      make(map[string]map[RoleGrant]struct{}),
	}
}

func NewRelationIndex() *RelationIndex {
	return &RelationIndex{
		byFrom: make(map[string][]RelationPayload),
		byTo:   make(map[string][]RelationPayload),
		byRel:  make(map[string][]RelationPayload),
	}
}

func NewDateIndex() *DateIndex {
	return &DateIndex{
		data: make(map[string][]string),
	}
}

