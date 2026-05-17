package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/indraputrabh/gputui/internal/model"
)

const defaultBufferSize = 64 * 1024

// Options controls Recorder behaviour.
type Options struct {
	// FsyncEvery controls how often snapshot/hint files are fsynced.
	// A value of 1 (the default) preserves the pre-optimisation behaviour
	// of syncing after every snapshot. A value > 1 coalesces syncs to
	// amortise the disk cost. A value <= 0 disables fsync entirely until
	// Close (safe for JSONL on crash-friendly filesystems; the bufio
	// writer is still flushed on every snapshot).
	FsyncEvery int

	// BufferSize overrides the bufio.Writer buffer capacity (bytes).
	// Defaults to 64 KiB when zero.
	BufferSize int
}

// Recorder persists snapshots and hints as newline-delimited JSON.
type Recorder struct {
	mu            sync.Mutex
	snapshotsPath string
	hintsPath     string
	snapshotsFh   *os.File
	hintsFh       *os.File
	snapshotsBuf  *bufio.Writer
	hintsBuf      *bufio.Writer
	snapshotsEnc  *json.Encoder
	hintsEnc      *json.Encoder

	fsyncEvery int
	writes     int
}

type hintRecord struct {
	TS   string     `json:"ts"`
	Hint model.Hint `json:"hint"`
}

// New creates a JSONL recorder under outDir with default options
// (fsync every snapshot).
func New(outDir string) (*Recorder, error) {
	return NewWithOptions(outDir, Options{FsyncEvery: 1})
}

// NewWithOptions creates a JSONL recorder with explicit durability and
// buffering behaviour.
func NewWithOptions(outDir string, opts Options) (*Recorder, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	bufSize := opts.BufferSize
	if bufSize <= 0 {
		bufSize = defaultBufferSize
	}

	snapshotsPath := filepath.Join(outDir, "snapshots.jsonl")
	hintsPath := filepath.Join(outDir, "hints.jsonl")

	snapshotsFh, err := os.OpenFile(snapshotsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open snapshots file: %w", err)
	}

	hintsFh, err := os.OpenFile(hintsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = snapshotsFh.Close()
		return nil, fmt.Errorf("open hints file: %w", err)
	}

	snapshotsBuf := bufio.NewWriterSize(snapshotsFh, bufSize)
	hintsBuf := bufio.NewWriterSize(hintsFh, bufSize)

	return &Recorder{
		snapshotsPath: snapshotsPath,
		hintsPath:     hintsPath,
		snapshotsFh:   snapshotsFh,
		hintsFh:       hintsFh,
		snapshotsBuf:  snapshotsBuf,
		hintsBuf:      hintsBuf,
		snapshotsEnc:  json.NewEncoder(snapshotsBuf),
		hintsEnc:      json.NewEncoder(hintsBuf),
		fsyncEvery:    opts.FsyncEvery,
	}, nil
}

// WriteSnapshot writes a snapshot and each hint to dedicated JSONL streams.
// Writes go through bufio.Writer; fsync occurs once per fsyncEvery writes
// (or never until Close when fsyncEvery <= 0).
func (r *Recorder) WriteSnapshot(s model.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.snapshotsEnc.Encode(s); err != nil {
		return fmt.Errorf("encode snapshot to %s: %w", r.snapshotsPath, err)
	}

	ts := s.TS.UTC().Format("2006-01-02T15:04:05Z07:00")
	for _, h := range s.Hints {
		rec := hintRecord{TS: ts, Hint: h}
		if err := r.hintsEnc.Encode(rec); err != nil {
			return fmt.Errorf("encode hint to %s: %w", r.hintsPath, err)
		}
	}

	// Flush buffered bytes to the OS so crash-only loss is bounded by a
	// single snapshot even when fsync is deferred.
	if err := r.snapshotsBuf.Flush(); err != nil {
		return fmt.Errorf("flush snapshots buffer %s: %w", r.snapshotsPath, err)
	}
	if err := r.hintsBuf.Flush(); err != nil {
		return fmt.Errorf("flush hints buffer %s: %w", r.hintsPath, err)
	}

	r.writes++
	if r.fsyncEvery > 0 && r.writes%r.fsyncEvery == 0 {
		if err := r.snapshotsFh.Sync(); err != nil {
			return fmt.Errorf("sync snapshots file %s: %w", r.snapshotsPath, err)
		}
		if err := r.hintsFh.Sync(); err != nil {
			return fmt.Errorf("sync hints file %s: %w", r.hintsPath, err)
		}
	}

	return nil
}

// Close flushes buffers, fsyncs once, and closes output files.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	if r.snapshotsBuf != nil {
		if err := r.snapshotsBuf.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.hintsBuf != nil {
		if err := r.hintsBuf.Flush(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.snapshotsFh != nil {
		if err := r.snapshotsFh.Sync(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := r.snapshotsFh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if r.hintsFh != nil {
		if err := r.hintsFh.Sync(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := r.hintsFh.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
