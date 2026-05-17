package hints

import (
	"math"

	"github.com/indraputrabh/gputui/internal/model"
)

// HysteresisConfig controls the decay-based suppression of flapping hints.
type HysteresisConfig struct {
	HalfLifeSamples float64
	EmitThreshold   float64
}

// DefaultHysteresisConfig returns sensible defaults: a half-life of 3 samples
// and an emit threshold of 2.0 (a hint must fire roughly twice within the
// decay window to be emitted).
func DefaultHysteresisConfig() HysteresisConfig {
	return HysteresisConfig{
		HalfLifeSamples: 3.0,
		EmitThreshold:   2.0,
	}
}

var bypassHysteresisCategories = map[string]bool{
	"hardware_health": true,
}

// HysteresisEvaluator wraps an Evaluator and suppresses hints that have not
// accumulated enough score through repeated firing. Hardware health hints
// bypass hysteresis entirely since they represent persistent state.
type HysteresisEvaluator struct {
	inner  *Evaluator
	config HysteresisConfig
	scores map[string]float64
}

// NewHysteresisEvaluator creates a decay-based wrapper around an evaluator.
func NewHysteresisEvaluator(inner *Evaluator, cfg HysteresisConfig) *HysteresisEvaluator {
	return &HysteresisEvaluator{
		inner:  inner,
		config: cfg,
		scores: make(map[string]float64),
	}
}

// Evaluate runs the inner evaluator, applies decay to all tracked hints,
// increments scores for hints that fired, and returns only those that
// exceed the emission threshold. Hardware health hints always pass through.
func (h *HysteresisEvaluator) Evaluate(snap model.Snapshot) []model.Hint {
	raw := h.inner.Evaluate(snap)

	fired := make(map[string]bool, len(raw))
	hintsByName := make(map[string]model.Hint, len(raw))
	for _, hint := range raw {
		fired[hint.Name] = true
		hintsByName[hint.Name] = hint
	}

	decayFactor := math.Pow(0.5, 1.0/h.config.HalfLifeSamples)

	for name := range h.scores {
		h.scores[name] *= decayFactor
		if h.scores[name] < 0.001 {
			delete(h.scores, name)
		}
	}

	for name := range fired {
		h.scores[name] += 1.0
	}

	var result []model.Hint
	for _, hint := range raw {
		if bypassHysteresisCategories[hint.Category] {
			result = append(result, hint)
			continue
		}
		if h.scores[hint.Name] >= h.config.EmitThreshold {
			result = append(result, hint)
		}
	}

	return result
}
