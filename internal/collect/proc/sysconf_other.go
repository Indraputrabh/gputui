//go:build !linux

package proc

import "runtime"

func sysClkTck() int {
	return 100
}

func cpuCountFallback() int {
	return runtime.NumCPU()
}
