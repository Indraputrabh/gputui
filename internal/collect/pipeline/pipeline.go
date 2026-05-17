package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/indraputrabh/gputui/internal/collect/gpu"
	"github.com/indraputrabh/gputui/internal/collect/host"
	logcollect "github.com/indraputrabh/gputui/internal/collect/log"
	"github.com/indraputrabh/gputui/internal/collect/proc"
	"github.com/indraputrabh/gputui/internal/hints"
	"github.com/indraputrabh/gputui/internal/model"
)

const (
	defaultCollectTimeout = 10 * time.Second
	healthCollectPeriod   = 10 * time.Second
	defaultHealthTimeout  = 15 * time.Second
)

// hintEvaluator is satisfied by both *hints.Evaluator and
// *hints.HysteresisEvaluator; the live pipeline uses the wrapped form,
// tests can opt out of decay via WithEvaluator.
type hintEvaluator interface {
	Evaluate(snap model.Snapshot) []model.Hint
}

// Pipeline collects GPU + host metrics and evaluates hints in one call.
type Pipeline struct {
	demo           bool
	parallel       bool
	collectTimeout time.Duration
	healthTimeout  time.Duration
	gpuCollector   gpu.Collector
	hostCollector  host.Collector
	logCollector   logcollect.Collector
	procEnricher   *proc.Enricher
	eval           hintEvaluator

	healthMu      sync.Mutex
	healthCache   []model.GPUHealthSignal
	healthRunning bool
	healthLastRun time.Time
	healthLastErr error
}

// Option configures Pipeline construction.
type Option func(*Pipeline)

// WithParallelCollection toggles concurrent execution of the independent
// host, GPU and log collectors. Defaults to true.
func WithParallelCollection(enabled bool) Option {
	return func(p *Pipeline) { p.parallel = enabled }
}

// WithCollectTimeout overrides the per-sample collection deadline. Useful
// when running against slow or remote hardware where the default (10s)
// would otherwise clip legitimate samples. A value <=0 keeps the default.
func WithCollectTimeout(d time.Duration) Option {
	return func(p *Pipeline) {
		if d > 0 {
			p.collectTimeout = d
		}
	}
}

// WithHealthTimeout overrides the deadline for background health-signal
// collection (ECC, NVLink, remapped rows). A value <=0 keeps the default.
func WithHealthTimeout(d time.Duration) Option {
	return func(p *Pipeline) {
		if d > 0 {
			p.healthTimeout = d
		}
	}
}

// WithEvaluator overrides the hint evaluator. The default wraps the
// rule set with a hysteresis filter so transient borderline conditions
// don't flood the hint stream; tests that need instant emission can
// pass an unwrapped DefaultEvaluator() here.
func WithEvaluator(eval hintEvaluator) Option {
	return func(p *Pipeline) {
		if eval != nil {
			p.eval = eval
		}
	}
}

// New creates a pipeline. When demo is true, fake data is returned instead
// of querying real hardware. The optional intervalSec parameter sets the
// expected polling interval for CPU% delta computation (default 2s).
func New(gpuSource gpu.Source, demo bool, intervalSec ...float64) (*Pipeline, error) {
	return NewWithOptions(gpuSource, demo, intervalSec, nil)
}

// NewWithOptions is like New but accepts functional options.
func NewWithOptions(gpuSource gpu.Source, demo bool, intervalSec []float64, opts []Option) (*Pipeline, error) {
	interval := 2.0
	if len(intervalSec) > 0 && intervalSec[0] > 0 {
		interval = intervalSec[0]
	}

	p := &Pipeline{
		demo:           demo,
		parallel:       true,
		collectTimeout: defaultCollectTimeout,
		healthTimeout:  defaultHealthTimeout,
		eval:           hints.NewHysteresisEvaluator(hints.DefaultEvaluator(), hints.DefaultHysteresisConfig()),
		procEnricher:   proc.NewEnricher(interval),
	}
	for _, opt := range opts {
		opt(p)
	}
	if demo {
		return p, nil
	}

	gc, err := gpu.NewCollector(gpuSource)
	if err != nil {
		return nil, fmt.Errorf("gpu collector: %w", err)
	}
	p.gpuCollector = gc
	p.hostCollector = host.NewCollector()
	p.logCollector = logcollect.NewCollector()
	return p, nil
}

// Collect gathers a single snapshot with hints.
func (p *Pipeline) Collect(ctx context.Context) (model.Snapshot, error) {
	var snap model.Snapshot
	var err error
	if p.demo {
		snap = demoSnapshot()
	} else {
		snap, err = p.realSnapshot(ctx)
		if err != nil {
			return snap, err
		}
	}
	snap.Hints = p.eval.Evaluate(snap)
	return snap, nil
}

func (p *Pipeline) realSnapshot(ctx context.Context) (model.Snapshot, error) {
	collectCtx, cancel := context.WithTimeout(ctx, p.collectTimeout)
	defer cancel()

	now := time.Now().UTC()

	var (
		node       model.NodeStat
		gpus       []model.GPUStat
		procs      []model.ProcStat
		logMarkers []model.Marker
		markers    []model.Marker

		hostErr error
		gpuErr  error
		procErr error
		logErr  error

		procCollector gpu.ProcessCollector
		hasProc       bool
	)

	procCollector, hasProc = p.gpuCollector.(gpu.ProcessCollector)

	hostFn := func() error {
		n, err := p.hostCollector.Collect(collectCtx)
		if err != nil {
			hostErr = err
			return err
		}
		node = n
		return nil
	}

	gpuFn := func() error {
		g, err := p.gpuCollector.Collect(collectCtx)
		if err != nil {
			if errors.Is(err, gpu.ErrNotImplemented) {
				gpuErr = err
				return nil
			}
			gpuErr = err
			return err
		}
		gpus = g

		if !hasProc {
			return nil
		}
		collected, pErr := procCollector.CollectProcesses(collectCtx)
		if pErr != nil {
			procErr = pErr
			return nil
		}
		procs = collected
		return nil
	}

	logFn := func() error {
		if p.logCollector == nil {
			return nil
		}
		lm, err := p.logCollector.Collect(collectCtx)
		if err != nil {
			logErr = err
			return nil
		}
		logMarkers = lm
		return nil
	}

	if p.parallel {
		eg, _ := errgroup.WithContext(collectCtx)
		eg.Go(hostFn)
		eg.Go(gpuFn)
		eg.Go(logFn)
		if err := eg.Wait(); err != nil {
			// Only host/GPU fatal errors propagate; log/proc/ErrNotImplemented
			// are captured as markers below.
			if hostErr != nil {
				return model.Snapshot{}, fmt.Errorf("collect host metrics: %w", hostErr)
			}
			if gpuErr != nil && !errors.Is(gpuErr, gpu.ErrNotImplemented) {
				return model.Snapshot{}, fmt.Errorf("collect from source=%s: %w", p.gpuCollector.Source(), gpuErr)
			}
		}
	} else {
		if err := hostFn(); err != nil {
			return model.Snapshot{}, fmt.Errorf("collect host metrics: %w", hostErr)
		}
		if err := gpuFn(); err != nil {
			return model.Snapshot{}, fmt.Errorf("collect from source=%s: %w", p.gpuCollector.Source(), gpuErr)
		}
		_ = logFn()
	}

	if gpuErr != nil && errors.Is(gpuErr, gpu.ErrNotImplemented) {
		markers = append(markers, model.Marker{
			TS:   now,
			Kind: "collector_status",
			Msg:  gpuErr.Error(),
		})
	}
	if procErr != nil {
		markers = append(markers, model.Marker{
			TS:   now,
			Kind: "collector_status",
			Msg:  fmt.Sprintf("process collection: %v", procErr),
		})
	}
	if logErr != nil {
		markers = append(markers, model.Marker{
			TS:   now,
			Kind: "collector_status",
			Msg:  fmt.Sprintf("log collection: %v", logErr),
		})
	}
	if len(logMarkers) > 0 {
		markers = append(markers, logMarkers...)
	}

	if procs == nil {
		procs = []model.ProcStat{}
	}
	if len(procs) > 0 && p.procEnricher != nil {
		p.procEnricher.Enrich(procs)
	}

	healthSignals := p.cachedHealth(ctx, &markers, now)

	return model.Snapshot{
		TS:            now,
		GPUs:          gpus,
		Procs:         procs,
		Node:          node,
		HealthSignals: healthSignals,
		Markers:       markers,
		Hints:         []model.Hint{},
	}, nil
}

// cachedHealth returns the most recent cached health signals and, if
// they are stale, kicks off a background refresh. The sample path never
// waits on a running CollectHealth, so a slow NVML health pass can't
// stall the TUI or cause the per-sample deadline to blow out.
func (p *Pipeline) cachedHealth(ctx context.Context, markers *[]model.Marker, now time.Time) []model.GPUHealthSignal {
	hc, ok := p.gpuCollector.(gpu.HealthCollector)
	if !ok {
		return nil
	}

	p.healthMu.Lock()
	cache := p.healthCache
	stale := p.healthLastRun.IsZero() ||
		time.Since(p.healthLastRun) >= healthCollectPeriod
	shouldStart := stale && !p.healthRunning
	if shouldStart {
		p.healthRunning = true
	}
	p.healthMu.Unlock()

	if shouldStart {
		go p.refreshHealth(hc)
	}

	// Surface the most recent refresh error (if any) without blocking.
	p.healthMu.Lock()
	if p.healthLastErr != nil {
		*markers = append(*markers, model.Marker{
			TS:   now,
			Kind: "collector_status",
			Msg:  fmt.Sprintf("health signal collection: %v", p.healthLastErr),
		})
		p.healthLastErr = nil
	}
	p.healthMu.Unlock()

	return cache
}

// refreshHealth runs one CollectHealth pass with its own timeout and
// publishes the result to healthCache. Safe to call from a goroutine;
// cachedHealth guarantees at most one refresh is in flight.
func (p *Pipeline) refreshHealth(hc gpu.HealthCollector) {
	ctx, cancel := context.WithTimeout(context.Background(), p.healthTimeout)
	defer cancel()

	collected, err := hc.CollectHealth(ctx)

	p.healthMu.Lock()
	if err != nil {
		p.healthLastErr = err
	} else {
		p.healthCache = collected
	}
	p.healthLastRun = time.Now()
	p.healthRunning = false
	p.healthMu.Unlock()
}

// demoVRAMTotalMB is the per-GPU VRAM capacity reported by the demo
// backend, picked to match the H100 80GB HBM3 SKU.
const demoVRAMTotalMB uint64 = 81559

// demoNowSec returns the absolute wall-clock phase used by the demo
// backend's time-varying waves. We deliberately anchor to the Unix
// epoch (rather than process-start) so that repeated one-shot invocations
// like `gputui --demo --plain` also show motion between calls — the TUI
// and headless harnesses are both verifiable. Using a 24h modulus keeps
// the float values small and avoids precision loss on very long-running
// processes.
func demoNowSec() float64 {
	const dayNanos = float64(24 * 60 * 60 * 1e9)
	return float64(time.Now().UnixNano()%int64(dayNanos)) / 1e9
}

// demoWave maps an absolute wall-clock phase through a sine of the given
// period and offset, returning a value in [min, max]. Periods staggered
// across personalities prevent visible resonance ("everything peaks at
// once") in the TUI.
func demoWave(periodSec, min, max, offsetSec float64) float64 {
	p := demoNowSec() + offsetSec
	return min + (max-min)*0.5*(1+math.Sin(2*math.Pi*p/periodSec))
}

// demoGPUPersonality describes one synthetic GPU on the demo node. Each
// field is a band that demoBuildGPU samples through a staggered sine, so
// the TUI shows continuous motion rather than a frozen postcard. The
// personalities are deliberately differentiated so multiple hint rules
// fire concurrently and the engine looks alive on camera.
type demoGPUPersonality struct {
	utilMin, utilMax           float64
	utilPeriod, utilOffset     float64
	memUtilMin, memUtilMax     float64
	memUtilPeriod, memUtilOff  float64
	vramFracMin, vramFracMax   float64
	vramPeriod, vramOffset     float64
	tempMin, tempMax           float64
	tempPeriod, tempOffset     float64
	powerFracMin, powerFracMax float64
	powerPeriod, powerOffset   float64

	// perfState is a fixed P-state for the demo (0 = full perf, 8 = parked).
	perfState int
	// throttleReasons is a fixed driver-throttle bitmap for the demo
	// (0 = none, see ClocksThrottleReason* constants for bits).
	throttleReasons uint64
	// pcieGenCurrent / pcieGenMax: 0 means "use fleet default" (Gen5 x16).
	pcieGenCurrent, pcieGenMax     int
	pcieWidthCurrent, pcieWidthMax int
}

// demoClocksThrottleReasonHwThermal mirrors NVIDIA's
// nvmlClocksThrottleReasonHwThermalSlowdown bit (0x40) without forcing
// the gonvml import into a non-Linux build context.
const demoClocksThrottleReasonHwThermal uint64 = 0x40

// demoPersonalities lays out the 8x H100 demo node. Indexing is
// load-bearing: the health-signal builder and the process roster both
// look up GPUs by index. Edits here cascade to those tables.
//
//nolint:gochecknoglobals // table-driven personality data, not runtime mutable state
var demoPersonalities = [8]demoGPUPersonality{
	// GPU 0 — memory-bandwidth-bound exemplar: low GPU util but
	// memory subsystem saturated (model fits, kernel arithmetic
	// intensity is too low to keep SMs busy). Util ceiling sits
	// firmly below the 50% rule threshold so the hint fires
	// continuously through the demo.
	0: {
		utilMin: 25, utilMax: 48, utilPeriod: 17, utilOffset: 0,
		memUtilMin: 82, memUtilMax: 92, memUtilPeriod: 19, memUtilOff: 0,
		vramFracMin: 0.40, vramFracMax: 0.48, vramPeriod: 41, vramOffset: 0,
		tempMin: 60, tempMax: 67, tempPeriod: 23, tempOffset: 1,
		powerFracMin: 0.32, powerFracMax: 0.45, powerPeriod: 19, powerOffset: 0,
	},
	// GPU 1 — healthy busy worker: high util, normal temps, no flags.
	1: {
		utilMin: 80, utilMax: 98, utilPeriod: 23, utilOffset: 5,
		memUtilMin: 55, memUtilMax: 65, memUtilPeriod: 29, memUtilOff: 1,
		vramFracMin: 0.62, vramFracMax: 0.70, vramPeriod: 47, vramOffset: 2,
		tempMin: 70, tempMax: 76, tempPeriod: 29, tempOffset: 3,
		powerFracMin: 0.78, powerFracMax: 0.92, powerPeriod: 23, powerOffset: 4,
	},
	// GPU 2 — VRAM full but otherwise healthy: model loaded, util high.
	// (No hint fires from VRAM% alone in the new ground-truth ruleset.)
	2: {
		utilMin: 68, utilMax: 86, utilPeriod: 19, utilOffset: 8,
		memUtilMin: 50, memUtilMax: 60, memUtilPeriod: 23, memUtilOff: 3,
		vramFracMin: 0.92, vramFracMax: 0.94, vramPeriod: 53, vramOffset: 4,
		tempMin: 72, tempMax: 78, tempPeriod: 31, tempOffset: 5,
		powerFracMin: 0.70, powerFracMax: 0.84, powerPeriod: 17, powerOffset: 7,
	},
	// GPU 3 — gpu-parked: model loaded but driver has parked clocks
	// (P8). VRAM stays full; util goes to 0 so the parked-state rule
	// fires authoritatively (PerfState >= 8 + VRAMUsedMB > 1 GB).
	3: {
		utilMin: 0, utilMax: 8, utilPeriod: 21, utilOffset: 11,
		memUtilMin: 0, memUtilMax: 5, memUtilPeriod: 29, memUtilOff: 4,
		vramFracMin: 0.96, vramFracMax: 0.985, vramPeriod: 59, vramOffset: 6,
		tempMin: 45, tempMax: 50, tempPeriod: 37, tempOffset: 7,
		powerFracMin: 0.10, powerFracMax: 0.18, powerPeriod: 19, powerOffset: 10,
		perfState: 8,
	},
	// GPU 4 — NVLink degraded: util normal, link state handled in
	// the health builder.
	4: {
		utilMin: 75, utilMax: 90, utilPeriod: 27, utilOffset: 14,
		memUtilMin: 60, memUtilMax: 72, memUtilPeriod: 19, memUtilOff: 6,
		vramFracMin: 0.55, vramFracMax: 0.62, vramPeriod: 43, vramOffset: 8,
		tempMin: 71, tempMax: 77, tempPeriod: 33, tempOffset: 9,
		powerFracMin: 0.72, powerFracMax: 0.85, powerPeriod: 21, powerOffset: 13,
	},
	// GPU 5 — confirmed-throttle: HW thermal slowdown bit set in
	// driver bitmap, plus high cumulative thermal-violation counter
	// so the violation-outlier rule also fires.
	5: {
		utilMin: 92, utilMax: 99, utilPeriod: 13, utilOffset: 17,
		memUtilMin: 70, memUtilMax: 82, memUtilPeriod: 17, memUtilOff: 8,
		vramFracMin: 0.80, vramFracMax: 0.86, vramPeriod: 61, vramOffset: 10,
		tempMin: 84, tempMax: 91, tempPeriod: 41, tempOffset: 11,
		powerFracMin: 0.96, powerFracMax: 0.99, powerPeriod: 13, powerOffset: 16,
		throttleReasons: demoClocksThrottleReasonHwThermal,
	},
	// GPU 6 — cool, healthy runner: no flags. The violation-counter
	// rule means this GPU is not flagged just because the rest of the
	// fleet happens to be hot.
	6: {
		utilMin: 60, utilMax: 80, utilPeriod: 31, utilOffset: 20,
		memUtilMin: 45, memUtilMax: 55, memUtilPeriod: 27, memUtilOff: 11,
		vramFracMin: 0.40, vramFracMax: 0.48, vramPeriod: 67, vramOffset: 12,
		tempMin: 62, tempMax: 68, tempPeriod: 47, tempOffset: 13,
		powerFracMin: 0.55, powerFracMax: 0.70, powerPeriod: 27, powerOffset: 19,
	},
	// GPU 7 — pcie-link-degraded + recent ECC blip: PCIe link
	// renegotiated to Gen4 x16 instead of Gen5 x16.
	7: {
		utilMin: 65, utilMax: 82, utilPeriod: 25, utilOffset: 23,
		memUtilMin: 50, memUtilMax: 62, memUtilPeriod: 21, memUtilOff: 14,
		vramFracMin: 0.50, vramFracMax: 0.58, vramPeriod: 71, vramOffset: 14,
		tempMin: 73, tempMax: 80, tempPeriod: 43, tempOffset: 15,
		powerFracMin: 0.66, powerFracMax: 0.80, powerPeriod: 25, powerOffset: 22,
		pcieGenCurrent: 4, pcieGenMax: 5,
		pcieWidthCurrent: 16, pcieWidthMax: 16,
	},
}

// demoBuildGPU samples one GPU's wave personality at the current wall-clock
// phase and produces a model.GPUStat. Naming uses the H100 80GB HBM3 SKU
// so the TUI looks plausible alongside real captures from production nodes.
func demoBuildGPU(idx int) model.GPUStat {
	p := demoPersonalities[idx]
	util := demoWave(p.utilPeriod, p.utilMin, p.utilMax, p.utilOffset)
	memUtil := demoWave(p.memUtilPeriod, p.memUtilMin, p.memUtilMax, p.memUtilOff)
	tempF := demoWave(p.tempPeriod, p.tempMin, p.tempMax, p.tempOffset)
	powerFrac := demoWave(p.powerPeriod, p.powerFracMin, p.powerFracMax, p.powerOffset)
	vramFrac := demoWave(p.vramPeriod, p.vramFracMin, p.vramFracMax, p.vramOffset)

	// Memory clock is stable on H100 (HBM3); graphics clock tracks utilisation.
	graphicsClock := uint32(1200 + (1980-1200)*util/100.0)
	if p.perfState >= 8 {
		// Parked GPUs run at the idle clock floor.
		graphicsClock = 210
	}

	pcieGenCurrent, pcieGenMax := p.pcieGenCurrent, p.pcieGenMax
	if pcieGenMax == 0 {
		pcieGenCurrent, pcieGenMax = 5, 5 // H100 PCIe Gen5
	}
	pcieWidthCurrent, pcieWidthMax := p.pcieWidthCurrent, p.pcieWidthMax
	if pcieWidthMax == 0 {
		pcieWidthCurrent, pcieWidthMax = 16, 16
	}

	stat := model.GPUStat{
		Index:       idx,
		UUID:        fmt.Sprintf("GPU-demo-%08d", idx),
		Name:        "NVIDIA H100 80GB HBM3",
		UtilPct:     util,
		MemUtilPct:  memUtil,
		VRAMUsedMB:  uint64(float64(demoVRAMTotalMB) * vramFrac),
		VRAMTotalMB: demoVRAMTotalMB,
		TempC:       int(tempF),
		PowerW:      700.0 * powerFrac,
		PowerLimitW: 700,
		ClocksMHz: struct {
			Graphics uint32 `json:"graphics,omitempty"`
			Mem      uint32 `json:"mem,omitempty"`
		}{graphicsClock, 2619},
		MaxClocksMHz: struct {
			Graphics uint32 `json:"graphics,omitempty"`
			Mem      uint32 `json:"mem,omitempty"`
		}{1980, 2619},
		ThrottleReasons:  p.throttleReasons,
		PerfState:        p.perfState,
		PCIeGenCurrent:   pcieGenCurrent,
		PCIeGenMax:       pcieGenMax,
		PCIeWidthCurrent: pcieWidthCurrent,
		PCIeWidthMax:     pcieWidthMax,
	}
	return stat
}

// demoProcSpec describes one synthetic process attributed to one demo GPU.
// PIDs are stable across ticks so PID-keyed UI sorts don't churn between
// renders; CPU% and util follow the host GPU's wave with a small offset.
type demoProcSpec struct {
	pid    int
	user   string
	cmd    string
	gpu    int
	vramMB uint64
	cpuMin float64
	cpuMax float64
	cpuOff float64
	rssMB  uint64
}

//nolint:gochecknoglobals // demo fixture data
var demoProcs = []demoProcSpec{
	// GPU 0 — high CPU, low GPU util: classic cpu-bound preprocessing.
	{12001, "researcher", "python preprocess.py --workers=8", 0, 6500, 78, 96, 0, 1820},
	// GPU 1 — healthy heavy training.
	{12042, "researcher", "python train.py --model=llama-7b", 1, 56000, 18, 28, 2, 4096},
	// GPU 2 — VRAM-warn user.
	{12118, "alice", "jupyter-notebook", 2, 71000, 12, 22, 4, 2048},
	{12119, "alice", "python finetune.py --batch=16", 2, 4500, 24, 38, 6, 1024},
	// GPU 3 — VRAM-critical user.
	{12211, "bob", "pytorch_lightning train.py", 3, 75000, 22, 36, 7, 5120},
	{12212, "bob", "python validate.py --large", 3, 4800, 18, 28, 9, 1536},
	// GPU 4 — NVLink benchmarks.
	{12317, "alice", "nccl-tests/all_reduce_perf -b 8 -e 1G", 4, 38000, 30, 50, 11, 768},
	// GPU 5 — hot inference.
	{12404, "researcher", "python eval.py --bench=mlperf", 5, 60000, 35, 55, 13, 3072},
	// GPU 6 — idle-ish exploration.
	{12508, "carol", "python explore.ipynb", 6, 30000, 8, 18, 15, 512},
	// GPU 7 — production inference.
	{12601, "service", "python serve.py --port=8080", 7, 38000, 22, 38, 17, 2560},
	// Host-only / multi-GPU coordination workers (no GPU attribution).
	{12700, "service", "ray::IDLE", 0, 0, 4, 12, 19, 256},
	{12701, "researcher", "tensorboard --logdir=runs/", 1, 0, 6, 14, 21, 384},
}

func demoBuildProcs() []model.ProcStat {
	out := make([]model.ProcStat, 0, len(demoProcs))
	for _, ps := range demoProcs {
		// CPU% and util share the GPU's wave so the table re-sorts visibly.
		gp := demoPersonalities[ps.gpu]
		gpuUtil := demoWave(gp.utilPeriod, gp.utilMin, gp.utilMax, gp.utilOffset+ps.cpuOff)
		cpu := demoWave(13+float64(ps.gpu%5), ps.cpuMin, ps.cpuMax, ps.cpuOff)

		out = append(out, model.ProcStat{
			PID:      ps.pid,
			User:     ps.user,
			Cmd:      ps.cmd,
			GPUIndex: ps.gpu,
			VRAMMB:   ps.vramMB,
			UtilPct:  gpuUtil * 0.85,
			CPUPct:   cpu,
			RSSMB:    ps.rssMB,
		})
	}
	return out
}

// demoBuildHealth produces one GPUHealthSignal per GPU. The personalities
// here line up with demoPersonalities so multiple ground-truth-driven
// hint rules fire concurrently:
//
//   - GPU 4 has 17/18 active NVLinks plus a slowly accumulating CRC
//     counter (trips the nvlink-health rule on outlier ratio + CRC delta;
//     hardware_health rules bypass hysteresis).
//   - GPU 5 has a high cumulative thermal-violation counter so the
//     thermal-violation-outlier rule fires alongside the confirmed-throttle
//     rule that reads the live throttle bitmap.
//   - GPU 7 has a slowly climbing ECCCorrectableVolatile -> info hint
//     (ECC was designed to correct these, so it is no longer warning).
//
// All other GPUs report healthy.
func demoBuildHealth() []model.GPUHealthSignal {
	elapsed := demoNowSec()
	out := make([]model.GPUHealthSignal, 8)
	for i := 0; i < 8; i++ {
		out[i] = model.GPUHealthSignal{
			Index:             i,
			NVLinkTotalLinks:  18,
			NVLinkActiveLinks: 18,
		}
	}
	// GPU 4 — degraded NVLink. CRC counter increments by ~1/sec so the
	// nvlink-health rule observes a positive delta between samples.
	out[4].NVLinkActiveLinks = 17
	out[4].NVLinkCRCErrors = uint64(120 + elapsed)
	// GPU 5 — accumulated thermal-policy enforcement time (driver
	// repeatedly hit the thermal cap while keeping clocks).
	// Counters are cumulative-since-driver-load nanoseconds, so we
	// pick a value well above the per-rule fleet floor.
	out[5].ViolationThermalNs = uint64(60 * 1e9) // ~60 s of cumulative thermal cap
	out[5].ViolationPowerNs = uint64(15 * 1e9)
	// GPU 7 — recent correctable ECC blip; uncorrectable stays at 0.
	out[7].ECCCorrectableVolatile = uint64(3 + elapsed/30.0)
	return out
}

func demoBuildNode() model.NodeStat {
	loadShort := demoWave(31, 3.5, 5.5, 0)
	loadMid := demoWave(73, 3.0, 5.0, 7)
	loadLong := demoWave(151, 2.5, 4.5, 13)
	cpuUser := demoWave(29, 55, 75, 3)
	cpuSys := demoWave(19, 8, 16, 5)
	// CPU iowait spikes on a slower wave so io-bound trips occasionally
	// (only when avg GPU util also dips below 60).
	cpuIowait := demoWave(43, 4, 18, 11)

	return model.NodeStat{
		LoadAvg:      []float64{loadShort, loadMid, loadLong},
		CPUUser:      cpuUser,
		CPUSys:       cpuSys,
		CPUIowait:    cpuIowait,
		MemAvailable: 384 * 1024 * 1024 * 1024,
		MemTotal:     2 * 1024 * 1024 * 1024 * 1024,
	}
}

// demoSnapshot returns a fully synthetic 8x H100 snapshot whose values
// vary tick-to-tick via wall-clock phase, so a viewer watching the TUI
// sees continuous motion. The personality table is wired so multiple
// ground-truth-driven rules fire simultaneously (confirmed-throttle,
// gpu-parked, memory-bandwidth-bound, pcie-link-degraded, nvlink-health,
// thermal-violation-outlier, plus an intermittent cpu-bound trip) —
// making the demo backend a faithful, animated stand-in for the live
// H100 path.
func demoSnapshot() model.Snapshot {
	now := time.Now().UTC()
	gpus := make([]model.GPUStat, 8)
	for i := 0; i < 8; i++ {
		gpus[i] = demoBuildGPU(i)
	}

	return model.Snapshot{
		TS:            now,
		GPUs:          gpus,
		Procs:         demoBuildProcs(),
		Node:          demoBuildNode(),
		HealthSignals: demoBuildHealth(),
	}
}
