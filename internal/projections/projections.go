// projections/projection.go
package projections

import (
	"context"

	"github.com/steveknoblock/hatcheck-go/events"
)

type Projection interface {
	Name() string
	Handle(ctx context.Context, event events.Event) error
	Query(ctx context.Context, queryType string, params map[string]interface{}) (interface{}, error)
	Version() int
}
