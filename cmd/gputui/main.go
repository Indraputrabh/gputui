package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/indraputrabh/gputui/internal/api/client"
	"github.com/indraputrabh/gputui/internal/collect/gpu"
	"github.com/indraputrabh/gputui/internal/collect/pipeline"
	"github.com/indraputrabh/gputui/internal/hints"
	"github.com/indraputrabh/gputui/internal/model"

	tea "github.com/charmbracelet/bubbletea"
)

// SnapshotProvider abstracts where snapshots come from.
type SnapshotProvider interface {
	FetchSnapshot() (model.Snapshot, error)
}

type standaloneProvider struct {
	pipe *pipeline.Pipeline
}

func (s *standaloneProvider) FetchSnapshot() (model.Snapshot, error) {
	return s.pipe.Collect(context.Background())
}

func main() {
	agent := flag.String("agent", "", "connect to a running gputui-agent at this unix socket path (default: standalone mode)")
	refresh := flag.Duration("refresh", 2*time.Second, "snapshot poll interval")
	plain := flag.Bool("plain", false, "print a single snapshot as text and exit")
	demo := flag.Bool("demo", false, "use fake demo data (standalone mode only)")
	gpuSource := flag.String("gpu-source", string(gpu.SourceNVML), "gpu metric source: nvml or dcgm")
	parallelCollect := flag.Bool("parallel-collect", true, "run host/GPU/log collectors concurrently inside each sample")
	collectTimeout := flag.Duration("collect-timeout", 10*time.Second, "per-sample collection deadline")
	healthTimeout := flag.Duration("health-timeout", 15*time.Second, "deadline for background GPU health-signal collection")
	flag.Parse()

	if err := run(*agent, *refresh, *plain, *demo, *gpuSource, *parallelCollect, *collectTimeout, *healthTimeout); err != nil {
		log.Fatalf("gputui: %v", err)
	}
}

func run(agentSocket string, refresh time.Duration, plain, demo bool, gpuSource string, parallelCollect bool, collectTimeout, healthTimeout time.Duration) error {
	var provider SnapshotProvider

	if agentSocket != "" {
		c := client.New(agentSocket)
		defer c.Close()
		provider = c
	} else {
		opts := []pipeline.Option{
			pipeline.WithParallelCollection(parallelCollect),
			pipeline.WithCollectTimeout(collectTimeout),
			pipeline.WithHealthTimeout(healthTimeout),
		}
		// Plain mode is single-shot: skip hysteresis so all rules
		// fire on the lone snapshot rather than being suppressed
		// until they accumulate score across multiple samples.
		if plain {
			opts = append(opts, pipeline.WithEvaluator(hints.DefaultEvaluator()))
		}
		pipe, err := pipeline.NewWithOptions(
			gpu.Source(gpuSource),
			demo,
			[]float64{refresh.Seconds()},
			opts,
		)
		if err != nil {
			return fmt.Errorf("create pipeline: %w", err)
		}
		provider = &standaloneProvider{pipe: pipe}
	}

	if plain {
		return runPlain(provider)
	}

	hostname, _ := os.Hostname()
	p := tea.NewProgram(
		newModel(provider, refresh, hostname),
		tea.WithAltScreen(),
		tea.WithFPS(15),
	)
	_, err := p.Run()
	return err
}
