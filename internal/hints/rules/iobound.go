package rules

import (
	"math"

	"github.com/indraputrabh/gputui/internal/hints/shared"
	"github.com/indraputrabh/gputui/internal/model"
)

const (
	ioBoundGPUUtilCeil = 60.0
	ioBoundIowaitFloor = 15.0
)

// IOBound fires when GPU utilisation is low while IO wait is high,
// suggesting the workload is bottlenecked by data loading (storage or
// network I/O cannot keep the GPUs fed).
type IOBound struct{}

func (r *IOBound) Evaluate(snap model.Snapshot) []model.Hint {
	return r.eval(snap, 0, false)
}

func (r *IOBound) EvaluateWithContext(snap model.Snapshot, ctx *shared.EvalContext) []model.Hint {
	if ctx == nil || !ctx.HasGPUs {
		return r.Evaluate(snap)
	}
	return r.eval(snap, ctx.MeanGPUUtil, true)
}

func (r *IOBound) eval(snap model.Snapshot, meanUtil float64, haveMean bool) []model.Hint {
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

	iowait := snap.Node.CPUIowait
	if avgGPU >= ioBoundGPUUtilCeil || iowait <= ioBoundIowaitFloor {
		return nil
	}

	gap := iowait - ioBoundIowaitFloor + (ioBoundGPUUtilCeil - avgGPU)
	confidence := math.Min(0.5+(gap/100.0), 0.90)

	return []model.Hint{{
		Name:       "potential-io-bound-pipeline",
		Category:   "io_bound",
		Severity:   "warning",
		Confidence: confidence,
		Summary:    "GPU utilisation is low while IO wait is high -- consistent with an IO-bound data pipeline if a GPU workload is expected",
		Evidence: []model.Evidence{
			{Metric: "avg_gpu_util_pct", Value: avgGPU, Threshold: ioBoundGPUUtilCeil, Unit: "%"},
			{Metric: "cpu_iowait_pct", Value: iowait, Threshold: ioBoundIowaitFloor, Unit: "%"},
		},
	}}
}
