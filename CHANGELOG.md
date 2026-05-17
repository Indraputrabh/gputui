# Changelog

All notable changes to this project are documented here. Format roughly follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows [Semantic Versioning](https://semver.org/).

## [v0.1.0] - 2026-05-17

Initial public release.

### Added

* Standalone `gputui` binary with built-in collectors -- no agent required by default. Split `gputui-agent` + `gputui --agent <socket>` mode for daemon deployments.
* NVML collector reads ground-truth driver signals: utilisation (gpu / memory), VRAM, temperature, power, current and max clocks, current and max PCIe gen + width, performance state, and the clocks-throttle reasons bitmap.
* NVML health collector: ECC volatile (correctable / uncorrectable), NVLink active links + CRC counters per link, remapped rows, and per-policy violation counters (`nvmlDeviceGetViolationStatus`) for thermal, power, sync-boost, board-limit, low-utilisation, and reliability policies.
* Per-process GPU attribution via NVML (compute + graphics processes).
* `/proc` enrichment: user, command, CPU% (delta-based), RSS, IO bytes.
* Host collector: load average, CPU breakdown including iowait, `MemAvailable`, swap.
* Log collector: NVIDIA Xid errors and Linux OOM kill events parsed from `dmesg`.
* Hints engine with 11 rules driven primarily by NVML ground-truth signals, plus decay-based hysteresis on borderline performance hints (hardware-health hints bypass it):
  * `confirmed-throttle` -- driver throttle bitmap (HW thermal/power-brake = critical, SW caps = warning, idle/applications-clocks/display ignored).
  * `gpu-parked` -- model loaded (>1 GiB VRAM) but driver parked at P8+.
  * `memory-bandwidth-bound` -- low GPU util + high memory-controller util (low arithmetic-intensity kernel).
  * `pcie-link-degraded` -- PCIe link below max generation or width.
  * `thermal-violation-outlier` -- cumulative thermal-cap enforcement >=4× fleet median (>= 2 s floor).
  * `nvlink-health` -- inactive lanes (only when this GPU is below the fleet median active ratio) or CRC errors accumulating between samples.
  * `gpu-ecc-errors` -- volatile uncorrectable -> critical, correctable -> info (ECC was designed to correct correctable events silently).
  * `potential-cpu-bound-preprocessing` -- low GPU util + high host CPU user time (correlation, not assertion).
  * `potential-io-bound-pipeline` -- low GPU util + high `iowait` (correlation, not assertion).
  * `gpu-xid-error` / `host-oom-kill` -- log-derived stability markers, surfaced verbatim with evidence so the operator can interpret.
* Bubble Tea TUI: GPU table with throttle / parked badges, scrollable process table with cursor, sparkline history (up to 300 samples), active and history hints panels with severity badges, NVLink + errors-seen panel, pause/resume, help overlay, hint detail popup.
* Plain text mode (`--plain`) for SSH and scripting -- includes throttle reason and P-state in the per-GPU row.
* Demo mode (`--demo`) for development without GPU hardware.
* Unix-socket HTTP API: `/healthz`, `/v1/snapshot`.
* JSONL recorder for snapshots and hints.
* GitHub Actions CI: `go build`, `go test -race`, `go vet`, `golangci-lint`, `gofmt`.

[v0.1.0]: https://github.com/indraputrabh/gputui/releases/tag/v0.1.0
