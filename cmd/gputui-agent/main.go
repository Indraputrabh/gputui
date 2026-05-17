package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/indraputrabh/gputui/internal/api/unixsock"
	"github.com/indraputrabh/gputui/internal/collect/gpu"
	"github.com/indraputrabh/gputui/internal/collect/pipeline"
	"github.com/indraputrabh/gputui/internal/model"
	"github.com/indraputrabh/gputui/internal/record/jsonl"
)

type runOptions struct {
	interval        time.Duration
	outDir          string
	once            bool
	demo            bool
	gpuSource       string
	enableAPI       bool
	socketPath      string
	parallelCollect bool
	fsyncEvery      int
	collectTimeout  time.Duration
	healthTimeout   time.Duration
}

const shutdownTimeout = 3 * time.Second

func main() {
	opts, err := parseRunOptions(os.Args[1:])
	if err != nil {
		log.Printf("invalid options: %v", err)
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		log.Printf("gputui-agent failed: %v", err)
		os.Exit(1)
	}
}

func parseRunOptions(args []string) (runOptions, error) {
	fs := flag.NewFlagSet("gputui-agent", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	interval := fs.Duration("interval", 2*time.Second, "snapshot interval")
	outDir := fs.String("out", "./milestones/M1/outputs", "output directory for JSONL files")
	once := fs.Bool("once", false, "write one snapshot then exit")
	demo := fs.Bool("demo", false, "use demo/fake snapshot source")
	gpuSource := fs.String("gpu-source", string(gpu.SourceNVML), "gpu metric source for real mode: nvml or dcgm")
	enableAPI := fs.Bool("api", true, "enable local API server over unix socket")
	socketPath := fs.String("socket", "/tmp/gputui-agent.sock", "unix socket path for local API server")
	parallelCollect := fs.Bool("parallel-collect", true, "run host/GPU/log collectors concurrently inside each sample")
	fsyncEvery := fs.Int("fsync-every", 1, "fsync JSONL files every N snapshots (1=every snapshot, <=0 disables fsync until close)")
	collectTimeout := fs.Duration("collect-timeout", 10*time.Second, "per-sample collection deadline")
	healthTimeout := fs.Duration("health-timeout", 15*time.Second, "deadline for background GPU health-signal collection")

	if err := fs.Parse(args); err != nil {
		return runOptions{}, err
	}
	if *interval <= 0 {
		return runOptions{}, fmt.Errorf("invalid --interval %s: must be > 0", interval.String())
	}
	if strings.TrimSpace(*gpuSource) == "" {
		return runOptions{}, fmt.Errorf("invalid --gpu-source: must not be empty")
	}
	if *enableAPI && strings.TrimSpace(*socketPath) == "" {
		return runOptions{}, fmt.Errorf("invalid --socket: must not be empty when --api is enabled")
	}

	return runOptions{
		interval:        *interval,
		outDir:          *outDir,
		once:            *once,
		demo:            *demo,
		gpuSource:       strings.TrimSpace(*gpuSource),
		enableAPI:       *enableAPI,
		socketPath:      strings.TrimSpace(*socketPath),
		parallelCollect: *parallelCollect,
		fsyncEvery:      *fsyncEvery,
		collectTimeout:  *collectTimeout,
		healthTimeout:   *healthTimeout,
	}, nil
}

func run(opts runOptions) (err error) {
	log.Printf(
		"starting gputui-agent mode=%s interval=%s once=%t out=%s",
		runMode(opts.demo),
		opts.interval.String(),
		opts.once,
		opts.outDir,
	)
	log.Printf("selected gpu_source=%s", opts.gpuSource)
	log.Printf("local api enabled=%t socket=%s", opts.enableAPI, opts.socketPath)

	pipe, err := pipeline.NewWithOptions(
		gpu.Source(opts.gpuSource),
		opts.demo,
		[]float64{opts.interval.Seconds()},
		[]pipeline.Option{
			pipeline.WithParallelCollection(opts.parallelCollect),
			pipeline.WithCollectTimeout(opts.collectTimeout),
			pipeline.WithHealthTimeout(opts.healthTimeout),
		},
	)
	if err != nil {
		return fmt.Errorf("create pipeline: %w", err)
	}

	recorder, err := jsonl.NewWithOptions(opts.outDir, jsonl.Options{FsyncEvery: opts.fsyncEvery})
	if err != nil {
		return fmt.Errorf("create recorder: %w", err)
	}
	defer func() {
		if closeErr := recorder.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close recorder: %w", closeErr)
		}
	}()

	store := &snapshotStore{}
	if opts.enableAPI {
		server, apiErr := unixsock.NewServer(opts.socketPath, store)
		if apiErr != nil {
			return fmt.Errorf("create api server: %w", apiErr)
		}
		if apiErr = server.Start(); apiErr != nil {
			return fmt.Errorf("start api server: %w", apiErr)
		}
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if closeErr := server.Shutdown(ctx); closeErr != nil && err == nil {
				err = fmt.Errorf("shutdown api server: %w", closeErr)
			}
		}()
	}

	writeSnap := func() error {
		snap, collectErr := pipe.Collect(context.Background())
		if collectErr != nil {
			return collectErr
		}
		if recErr := recorder.WriteSnapshot(snap); recErr != nil {
			return fmt.Errorf("write snapshot: %w", recErr)
		}
		store.Set(snap)
		log.Printf("snapshot recorded at %s", snap.TS.Format(time.RFC3339))
		return nil
	}

	if opts.once {
		return writeSnap()
	}

	if err := writeSnap(); err != nil {
		log.Printf("initial snapshot failed: %v", err)
	}

	return runLoop(opts.interval, writeSnap)
}

func runMode(demo bool) string {
	if demo {
		return "demo"
	}
	return "real"
}

type snapshotStore struct {
	mu       sync.RWMutex
	snapshot model.Snapshot
	ok       bool
}

func (s *snapshotStore) Set(snapshot model.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot = snapshot
	s.ok = true
}

func (s *snapshotStore) LatestSnapshot() (model.Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshot, s.ok
}

func runLoop(interval time.Duration, writeSnapshot func() error) error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := writeSnapshot(); err != nil {
				log.Printf("snapshot collection failed: %v", err)
			}
		case sig := <-sigCh:
			log.Printf("received signal %s, exiting", sig)
			return nil
		}
	}
}
