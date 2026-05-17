# Metric Mapping (NVML/DCGM -> gputui)

This document is the source of truth for metric semantics and unit conversions.

Status legend:
- `verified`: explicitly confirmed in official docs.
- `needs_verification`: likely correct, but not explicitly stated in a cited line.

Last reviewed: 2026-05-16

---

## Official References

- NVML API docs (official): `https://docs.nvidia.com/deploy/nvml-api/`
- NVML device queries: `https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html`
- NVML memory struct: `https://docs.nvidia.com/deploy/nvml-api/structnvmlMemory__t.html`
- NVML utilization struct: `https://docs.nvidia.com/deploy/nvml-api/structnvmlUtilization__t.html`
- NVML clocks-throttle reasons: `https://docs.nvidia.com/deploy/nvml-api/group__nvmlClocksThrottleReasons.html`
- DCGM field IDs (official): `https://docs.nvidia.com/datacenter/dcgm/latest/dcgm-api/dcgm-api-field-ids.html`
- NVIDIA go-nvml header mirror: `https://raw.githubusercontent.com/NVIDIA/go-nvml/main/gen/nvml/nvml.h`
- NVIDIA DCGM header: `https://raw.githubusercontent.com/NVIDIA/DCGM/master/dcgmlib/dcgm_fields.h`

---

## GPUStat Mapping

| gputui field | NVML source | DCGM source | Unit in source | Conversion to model | Status | Notes |
|---|---|---|---|---|---|---|
| `util_pct` | `nvmlDeviceGetUtilizationRates()` -> `nvmlUtilization_t.gpu` | `DCGM_FI_DEV_GPU_UTIL` | NVML: percent over sample period | none (`float64`) | verified | NVML defines this as the *fraction of the sample window during which any kernel was executing on any SM*, not the fraction of peak FLOP throughput. A small memory-bound kernel can pin this at 100% while delivering <5% of peak. See "util.gpu trap" below. |
| `mem_util_pct` | `nvmlDeviceGetUtilizationRates()` -> `nvmlUtilization_t.memory` | `DCGM_FI_DEV_MEM_COPY_UTIL` | NVML: percent over sample period | none (`float64`) | verified | Fraction of the sample window the memory controller was busy. The `memory-bandwidth-bound` hint fires on low `util_pct` + high `mem_util_pct`. |
| `vram_used_mb` | `nvmlDeviceGetMemoryInfo()` -> `nvmlMemory_t.used` | `DCGM_FI_DEV_FB_USED` | NVML: bytes, DCGM: MB | NVML: `bytes / (1024*1024)`; DCGM: none | verified | NVML docs explicitly say bytes; DCGM field description says "Used Frame Buffer in MB." |
| `vram_total_mb` | `nvmlDeviceGetMemoryInfo()` -> `nvmlMemory_t.total` | `DCGM_FI_DEV_FB_TOTAL` | NVML: bytes, DCGM: MB | NVML: `bytes / (1024*1024)`; DCGM: none | verified | DCGM field description says "Total Frame Buffer ... in MB." |
| `temp_c` | `nvmlDeviceGetTemperatureV()` (preferred), `nvmlDeviceGetTemperature()` (legacy) | `DCGM_FI_DEV_GPU_TEMP` | degrees C | none | verified | NVML headers mark `nvmlDeviceGetTemperature()` deprecated and document `GetTemperatureV` for current readings in degrees C. DCGM explicitly says "in degrees C." |
| `power_w` | `nvmlDeviceGetPowerUsage()` | `DCGM_FI_DEV_POWER_USAGE` | NVML: milliwatts, DCGM: Watts | NVML: `mW / 1000.0`; DCGM: none | verified | NVML query docs: "power usage ... in milliwatts." DCGM says "Power usage ... in Watts." |
| `power_limit_w` | `nvmlDeviceGetPowerManagementLimit()` | (TBD) | NVML: milliwatts | `mW / 1000.0` | needs_verification | NVML device query page states power management limit values in mW; DCGM equivalent not locked in yet. |
| `clocks_mhz.graphics` | `nvmlDeviceGetClockInfo(NVML_CLOCK_GRAPHICS)` | (TBD) | MHz | none | verified | NVML query docs explicitly say returned clock speed is in MHz. |
| `clocks_mhz.mem` | `nvmlDeviceGetClockInfo(NVML_CLOCK_MEM)` | (TBD) | MHz | none | verified | Same API and units (MHz). |
| `max_clocks_mhz.graphics` | `nvmlDeviceGetMaxClockInfo(NVML_CLOCK_GRAPHICS)` | (TBD) | MHz | none | verified | The maximum allowed clock for the device. The `confirmed-throttle` hint compares current/max as evidence so users see how much the throttle is actually costing. |
| `max_clocks_mhz.mem` | `nvmlDeviceGetMaxClockInfo(NVML_CLOCK_MEM)` | (TBD) | MHz | none | verified | Same API, memory clock domain. |
| `throttle_reasons` | `nvmlDeviceGetCurrentClocksThrottleReasons()` | `DCGM_FI_DEV_CLOCK_THROTTLE_REASONS` | bitmap | none (`uint64`) | verified | Bitmap of `nvmlClocksThrottleReason*`. The `confirmed-throttle` rule decodes it; HW thermal/power-brake bits = critical, SW thermal/power cap = warning, idle/applications-clocks/display-clock = ignored. |
| `perf_state` | `nvmlDeviceGetPerformanceState()` | `DCGM_FI_DEV_PSTATE` | enum 0-15 (32 = unknown) | `int(pstate)` | verified | 0 = max performance, 15 = min, 32 = unknown. The `gpu-parked` rule fires on P8+ with VRAM allocated. |
| `pcie_gen_current` | `nvmlDeviceGetCurrPcieLinkGeneration()` | `DCGM_FI_DEV_PCIE_LINK_GEN` | int | none | verified | Current negotiated PCIe generation. |
| `pcie_gen_max` | `nvmlDeviceGetMaxPcieLinkGeneration()` | (TBD) | int | none | verified | Hardware-supported max. |
| `pcie_width_current` | `nvmlDeviceGetCurrPcieLinkWidth()` | `DCGM_FI_DEV_PCIE_LINK_WIDTH` | int (lanes) | none | verified | Current negotiated lane count. |
| `pcie_width_max` | `nvmlDeviceGetMaxPcieLinkWidth()` | (TBD) | int (lanes) | none | verified | Hardware-supported max lane count. |

## GPUHealthSignal Mapping

| gputui field | NVML source | Unit in source | Conversion | Notes |
|---|---|---|---|---|
| `ecc_uncorrectable_volatile` | `GetTotalEccErrors(MEMORY_ERROR_TYPE_UNCORRECTED, VOLATILE_ECC)` | error count | none | Reset on driver reload. Uncorrectable -> critical hint. |
| `ecc_correctable_volatile` | `GetTotalEccErrors(MEMORY_ERROR_TYPE_CORRECTED, VOLATILE_ECC)` | error count | none | ECC was designed to correct these silently; surfaced as `info` only. |
| `nvlink_active_links` / `nvlink_total_links` | `GetNvLinkState()` per link index | bool per link | aggregated counts | Used to compute the fleet-median active ratio for the nvlink-health rule. |
| `nvlink_crc_errors` | `GetNvLinkErrorCounter(DL_CRC_DATA + DL_CRC_FLIT)` summed | error count | none | Counter is cumulative; rule fires on the *delta* between samples. |
| `remapped_rows_correctable` / `_uncorrectable` / `_pending` | `GetRemappedRows()` | int / bool | none | A8 / H100 row-remapping state. |
| `violation_thermal_ns` | `GetViolationStatus(PERF_POLICY_THERMAL).violationTime` | nanoseconds | none | Cumulative time the driver enforced the thermal cap. The `thermal-violation-outlier` rule flags GPUs whose counter is >=4× fleet median (with a 2 s floor). |
| `violation_power_ns` | `GetViolationStatus(PERF_POLICY_POWER)` | nanoseconds | none | Cumulative time enforcing the board power cap. |
| `violation_sync_boost_ns` | `GetViolationStatus(PERF_POLICY_SYNC_BOOST)` | nanoseconds | none | Reserved for future hints. |
| `violation_board_limit_ns` | `GetViolationStatus(PERF_POLICY_BOARD_LIMIT)` | nanoseconds | none | Reserved for future hints. |
| `violation_low_util_ns` | `GetViolationStatus(PERF_POLICY_LOW_UTILIZATION)` | nanoseconds | none | Reserved for future hints. |
| `violation_reliability_ns` | `GetViolationStatus(PERF_POLICY_RELIABILITY)` | nanoseconds | none | Reserved for future hints. |

> `nvmlDeviceGetViolationStatus` is marked deprecated by NVIDIA in favour of `DeviceGetFieldValues`. It is still implemented and returning data on current drivers, and the gputui hints surface it as evidence rather than as a primary source of truth -- if a future driver removes it, the rule degrades gracefully (counters stay at 0; the outlier rule simply produces no hint).

---

## Snapshot/Collector Semantics

### Sampling-window caveat (util.gpu and util.memory)

NVML's `nvmlUtilization_t.gpu` is the *fraction of the sample window during which one or more kernels were running on any SM*. It is **not** the fraction of peak FLOPs delivered. A small memory-bound kernel can pin this at 100% while delivering well below 10% of arithmetic peak. The `memory-bandwidth-bound` hint exploits the matching `nvmlUtilization_t.memory` field to spot this case.

For deeper arithmetic-intensity profiling, use NVIDIA Nsight Compute or DCGM profiling fields (`DCGM_FI_PROF_SM_ACTIVE`, `DCGM_FI_PROF_TENSOR_ACTIVE`); these are out of scope for v0.1.0 because they require linking libdcgm.

Power readings are also windowed: Yang, Adamek and Armour (SC'24) found that NVML's built-in power sensor on A100/H100 GPUs samples roughly 25% of runtime, so `PowerW` should be treated as an averaged window rather than instantaneous draw.

### Event-sourced signals (no sampling-window caveat)

`throttle_reasons`, `perf_state`, ECC counters, NVLink state, and the `violation_*_ns` counters are all driver-maintained event/cumulative values, not sampled. They reflect the driver's authoritative answer at the moment of the query, so the 25% sampling caveat above does not apply.

### API version caveat

- Prefer non-deprecated NVML calls when both old/new variants exist.
- Keep the fallback path documented in code comments (for older drivers/toolkits).

### Reserved memory caveat

- NVML memory docs note reserved memory behavior (and v2 call differences).
- If mixing NVML and DCGM sources, document whether "reserved" is included.

---

## Implementation Rules (to prevent unit bugs)

1. Always normalize units before writing to `model`:
   - bytes -> MiB for `vram_*_mb`
   - mW -> W for `power_*_w`
2. Keep conversion code close to collector calls (not spread across UI).
3. Add source tags in debug logs (e.g., `source=nvml` or `source=dcgm`).
4. Never merge NVML + DCGM values for the same field in one snapshot unless explicitly configured.

---

## Validation Checklist

- [ ] Run collector with NVML path and save sample JSONL.
- [ ] Run `nvidia-smi --query-gpu=utilization.gpu,utilization.memory,memory.used,memory.total,temperature.gpu,power.draw,clocks.current.graphics,clocks.max.graphics,pcie.link.gen.current,pcie.link.gen.max,pcie.link.width.current,pcie.link.width.max,pstate --format=csv,noheader,nounits`.
- [ ] Confirm value deltas are within expected sampling variance.
- [ ] Compare `throttle_reasons` against `nvidia-smi --query-gpu=clocks_throttle_reasons.active --format=csv`.
- [ ] Compare `violation_thermal_ns` deltas against `nvidia-smi -q -d PERFORMANCE`.
- [ ] Record GPU model + driver + CUDA/NVML versions in milestone notes.

---

## Per-Process Host Stats Mapping

| gputui field | Source | Unit | Notes |
|---|---|---|---|
| `cpu_pct` | `/proc/[pid]/stat` (utime + stime delta) | % | Delta-based: requires two samples. First sample yields 0%. |
| `rss_mb` | `/proc/[pid]/status` -> `VmRSS` | kB -> MB | Divided by 1024 to convert from kB to MB. |
| `io_read_bytes` | `/proc/[pid]/io` -> `read_bytes` | bytes | Cumulative; may require root for other users' processes. |
| `io_write_bytes` | `/proc/[pid]/io` -> `write_bytes` | bytes | Cumulative; same permissions caveat. |

---

## Notes for This Repo (current state)

- NVML Linux baseline collector covers all GPUStat and GPUHealthSignal fields above.
- DCGM collector is scaffolded but not implemented; NVML covers v0.1.0 needs.
- Per-process host stats (CPU%, RSS, IO) are collected via `/proc` and enriched in the pipeline.
- Log collector parses `dmesg` for NVIDIA Xid errors and OOM kills.
- Treat this file as the mandatory reference for any new collector field.
