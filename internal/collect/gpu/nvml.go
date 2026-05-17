//go:build !linux

package gpu

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

type nvmlCollector struct{}

func (c *nvmlCollector) Source() Source {
	return SourceNVML
}

func (c *nvmlCollector) Collect(ctx context.Context) ([]model.GPUStat, error) {
	_ = ctx
	return nil, &NotImplementedError{
		Source: SourceNVML,
		Detail: "NVML collector is implemented for linux targets only",
	}
}
