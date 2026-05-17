package proc

import (
	"sync"

	"github.com/indraputrabh/gputui/internal/model"
)

// cpuSample stores the previous CPU time reading plus the process
// starttime so PID recycling is detected (stale deltas are discarded).
type cpuSample struct {
	utime     uint64
	stime     uint64
	starttime uint64
}

// procIdent caches fields that are stable for a process's lifetime.
// Keyed on (pid, starttime) so a recycled PID drops the old entry.
type procIdent struct {
	user      string
	cmd       string
	starttime uint64
}

// Enricher populates per-process host stats (CPU%, RSS, IO) on ProcStat
// entries. It maintains previous-sample state for delta-based CPU%, plus
// caches for user and cmdline resolution keyed on (pid, starttime) so the
// hot path does only a lightweight /proc/[pid]/stat + io + status read
// per process.
type Enricher struct {
	mu        sync.Mutex
	prev      map[int]cpuSample
	curr      map[int]cpuSample
	identity  map[int]procIdent
	clkTicks  float64
	cpuCapPct float64
}

// NewEnricher creates an Enricher. The interval parameter is the expected
// number of seconds between successive calls to Enrich.
func NewEnricher(intervalSec float64) *Enricher {
	ticks := sysClkTck()
	if ticks <= 0 {
		ticks = 100
	}
	cpuCount := cpuCountFallback()
	if cpuCount <= 0 {
		cpuCount = 1
	}
	return &Enricher{
		prev:      make(map[int]cpuSample),
		curr:      make(map[int]cpuSample),
		identity:  make(map[int]procIdent),
		clkTicks:  float64(ticks) * intervalSec,
		cpuCapPct: 100 * float64(cpuCount),
	}
}

// Enrich resolves per-process host metrics and populates them on each ProcStat.
// CPU% is computed from the delta in utime+stime between consecutive calls.
func (e *Enricher) Enrich(procs []model.ProcStat) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for k := range e.curr {
		delete(e.curr, k)
	}

	for i := range procs {
		pid := procs[i].PID
		sample := LookupCPU(pid)

		// If we've never seen this PID or the starttime changed (PID reuse),
		// refresh the identity cache with a full Lookup.
		ident, ok := e.identity[pid]
		if !ok || ident.starttime != sample.StartTime || sample.StartTime == 0 {
			info := Lookup(pid)
			ident = procIdent{user: info.User, cmd: info.Cmd, starttime: info.StartTime}
			if sample.StartTime != 0 {
				e.identity[pid] = ident
			}
			// Fold fresh fields into the CPU sample so we don't re-read below.
			sample.Utime = info.Utime
			sample.Stime = info.Stime
			sample.StartTime = info.StartTime
			sample.RSSKB = info.RSSKB
			sample.IOReadBytes = info.IOReadBytes
			sample.IOWriteBytes = info.IOWriteBytes
		}

		// Only overwrite User/Cmd when the caller hasn't supplied them
		// (standalone TUI passes empty strings; tests may pre-fill).
		if procs[i].User == "" {
			procs[i].User = ident.user
		}
		if procs[i].Cmd == "" {
			procs[i].Cmd = ident.cmd
		}
		procs[i].RSSMB = sample.RSSKB / 1024
		procs[i].IORead = sample.IOReadBytes
		procs[i].IOWrite = sample.IOWriteBytes

		now := cpuSample{utime: sample.Utime, stime: sample.Stime, starttime: sample.StartTime}
		e.curr[pid] = now

		if prev, havePrev := e.prev[pid]; havePrev && e.clkTicks > 0 {
			if prev.starttime != now.starttime && now.starttime != 0 {
				// PID was recycled between samples; no valid delta.
				continue
			}
			delta := float64((now.utime - prev.utime) + (now.stime - prev.stime))
			pct := (delta / e.clkTicks) * 100.0
			if pct > e.cpuCapPct {
				pct = 0
			}
			procs[i].CPUPct = pct
		}
	}

	// Evict identity entries for PIDs no longer present to bound memory.
	if len(e.identity) > 0 && len(e.identity) > len(e.curr)*4 {
		for pid := range e.identity {
			if _, alive := e.curr[pid]; !alive {
				delete(e.identity, pid)
			}
		}
	}

	// Swap prev/curr buffers: this generation's samples become next tick's
	// baseline without any allocation.
	e.prev, e.curr = e.curr, e.prev
}
