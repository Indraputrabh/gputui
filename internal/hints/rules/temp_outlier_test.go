package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestThermalViolationOutlierFires(t *testing.T) {
	t.Parallel()

	// One GPU has accumulated 60s of thermal cap; the rest negligible.
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ViolationThermalNs: 60 * 1e9},
			{Index: 1, ViolationThermalNs: 0},
			{Index: 2, ViolationThermalNs: 0},
			{Index: 3, ViolationThermalNs: 1e8}, // 100 ms
		},
	}

	hints := (&ThermalViolationOutlier{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for outlier GPU, got %d", len(hints))
	}
	if hints[0].Name != "thermal-violation-outlier" {
		t.Errorf("unexpected name: %s", hints[0].Name)
	}
}

func TestThermalViolationOutlierNoFireBelowFloor(t *testing.T) {
	t.Parallel()

	// Highest GPU has 1.5s -> below 2s floor.
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ViolationThermalNs: 1500 * 1e6},
			{Index: 1, ViolationThermalNs: 0},
			{Index: 2, ViolationThermalNs: 0},
		},
	}

	hints := (&ThermalViolationOutlier{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints below floor, got %d", len(hints))
	}
}

func TestThermalViolationOutlierNoFireUniform(t *testing.T) {
	t.Parallel()

	// All GPUs roughly equal -> no outlier.
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ViolationThermalNs: 30 * 1e9},
			{Index: 1, ViolationThermalNs: 28 * 1e9},
			{Index: 2, ViolationThermalNs: 32 * 1e9},
			{Index: 3, ViolationThermalNs: 29 * 1e9},
		},
	}

	hints := (&ThermalViolationOutlier{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when fleet uniform, got %d", len(hints))
	}
}

func TestThermalViolationOutlierNoFireSingleGPU(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ViolationThermalNs: 60 * 1e9},
		},
	}

	hints := (&ThermalViolationOutlier{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints with single GPU, got %d", len(hints))
	}
}
