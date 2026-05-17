//go:build linux

package proc

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// sysClkTck returns the system clock tick rate (USER_HZ).
// On Linux USER_HZ is effectively always 100 (kernel compile-time constant).
func sysClkTck() int {
	return 100
}

// cpuCountFallback returns the number of logical CPUs available,
// used to cap unreasonable CPU% values.
func cpuCountFallback() int {
	n := runtime.NumCPU()
	// Also check cgroup CPU quota for containerised environments.
	if quota := readCgroupCPUQuota(); quota > 0 {
		return quota
	}
	return n
}

func readCgroupCPUQuota() int {
	data, err := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	if err != nil {
		return 0
	}
	quota, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || quota <= 0 {
		return 0
	}
	period, err := os.ReadFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if err != nil {
		return 0
	}
	p, err := strconv.Atoi(strings.TrimSpace(string(period)))
	if err != nil || p <= 0 {
		return 0
	}
	return quota / p
}
