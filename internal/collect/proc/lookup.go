package proc

// Info holds process metadata resolved from the operating system.
type Info struct {
	User         string
	Cmd          string
	RSSKB        uint64
	IOReadBytes  uint64
	IOWriteBytes uint64
	// Raw CPU time fields (in clock ticks) for delta-based CPU% calculation.
	Utime uint64
	Stime uint64
	// StartTime is field 22 of /proc/[pid]/stat (clock ticks since boot).
	// Used together with PID as a cache key so recycled PIDs are detected.
	StartTime uint64
}

// CPUSample is a light /proc read that only retrieves fields needed for
// per-process CPU% (utime, stime, starttime). Used on the hot enrichment
// path once User/Cmd have been cached for a given (pid, starttime).
type CPUSample struct {
	Utime        uint64
	Stime        uint64
	StartTime    uint64
	RSSKB        uint64
	IOReadBytes  uint64
	IOWriteBytes uint64
}

// Lookup resolves user, command, RSS, IO stats, and CPU times for a PID.
// Returns best-effort results; fields default to zero/fallback on failure.
func Lookup(pid int) Info {
	return lookup(pid)
}

// LookupCPU reads only the fast fields (CPU times, RSS, IO bytes, starttime)
// from /proc. It skips cmdline/status parsing and user.LookupId entirely,
// which makes it the preferred per-sample entry point once User/Cmd have
// been resolved once and cached.
func LookupCPU(pid int) CPUSample {
	return lookupCPU(pid)
}
