package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestIOBoundFires(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 20}},
		Node: model.NodeStat{CPUIowait: 30},
	}

	hints := (&IOBound{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "potential-io-bound-pipeline" {
		t.Fatalf("unexpected hint name: %s", hints[0].Name)
	}
	if hints[0].Confidence <= 0.4 || hints[0].Confidence > 1.0 {
		t.Fatalf("unexpected confidence: %f", hints[0].Confidence)
	}
}

func TestIOBoundDoesNotFireWhenGPUBusy(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 85}},
		Node: model.NodeStat{CPUIowait: 30},
	}

	hints := (&IOBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when GPU is busy, got %d", len(hints))
	}
}

func TestIOBoundDoesNotFireWhenIowaitLow(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{{UtilPct: 20}},
		Node: model.NodeStat{CPUIowait: 5},
	}

	hints := (&IOBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when iowait is low, got %d", len(hints))
	}
}

func TestIOBoundDoesNotFireWithNoGPUs(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		Node: model.NodeStat{CPUIowait: 40},
	}

	hints := (&IOBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints with no GPUs, got %d", len(hints))
	}
}

func TestIOBoundAveragesMultipleGPUs(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{UtilPct: 10},
			{UtilPct: 30},
		},
		Node: model.NodeStat{CPUIowait: 25},
	}

	hints := (&IOBound{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for avg GPU util 20%%, got %d", len(hints))
	}
}
