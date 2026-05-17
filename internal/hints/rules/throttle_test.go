package rules

import (
	"strings"
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestConfirmedThrottleHWThermalCritical(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index:           0,
				ThrottleReasons: throttleHwThermalSlowdown,
				ClocksMHz: struct {
					Graphics uint32 `json:"graphics,omitempty"`
					Mem      uint32 `json:"mem,omitempty"`
				}{Graphics: 1200, Mem: 2619},
				MaxClocksMHz: struct {
					Graphics uint32 `json:"graphics,omitempty"`
					Mem      uint32 `json:"mem,omitempty"`
				}{Graphics: 1980, Mem: 2619},
			},
		},
	}

	hints := (&ConfirmedThrottle{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "critical" {
		t.Errorf("expected critical for HW thermal, got %s", hints[0].Severity)
	}
	if !strings.Contains(hints[0].Summary, "HW_THERMAL") {
		t.Errorf("expected summary to mention HW_THERMAL, got %q", hints[0].Summary)
	}
	if hints[0].Category != "hardware_health" {
		t.Errorf("expected category=hardware_health, got %s", hints[0].Category)
	}
}

func TestConfirmedThrottleSwPowerCapWarning(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, ThrottleReasons: throttleSwPowerCap},
		},
	}

	hints := (&ConfirmedThrottle{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning for SW power cap, got %s", hints[0].Severity)
	}
}

func TestConfirmedThrottleSuppressesIdleAndAppClocks(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, ThrottleReasons: throttleGpuIdle},
			{Index: 1, ThrottleReasons: throttleApplicationsClocksSetting},
			{Index: 2, ThrottleReasons: throttleNone},
		},
	}

	hints := (&ConfirmedThrottle{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints for idle/operator-set/none, got %d", len(hints))
	}
}

func TestConfirmedThrottleMixedBitsKeepsCritical(t *testing.T) {
	t.Parallel()

	// HW thermal + SW power cap together -> still critical because
	// HW bit is set.
	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, ThrottleReasons: throttleHwThermalSlowdown | throttleSwPowerCap},
		},
	}

	hints := (&ConfirmedThrottle{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "critical" {
		t.Errorf("expected critical (HW dominates), got %s", hints[0].Severity)
	}
}
