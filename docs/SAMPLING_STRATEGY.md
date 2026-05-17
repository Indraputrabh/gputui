# Sampling Strategy and Overhead

## Polling Interval

`gputui` defaults to a 2-second polling interval (`--refresh 2s`).

### Trade-offs

| Interval | GPU overhead | Host overhead | UI responsiveness |
|----------|-------------|---------------|-------------------|
| 500ms | ~0.1% CPU per GPU | Negligible | Very responsive |
| 1s | ~0.05% CPU per GPU | Negligible | Good |
| **2s** | **~0.025% CPU per GPU** | **Negligible** | **Default** |
| 5s | ~0.01% CPU per GPU | Negligible | Sluggish |

The 2 s default balances responsiveness with minimal overhead. NVML
calls are non-blocking and typically complete in well under 1 ms per
GPU.

## Sampled vs. event-sourced NVML signals

`gputui` reads two distinct kinds of NVML data, with different sampling
semantics:

### Sampled (subject to NVML's internal sample window)

| Field | Reads | Notes |
|-------|-------|-------|
| `util_pct` | `nvmlUtilization_t.gpu` | Fraction of the sample window any kernel was on the SMs (~1 s hardware window). |
| `mem_util_pct` | `nvmlUtilization_t.memory` | Fraction of the sample window the memory controller was busy. |
| `power_w` | `GetPowerUsage()` | Yang, Adamek and Armour (SC'24) found NVML's power sensor on A100/H100 samples ~25% of runtime, so this is a *windowed* draw, not instantaneous. |
| `temp_c` | `GetTemperature()` | Driver-side temp sample. |
| `clocks_mhz.*` | `GetClockInfo()` | Reported as recent values; effectively instantaneous on current drivers. |

### Event-sourced (no sampling-window dependence)

| Field | Reads | Notes |
|-------|-------|-------|
| `throttle_reasons` | `GetCurrentClocksThrottleReasons` | Driver-maintained bitmap; reflects state at the moment of the query. |
| `perf_state` | `GetPerformanceState` | Driver-maintained P-state. |
| `max_clocks_mhz.*` | `GetMaxClockInfo` | Static hardware capability. |
| `pcie_gen_*`, `pcie_width_*` | PCIe link getters | Driver-reported link state. |
| `violation_*_ns` | `GetViolationStatus(...)` | Cumulative-since-driver-load nanoseconds enforcing each perf policy. The hint engine uses values *as reported* and (for the outlier rule) compares them to the fleet median. |
| ECC counters, NVLink state, remapped rows, NVLink CRC | various | Same: driver-maintained, query-on-demand. |
| Xid / OOM markers | `dmesg` | Kernel log lines, parsed once per sample. |

Because hints like `confirmed-throttle`, `gpu-parked`, and
`pcie-link-degraded` read event-sourced signals, their behaviour does
**not** depend on the NVML sampling-window caveat. They reflect what
the driver believes right now, regardless of how busy the sampling
window happened to be.

The `memory-bandwidth-bound` rule does depend on the sampling window
(both inputs are `nvmlUtilization_t` fields), so its accuracy improves
with sustained workloads and degrades on very short bursts -- in
practice the hysteresis layer absorbs most of the noise.

## Host Metric Collection (`/proc`)

### CPU Percentages

CPU user/sys/iowait are computed from delta between two consecutive
reads of `/proc/stat`. The first sample after startup returns 0% (no
previous baseline). Subsequent reads reflect the true interval.

Cost: one `read()` syscall per sample (~5µs).

### Memory

`/proc/meminfo` is a single read. `MemAvailable` is preferred over
`MemFree` as it accounts for reclaimable caches.

Cost: one `read()` syscall (~8µs).

### Load Average

`/proc/loadavg` is a single read returning 1m/5m/15m kernel-maintained
running averages.

Cost: one `read()` syscall (~3µs).

## Ring Buffer Memory Budget

The TUI maintains a per-GPU ring buffer for sparkline history (up to
the longest configured zoom window, 10 minutes at 2 s interval =
300 samples).

Per GPU: 300 samples × 5 metrics × 8 bytes = 12 KB
8 GPUs: ~96 KB total.

This is negligible relative to the terminal buffer and Go runtime
overhead.

## Process Attribution

`GetComputeRunningProcesses` and `GetGraphicsRunningProcesses` are
called once per sample per GPU. Each returned PID triggers a `/proc`
lookup (cached by `(pid, starttime)`) for user and command resolution.

Cost per GPU: ~50µs (NVML) + ~20µs per process (cold `/proc` reads,
warm reads are essentially free thanks to the cache).

## Health Signal Collection

ECC error counters, NVLink state, remapped rows, and the six perf-policy
violation counters are NVML register reads. They add roughly
~150 µs per GPU per sample. Health collection runs on its own 10 s
cadence in a background goroutine so a slow health pass cannot stall
the per-sample TUI / record loop.

## References

Yang, Z., Adamek, K. and Armour, W. (2024) 'Accurate and Convenient
Energy Measurements for GPUs', SC'24. doi:10.1109/SC41406.2024.00028.
