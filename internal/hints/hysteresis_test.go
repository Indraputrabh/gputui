package hints

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func stressedSnap() model.Snapshot {
	// CPU-bound borderline (avg GPU util 30% < 60% ceiling, CPU user
	// 80% > 50% floor) -- the cpu_bound category is hysteresis-gated.
	return model.Snapshot{
		GPUs: []model.GPUStat{
			{UtilPct: 30, MemUtilPct: 40, VRAMUsedMB: 15000, VRAMTotalMB: 16384, TempC: 70, PowerW: 200, PowerLimitW: 250},
		},
		Node: model.NodeStat{CPUUser: 80},
	}
}

func healthySnap() model.Snapshot {
	return model.Snapshot{
		GPUs: []model.GPUStat{
			{UtilPct: 85, MemUtilPct: 55, VRAMUsedMB: 8000, VRAMTotalMB: 16384, TempC: 55, PowerW: 150, PowerLimitW: 250,
				PerfState: 0, PCIeGenCurrent: 5, PCIeGenMax: 5, PCIeWidthCurrent: 16, PCIeWidthMax: 16},
		},
		Node: model.NodeStat{CPUUser: 30},
	}
}

func TestHysteresisSuppressesSingleFire(t *testing.T) {
	t.Parallel()

	eval := DefaultEvaluator()
	hyst := NewHysteresisEvaluator(eval, DefaultHysteresisConfig())

	hints := hyst.Evaluate(stressedSnap())
	if len(hints) != 0 {
		t.Fatalf("expected 0 hints on first fire (below threshold), got %d", len(hints))
	}
}

func TestHysteresisEmitsAfterSustainedFiring(t *testing.T) {
	t.Parallel()

	eval := DefaultEvaluator()
	hyst := NewHysteresisEvaluator(eval, DefaultHysteresisConfig())

	snap := stressedSnap()
	for i := 0; i < 5; i++ {
		hyst.Evaluate(snap)
	}

	hints := hyst.Evaluate(snap)
	if len(hints) == 0 {
		t.Fatal("expected hints after sustained firing, got 0")
	}
}

func TestHysteresisResetsAfterSilence(t *testing.T) {
	t.Parallel()

	eval := DefaultEvaluator()
	hyst := NewHysteresisEvaluator(eval, DefaultHysteresisConfig())

	snap := stressedSnap()
	for i := 0; i < 5; i++ {
		hyst.Evaluate(snap)
	}

	hints := hyst.Evaluate(snap)
	if len(hints) == 0 {
		t.Fatal("expected hints after sustained firing")
	}

	healthy := healthySnap()
	for i := 0; i < 20; i++ {
		hyst.Evaluate(healthy)
	}

	hints = hyst.Evaluate(stressedSnap())
	if len(hints) != 0 {
		t.Fatalf("expected suppressed hints after long silence, got %d", len(hints))
	}
}

func TestHysteresisBypassesHardwareHealth(t *testing.T) {
	t.Parallel()

	eval := DefaultEvaluator()
	hyst := NewHysteresisEvaluator(eval, DefaultHysteresisConfig())

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{UtilPct: 85, MemUtilPct: 55, VRAMUsedMB: 8000, VRAMTotalMB: 16384, TempC: 55, PowerW: 150, PowerLimitW: 250,
				PerfState: 0, PCIeGenCurrent: 5, PCIeGenMax: 5, PCIeWidthCurrent: 16, PCIeWidthMax: 16},
		},
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ECCUncorrectableVolatile: 10},
		},
		Node: model.NodeStat{CPUUser: 30},
	}

	hints := hyst.Evaluate(snap)

	found := false
	for _, h := range hints {
		if h.Category == "hardware_health" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected hardware_health hint to bypass hysteresis on first fire")
	}
}

func TestHysteresisNoHintsForHealthy(t *testing.T) {
	t.Parallel()

	eval := DefaultEvaluator()
	hyst := NewHysteresisEvaluator(eval, DefaultHysteresisConfig())

	for i := 0; i < 10; i++ {
		hints := hyst.Evaluate(healthySnap())
		if len(hints) != 0 {
			t.Fatalf("iteration %d: expected 0 hints for healthy snapshot, got %d", i, len(hints))
		}
	}
}
