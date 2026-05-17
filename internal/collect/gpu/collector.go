package gpu

import (
	"context"
	"fmt"

	"github.com/indraputrabh/gputui/internal/model"
)

// Source represents the GPU metric backend used by the collector.
type Source string

const (
	SourceNVML Source = "nvml"
	SourceDCGM Source = "dcgm"
)

// Collector is the backend-agnostic GPU collector contract.
type Collector interface {
	Collect(ctx context.Context) ([]model.GPUStat, error)
	Source() Source
}

// ProcessCollector extends a GPU backend with per-process attribution.
// Backends that support this should implement it alongside Collector.
type ProcessCollector interface {
	CollectProcesses(ctx context.Context) ([]model.ProcStat, error)
}

// NewCollector creates a collector for the requested backend.
func NewCollector(source Source) (Collector, error) {
	switch source {
	case SourceNVML:
		return &nvmlCollector{}, nil
	case SourceDCGM:
		return &dcgmCollector{}, nil
	default:
		return nil, fmt.Errorf("unknown gpu source %q (expected: %q or %q)", source, SourceNVML, SourceDCGM)
	}
}
