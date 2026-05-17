// Command gputui-perf runs the real collection pipeline N times and
// writes a CSV of per-sample wall-time measurements. It is the
// before/after harness referenced by the Makefile and the remote CI
// scripts.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/indraputrabh/gputui/internal/collect/gpu"
	"github.com/indraputrabh/gputui/internal/collect/pipeline"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

func run() error {
	samples := flag.Int("samples", 100, "number of collection samples to record")
	interval := flag.Duration("interval", 1*time.Second, "interval between samples")
	outPath := flag.String("out", "./bench_results/pipeline_latency.csv", "output CSV path")
	gpuSource := flag.String("gpu-source", string(gpu.SourceNVML), "gpu source for real-mode runs")
	demo := flag.Bool("demo", true, "use demo data (no NVML required)")
	parallel := flag.Bool("parallel-collect", true, "enable parallel host/GPU/log collection")
	flag.Parse()

	if *samples <= 0 {
		return fmt.Errorf("samples must be > 0")
	}

	pipe, err := pipeline.NewWithOptions(
		gpu.Source(*gpuSource),
		*demo,
		[]float64{interval.Seconds()},
		[]pipeline.Option{pipeline.WithParallelCollection(*parallel)},
	)
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	fh, err := os.Create(*outPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", *outPath, err)
	}
	defer fh.Close()

	w := csv.NewWriter(fh)
	defer w.Flush()

	if err := w.Write([]string{"sample", "ts", "wall_us", "gpus", "procs", "hints"}); err != nil {
		return fmt.Errorf("csv header: %w", err)
	}

	ctx := context.Background()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for i := 0; i < *samples; i++ {
		start := time.Now()
		snap, err := pipe.Collect(ctx)
		elapsed := time.Since(start)
		if err != nil {
			log.Printf("sample %d error: %v", i, err)
		}

		row := []string{
			strconv.Itoa(i),
			snap.TS.UTC().Format(time.RFC3339Nano),
			strconv.FormatInt(elapsed.Microseconds(), 10),
			strconv.Itoa(len(snap.GPUs)),
			strconv.Itoa(len(snap.Procs)),
			strconv.Itoa(len(snap.Hints)),
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("csv write: %w", err)
		}
		w.Flush()

		if i < *samples-1 {
			<-ticker.C
		}
	}

	fmt.Printf("wrote %d samples to %s\n", *samples, *outPath)
	return nil
}
