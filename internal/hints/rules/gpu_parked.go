package rules

import (
	"fmt"

	"github.com/indraputrabh/gputui/internal/model"
)

const (
	parkedPerfStateFloor    = 8    // P-state 8+: idle / clocks parked
	parkedVRAMUsedMBFloor   = 1024 // 1 GiB allocated -> a model is loaded
	parkedKnownStateUnknown = 32   // NVML's PSTATE_UNKNOWN sentinel
)

// GPUParked fires when a GPU has a workload's worth of VRAM allocated
// but the driver has dropped it to a low-power performance state. The
// model is in memory, the kernels just aren't running -- a clear
// "loaded but idle" signal that doesn't depend on heuristic VRAM-%
// thresholds (the previous rule got this wrong by treating high VRAM
// utilisation alone as memory pressure).
type GPUParked struct{}

func (r *GPUParked) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint
	for _, g := range snap.GPUs {
		if g.PerfState < parkedPerfStateFloor || g.PerfState >= parkedKnownStateUnknown {
			continue
		}
		if g.VRAMUsedMB < parkedVRAMUsedMBFloor {
			continue
		}

		hints = append(hints, model.Hint{
			Name:       "gpu-parked",
			Category:   "gpu_state",
			Severity:   "warning",
			Confidence: 0.90,
			Summary: fmt.Sprintf(
				"GPU%d is in low-power state P%d with %d MB VRAM allocated -- workload is loaded but no kernels are running",
				g.Index, g.PerfState, g.VRAMUsedMB),
			Evidence: []model.Evidence{
				{Metric: "perf_state", Value: float64(g.PerfState), Unit: "P-state"},
				{Metric: "vram_used_mb", Value: float64(g.VRAMUsedMB), Threshold: parkedVRAMUsedMBFloor, Unit: "MB"},
				{Metric: "gpu_util_pct", Value: g.UtilPct, Unit: "%"},
				{Metric: "mem_util_pct", Value: g.MemUtilPct, Unit: "%"},
			},
		})
	}
	return hints
}
