package rules

import (
	"math"

	"github.com/indraputrabh/gputui/internal/hints/shared"
	"github.com/indraputrabh/gputui/internal/model"
)

const (
	cpuBoundGPUUtilCeil  = 60.0
	cpuBoundCPUUserFloor = 50.0
)

// CPUBound fires when GPU utilisation is low while CPU user time is high,
// suggesting the workload is limited by CPU-side preprocessing.
type CPUBound struct{}

func (r *CPUBound) Evaluate(snap model.Snapshot) []model.Hint {
	return r.eval(snap, 0, false)
}

// EvaluateWithContext uses a precomputed mean GPU utilisation so multiple
// util-dependent rules don't re-loop over snap.GPUs.
func (r *CPUBound) EvaluateWithContext(snap model.Snapshot, ctx *shared.EvalContext) []model.Hint {
	if ctx == nil || !ctx.HasGPUs {
		return r.Evaluate(snap)
	}
	return r.eval(snap, ctx.MeanGPUUtil, true)
}

func (r *CPUBound) eval(snap model.Snapshot, meanUtil float64, haveMean bool) []model.Hint {
	if len(snap.GPUs) == 0 {
		return nil
	}

	avgGPU := meanUtil
	if !haveMean {
		for _, g := range snap.GPUs {
			avgGPU += g.UtilPct
		}
		avgGPU /= float64(len(snap.GPUs))
	}

	cpuUser := snap.Node.CPUUser
	if avgGPU >= cpuBoundGPUUtilCeil || cpuUser <= cpuBoundCPUUserFloor {
		return nil
	}

	gap := cpuUser - avgGPU
	confidence := math.Min(0.5+(gap/100.0), 0.95)

	return []model.Hint{{
		Name:       "potential-cpu-bound-preprocessing",
		Category:   "cpu_bound",
		Severity:   "warning",
		Confidence: confidence,
		Summary:    "GPU utilisation is low while CPU user time is high -- consistent with CPU-bound preprocessing if a GPU workload is expected",
		Evidence: []model.Evidence{
			{Metric: "avg_gpu_util_pct", Value: avgGPU, Threshold: cpuBoundGPUUtilCeil, Unit: "%"},
			{Metric: "cpu_user_pct", Value: cpuUser, Threshold: cpuBoundCPUUserFloor, Unit: "%"},
		},
	}}
}
