//go:build linux

package host

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/indraputrabh/gputui/internal/model"
)

// bufPool amortises /proc read buffer allocations across samples.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 8192)
		return &b
	},
}

func acquireBuf() *[]byte {
	p := bufPool.Get().(*[]byte)
	*p = (*p)[:0]
	return p
}

func releaseBuf(p *[]byte) {
	// Avoid unbounded retention of very large buffers.
	if cap(*p) > 64*1024 {
		return
	}
	bufPool.Put(p)
}

type linuxCollector struct {
	prev     CPUSample
	havePrev bool
}

func newCollector() Collector {
	return &linuxCollector{}
}

func (c *linuxCollector) Collect(ctx context.Context) (model.NodeStat, error) {
	_ = ctx

	loadavg, err := readLoadAvg()
	if err != nil {
		return model.NodeStat{}, err
	}

	mem, err := readMeminfo()
	if err != nil {
		return model.NodeStat{}, err
	}

	cpuDelta, err := c.readCPUPercentages()
	if err != nil {
		return model.NodeStat{}, err
	}

	return model.NodeStat{
		LoadAvg:      loadavg,
		CPUUser:      cpuDelta.User,
		CPUSys:       cpuDelta.Sys,
		CPUIowait:    cpuDelta.Iowait,
		MemAvailable: mem.MemAvailable,
		MemTotal:     mem.MemTotal,
		SwapUsed:     mem.SwapUsed,
		SwapTotal:    mem.SwapTotal,
	}, nil
}

// readFileInto reads the entire contents of path into a pooled buffer.
func readFileInto(path string, buf *[]byte) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	// /proc files don't expose a useful size, so grow incrementally.
	tmp := make([]byte, 4096)
	for {
		n, err := fh.Read(tmp)
		if n > 0 {
			*buf = append(*buf, tmp[:n]...)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func readLoadAvg() ([]float64, error) {
	buf := acquireBuf()
	defer releaseBuf(buf)
	if err := readFileInto("/proc/loadavg", buf); err != nil {
		return nil, fmt.Errorf("read /proc/loadavg: %w", err)
	}
	return ParseLoadAvg(string(*buf))
}

func readMeminfo() (MeminfoResult, error) {
	buf := acquireBuf()
	defer releaseBuf(buf)
	if err := readFileInto("/proc/meminfo", buf); err != nil {
		return MeminfoResult{}, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	return parseMeminfoBytes(*buf), nil
}

func (c *linuxCollector) readCPUPercentages() (CPUDelta, error) {
	buf := acquireBuf()
	defer releaseBuf(buf)
	if err := readFileInto("/proc/stat", buf); err != nil {
		return CPUDelta{}, fmt.Errorf("read /proc/stat: %w", err)
	}

	sample, err := parseCPUAggregate(*buf)
	if err != nil {
		return CPUDelta{}, err
	}

	if !c.havePrev {
		c.prev = sample
		c.havePrev = true
		return CPUDelta{}, nil
	}

	delta := ComputeCPUDelta(c.prev, sample)
	c.prev = sample
	return delta, nil
}

// parseCPUAggregate scans /proc/stat bytes for the aggregate "cpu " line
// without allocating one string per line.
func parseCPUAggregate(data []byte) (CPUSample, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 4096), 64*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) < 4 || line[0] != 'c' || line[1] != 'p' || line[2] != 'u' || line[3] != ' ' {
			continue
		}
		fields := bytesFields(line)
		sample, err := ParseCPUStatLine(fields)
		if err != nil {
			return CPUSample{}, err
		}
		return sample, nil
	}
	if err := scanner.Err(); err != nil {
		return CPUSample{}, fmt.Errorf("scan /proc/stat: %w", err)
	}
	return CPUSample{}, fmt.Errorf("parse /proc/stat: missing aggregate cpu line")
}

// bytesFields is a zero-allocation(ish) analogue of strings.Fields that
// returns []string slices pointing at freshly allocated substrings.
// We still allocate the substrings because ParseCPUStatLine expects
// []string, but we avoid the full-buffer strings.Split.
func bytesFields(line []byte) []string {
	var out []string
	start := -1
	for i, c := range line {
		if c == ' ' || c == '\t' {
			if start >= 0 {
				out = append(out, string(line[start:i]))
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, string(line[start:]))
	}
	return out
}
