# Roadmap

This is a personal project. The roadmap is a wishlist, not a commitment.

---

## v0.1.0 (current)

Shipped in the initial public release:

* Standalone `gputui` and split `gputui-agent` + client modes
* NVML GPU collector: util.gpu / util.memory, VRAM, temp, power, clocks (current and max), throttle reasons bitmap, performance state, PCIe gen + width (current and max)
* NVML health collector: ECC volatile (correctable / uncorrectable), NVLink lanes + CRC, remapped rows, per-policy violation counters (thermal, power, sync-boost, board-limit, low-util, reliability)
* Per-process GPU attribution (NVML compute + graphics processes)
* Per-process host enrichment: user, command, CPU%, RSS, IO bytes
* Host collector: load average, CPU (incl. iowait), MemAvailable, swap
* Log collector: dmesg-based NVIDIA Xid and Linux OOM marker extraction
* Hint rules driven primarily by NVML ground-truth signals:
  * `confirmed-throttle` -- driver throttle bitmap (HW thermal/power-brake = critical, SW caps = warning)
  * `gpu-parked` -- model loaded (>1 GiB VRAM) but driver parked at P8+
  * `memory-bandwidth-bound` -- low GPU util + high memory-controller util
  * `pcie-link-degraded` -- PCIe link below max generation or width
  * `thermal-violation-outlier` -- cumulative thermal-cap enforcement time vs fleet median
  * `nvlink-health` -- inactive lanes (vs fleet median) or CRC errors accumulating
  * `gpu-ecc-errors` -- volatile uncorrectable (critical) / correctable (info)
  * `potential-cpu-bound-preprocessing` -- low GPU util + high host CPU user time
  * `potential-io-bound-pipeline` -- low GPU util + high `iowait`
  * `gpu-xid-error` / `host-oom-kill` -- log-derived stability markers
* Decay-based hysteresis on borderline performance hints; hardware-health hints bypass it.
* Bubble Tea TUI: GPU table with throttle / parked badges, scrollable process table, active + history hints panel, sparklines, NVLink + errors-seen, pause/help, hint detail popup.
* Plain text mode (`--plain`) for SSH and scripting.
* Unix-socket API: `/healthz`, `/v1/snapshot`.
* JSONL snapshot + hint recorder.
* GitHub Actions CI: build, test -race, vet, golangci-lint, gofmt.

---

## Ideas (no commitment)

* DCGM profiling fields (`DCGM_FI_PROF_SM_ACTIVE`, `TENSOR_ACTIVE`, etc.) -- requires linking libdcgm; would let us distinguish "100% util" from "100% peak FLOPs" directly.
* `nvmlDeviceGetSamples` historical-API integration for per-sample timestamped data.
* Per-process throttle attribution (DCGM or `nvidia-smi pmon`).
* Optional Prometheus exporter for hints + GPU/host metrics.
* AMD ROCm backend (`rocm-smi`) behind a build tag.
* `--config` file for hint thresholds and rule toggles.
* Per-rule confidence calibration based on recorded JSONL.
* Container build (`Dockerfile`, optional Helm chart) for daemon deployments.
* `gputui replay <snapshots.jsonl>` for offline review.
* Web UI mirror of the TUI for shared sessions.

---

## Non-goals

* Not a full monitoring platform (Prometheus/Grafana integration is up to you).
* Not an auto-remediation system (no evictions, taints, or restarts).
* Not deep GPU microarchitecture profiling (use Nsight for that).
* Not universal portability across all distros/kernels (Linux + NVIDIA only for now).

PRs and issues welcome. See [CONTRIBUTING.md](CONTRIBUTING.md).
