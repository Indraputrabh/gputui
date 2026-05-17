package gpu

import (
	"context"

	"github.com/indraputrabh/gputui/internal/model"
)

// dcgmCollector implements the Collector interface using NVIDIA DCGM.
// DCGM provides higher-fidelity sampling than NVML for some metrics
// (especially power and utilisation) at the cost of requiring the DCGM
// daemon. The implementation is Linux-only; on other platforms this
// returns NotImplementedError.
//
// DCGM field IDs mapped to GPUStat fields:
//
//	DCGM_FI_DEV_GPU_UTIL      -> UtilPct
//	DCGM_FI_DEV_FB_USED       -> VRAMUsedMB
//	DCGM_FI_DEV_FB_TOTAL      -> VRAMTotalMB (from device info)
//	DCGM_FI_DEV_GPU_TEMP      -> TempC
//	DCGM_FI_DEV_POWER_USAGE   -> PowerW
//	DCGM_FI_DEV_POWER_LIMIT   -> PowerLimitW (from device info)
//	DCGM_FI_DEV_SM_CLOCK      -> ClocksMHz.Graphics
//	DCGM_FI_DEV_MEM_CLOCK     -> ClocksMHz.Mem
type dcgmCollector struct{}

func (c *dcgmCollector) Source() Source {
	return SourceDCGM
}

func (c *dcgmCollector) Collect(ctx context.Context) ([]model.GPUStat, error) {
	_ = ctx
	return nil, &NotImplementedError{
		Source: SourceDCGM,
		Detail: "DCGM collector requires libdcgm and the nv-hostengine daemon; " +
			"use --gpu-source=nvml for direct NVML access without DCGM",
	}
}
