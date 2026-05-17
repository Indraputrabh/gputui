package rules

import (
	"fmt"

	"github.com/indraputrabh/gputui/internal/model"
)

// ECCError fires on volatile ECC error counters reported by NVML.
//   - Uncorrectable > 0 -> critical (a single uncorrectable VRAM error
//     can corrupt computation; replace the GPU or report the row).
//   - Correctable > 0 -> info (ECC was designed to correct these
//     silently; the count is informational, not actionable on its own).
type ECCError struct{}

// DependsOnHealthOnly signals the evaluator that this rule is a pure
// function of snap.HealthSignals, so results can be cached and replayed
// until those signals change.
func (r *ECCError) DependsOnHealthOnly() bool { return true }

func (r *ECCError) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint
	for _, sig := range snap.HealthSignals {
		if sig.ECCUncorrectableVolatile == 0 && sig.ECCCorrectableVolatile == 0 {
			continue
		}

		severity := "info"
		confidence := 0.85
		summary := fmt.Sprintf("GPU%d reports %d correctable ECC events (auto-corrected by hardware; informational)", sig.Index, sig.ECCCorrectableVolatile)
		if sig.ECCUncorrectableVolatile > 0 {
			severity = "critical"
			confidence = 0.95
			summary = fmt.Sprintf(
				"GPU%d reports %d uncorrectable ECC events -- VRAM contents may be corrupt; consider draining and replacing the GPU",
				sig.Index, sig.ECCUncorrectableVolatile)
		}

		hints = append(hints, model.Hint{
			Name:       "gpu-ecc-errors",
			Category:   "hardware_health",
			Severity:   severity,
			Confidence: confidence,
			Summary:    summary,
			Evidence: []model.Evidence{
				{Metric: "ecc_uncorrectable_volatile", Value: float64(sig.ECCUncorrectableVolatile), Unit: "errors"},
				{Metric: "ecc_correctable_volatile", Value: float64(sig.ECCCorrectableVolatile), Unit: "errors"},
			},
		})
	}
	return hints
}
