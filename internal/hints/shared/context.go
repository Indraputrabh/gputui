// Package shared holds types exchanged between the hints evaluator and
// its rules. It lives in its own package so rules can depend on it
// without introducing a cycle with the hints package.
package shared

// EvalContext is a per-snapshot set of derived values shared across
// rules. Precomputing these once per snapshot saves redundant work when
// multiple rules look at the same aggregate (e.g. mean GPU utilisation).
type EvalContext struct {
	MeanGPUUtil float64
	HasGPUs     bool
	HasIOWait   bool
}
