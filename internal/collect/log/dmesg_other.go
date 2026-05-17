//go:build !linux

package log

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

type stubCollector struct{}

func newCollector() Collector {
	return &stubCollector{}
}

func (c *stubCollector) Collect(_ context.Context) ([]model.Marker, error) {
	return nil, nil
}
