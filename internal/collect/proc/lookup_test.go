package proc

import (
	"testing"
)

func TestLookupReturnsInfo(t *testing.T) {
	t.Parallel()

	info := Lookup(99999999)

	if info.User == "" {
		t.Fatal("expected non-empty User field (at least fallback)")
	}
	if info.Cmd == "" {
		t.Fatal("expected non-empty Cmd field (at least fallback)")
	}
}

func TestLookupNonExistentPID(t *testing.T) {
	t.Parallel()

	info := Lookup(-1)

	if info.User != "?" {
		t.Errorf("expected User=? for invalid PID, got %q", info.User)
	}
	if info.Cmd != "?" {
		t.Errorf("expected Cmd=? for invalid PID, got %q", info.Cmd)
	}
}

func TestLookupSelfPID(t *testing.T) {
	t.Parallel()

	info := Lookup(1)

	if info.User == "" {
		t.Fatal("expected non-empty User for PID 1")
	}
	if info.Cmd == "" {
		t.Fatal("expected non-empty Cmd for PID 1")
	}
}

func TestInfoDefaults(t *testing.T) {
	t.Parallel()

	var info Info
	if info.User != "" {
		t.Errorf("expected zero-value User to be empty, got %q", info.User)
	}
	if info.Cmd != "" {
		t.Errorf("expected zero-value Cmd to be empty, got %q", info.Cmd)
	}
}
