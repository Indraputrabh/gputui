package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestGPUParkedFires(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, PerfState: 8, VRAMUsedMB: 70000, UtilPct: 0},
		},
	}

	hints := (&GPUParked{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "gpu-parked" {
		t.Errorf("unexpected name: %s", hints[0].Name)
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning, got %s", hints[0].Severity)
	}
}

func TestGPUParkedSkipsLowVRAM(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, PerfState: 8, VRAMUsedMB: 100},
		},
	}

	hints := (&GPUParked{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when little VRAM allocated, got %d", len(hints))
	}
}

func TestGPUParkedSkipsActivePerfState(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, PerfState: 0, VRAMUsedMB: 70000},
			{Index: 1, PerfState: 7, VRAMUsedMB: 70000},
		},
	}

	hints := (&GPUParked{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints below P8, got %d", len(hints))
	}
}

func TestGPUParkedSkipsUnknownPState(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, PerfState: 32, VRAMUsedMB: 70000}, // PSTATE_UNKNOWN
		},
	}

	hints := (&GPUParked{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when P-state unknown, got %d", len(hints))
	}
}
