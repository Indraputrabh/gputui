package log

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

// Collector reads kernel log sources for GPU and system stability markers.
type Collector interface {
	Collect(ctx context.Context) ([]model.Marker, error)
}

// NewCollector returns an OS-appropriate log collector.
func NewCollector() Collector {
	return newCollector()
}
