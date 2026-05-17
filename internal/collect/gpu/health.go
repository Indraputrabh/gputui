package gpu

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

// HealthCollector extends a GPU backend with hardware health signal queries
// (ECC errors, NVLink status, remapped rows). Backends that support this
// should implement it alongside Collector.
type HealthCollector interface {
	CollectHealth(ctx context.Context) ([]model.GPUHealthSignal, error)
}
