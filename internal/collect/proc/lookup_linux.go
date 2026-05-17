//go:build linux

package proc

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
)

// uidUserCache memoises UID→username lookups. Hit rate on HPC nodes is
// effectively 100% (same researcher running many ranks).
var (
	uidUserMu    sync.RWMutex
	uidUserCache = map[string]string{}
)

func resolveUser(uid string) string {
	uidUserMu.RLock()
	name, ok := uidUserCache[uid]
	uidUserMu.RUnlock()
	if ok {
		return name
	}

	if u, err := user.LookupId(uid); err == nil {
		uidUserMu.Lock()
		uidUserCache[uid] = u.Username
		uidUserMu.Unlock()
		return u.Username
	}

	uidUserMu.Lock()
	uidUserCache[uid] = uid
	uidUserMu.Unlock()
	return uid
}

func lookup(pid int) Info {
	info := Info{User: "?", Cmd: "?"}

	if cmd := readCmdline(pid); cmd != "" {
		info.Cmd = cmd
	}

	statusData := readFileString(fmt.Sprintf("/proc/%d/status", pid))
	if statusData != "" {
		info.User = parseUser(statusData)
		info.RSSKB = parseVmRSS(statusData)
	}

	info.IOReadBytes, info.IOWriteBytes = readIOStats(pid)
	info.Utime, info.Stime, info.StartTime = readCPUTimes(pid)

	return info
}

func lookupCPU(pid int) CPUSample {
	var s CPUSample
	s.Utime, s.Stime, s.StartTime = readCPUTimes(pid)
	s.IOReadBytes, s.IOWriteBytes = readIOStats(pid)
	// VmRSS is cheap to re-read because /proc/[pid]/status is short and
	// lives in kernel memory; the alternative (statm) costs an extra read.
	statusData := readFileString(fmt.Sprintf("/proc/%d/status", pid))
	if statusData != "" {
		s.RSSKB = parseVmRSS(statusData)
	}
	return s
}

func readCmdline(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(data) == 0 {
		return ""
	}
	cleaned := strings.TrimRight(string(data), "\x00")
	parts := strings.Split(cleaned, "\x00")
	return strings.Join(parts, " ")
}

func readFileString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func parseUser(statusData string) string {
	for _, line := range strings.Split(statusData, "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return "?"
		}
		return resolveUser(fields[1])
	}
	return "?"
}

// parseVmRSS extracts VmRSS (in kB) from /proc/[pid]/status content.
func parseVmRSS(statusData string) uint64 {
	for _, line := range strings.Split(statusData, "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return val
	}
	return 0
}

// readIOStats reads cumulative read/write bytes from /proc/[pid]/io.
func readIOStats(pid int) (readBytes, writeBytes uint64) {
	data := readFileString(fmt.Sprintf("/proc/%d/io", pid))
	if data == "" {
		return 0, 0
	}
	for _, line := range strings.Split(data, "\n") {
		if strings.HasPrefix(line, "read_bytes:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				readBytes, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		} else if strings.HasPrefix(line, "write_bytes:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				writeBytes, _ = strconv.ParseUint(fields[1], 10, 64)
			}
		}
	}
	return readBytes, writeBytes
}

// readCPUTimes reads utime, stime, and starttime (all in clock ticks) from
// /proc/[pid]/stat. Fields 14, 15 and 22 in the stat file are utime, stime
// and starttime respectively.
func readCPUTimes(pid int) (utime, stime, starttime uint64) {
	data := readFileString(fmt.Sprintf("/proc/%d/stat", pid))
	if data == "" {
		return 0, 0, 0
	}
	// /proc/[pid]/stat contains the comm field in parentheses which may
	// include spaces, so find the closing ')' to parse reliably.
	idx := strings.LastIndex(data, ")")
	if idx < 0 || idx+2 >= len(data) {
		return 0, 0, 0
	}
	fields := strings.Fields(data[idx+2:])
	// After ')' the fields are: state(0), ppid(1), ..., utime(11), stime(12),
	// cutime(13), cstime(14), priority(15), nice(16), threads(17), itrealvalue(18),
	// starttime(19).
	if len(fields) < 13 {
		return 0, 0, 0
	}
	utime, _ = strconv.ParseUint(fields[11], 10, 64)
	stime, _ = strconv.ParseUint(fields[12], 10, 64)
	if len(fields) >= 20 {
		starttime, _ = strconv.ParseUint(fields[19], 10, 64)
	}
	return utime, stime, starttime
}
