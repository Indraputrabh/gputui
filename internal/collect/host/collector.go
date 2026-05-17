package host

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

// Collector gathers host-level node metrics used in Snapshot.Node.
type Collector interface {
	Collect(ctx context.Context) (model.NodeStat, error)
}

// NewCollector returns an OS-appropriate host collector.
func NewCollector() Collector {
	return newCollector()
}
