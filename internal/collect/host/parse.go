package host

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// ParseLoadAvg parses the contents of /proc/loadavg and returns the three
// load average values (1m, 5m, 15m).
func ParseLoadAvg(raw string) ([]float64, error) {
	parts := strings.Fields(raw)
	if len(parts) < 3 {
		return nil, fmt.Errorf("parse loadavg: expected at least 3 fields, got %d", len(parts))
	}

	vals := make([]float64, 0, 3)
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseFloat(parts[i], 64)
		if err != nil {
			return nil, fmt.Errorf("parse loadavg field %d: %w", i, err)
		}
		vals = append(vals, v)
	}
	return vals, nil
}

// MeminfoResult holds parsed values from /proc/meminfo.
type MeminfoResult struct {
	MemAvailable uint64
	MemTotal     uint64
	SwapUsed     uint64
	SwapTotal    uint64
}

// ParseMeminfo parses the contents of /proc/meminfo and extracts memory
// and swap values. Values are returned in bytes (input is in kB).
func ParseMeminfo(raw string) MeminfoResult {
	return parseMeminfoBytes([]byte(raw))
}

// parseMeminfoBytes is the zero-alloc path that scans bytes directly.
func parseMeminfoBytes(raw []byte) MeminfoResult {
	var memAvail, memTotal, swapTotal, swapFree uint64

	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		colon := -1
		for i, c := range line {
			if c == ':' {
				colon = i
				break
			}
		}
		if colon < 0 || colon >= len(line)-1 {
			continue
		}
		name := line[:colon]

		// Skip whitespace then parse the first numeric field.
		rest := line[colon+1:]
		start := 0
		for start < len(rest) && (rest[start] == ' ' || rest[start] == '\t') {
			start++
		}
		end := start
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == start {
			continue
		}
		kb, err := strconv.ParseUint(string(rest[start:end]), 10, 64)
		if err != nil {
			continue
		}
		b := kb * 1024

		switch {
		case bytes.Equal(name, memAvailKey):
			memAvail = b
		case bytes.Equal(name, memTotalKey):
			memTotal = b
		case bytes.Equal(name, swapTotalKey):
			swapTotal = b
		case bytes.Equal(name, swapFreeKey):
			swapFree = b
		}
	}

	var swapUsed uint64
	if swapTotal >= swapFree {
		swapUsed = swapTotal - swapFree
	}

	return MeminfoResult{
		MemAvailable: memAvail,
		MemTotal:     memTotal,
		SwapUsed:     swapUsed,
		SwapTotal:    swapTotal,
	}
}

var (
	memAvailKey  = []byte("MemAvailable")
	memTotalKey  = []byte("MemTotal")
	swapTotalKey = []byte("SwapTotal")
	swapFreeKey  = []byte("SwapFree")
)

// CPUDelta holds the delta percentages between two CPU samples.
type CPUDelta struct {
	User   float64
	Sys    float64
	Iowait float64
}

// CPUSample holds parsed aggregate CPU counters from /proc/stat.
type CPUSample struct {
	Total   uint64
	User    uint64
	Nice    uint64
	Sys     uint64
	Idle    uint64
	Iowait  uint64
	IRQ     uint64
	SoftIRQ uint64
}

// ParseCPUStatLine parses the aggregate "cpu " line from /proc/stat into
// a CPUSample. The fields slice must start with "cpu" as the first element.
func ParseCPUStatLine(fields []string) (CPUSample, error) {
	if len(fields) < 8 {
		return CPUSample{}, fmt.Errorf("parse cpu stat: expected >= 8 fields, got %d", len(fields))
	}

	parse := func(idx int, name string) (uint64, error) {
		v, err := strconv.ParseUint(fields[idx], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse cpu %s: %w", name, err)
		}
		return v, nil
	}

	user, err := parse(1, "user")
	if err != nil {
		return CPUSample{}, err
	}
	nice, err := parse(2, "nice")
	if err != nil {
		return CPUSample{}, err
	}
	sys, err := parse(3, "system")
	if err != nil {
		return CPUSample{}, err
	}
	idle, err := parse(4, "idle")
	if err != nil {
		return CPUSample{}, err
	}
	iowait, err := parse(5, "iowait")
	if err != nil {
		return CPUSample{}, err
	}
	irq, err := parse(6, "irq")
	if err != nil {
		return CPUSample{}, err
	}
	softirq, err := parse(7, "softirq")
	if err != nil {
		return CPUSample{}, err
	}

	return CPUSample{
		Total:   user + nice + sys + idle + iowait + irq + softirq,
		User:    user,
		Nice:    nice,
		Sys:     sys,
		Idle:    idle,
		Iowait:  iowait,
		IRQ:     irq,
		SoftIRQ: softirq,
	}, nil
}

// ComputeCPUDelta calculates user/sys/iowait percentages from two consecutive
// CPU samples. Returns zero values if the total delta is zero.
func ComputeCPUDelta(prev, cur CPUSample) CPUDelta {
	deltaTotal := cur.Total - prev.Total
	if deltaTotal == 0 {
		return CPUDelta{}
	}

	deltaUser := (cur.User - prev.User) + (cur.Nice - prev.Nice)
	deltaSys := (cur.Sys - prev.Sys) + (cur.IRQ - prev.IRQ) + (cur.SoftIRQ - prev.SoftIRQ)
	deltaIowait := cur.Iowait - prev.Iowait

	return CPUDelta{
		User:   100.0 * float64(deltaUser) / float64(deltaTotal),
		Sys:    100.0 * float64(deltaSys) / float64(deltaTotal),
		Iowait: 100.0 * float64(deltaIowait) / float64(deltaTotal),
	}
}
