package metadata

import "encoding/json"

// CapabilityIndex maps principals to the capabilities issued to them.
//
// Bearer capabilities (Principal == "") are grouped under the empty-string
// key rather than dropped — they're real issued capabilities, just not
// bound to anyone. Principals() excludes that bucket since "" isn't a
// principal.
//
// This index only tracks issuance (OpCapability entries). Revocation state
// deliberately does not live here — it lives in RevokedSet. A capability's
// identity and its revocation status are separate concerns, the same way
// Sig (forgery) and RevokedSet (post-issuance revocation) are kept as two
// independent checks at verification time. Callers that need "is this one
// still good" join a CapabilityIndex lookup with a RevokedSet lookup
// themselves, same as CapabilityMiddleware.Protect already does.
type CapabilityIndex struct {
	byPrincipal map[string][]CapabilityPayload
	byID        map[string]CapabilityPayload
	all         []CapabilityPayload
}

func NewCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		byPrincipal: make(map[string][]CapabilityPayload),
		byID:        make(map[string]CapabilityPayload),
	}
}

func (c *CapabilityIndex) Name() string { return "capability" }

func (c *CapabilityIndex) Add(entry Entry) {
	if entry.Op != OpCapability {
		return
	}
	if c.byPrincipal == nil {
		c.byPrincipal = make(map[string][]CapabilityPayload)
		c.byID = make(map[string]CapabilityPayload)
	}

	var cap CapabilityPayload
	if err := json.Unmarshal(entry.Payload, &cap); err != nil {
		return
	}

	c.byPrincipal[cap.Principal] = append(c.byPrincipal[cap.Principal], cap)
	c.byID[cap.ID] = cap
	c.all = append(c.all, cap)
}

// Query returns the IDs of capabilities issued to the given principal, for
// use via the generic GET /query?index=capability&key=<principal> endpoint.
// Pass "" to retrieve bearer-token capability IDs.
func (c *CapabilityIndex) Query(key string) []string {
	caps := c.byPrincipal[key]
	ids := make([]string, len(caps))
	for i, cap := range caps {
		ids[i] = cap.ID
	}
	return ids
}

// QueryRich returns the full CapabilityPayload records issued to the given
// principal, in log (issuance) order.
func (c *CapabilityIndex) QueryRich(key string) []CapabilityPayload {
	return c.byPrincipal[key]
}

// All returns every capability ever issued, in log order.
func (c *CapabilityIndex) All() []CapabilityPayload {
	return c.all
}

// ByID returns a single capability by its ID.
func (c *CapabilityIndex) ByID(id string) (CapabilityPayload, bool) {
	cap, ok := c.byID[id]
	return cap, ok
}

// Principals returns all distinct principals that have been issued at least
// one bound capability. The bearer-token bucket (key "") is excluded.
func (c *CapabilityIndex) Principals() []string {
	result := make([]string, 0, len(c.byPrincipal))
	for p := range c.byPrincipal {
		if p == "" {
			continue
		}
		result = append(result, p)
	}
	return result
}
