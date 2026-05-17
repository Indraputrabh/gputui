package log

import (
	"testing"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestMatchXid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		wantNil bool
		wantXid string
	}{
		{
			name:    "standard xid error",
			line:    "NVRM: Xid (PCI:0000:41:00): 79, pid=12345",
			wantXid: "79",
		},
		{
			name:    "xid with different slot",
			line:    "NVRM: Xid (PCI:0000:c1:00): 48, GPU has fallen off the bus",
			wantXid: "48",
		},
		{
			name:    "no xid",
			line:    "some other kernel message",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := time.Now()
			m := MatchXid(tc.line, ts)

			if tc.wantNil {
				if m != nil {
					t.Fatalf("expected nil marker, got %+v", m)
				}
				return
			}

			if m == nil {
				t.Fatal("expected non-nil marker")
			}
			if m.Kind != "xid" {
				t.Errorf("expected Kind=xid, got %q", m.Kind)
			}
			if m.Msg != "NVIDIA Xid error "+tc.wantXid {
				t.Errorf("unexpected Msg: %q", m.Msg)
			}
		})
	}
}

func TestMatchOOM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		wantNil bool
		wantPID string
	}{
		{
			name:    "standard oom kill",
			line:    "Out of memory: Killed process 42 (python3) total-vm:1234kB",
			wantPID: "42",
		},
		{
			name:    "no oom",
			line:    "normal kernel log message",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := time.Now()
			m := MatchOOM(tc.line, ts)

			if tc.wantNil {
				if m != nil {
					t.Fatalf("expected nil marker, got %+v", m)
				}
				return
			}

			if m == nil {
				t.Fatal("expected non-nil marker")
			}
			if m.Kind != "oom" {
				t.Errorf("expected Kind=oom, got %q", m.Kind)
			}
			if m.Extra != "pid="+tc.wantPID {
				t.Errorf("expected Extra=pid=%s, got %q", tc.wantPID, m.Extra)
			}
		})
	}
}

func TestParseDmesgOutputMultipleMarkers(t *testing.T) {
	t.Parallel()

	sample := "NVRM: Xid (PCI:0000:41:00): 79, pid=1\n" +
		"NVRM: Xid (PCI:0000:41:00): 48, pid=2\n" +
		"Out of memory: Killed process 99 (train.py) total-vm:8192kB\n"

	markers := ParseDmesgOutput(sample, time.Time{})

	if len(markers) != 3 {
		t.Fatalf("expected 3 markers, got %d", len(markers))
	}

	wantKinds := []string{"xid", "xid", "oom"}
	for i, m := range markers {
		if m.Kind != wantKinds[i] {
			t.Errorf("marker[%d]: expected Kind=%q, got %q", i, wantKinds[i], m.Kind)
		}
	}
}

func TestParseDmesgOutputEmptyInput(t *testing.T) {
	t.Parallel()

	markers := ParseDmesgOutput("", time.Time{})
	if len(markers) != 0 {
		t.Errorf("expected 0 markers for empty input, got %d", len(markers))
	}
}

func TestParseDmesgOutputNoMatch(t *testing.T) {
	t.Parallel()

	sample := "eth0: link up\nusb 1-1: new device\nkernel: normal operation\n"
	markers := ParseDmesgOutput(sample, time.Time{})
	if len(markers) != 0 {
		t.Errorf("expected 0 markers for non-matching input, got %d", len(markers))
	}
}

func TestNewCollectorReturnsNonNil(t *testing.T) {
	t.Parallel()

	c := NewCollector()
	if c == nil {
		t.Fatal("expected non-nil collector")
	}
}

func TestMarkerFields(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 16, 12, 0, 0, 0, time.UTC)
	m := &model.Marker{
		TS:    ts,
		Kind:  "xid",
		Msg:   "NVIDIA Xid error 79",
		Extra: "PCI:0000:41:00",
	}

	if m.TS != ts {
		t.Errorf("expected TS=%v, got %v", ts, m.TS)
	}
	if m.Kind != "xid" {
		t.Errorf("expected Kind=xid, got %q", m.Kind)
	}
}
