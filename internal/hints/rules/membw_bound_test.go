package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestMemoryBandwidthBoundFires(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, UtilPct: 30, MemUtilPct: 90},
		},
	}

	hints := (&MemoryBandwidthBound{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "memory-bandwidth-bound" {
		t.Errorf("unexpected name: %s", hints[0].Name)
	}
}

func TestMemoryBandwidthBoundSkipsHighGPUUtil(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, UtilPct: 80, MemUtilPct: 90},
		},
	}

	hints := (&MemoryBandwidthBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when GPU util is high, got %d", len(hints))
	}
}

func TestMemoryBandwidthBoundSkipsLowMemUtil(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, UtilPct: 30, MemUtilPct: 50},
		},
	}

	hints := (&MemoryBandwidthBound{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when mem util is low, got %d", len(hints))
	}
}
