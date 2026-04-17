// projections/content_index.go
package projections

import (
	"context"
	"sync"

	"github.com/steveknoblock/hatcheck-go/events"
)

type ContentIndexProjection struct {
	mu          sync.RWMutex
	contentHash map[string]ContentMetadata
}

type ContentMetadata struct {
	Hash      string
	Size      int64
	StashedAt int64
	Metadata  map[string]interface{}
}

func (cip *ContentIndexProjection) Handle(ctx context.Context, event events.Event) error {
	switch event.Type {
	case "content.stashed":
		var data struct {
			Hash string `json:"hash"`
			Size int64  `json:"size"`
		}
		if err := event.UnmarshalPayload(&data); err != nil {
			return err
		}
		// ...
	}
	return nil
}
