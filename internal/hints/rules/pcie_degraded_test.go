package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestPCIeLinkDegradedFiresOnGenDrop(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index: 0, PCIeGenCurrent: 4, PCIeGenMax: 5,
				PCIeWidthCurrent: 16, PCIeWidthMax: 16,
			},
		},
	}

	hints := (&PCIeLinkDegraded{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "pcie-link-degraded" {
		t.Errorf("unexpected name: %s", hints[0].Name)
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning, got %s", hints[0].Severity)
	}
}

func TestPCIeLinkDegradedFiresOnWidthDrop(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index: 0, PCIeGenCurrent: 5, PCIeGenMax: 5,
				PCIeWidthCurrent: 8, PCIeWidthMax: 16,
			},
		},
	}

	hints := (&PCIeLinkDegraded{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
}

func TestPCIeLinkDegradedNoFireWhenAtMax(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{
				Index: 0, PCIeGenCurrent: 5, PCIeGenMax: 5,
				PCIeWidthCurrent: 16, PCIeWidthMax: 16,
			},
		},
	}

	hints := (&PCIeLinkDegraded{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints at full link, got %d", len(hints))
	}
}

func TestPCIeLinkDegradedNoFireMissingMax(t *testing.T) {
	t.Parallel()

	// Some virtualised drivers don't report max gen/width; skip.
	snap := model.Snapshot{
		GPUs: []model.GPUStat{
			{Index: 0, PCIeGenCurrent: 4, PCIeWidthCurrent: 8},
		},
	}

	hints := (&PCIeLinkDegraded{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when max unknown, got %d", len(hints))
	}
}
