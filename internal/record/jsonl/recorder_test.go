package jsonl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

func TestWriteSnapshotJSONLAndAppend(t *testing.T) {
	t.Parallel()

	outDir := t.TempDir()

	makeSnapshot := func(ts time.Time, hintName string) model.Snapshot {
		return model.Snapshot{
			TS: ts,
			GPUs: []model.GPUStat{
				{Index: 0, UtilPct: 10.0, VRAMUsedMB: 1, VRAMTotalMB: 2, TempC: 40, PowerW: 100},
			},
			Procs: []model.ProcStat{},
			Node:  model.NodeStat{LoadAvg: []float64{0.1, 0.2, 0.3}},
			Hints: []model.Hint{
				{
					Name:       hintName,
					Category:   "test",
					Severity:   "info",
					Confidence: 0.5,
					Summary:    "test hint",
					Evidence:   []model.Evidence{{Metric: "x", Value: 1}},
				},
			},
		}
	}

	rec, err := New(outDir)
	if err != nil {
		t.Fatalf("new recorder: %v", err)
	}
	if err := rec.WriteSnapshot(makeSnapshot(time.Unix(1, 0).UTC(), "hint-a")); err != nil {
		t.Fatalf("write snapshot #1: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder #1: %v", err)
	}

	// Re-open and write again to verify append behavior.
	rec2, err := New(outDir)
	if err != nil {
		t.Fatalf("new recorder #2: %v", err)
	}
	if err := rec2.WriteSnapshot(makeSnapshot(time.Unix(2, 0).UTC(), "hint-b")); err != nil {
		t.Fatalf("write snapshot #2: %v", err)
	}
	if err := rec2.Close(); err != nil {
		t.Fatalf("close recorder #2: %v", err)
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
	if got, want := len(hintLines), 2; got != want {
		t.Fatalf("hint line count got=%d want=%d", got, want)
	}

	for i, line := range snapshotLines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("snapshot line %d invalid json: %v", i+1, err)
		}
		for _, key := range []string{"ts", "gpus", "procs", "node"} {
			if _, ok := obj[key]; !ok {
				t.Fatalf("snapshot line %d missing key %q", i+1, key)
			}
		}
	}

	for i, line := range hintLines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("hint line %d invalid json: %v", i+1, err)
		}
		for _, key := range []string{"ts", "hint"} {
			if _, ok := obj[key]; !ok {
				t.Fatalf("hint line %d missing key %q", i+1, key)
			}
		}
	}
}
