# Hint Validation Methodology

## Overview

The `gputui` hints engine emits two classes of hints with very
different validation characteristics. Documenting them honestly matters
more than chasing a single accuracy number -- a percentage out of
context isn't useful, and the previous draft of this document quoted
"~100%" against synthetic scenarios in a way that obscured what was
actually being measured.

### Class 1: NVML ground-truth signals

Hints whose firing condition is a value the driver explicitly publishes:

| Hint | Driver source | Firing condition |
|------|---------------|------------------|
| `confirmed-throttle` | `GetCurrentClocksThrottleReasons` | bitmap & meaningful-bits != 0 |
| `gpu-parked` | `GetPerformanceState` + memory info | P-state >= 8 with VRAM > 1 GiB |
| `pcie-link-degraded` | `GetCurrPcieLinkGen/Width` vs `Max` | current < max |
| `gpu-ecc-errors` | `GetTotalEccErrors(VOLATILE_ECC)` | uncorrectable > 0 (critical) / correctable > 0 (info) |
| `nvlink-health` (inactive lanes) | `GetNvLinkState` | this GPU's active ratio < fleet median |
| `gpu-xid-error` / `host-oom-kill` | `dmesg` | matching log line found |

These rules are deterministic in the published driver values: if NVML
reports the value, the rule fires; if not, it doesn't. Their
trustworthiness is bounded by NVML's own correctness, which is the
strongest substrate available short of profiling counters.

False positives in this class are rare and tractable to characterise:

- `confirmed-throttle`: false-positive only if the operator deliberately
  set `nvmlDeviceSetApplicationsClocks`, which the rule already filters
  by ignoring `ClocksThrottleReasonApplicationsClocksSetting`.
- `pcie-link-degraded`: false-positive on virtualised drivers that
  don't report a max gen/width; the rule skips when max is 0.
- `gpu-parked`: false-positive on idle systems that just happen to have
  >1 GiB resident; the >=P8 + >1 GiB combination is a fairly narrow
  window in practice.
- `gpu-ecc-errors`: NVML resets volatile counters on driver reload, so
  a recent reload can mask a previously failing GPU; persistent ECC
  errors are read from the persistent counters separately if added.

False negatives happen when a meaningful condition exists but the
driver isn't telling us about it (e.g. very old driver versions where
specific throttle bits aren't surfaced). These are inherent to NVML.

### Class 2: Statistical / correlation rules

Hints where the firing condition is a relationship between metrics
rather than a single ground-truth value:

| Hint | Inputs | Firing condition |
|------|--------|------------------|
| `memory-bandwidth-bound` | `util.gpu`, `util.memory` | util.gpu < 50% AND util.memory > 80% |
| `thermal-violation-outlier` | `violation_thermal_ns` per GPU | this GPU >=4× fleet median + >=2 s |
| `nvlink-health` (CRC delta) | NVLink CRC counters across samples | counter increased between samples |
| `potential-cpu-bound-preprocessing` | avg GPU util + host CPU user | util < 60% AND CPU user > 50% |
| `potential-io-bound-pipeline` | avg GPU util + host iowait | util < 60% AND iowait > 15% |

These rules carry threshold choices and report `consistent with X if a
GPU workload is expected` framing rather than asserting a root cause.
They are intended as correlation prompts; the operator decides whether
the suggestion matches the workload they expected.

### Class 3: Log-derived markers

`gpu-xid-error` and `host-oom-kill` surface `dmesg` lines verbatim. A
hit means the kernel logged the event. The rule does **not** classify
the Xid code (some are hardware, some driver, some software per the
official Xid reference) or claim that an OOM was due to host vs.
container exhaustion -- it simply preserves the evidence so the
operator can interpret.

---

## Validation Approach

### Per-rule unit tests

Every rule has unit tests for fire/no-fire behaviour at the
boundary thresholds. See `internal/hints/rules/*_test.go`. These run
in CI on every push.

### Hysteresis comparison harness

`cmd/eval-hysteresis` runs the engine over a borderline workload with
hysteresis off and on, writing JSONL traces and a Markdown summary
that quantifies the reduction in flap rate. This is illustrative
rather than a numeric promise -- workloads vary.

### Live-hardware validation (manual)

On a real NVIDIA node, `gputui` can be cross-checked against
`nvidia-smi`:

| gputui hint | Cross-check |
|-------------|-------------|
| `confirmed-throttle` | `nvidia-smi --query-gpu=clocks_throttle_reasons.active --format=csv` |
| `gpu-parked` | `nvidia-smi --query-gpu=pstate,memory.used --format=csv` |
| `pcie-link-degraded` | `nvidia-smi --query-gpu=pcie.link.gen.current,pcie.link.gen.max,pcie.link.width.current,pcie.link.width.max --format=csv` |
| `thermal-violation-outlier` | `nvidia-smi -q -d PERFORMANCE` (look at the Violation Status block) |
| `gpu-ecc-errors` | `nvidia-smi --query-gpu=ecc.errors.uncorrected.volatile.total --format=csv` |
| `nvlink-health` | `nvidia-smi nvlink -s`, `nvidia-smi nvlink -e` |
| `gpu-xid-error` | `dmesg | grep NVRM` |
| `host-oom-kill` | `dmesg | grep -i 'killed process'` |

If the gputui hint and the matching `nvidia-smi` query disagree, that
is a bug (file an issue with the driver / NVML version).

---

## Comparison with Existing Tools

| Capability | gputui | nvidia-smi | nvtop |
|------------|--------|------------|-------|
| Per-GPU metrics | Yes | Yes | Yes |
| Per-process GPU attribution | Yes | Yes (`pmon`) | Yes |
| Host metrics (CPU/mem/IO) | Yes | No | Limited |
| Throttle-reason decode | Yes | Yes (`--query-gpu=clocks_throttle_reasons.active`) | No |
| PCIe-link-degraded check | Yes | Manual query | No |
| Memory-bandwidth-bound hint | Yes | No | No |
| Hardware health (ECC / NVLink / remap) | Yes | Manual query | No |
| Hysteresis / flap suppression | Yes | N/A | N/A |
| Evidence-backed hints | Yes | No | No |
