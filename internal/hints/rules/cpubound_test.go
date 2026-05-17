package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestCPUBoundFires(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 20}},
		Node: model.NodeStat{CPUUser: 75},
	}

	hints := (&CPUBound{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "potential-cpu-bound-preprocessing" {
		t.Fatalf("unexpected hint name: %s", hints[0].Name)
	}
	if hints[0].Confidence <= 0.5 || hints[0].Confidence > 1.0 {
		t.Fatalf("unexpected confidence: %f", hints[0].Confidence)
	}
}

func TestCPUBoundDoesNotFireWhenGPUBusy(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 85}},
		Node: model.NodeStat{CPUUser: 75},
	}

	hints := (&CPUBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when GPU is busy, got %d", len(hints))
	}
}

func TestCPUBoundDoesNotFireWhenCPULow(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 20}},
		Node: model.NodeStat{CPUUser: 30},
	}

	hints := (&CPUBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when CPU is low, got %d", len(hints))
	}
}

func TestCPUBoundDoesNotFireWithNoGPUs(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		Node: model.NodeStat{CPUUser: 90},
	}

	hints := (&CPUBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints with no GPUs, got %d", len(hints))
	}
}

func TestCPUBoundAveragesMultipleGPUs(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{UtilPct: 10},
			{UtilPct: 30},
		},
		Node: model.NodeStat{CPUUser: 80},
	}

	hints := (&CPUBound{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for avg GPU util 20%%, got %d", len(hints))
	}
}
