//go:build !linux

package host

import (
	"context"
	"errors"

	"github.com/indraputrabh/gputui/internal/model"
)

type unsupportedCollector struct{}

var errUnsupportedOS = errors.New("host collector is currently implemented for linux only")

func newCollector() Collector {
	return &unsupportedCollector{}
}

func (c *unsupportedCollector) Collect(ctx context.Context) (model.NodeStat, error) {
	_ = ctx
	return model.NodeStat{}, errUnsupportedOS
}
