package rules

import (
	"fmt"
	"sort"

	"github.com/indraputrabh/gputui/internal/model"
)

const (
	// thermalViolationOutlierFloorNs sets a baseline below which we
	// don't even consider the violation counter interesting (matters
	// because counters are cumulative since driver load -- a 100ms
	// blip from yesterday is noise).
	thermalViolationOutlierFloorNs uint64 = 2 * 1e9 // 2 seconds

	// thermalViolationOutlierMultiplier flags GPUs whose thermal
	// violation counter is N× the fleet median or higher.
	thermalViolationOutlierMultiplier float64 = 4.0
)

// ThermalViolationOutlier flags a GPU whose cumulative thermal-policy
// enforcement time is significantly higher than its peers'.
//
// Reads ViolationThermalNs which is the driver's own count of
// nanoseconds spent enforcing the thermal cap -- a directly
// authoritative signal that this specific GPU has actually been
// throttled more than the rest, rather than just running hotter under
// a different workload.
type ThermalViolationOutlier struct{}

// DependsOnHealthOnly: this rule reads only HealthSignals and is a
// pure function of them; the evaluator can replay cached output when
// the signal hash hasn't changed.
func (r *ThermalViolationOutlier) DependsOnHealthOnly() bool { return true }

func (r *ThermalViolationOutlier) Evaluate(snap model.Snapshot) []model.Hint {
	if len(snap.HealthSignals) < 2 {
		return nil
	}

	values := make([]float64, 0, len(snap.HealthSignals))
	for _, sig := range snap.HealthSignals {
		values = append(values, float64(sig.ViolationThermalNs))
	}
	median := medianFloat64Sorted(values)

	var hints []model.Hint
	for _, sig := range snap.HealthSignals {
		if sig.ViolationThermalNs < thermalViolationOutlierFloorNs {
			continue
		}
		// Need a non-trivial fleet baseline; if everyone is roughly
		// equal we don't flag anyone.
		threshold := median * thermalViolationOutlierMultiplier
		if threshold < float64(thermalViolationOutlierFloorNs) {
			threshold = float64(thermalViolationOutlierFloorNs)
		}
		if float64(sig.ViolationThermalNs) < threshold {
			continue
		}

		hints = append(hints, model.Hint{
			Name:       "thermal-violation-outlier",
			Category:   "hardware_health",
			Severity:   "warning",
			Confidence: 0.85,
			Summary: fmt.Sprintf(
				"GPU%d cumulative thermal-cap enforcement time is %.1fs (fleet median %.1fs) -- this GPU has been thermally limited more than its peers",
				sig.Index,
				float64(sig.ViolationThermalNs)/1e9,
				median/1e9,
			),
			Evidence: []model.Evidence{
				{Metric: "violation_thermal_s", Value: float64(sig.ViolationThermalNs) / 1e9, Threshold: threshold / 1e9, Unit: "s"},
				{Metric: "fleet_median_violation_thermal_s", Value: median / 1e9, Unit: "s"},
			},
		})
	}
	return hints
}

func medianFloat64Sorted(vals []float64) float64 {
	switch len(vals) {
	case 0:
		return 0
	case 1:
		return vals[0]
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2.0
	}
	return sorted[n/2]
}
