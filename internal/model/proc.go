package model

// ProcStat maps a process to GPU usage plus optional host stats.
type ProcStat struct {
	PID      int     `json:"pid"`
	User     string  `json:"user"`
	Cmd      string  `json:"cmd"`
	GPUIndex int     `json:"gpu_index"`
	VRAMMB   uint64  `json:"vram_mb"`
	UtilPct  float64 `json:"util_pct,omitempty"`
	CPUPct   float64 `json:"cpu_pct,omitempty"`
	RSSMB    uint64  `json:"rss_mb,omitempty"`
	IORead   uint64  `json:"io_read_bytes,omitempty"`
	IOWrite  uint64  `json:"io_write_bytes,omitempty"`
}
