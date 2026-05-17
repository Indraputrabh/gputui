package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseRunOptionsValidation(t *testing.T) {
	t.Parallel()

	_, err := parseRunOptions([]string{"--interval", "0s"})
	if err == nil {
		t.Fatalf("expected validation error for non-positive interval")
	}

	_, err = parseRunOptions([]string{"--api", "--socket", "  "})
	if err == nil {
		t.Fatalf("expected validation error for empty socket path when api enabled")
	}
}

func TestRunOnceDemoWritesJSONL(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()
	opts := runOptions{
		interval:  time.Second,
		outDir:    outDir,
		once:      true,
		demo:      true,
		gpuSource: "nvml",
	}

	if err := run(opts); err != nil {
		t.Fatalf("run once demo: %v", err)
	}
	if err := run(opts); err != nil {
		t.Fatalf("run once demo second time: %v", err)
	}

	snapshotsPath := filepath.Join(outDir, "snapshots.jsonl")
	hintsPath := filepath.Join(outDir, "hints.jsonl")

	snapshotsRaw, err := os.ReadFile(snapshotsPath)
	if err != nil {
		t.Fatalf("read snapshots.jsonl: %v", err)
	}
	hintsRaw, err := os.ReadFile(hintsPath)
	if err != nil {
		t.Fatalf("read hints.jsonl: %v", err)
	}

	snapshotLines := strings.Split(strings.TrimSpace(string(snapshotsRaw)), "\n")
	hintLines := strings.Split(strings.TrimSpace(string(hintsRaw)), "\n")
	if got, want := len(snapshotLines), 2; got != want {
		t.Fatalf("snapshot line count got=%d want=%d", got, want)
	}
	if len(hintLines) < 2 {
		t.Fatalf("expected at least 2 hint lines, got %d", len(hintLines))
	}

	// Sanity-check first snapshot shape.
	var first map[string]any
	if err := json.Unmarshal([]byte(snapshotLines[0]), &first); err != nil {
		t.Fatalf("first snapshot invalid json: %v", err)
	}
	for _, key := range []string{"ts", "gpus", "procs", "node"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("first snapshot missing key %q", key)
		}
	}
}
