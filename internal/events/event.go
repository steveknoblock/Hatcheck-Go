// events/event.go
// Package events defines the structure of domain events used in the events system.

package events

import (
	"encoding/json"
	"time"
)

// Event represents a domain event with a type and payload.
type Event struct {
	ID      string          `json:"id"`      // Unique event identifier
	Type    string          `json:"type"`    // Event type ("stash", "fetch")
	Created time.Time       `json:"created"` // RFC3339 timestamp of when the event occurred
	Payload json.RawMessage `json:"payload"` // Event-specific data
}
