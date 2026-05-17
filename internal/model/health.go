package model

// GPUHealthSignal holds hardware health indicators for a single GPU,
// collected via NVML queries beyond the standard utilisation metrics.
//
// Violation* counters are cumulative-since-driver-load nanoseconds the
// driver was actively enforcing the named perf policy (thermal cap,
// power cap, etc.). They come from nvmlDeviceGetViolationStatus.
type GPUHealthSignal struct {
	Index                     int    `json:"index"`
	ECCUncorrectableVolatile  uint64 `json:"ecc_uncorrectable_volatile"`
	ECCCorrectableVolatile    uint64 `json:"ecc_correctable_volatile"`
	NVLinkTotalLinks          int    `json:"nvlink_total_links"`
	NVLinkActiveLinks         int    `json:"nvlink_active_links"`
	NVLinkCRCErrors           uint64 `json:"nvlink_crc_errors"`
	RemappedRowsUncorrectable int    `json:"remapped_rows_uncorrectable"`
	RemappedRowsCorrectable   int    `json:"remapped_rows_correctable"`
	RemappedRowsPending       bool   `json:"remapped_rows_pending"`

	ViolationThermalNs     uint64 `json:"violation_thermal_ns"`
	ViolationPowerNs       uint64 `json:"violation_power_ns"`
	ViolationSyncBoostNs   uint64 `json:"violation_sync_boost_ns"`
	ViolationBoardLimitNs  uint64 `json:"violation_board_limit_ns"`
	ViolationLowUtilNs     uint64 `json:"violation_low_util_ns"`
	ViolationReliabilityNs uint64 `json:"violation_reliability_ns"`
}
