package model

import "time"

// Snapshot is a point-in-time view of node state and derived hints.
type Snapshot struct {
	TS            time.Time         `json:"ts"`
	GPUs          []GPUStat         `json:"gpus"`
	Procs         []ProcStat        `json:"procs"`
	Node          NodeStat          `json:"node"`
	HealthSignals []GPUHealthSignal `json:"health_signals,omitempty"`
	Markers       []Marker          `json:"markers,omitempty"`
	Hints         []Hint            `json:"hints,omitempty"`
}

// NodeStat captures host-level CPU/memory pressure signals.
type NodeStat struct {
	LoadAvg      []float64 `json:"loadavg,omitempty"`
	CPUUser      float64   `json:"cpu_user,omitempty"`
	CPUSys       float64   `json:"cpu_sys,omitempty"`
	CPUIowait    float64   `json:"cpu_iowait,omitempty"`
	MemAvailable uint64    `json:"mem_available_bytes,omitempty"`
	MemTotal     uint64    `json:"mem_total_bytes,omitempty"`
	SwapUsed     uint64    `json:"swap_used_bytes,omitempty"`
	SwapTotal    uint64    `json:"swap_total_bytes,omitempty"`
}

// Marker is a log-derived event such as Xid or OOM.
type Marker struct {
	TS    time.Time `json:"ts"`
	Kind  string    `json:"kind"`
	Msg   string    `json:"msg"`
	Extra string    `json:"extra,omitempty"`
}
