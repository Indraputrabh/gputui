package proc

import (
	"os"
	"testing"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestEnricherPopulatesRSS(t *testing.T) {
	t.Parallel()

	e := NewEnricher(2.0)
	procs := []model.ProcStat{
		{PID: os.Getpid(), User: "test", Cmd: "go test"},
	}
	e.Enrich(procs)

	// The current process should have non-zero RSS on any OS with /proc.
	if procs[0].RSSMB == 0 && hasProc() {
		t.Error("expected non-zero RSS for current process on Linux")
	}
}

func TestEnricherCPUDelta(t *testing.T) {
	t.Parallel()

	e := NewEnricher(1.0)
	pid := os.Getpid()

	first := []model.ProcStat{{PID: pid}}
	e.Enrich(first)

	// First call should yield 0% CPU (no previous sample).
	if first[0].CPUPct != 0 {
		t.Errorf("expected 0%% CPU on first sample, got %.2f%%", first[0].CPUPct)
	}

	// Second call may yield a small delta.
	second := []model.ProcStat{{PID: pid}}
	e.Enrich(second)

	// We can't assert an exact value, but it should be non-negative.
	if second[0].CPUPct < 0 {
		t.Errorf("expected non-negative CPU%%, got %.2f%%", second[0].CPUPct)
	}
}

func TestEnricherUnknownPID(t *testing.T) {
	t.Parallel()

	e := NewEnricher(2.0)
	procs := []model.ProcStat{
		{PID: 99999999, User: "nobody", Cmd: "ghost"},
	}
	e.Enrich(procs)

	if procs[0].CPUPct != 0 {
		t.Errorf("expected 0%% CPU for unknown PID, got %.2f%%", procs[0].CPUPct)
	}
	if procs[0].RSSMB != 0 {
		t.Errorf("expected 0 RSS for unknown PID, got %d", procs[0].RSSMB)
	}
}

func hasProc() bool {
	_, err := os.Stat("/proc/self/status")
	return err == nil
}
