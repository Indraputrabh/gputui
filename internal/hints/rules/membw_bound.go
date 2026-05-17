package rules

import (
	"fmt"

	"github.com/indraputrabh/gputui/internal/model"
)

const (
	membwGPUUtilCeil  = 50.0
	membwMemUtilFloor = 80.0
)

// MemoryBandwidthBound fires when the SMs are mostly idle while the
// memory controller is saturated -- the canonical low-arithmetic-
// intensity kernel pattern. NVML splits its utilisation counter into
// .gpu (SM occupancy fraction over the sample window) and .memory
// (memory controller busy fraction), so we get this signal directly
// instead of inferring it from temp/power.
//
// Distinct from CPU-bound (high cpu_user) and IO-bound (high iowait):
// here the GPU itself reports it's stuck waiting on memory.
type MemoryBandwidthBound struct{}

func (r *MemoryBandwidthBound) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint
	for _, g := range snap.GPUs {
		if g.UtilPct >= membwGPUUtilCeil {
			continue
		}
		if g.MemUtilPct < membwMemUtilFloor {
			continue
		}

		hints = append(hints, model.Hint{
			Name:       "memory-bandwidth-bound",
			Category:   "workload_pattern",
			Severity:   "warning",
			Confidence: 0.85,
			Summary: fmt.Sprintf(
				"GPU%d compute is idle (util %.0f%%) while memory subsystem is saturated (mem util %.0f%%) -- consistent with a memory-bandwidth-bound kernel",
				g.Index, g.UtilPct, g.MemUtilPct),
			Evidence: []model.Evidence{
				{Metric: "gpu_util_pct", Value: g.UtilPct, Threshold: membwGPUUtilCeil, Unit: "%"},
				{Metric: "mem_util_pct", Value: g.MemUtilPct, Threshold: membwMemUtilFloor, Unit: "%"},
			},
		})
	}
	return hints
}
