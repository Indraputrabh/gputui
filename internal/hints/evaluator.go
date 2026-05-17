package hints

import (
	"fmt"
	"hash/fnv"

	"github.com/indraputrabh/gputui/internal/hints/shared"
	"github.com/indraputrabh/gputui/internal/model"
)

// Rule evaluates a snapshot and returns any applicable hints.
type Rule interface {
	Evaluate(snap model.Snapshot) []model.Hint
}

// ContextAware is an optional extension for rules that benefit from
// precomputed snapshot-wide values (e.g. mean GPU utilisation). The
// Evaluator passes the shared EvalContext in when the rule implements it;
// otherwise it falls back to plain Evaluate.
type ContextAware interface {
	EvaluateWithContext(snap model.Snapshot, ctx *shared.EvalContext) []model.Hint
}

// HealthSensitive marks rules that only depend on snap.HealthSignals.
// Evaluator skips them when the signal set hasn't changed since the last
// evaluation and replays the cached hints instead.
type HealthSensitive interface {
	DependsOnHealthOnly() bool
}

// Evaluator runs a set of rules against each snapshot.
type Evaluator struct {
	rules []Rule

	// Cached health-rule output keyed on a stable hash of the health
	// signals. Rules implementing HealthSensitive replay the cached
	// output when the hash matches.
	healthHash   uint64
	healthCached map[string][]model.Hint
}

// NewEvaluator returns an evaluator with the given rules.
func NewEvaluator(rules ...Rule) *Evaluator {
	return &Evaluator{
		rules:        rules,
		healthCached: make(map[string][]model.Hint),
	}
}

// DefaultEvaluator returns an evaluator loaded with all built-in rules.
func DefaultEvaluator() *Evaluator {
	return NewEvaluator(defaultRules()...)
}

// Evaluate runs every registered rule and collects the resulting hints.
func (e *Evaluator) Evaluate(snap model.Snapshot) []model.Hint {
	ctx := buildEvalContext(snap)

	currentHealthHash := hashHealthSignals(snap.HealthSignals)
	healthChanged := currentHealthHash != e.healthHash
	if healthChanged {
		e.healthHash = currentHealthHash
	}

	var all []model.Hint
	for _, r := range e.rules {
		ruleKey := ruleCacheKey(r)

		if hs, ok := r.(HealthSensitive); ok && hs.DependsOnHealthOnly() && !healthChanged {
			if cached, found := e.healthCached[ruleKey]; found {
				all = append(all, cached...)
				continue
			}
		}

		var produced []model.Hint
		if cr, ok := r.(ContextAware); ok {
			produced = cr.EvaluateWithContext(snap, ctx)
		} else {
			produced = r.Evaluate(snap)
		}

		if hs, ok := r.(HealthSensitive); ok && hs.DependsOnHealthOnly() {
			// Copy to avoid aliasing with the returned slice.
			if produced != nil {
				cacheCopy := make([]model.Hint, len(produced))
				copy(cacheCopy, produced)
				e.healthCached[ruleKey] = cacheCopy
			} else {
				e.healthCached[ruleKey] = nil
			}
		}

		if len(produced) > 0 {
			all = append(all, produced...)
		}
	}
	return all
}

func buildEvalContext(snap model.Snapshot) *shared.EvalContext {
	ctx := &shared.EvalContext{HasGPUs: len(snap.GPUs) > 0}
	if ctx.HasGPUs {
		var sum float64
		for _, g := range snap.GPUs {
			sum += g.UtilPct
		}
		ctx.MeanGPUUtil = sum / float64(len(snap.GPUs))
	}
	ctx.HasIOWait = snap.Node.CPUIowait > 0
	return ctx
}

// hashHealthSignals produces a stable non-cryptographic hash of the
// fields used by health-signal rules. Matching hashes across samples
// mean downstream health rules can skip re-evaluation.
func hashHealthSignals(sigs []model.GPUHealthSignal) uint64 {
	h := fnv.New64a()
	var scratch [64]byte
	for _, sig := range sigs {
		writeUint(h, scratch[:], uint64(sig.Index))
		writeUint(h, scratch[:], sig.ECCUncorrectableVolatile)
		writeUint(h, scratch[:], sig.ECCCorrectableVolatile)
		writeUint(h, scratch[:], uint64(sig.NVLinkActiveLinks))
		writeUint(h, scratch[:], uint64(sig.NVLinkTotalLinks))
		writeUint(h, scratch[:], sig.NVLinkCRCErrors)
		writeUint(h, scratch[:], uint64(sig.RemappedRowsCorrectable))
		writeUint(h, scratch[:], uint64(sig.RemappedRowsUncorrectable))
		if sig.RemappedRowsPending {
			writeUint(h, scratch[:], 1)
		} else {
			writeUint(h, scratch[:], 0)
		}
		writeUint(h, scratch[:], sig.ViolationThermalNs)
		writeUint(h, scratch[:], sig.ViolationPowerNs)
	}
	return h.Sum64()
}

func writeUint(h interface {
	Write([]byte) (int, error)
}, buf []byte, v uint64) {
	for i := 0; i < 8; i++ {
		buf[i] = byte(v >> (i * 8))
	}
	_, _ = h.Write(buf[:8])
}

// ruleCacheKey returns a stable identifier for a rule used by the
// health-signal cache. Rules are compared by concrete type.
func ruleCacheKey(r Rule) string {
	return fmt.Sprintf("%T", r)
}
