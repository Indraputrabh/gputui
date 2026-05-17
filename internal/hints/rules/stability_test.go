package rules

import (
	"testing"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestStabilityXidMarker(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		TS: time.Now(),
		Markers: []model.Marker{
			{
				TS:    time.Now(),
				Kind:  "xid",
				Msg:   "NVIDIA Xid error 79",
				Extra: "PCI:0000:41:00",
			},
		},
	}

	r := &Stability{}
	hints := r.Evaluate(snap)

	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "gpu-xid-error" {
		t.Errorf("expected Name=gpu-xid-error, got %q", hints[0].Name)
	}
	if hints[0].Severity != "critical" {
		t.Errorf("expected Severity=critical, got %q", hints[0].Severity)
	}
	if hints[0].Confidence != 1.0 {
		t.Errorf("expected Confidence=1.0, got %.2f", hints[0].Confidence)
	}
	if len(hints[0].Evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(hints[0].Evidence))
	}
}

func TestStabilityOOMMarker(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		TS: time.Now(),
		Markers: []model.Marker{
			{
				TS:    time.Now(),
				Kind:  "oom",
				Msg:   "OOM killed process 42 (python3)",
				Extra: "pid=42",
			},
		},
	}

	r := &Stability{}
	hints := r.Evaluate(snap)

	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Name != "host-oom-kill" {
		t.Errorf("expected Name=host-oom-kill, got %q", hints[0].Name)
	}
	if hints[0].Severity != "critical" {
		t.Errorf("expected Severity=critical, got %q", hints[0].Severity)
	}
}

func TestStabilityMultipleMarkers(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		TS: time.Now(),
		Markers: []model.Marker{
			{Kind: "xid", Msg: "NVIDIA Xid error 79", Extra: "PCI:0000:41:00"},
			{Kind: "oom", Msg: "OOM killed process 99 (train.py)", Extra: "pid=99"},
			{Kind: "xid", Msg: "NVIDIA Xid error 48", Extra: "PCI:0000:c1:00"},
		},
	}

	r := &Stability{}
	hints := r.Evaluate(snap)

	if len(hints) != 3 {
		t.Fatalf("expected 3 hints, got %d", len(hints))
	}

	wantNames := []string{"gpu-xid-error", "host-oom-kill", "gpu-xid-error"}
	for i, h := range hints {
		if h.Name != wantNames[i] {
			t.Errorf("hint[%d]: expected Name=%q, got %q", i, wantNames[i], h.Name)
		}
	}
}

func TestStabilityNoMarkers(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{TS: time.Now()}
	r := &Stability{}
	hints := r.Evaluate(snap)

	if len(hints) != 0 {
		t.Errorf("expected 0 hints for empty markers, got %d", len(hints))
	}
}

func TestStabilityIgnoresNonStabilityMarkers(t *testing.T) {
	t.Parallel()

	snap := model.Snapshot{
		TS: time.Now(),
		Markers: []model.Marker{
			{Kind: "collector_status", Msg: "process collection: timeout"},
		},
	}

	r := &Stability{}
	hints := r.Evaluate(snap)

	if len(hints) != 0 {
		t.Errorf("expected 0 hints for collector_status marker, got %d", len(hints))
	}
}
