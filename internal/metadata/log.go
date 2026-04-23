package metadata

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"
)

// --- Log entry envelope ---

// Entry is the envelope for every log event.
type Entry struct {
	Op      string          `json:"op"`
	Created time.Time       `json:"created"`
	Payload json.RawMessage `json:"payload"`
}

// Op constants.
const (
	OpStash            = "stash"
	OpCollection       = "collection"
	OpRelation         = "relation"
	OpNameCreate       = "name-create"
	OpNameUpdate       = "name-update"
	OpCapability       = "capability"
	OpCapabilityRevoke = "capability-revoke"
)

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

// CapabilityPayload represents a capability granting a principal permission
// to perform an operation on a specific object in the CAS.
//
// The ID field is a SHA-256 hash of the canonical signing message, providing
// a deterministic, self-verifying identifier for the capability.
//
// The Sig field is an HMAC-SHA256 over the canonical signing message using
// the server secret, covering ID, Hash, Perm, Principal, and Expires together.
//
// Principal is optional; when omitted the capability acts as a bearer token.
type CapabilityPayload struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Perm      string    `json:"perm"`
	Expires   time.Time `json:"expires"`
	Principal string    `json:"principal,omitempty"`
	Sig       string    `json:"sig"`
}

// CapabilityRevokePayload records the explicit revocation of a capability.
// Reason is optional but recommended for audit purposes.
// Revoked records when the revocation was decided, which may differ from
// Entry.Created if the entry is written after the fact.
type CapabilityRevokePayload struct {
	ID      string    `json:"id"`
	Reason  string    `json:"reason,omitempty"`
	Revoked time.Time `json:"revoked"`
}

type NamePayload struct {
	Label string `json:"label"`
	Hash  string `json:"hash"`
}

// --- Capability helpers ---

// capabilityMessage returns the canonical byte representation of a capability's
// signed fields. All fields are included to prevent substitution attacks.
func capabilityMessage(id, hash, perm, principal string, expires time.Time) []byte {
	return []byte(id + ":" + hash + ":" + perm + ":" + principal + ":" + expires.UTC().Format(time.RFC3339))
}

// CapabilityID computes the deterministic identifier for a capability from
// its content fields. The ID is a SHA-256 hash of the canonical message
// of all fields except the ID itself.
func CapabilityID(hash, perm, principal string, expires time.Time) string {
	msg := []byte(hash + ":" + perm + ":" + principal + ":" + expires.UTC().Format(time.RFC3339))
	sum := sha256.Sum256(msg)
	return hex.EncodeToString(sum[:])
}

// SignCapability issues a new CapabilityPayload signed with the provided key.
func SignCapability(key []byte, hash, perm, principal string, expires time.Time) CapabilityPayload {
	id := CapabilityID(hash, perm, principal, expires)
	msg := capabilityMessage(id, hash, perm, principal, expires)
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return CapabilityPayload{
		ID:        id,
		Hash:      hash,
		Perm:      perm,
		Expires:   expires,
		Principal: principal,
		Sig:       sig,
	}
}

// VerifyCapability checks the signature, expiry, and optionally the principal
// of a capability. It does not check revocation; callers should consult the
// RevokedSet after a successful verification.
func VerifyCapability(key []byte, cap CapabilityPayload, principal string) bool {
	// Check expiry first — cheap, no crypto needed.
	if time.Now().UTC().After(cap.Expires.UTC()) {
		return false
	}

	// Check principal if the capability is bound.
	if cap.Principal != "" && cap.Principal != principal {
		return false
	}

	// Recompute and compare signature in constant time.
	msg := capabilityMessage(cap.ID, cap.Hash, cap.Perm, cap.Principal, cap.Expires)
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(cap.Sig))
}

// --- Revocation index ---

// RevokedSet is a concurrency-safe in-memory set of revoked capability IDs.
// It is built from the log at startup and updated as new revocation entries
// arrive during the lifetime of the server.
type RevokedSet struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

// NewRevokedSet returns an initialised RevokedSet ready for use.
func NewRevokedSet() *RevokedSet {
	return &RevokedSet{ids: make(map[string]struct{})}
}

// Add marks a capability ID as revoked. Safe for concurrent use.
func (r *RevokedSet) Add(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ids[id] = struct{}{}
}

// IsRevoked reports whether a capability ID has been revoked. Safe for concurrent use.
func (r *RevokedSet) IsRevoked(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.ids[id]
	return ok
}

// --- ParseTags ---

var hashtagRe = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

func ParseTags(content string) []string {
	matches := hashtagRe.FindAllStringSubmatch(content, -1)
	seen := make(map[string]bool)
	tags := []string{}
	for _, m := range matches {
		tag := strings.ToLower(m[1])
		if !seen[tag] {
			seen[tag] = true
			tags = append(tags, tag)
		}
	}
	return tags
}
