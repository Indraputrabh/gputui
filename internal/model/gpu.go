package model

// GPUStat contains per-GPU telemetry values.
//
// Most fields come straight from NVML and are quoted from the driver. The
// "ground-truth" fields (ThrottleReasons, PerfState, MaxClocksMHz, PCIe*,
// MemUtilPct) are what the hint engine prefers over heuristic combinations
// of util/temp/power.
type GPUStat struct {
	Index       int     `json:"index"`
	UUID        string  `json:"uuid,omitempty"`
	Name        string  `json:"name,omitempty"`
	UtilPct     float64 `json:"util_pct"`
	MemUtilPct  float64 `json:"mem_util_pct"`
	VRAMUsedMB  uint64  `json:"vram_used_mb"`
	VRAMTotalMB uint64  `json:"vram_total_mb"`
	TempC       int     `json:"temp_c"`
	PowerW      float64 `json:"power_w"`
	PowerLimitW float64 `json:"power_limit_w,omitempty"`
	ClocksMHz   struct {
		Graphics uint32 `json:"graphics,omitempty"`
		Mem      uint32 `json:"mem,omitempty"`
	} `json:"clocks_mhz,omitempty"`
	MaxClocksMHz struct {
		Graphics uint32 `json:"graphics,omitempty"`
		Mem      uint32 `json:"mem,omitempty"`
	} `json:"max_clocks_mhz,omitempty"`
	// ThrottleReasons is the NVML clocks-throttle bitmap as returned by
	// nvmlDeviceGetCurrentClocksThrottleReasons. Decoded by hint rules;
	// stored raw here so the TUI can render the reason badge.
	ThrottleReasons uint64 `json:"throttle_reasons"`
	// PerfState is the NVML P-state (0 = max, 15 = min, 32 = unknown).
	PerfState        int `json:"perf_state"`
	PCIeGenCurrent   int `json:"pcie_gen_current,omitempty"`
	PCIeGenMax       int `json:"pcie_gen_max,omitempty"`
	PCIeWidthCurrent int `json:"pcie_width_current,omitempty"`
	PCIeWidthMax     int `json:"pcie_width_max,omitempty"`
}
