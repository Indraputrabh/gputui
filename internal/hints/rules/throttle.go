package rules

import (
	"fmt"
	"strings"

	"github.com/indraputrabh/gputui/internal/model"
)

// NVML clocks-throttle bitmap constants. Mirrored here so the rule
// stays usable on platforms that don't compile with the gonvml import
// path (tests, darwin builds). Values are stable in nvml.h.
const (
	throttleNone                      uint64 = 0
	throttleGpuIdle                   uint64 = 0x0000000000000001
	throttleApplicationsClocksSetting uint64 = 0x0000000000000002
	throttleSwPowerCap                uint64 = 0x0000000000000004
	throttleHwSlowdown                uint64 = 0x0000000000000008
	throttleSyncBoost                 uint64 = 0x0000000000000010
	throttleSwThermalSlowdown         uint64 = 0x0000000000000020
	throttleHwThermalSlowdown         uint64 = 0x0000000000000040
	throttleHwPowerBrakeSlowdown      uint64 = 0x0000000000000080
	throttleDisplayClockSetting       uint64 = 0x0000000000000100

	// throttleIgnore is the set of bits that don't represent a problem
	// the operator should react to (idle = no work to do, application
	// clocks = operator-set cap, display clock = laptop-only).
	throttleIgnore = throttleGpuIdle | throttleApplicationsClocksSetting | throttleDisplayClockSetting

	// throttleHwCritical is the set of HW-driver-protective bits.
	// The driver is actively cutting clocks to keep silicon safe.
	throttleHwCritical = throttleHwSlowdown | throttleHwThermalSlowdown | throttleHwPowerBrakeSlowdown
)

// ConfirmedThrottle fires when NVML reports the driver is currently
// throttling clocks for a reason the operator should know about. The
// signal is nvmlDeviceGetCurrentClocksThrottleReasons (bitmap), so a
// hot-but-not-throttled GPU does not fire and a cool GPU under power
// brake does.
type ConfirmedThrottle struct{}

func (r *ConfirmedThrottle) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint
	for _, g := range snap.GPUs {
		bits := g.ThrottleReasons
		meaningful := bits &^ throttleIgnore
		if meaningful == throttleNone {
			continue
		}

		severity := "warning"
		if meaningful&throttleHwCritical != 0 {
			severity = "critical"
		}

		reasonNames := decodeThrottleReasons(meaningful)
		summary := fmt.Sprintf("GPU%d driver is throttling clocks: %s",
			g.Index, strings.Join(reasonNames, ", "))

		evidence := []model.Evidence{
			{Metric: "throttle_reasons", Value: float64(meaningful), Msg: strings.Join(reasonNames, ", ")},
		}
		if g.MaxClocksMHz.Graphics > 0 {
			ratio := float64(g.ClocksMHz.Graphics) / float64(g.MaxClocksMHz.Graphics) * 100.0
			evidence = append(evidence,
				model.Evidence{Metric: "current_sm_clock_mhz", Value: float64(g.ClocksMHz.Graphics), Unit: "MHz"},
				model.Evidence{Metric: "max_sm_clock_mhz", Value: float64(g.MaxClocksMHz.Graphics), Unit: "MHz"},
				model.Evidence{Metric: "clock_ratio_pct", Value: ratio, Unit: "%"},
			)
		}

		confidence := 0.95
		if severity == "warning" {
			confidence = 0.85
		}

		hints = append(hints, model.Hint{
			Name:       "confirmed-throttle",
			Category:   "hardware_health",
			Severity:   severity,
			Confidence: confidence,
			Summary:    summary,
			Evidence:   evidence,
		})
	}
	return hints
}

// decodeThrottleReasons turns the (already-masked) bitmap into a
// human-readable reason list ordered by severity-first.
func decodeThrottleReasons(bits uint64) []string {
	bitsOrdered := []struct {
		bit  uint64
		name string
	}{
		{throttleHwThermalSlowdown, "HW_THERMAL"},
		{throttleHwPowerBrakeSlowdown, "HW_POWER_BRAKE"},
		{throttleHwSlowdown, "HW_SLOWDOWN"},
		{throttleSwThermalSlowdown, "SW_THERMAL"},
		{throttleSwPowerCap, "SW_POWER_CAP"},
		{throttleSyncBoost, "SYNC_BOOST"},
	}
	out := make([]string, 0, len(bitsOrdered))
	for _, b := range bitsOrdered {
		if bits&b.bit != 0 {
			out = append(out, b.name)
		}
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("0x%x", bits))
	}
	return out
}
