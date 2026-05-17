package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestECCErrorFiresOnUncorrectable(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ECCUncorrectableVolatile: 5, ECCCorrectableVolatile: 100},
		},
	}

	hints := (&ECCError{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "critical" {
		t.Errorf("expected critical severity, got %s", hints[0].Severity)
	}
	if hints[0].Name != "gpu-ecc-errors" {
		t.Errorf("unexpected name: %s", hints[0].Name)
	}
}

func TestECCErrorInfoForCorrectableOnly(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ECCCorrectableVolatile: 50},
		},
	}

	hints := (&ECCError{}).Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "info" {
		t.Errorf("expected info severity for correctable-only, got %s", hints[0].Severity)
	}
}

func TestECCErrorNoFireClean(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, ECCUncorrectableVolatile: 0, ECCCorrectableVolatile: 0},
		},
	}

	hints := (&ECCError{}).Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints for clean ECC, got %d", len(hints))
	}
}

func TestECCErrorNoFireEmpty(t *testing.T) {
	t.Parallel()

	hints := (&ECCError{}).Evaluate(model.Snapshot{})
	if len(hints) != 0 {
		t.Fatalf("expected no hints with no health signals, got %d", len(hints))
	}
}
