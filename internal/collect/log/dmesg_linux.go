//go:build linux

package log

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

const kmsgPath = "/dev/kmsg"

// kmsgCollector tails /dev/kmsg incrementally so each Collect only reads
// kernel messages emitted since the previous call. When /dev/kmsg is not
// readable (permissions, container) it falls back to shelling out to
// dmesg the way the original collector did.
type kmsgCollector struct {
	mu       sync.Mutex
	kmsg     *os.File
	fallback *dmesgFallback
	bootTime time.Time
}

// dmesgFallback preserves the pre-optimisation behaviour for environments
// where /dev/kmsg is unavailable.
type dmesgFallback struct {
	lastSeen time.Time
}

func newCollector() Collector {
	c := &kmsgCollector{bootTime: readBootTime()}

	fh, err := os.OpenFile(kmsgPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		c.fallback = &dmesgFallback{lastSeen: time.Now()}
		return c
	}

	// Seek to end so we only emit messages that arrive after the collector
	// starts, matching the original "markers[-1].TS is the new lastSeen"
	// semantic. SEEK_END on /dev/kmsg is supported since 3.5.
	if _, err := fh.Seek(0, io.SeekEnd); err != nil {
		_ = fh.Close()
		c.fallback = &dmesgFallback{lastSeen: time.Now()}
		return c
	}

	c.kmsg = fh
	return c
}

func (c *kmsgCollector) Collect(ctx context.Context) ([]model.Marker, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.kmsg != nil {
		return c.readKmsg(ctx)
	}
	return c.readDmesgFallback(ctx)
}

func (c *kmsgCollector) readKmsg(ctx context.Context) ([]model.Marker, error) {
	var markers []model.Marker
	buf := make([]byte, 8192)

	for {
		select {
		case <-ctx.Done():
			return markers, nil
		default:
		}

		n, err := c.kmsg.Read(buf)
		if err != nil {
			// EAGAIN/EWOULDBLOCK on non-blocking fd means "no more data".
			if isWouldBlock(err) || errors.Is(err, io.EOF) {
				return markers, nil
			}
			// Pipe error or unexpected close: fall back.
			_ = c.kmsg.Close()
			c.kmsg = nil
			if c.fallback == nil {
				c.fallback = &dmesgFallback{lastSeen: time.Now()}
			}
			return markers, nil
		}
		if n == 0 {
			return markers, nil
		}

		line := string(buf[:n])
		ts, msg := parseKmsgLine(line, c.bootTime)
		if msg == "" {
			continue
		}
		if m := MatchXid(msg, ts); m != nil {
			markers = append(markers, *m)
		}
		if m := MatchOOM(msg, ts); m != nil {
			markers = append(markers, *m)
		}
	}
}

func (c *kmsgCollector) readDmesgFallback(ctx context.Context) ([]model.Marker, error) {
	output, err := runDmesg(ctx)
	if err != nil {
		return nil, nil
	}
	markers := ParseDmesgOutput(output, c.fallback.lastSeen)
	if len(markers) > 0 {
		c.fallback.lastSeen = markers[len(markers)-1].TS
	}
	return markers, nil
}

func runDmesg(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "dmesg", "--time-format", "iso", "--nopager")
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, "dmesg")
		out, err = cmd.Output()
		if err != nil {
			return "", err
		}
	}
	return string(out), nil
}

// parseKmsgLine converts a single /dev/kmsg record into a timestamped log
// message. The /dev/kmsg format is:
//
//	<priority_facility>,<seqnum>,<monotonic_usec>,<flags>;<message>\n
//
// Optional continuation lines after the first newline contain structured
// metadata (" KEY=VALUE\n" lines) which we discard.
func parseKmsgLine(raw string, bootTime time.Time) (time.Time, string) {
	// Strip trailing metadata continuation lines.
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		raw = raw[:idx]
	}
	sep := strings.IndexByte(raw, ';')
	if sep < 0 {
		return time.Now().UTC(), strings.TrimSpace(raw)
	}
	header := raw[:sep]
	msg := strings.TrimSpace(raw[sep+1:])
	if msg == "" {
		return time.Time{}, ""
	}

	fields := strings.Split(header, ",")
	if len(fields) < 3 || bootTime.IsZero() {
		return time.Now().UTC(), msg
	}
	usec, err := strconv.ParseInt(strings.TrimSpace(fields[2]), 10, 64)
	if err != nil {
		return time.Now().UTC(), msg
	}
	ts := bootTime.Add(time.Duration(usec) * time.Microsecond).UTC()
	return ts, msg
}

// readBootTime approximates the absolute wall-clock boot time by reading
// the btime field of /proc/stat. Returns zero on failure, in which case
// callers fall back to time.Now() for the per-message timestamp.
func readBootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "btime ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			return time.Time{}
		}
		sec, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(sec, 0).UTC()
	}
	return time.Time{}
}

func isWouldBlock(err error) bool {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EAGAIN || errno == syscall.EWOULDBLOCK
	}
	return false
}
