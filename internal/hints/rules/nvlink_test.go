package rules

import (
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestNVLinkHealthFiresOnInactiveLinks(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	// Single-GPU snapshot: no fleet baseline, but inactive links
	// still fire (we can't compare to peers, so we err on the side
	// of surfacing the signal).
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 16, NVLinkCRCErrors: 0},
		},
	}

	hints := rule.Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint, got %d", len(hints))
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning severity (downgraded from critical), got %s", hints[0].Severity)
	}
}

func TestNVLinkHealthSuppressesUniformAsymmetry(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	// Every GPU reports the same active/total ratio (e.g. documented
	// 16/18 NVLink topology). No outlier -> no hint.
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 16},
			{Index: 1, NVLinkTotalLinks: 18, NVLinkActiveLinks: 16},
			{Index: 2, NVLinkTotalLinks: 18, NVLinkActiveLinks: 16},
		},
	}

	hints := rule.Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when fleet is uniformly asymmetric, got %d", len(hints))
	}
}

func TestNVLinkHealthFlagsFleetOutlier(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	// Three GPUs with 18/18, one with 16/18 -> the 16/18 GPU is the outlier.
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18},
			{Index: 1, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18},
			{Index: 2, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18},
			{Index: 3, NVLinkTotalLinks: 18, NVLinkActiveLinks: 16},
		},
	}

	hints := rule.Evaluate(snap)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for fleet outlier, got %d", len(hints))
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning, got %s", hints[0].Severity)
	}
}

func TestNVLinkHealthFiresOnCRCDelta(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}

	snap1 := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18, NVLinkCRCErrors: 100},
		},
	}
	rule.Evaluate(snap1)

	snap2 := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18, NVLinkCRCErrors: 142},
		},
	}
	hints := rule.Evaluate(snap2)
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for growing CRC errors, got %d", len(hints))
	}
	if hints[0].Severity != "warning" {
		t.Errorf("expected warning severity for CRC delta, got %s", hints[0].Severity)
	}
}

func TestNVLinkHealthNoFireStaleCRCCounter(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}

	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18, NVLinkCRCErrors: 500},
		},
	}

	rule.Evaluate(snap)
	hints := rule.Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints for stale (unchanged) CRC counter, got %d", len(hints))
	}
}

func TestNVLinkHealthNoFireFirstSample(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18, NVLinkCRCErrors: 500},
		},
	}

	hints := rule.Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints on first sample (no baseline yet), got %d", len(hints))
	}
}

func TestNVLinkHealthNoFireAllActive(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 18, NVLinkActiveLinks: 18, NVLinkCRCErrors: 0},
		},
	}

	rule.Evaluate(snap)
	hints := rule.Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints for healthy NVLink, got %d", len(hints))
	}
}

func TestNVLinkHealthNoFireNoLinks(t *testing.T) {
	t.Parallel()

	rule := &NVLinkHealth{}
	snap := model.Snapshot{
		HealthSignals: []model.GPUHealthSignal{
			{Index: 0, NVLinkTotalLinks: 0},
		},
	}

	hints := rule.Evaluate(snap)
	if len(hints) != 0 {
		t.Fatalf("expected no hints when no NVLink, got %d", len(hints))
	}
}
