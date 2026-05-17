package log

import (
	"regexp"
	"strings"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

var (
	xidPattern = regexp.MustCompile(`NVRM:\s+Xid\s+\(PCI:([^)]+)\):\s+(\d+),`)
	oomPattern = regexp.MustCompile(`Out of memory: Killed process (\d+)\s+\(([^)]+)\)`)
)

// ParseDmesgOutput scans dmesg text for Xid and OOM markers, returning
// only lines newer than after.
func ParseDmesgOutput(output string, after time.Time) []model.Marker {
	var markers []model.Marker
	now := time.Now().UTC()

	for _, line := range strings.Split(output, "\n") {
		ts := parseDmesgTimestamp(line)
		if !ts.IsZero() && !ts.After(after) {
			continue
		}
		if ts.IsZero() {
			ts = now
		}

		if m := MatchXid(line, ts); m != nil {
			markers = append(markers, *m)
		}
		if m := MatchOOM(line, ts); m != nil {
			markers = append(markers, *m)
		}
	}
	return markers
}

// MatchXid checks a single dmesg line for an NVIDIA Xid error.
func MatchXid(line string, ts time.Time) *model.Marker {
	matches := xidPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}
	return &model.Marker{
		TS:    ts,
		Kind:  "xid",
		Msg:   "NVIDIA Xid error " + matches[2],
		Extra: "PCI:" + matches[1],
	}
}

// MatchOOM checks a single dmesg line for a Linux OOM kill event.
func MatchOOM(line string, ts time.Time) *model.Marker {
	matches := oomPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}
	return &model.Marker{
		TS:    ts,
		Kind:  "oom",
		Msg:   "OOM killed process " + matches[1] + " (" + matches[2] + ")",
		Extra: "pid=" + matches[1],
	}
}

// parseDmesgTimestamp attempts to extract an ISO timestamp from the start
// of a dmesg line (when --time-format iso is supported).
func parseDmesgTimestamp(line string) time.Time {
	if len(line) < 20 {
		return time.Time{}
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05,000000-0700",
		"2006-01-02T15:04:05,000000+0000",
		time.RFC3339,
	} {
		sep := strings.IndexByte(line[20:], ' ')
		if sep < 0 {
			continue
		}
		tsStr := line[:20+sep]
		if t, err := time.Parse(layout, tsStr); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
