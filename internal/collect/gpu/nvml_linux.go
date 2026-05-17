//go:build linux

package gpu

import (
	"context"
	"fmt"
	"sync"

	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/indraputrabh/gputui/internal/model"
)

// nvmlCollector queries NVML. Individual NVML query functions
// (GetUtilizationRates, GetMemoryInfo, ECC/NVLink getters, etc.) are
// documented thread-safe by NVIDIA once Init has completed, so there is
// no outer mutex serialising them. Init and handle-cache refresh are
// the only paths that mutate shared state; both are guarded by mu.
type nvmlCollector struct {
	mu          sync.Mutex
	initialized bool
	handles     []gonvml.Device
}

func (c *nvmlCollector) Source() Source {
	return SourceNVML
}

func (c *nvmlCollector) Collect(ctx context.Context) ([]model.GPUStat, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}

	handles, err := c.deviceHandles()
	if err != nil {
		return nil, err
	}

	gpus := make([]model.GPUStat, 0, len(handles))
	for i, device := range handles {
		stat, err := collectDeviceStat(ctx, i, device)
		if err != nil {
			return nil, err
		}
		gpus = append(gpus, stat)
	}

	return gpus, nil
}

func collectDeviceStat(ctx context.Context, index int, device gonvml.Device) (model.GPUStat, error) {
	select {
	case <-ctx.Done():
		return model.GPUStat{}, ctx.Err()
	default:
	}

	stat := model.GPUStat{Index: index}
	if name, r := device.GetName(); r == gonvml.SUCCESS {
		stat.Name = name
	}
	if uuid, r := device.GetUUID(); r == gonvml.SUCCESS {
		stat.UUID = uuid
	}
	if util, r := device.GetUtilizationRates(); r == gonvml.SUCCESS {
		stat.UtilPct = float64(util.Gpu)
		stat.MemUtilPct = float64(util.Memory)
	}
	if mem, r := device.GetMemoryInfo(); r == gonvml.SUCCESS {
		stat.VRAMUsedMB = mibFromBytes(mem.Used)
		stat.VRAMTotalMB = mibFromBytes(mem.Total)
	}
	if temp, r := device.GetTemperature(gonvml.TEMPERATURE_GPU); r == gonvml.SUCCESS {
		stat.TempC = int(temp)
	}
	if powerMw, r := device.GetPowerUsage(); r == gonvml.SUCCESS {
		stat.PowerW = wattsFromMilliwatts(powerMw)
	}
	if limitMw, r := device.GetPowerManagementLimit(); r == gonvml.SUCCESS {
		stat.PowerLimitW = wattsFromMilliwatts(limitMw)
	}
	if clk, r := device.GetClockInfo(gonvml.CLOCK_GRAPHICS); r == gonvml.SUCCESS {
		stat.ClocksMHz.Graphics = clk
	}
	if clk, r := device.GetClockInfo(gonvml.CLOCK_MEM); r == gonvml.SUCCESS {
		stat.ClocksMHz.Mem = clk
	}
	if clk, r := device.GetMaxClockInfo(gonvml.CLOCK_GRAPHICS); r == gonvml.SUCCESS {
		stat.MaxClocksMHz.Graphics = clk
	}
	if clk, r := device.GetMaxClockInfo(gonvml.CLOCK_MEM); r == gonvml.SUCCESS {
		stat.MaxClocksMHz.Mem = clk
	}
	if reasons, r := device.GetCurrentClocksThrottleReasons(); r == gonvml.SUCCESS {
		stat.ThrottleReasons = reasons
	}
	if pstate, r := device.GetPerformanceState(); r == gonvml.SUCCESS {
		stat.PerfState = int(pstate)
	} else {
		stat.PerfState = int(gonvml.PSTATE_UNKNOWN)
	}
	if gen, r := device.GetCurrPcieLinkGeneration(); r == gonvml.SUCCESS {
		stat.PCIeGenCurrent = gen
	}
	if gen, r := device.GetMaxPcieLinkGeneration(); r == gonvml.SUCCESS {
		stat.PCIeGenMax = gen
	}
	if width, r := device.GetCurrPcieLinkWidth(); r == gonvml.SUCCESS {
		stat.PCIeWidthCurrent = width
	}
	if width, r := device.GetMaxPcieLinkWidth(); r == gonvml.SUCCESS {
		stat.PCIeWidthMax = width
	}
	return stat, nil
}

// CollectProcesses enumerates GPU-attached processes across all devices.
func (c *nvmlCollector) CollectProcesses(ctx context.Context) ([]model.ProcStat, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}

	handles, err := c.deviceHandles()
	if err != nil {
		return nil, err
	}

	var procs []model.ProcStat
	seen := make(map[uint32]bool)

	for i, device := range handles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		for _, p := range collectDeviceProcesses(device, i) {
			pid := uint32(p.PID)
			if seen[pid] {
				continue
			}
			seen[pid] = true
			procs = append(procs, p)
		}
	}

	return procs, nil
}

func collectDeviceProcesses(device gonvml.Device, gpuIndex int) []model.ProcStat {
	var procs []model.ProcStat

	type getter func() ([]gonvml.ProcessInfo, gonvml.Return)
	for _, fn := range []getter{
		device.GetComputeRunningProcesses,
		device.GetGraphicsRunningProcesses,
	} {
		infos, ret := fn()
		if ret != gonvml.SUCCESS {
			continue
		}
		for _, info := range infos {
			// User/Cmd resolution is deferred to the enricher, which keeps
			// a cache keyed on (pid, starttime) to avoid repeating /proc
			// reads every sample. NVML only supplies PID + VRAM here.
			procs = append(procs, model.ProcStat{
				PID:      int(info.Pid),
				GPUIndex: gpuIndex,
				VRAMMB:   info.UsedGpuMemory / (1024 * 1024),
			})
		}
	}

	return procs
}

// ensureInit initialises NVML once per process. Safe to call from
// concurrent goroutines; only the first caller runs gonvml.Init().
func (c *nvmlCollector) ensureInit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	ret := gonvml.Init()
	if ret != gonvml.SUCCESS && ret != gonvml.ERROR_ALREADY_INITIALIZED {
		return fmt.Errorf("nvml Init failed: %s", ret)
	}

	c.initialized = true
	return nil
}

// deviceHandles returns cached device handles, refreshing the cache if the
// device count changed. Thread-safe; multiple callers may invoke this
// concurrently and the cache is refreshed at most once per count change.
func (c *nvmlCollector) deviceHandles() ([]gonvml.Device, error) {
	count, ret := gonvml.DeviceGetCount()
	if ret != gonvml.SUCCESS {
		return nil, fmt.Errorf("nvml DeviceGetCount failed: %s", ret)
	}

	c.mu.Lock()
	if len(c.handles) == count {
		handles := c.handles
		c.mu.Unlock()
		return handles, nil
	}
	c.mu.Unlock()

	handles := make([]gonvml.Device, 0, count)
	for i := 0; i < count; i++ {
		device, ret := gonvml.DeviceGetHandleByIndex(i)
		if ret != gonvml.SUCCESS {
			return nil, fmt.Errorf("nvml DeviceGetHandleByIndex(%d) failed: %s", i, ret)
		}
		handles = append(handles, device)
	}

	c.mu.Lock()
	c.handles = handles
	result := c.handles
	c.mu.Unlock()
	return result, nil
}

// invalidateHandles clears the cached handle list so the next Collect
// re-enumerates devices. Reserved for NVML re-init paths (e.g. driver
// reset detection) added in a follow-up; kept here so the locking
// contract lives next to the cache it protects.
//
//nolint:unused // wired up by re-init path landing in a follow-up commit
func (c *nvmlCollector) invalidateHandles() {
	c.mu.Lock()
	c.handles = nil
	c.mu.Unlock()
}

func mibFromBytes(v uint64) uint64 {
	return v / (1024 * 1024)
}

func wattsFromMilliwatts(v uint32) float64 {
	return float64(v) / 1000.0
}
