package hints

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestDefaultEvaluatorReturnsHintsForStressedSnapshot(t *testing.T) {
	t.Parallel()

	// Mix three ground-truth signals on a single GPU plus host CPU
	// pressure: confirmed-throttle (HW thermal bit), memory-bandwidth-
	// bound (low GPU util + high mem util), and CPU-bound preprocessing.
	const hwThermalBit = 0x40
	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index:           0,
				UtilPct:         30,
				MemUtilPct:      88,
				VRAMUsedMB:      15000,
				VRAMTotalMB:     16384,
				TempC:           85,
				PowerW:          245,
				PowerLimitW:     250,
				ThrottleReasons: hwThermalBit,
			},
		},
		Node: model.NodeStat{
			CPUUser: 80,
		},
	}

	eval := DefaultEvaluator()
	hints := eval.Evaluate(snap)

	if len(hints) == 0 {
		t.Fatal("expected at least one hint for stressed snapshot, got none")
	}

	names := map[string]bool{}
	for _, h := range hints {
		names[h.Name] = true
	}

	want := []string{
		"potential-cpu-bound-preprocessing",
		"memory-bandwidth-bound",
		"confirmed-throttle",
	}
	for _, name := range want {
		if !names[name] {
			t.Errorf("expected hint %q to be present", name)
		}
	}
}

func TestDefaultEvaluatorReturnsNoHintsForHealthySnapshot(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index:            0,
				UtilPct:          85,
				MemUtilPct:       55,
				VRAMUsedMB:       8000,
				VRAMTotalMB:      16384,
				TempC:            55,
				PowerW:           150,
				PowerLimitW:      250,
				PerfState:        0,
				PCIeGenCurrent:   5,
				PCIeGenMax:       5,
				PCIeWidthCurrent: 16,
				PCIeWidthMax:     16,
			},
		},
		Node: model.NodeStat{
			CPUUser: 30,
		},
	}

	eval := DefaultEvaluator()
	hints := eval.Evaluate(snap)

	if len(hints) != 0 {
		t.Fatalf("expected no hints for healthy snapshot, got %d: %v", len(hints), hintNames(hints))
	}
}

func TestEvaluatorWithNoRules(t *testing.T) {
	t.Parallel()

	eval := NewEvaluator()
	hints := eval.Evaluate(model.Snapshot{})

	if len(hints) != 0 {
		t.Fatalf("expected no hints with no rules, got %d", len(hints))
	}
}

func hintNames(hints []model.Hint) []string {
	names := make([]string, len(hints))
	for i, h := range hints {
		names[i] = h.Name
	}
	return names
}
