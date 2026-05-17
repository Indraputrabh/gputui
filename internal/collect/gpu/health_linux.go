//go:build linux

package gpu

import (
	"context"

	gonvml "github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/indraputrabh/gputui/internal/model"
)

// CollectHealth queries ECC errors, NVLink status, and remapped rows for
// each GPU via NVML. Runs without holding any outer mutex; NVML query
// functions are thread-safe once Init has completed, so a slow health
// pass does not block concurrent Collect/CollectProcesses calls.
func (c *nvmlCollector) CollectHealth(ctx context.Context) ([]model.GPUHealthSignal, error) {
	if err := c.ensureInit(); err != nil {
		return nil, err
	}

	handles, err := c.deviceHandles()
	if err != nil {
		return nil, err
	}

	signals := make([]model.GPUHealthSignal, 0, len(handles))
	for i, device := range handles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sig, healthErr := collectDeviceHealth(i, device)
		if healthErr != nil {
			continue
		}
		signals = append(signals, sig)
	}
	return signals, nil
}

func collectDeviceHealth(index int, device gonvml.Device) (model.GPUHealthSignal, error) {
	sig := model.GPUHealthSignal{Index: index}

	if val, r := device.GetTotalEccErrors(
		gonvml.MEMORY_ERROR_TYPE_UNCORRECTED,
		gonvml.VOLATILE_ECC,
	); r == gonvml.SUCCESS {
		sig.ECCUncorrectableVolatile = val
	}

	if val, r := device.GetTotalEccErrors(
		gonvml.MEMORY_ERROR_TYPE_CORRECTED,
		gonvml.VOLATILE_ECC,
	); r == gonvml.SUCCESS {
		sig.ECCCorrectableVolatile = val
	}

	collectNVLinkHealth(device, &sig)

	if correctable, uncorrectable, pending, _, r := device.GetRemappedRows(); r == gonvml.SUCCESS {
		sig.RemappedRowsCorrectable = int(correctable)
		sig.RemappedRowsUncorrectable = int(uncorrectable)
		sig.RemappedRowsPending = pending
	}

	collectViolationCounters(device, &sig)

	return sig, nil
}

// collectViolationCounters reads cumulative-since-driver-load nanoseconds
// the driver was actively enforcing each perf policy. Older drivers may
// not implement every policy; missing policies are silently skipped so
// one unsupported policy doesn't drop the others.
func collectViolationCounters(device gonvml.Device, sig *model.GPUHealthSignal) {
	policies := []struct {
		policy gonvml.PerfPolicyType
		dst    *uint64
	}{
		{gonvml.PERF_POLICY_THERMAL, &sig.ViolationThermalNs},
		{gonvml.PERF_POLICY_POWER, &sig.ViolationPowerNs},
		{gonvml.PERF_POLICY_SYNC_BOOST, &sig.ViolationSyncBoostNs},
		{gonvml.PERF_POLICY_BOARD_LIMIT, &sig.ViolationBoardLimitNs},
		{gonvml.PERF_POLICY_LOW_UTILIZATION, &sig.ViolationLowUtilNs},
		{gonvml.PERF_POLICY_RELIABILITY, &sig.ViolationReliabilityNs},
	}
	for _, p := range policies {
		if v, r := device.GetViolationStatus(p.policy); r == gonvml.SUCCESS {
			*p.dst = v.ViolationTime
		}
	}
}

func collectNVLinkHealth(device gonvml.Device, sig *model.GPUHealthSignal) {
	for link := 0; link < 18; link++ {
		state, ret := device.GetNvLinkState(link)
		if ret != gonvml.SUCCESS {
			break
		}
		sig.NVLinkTotalLinks++
		if state == gonvml.FEATURE_ENABLED {
			sig.NVLinkActiveLinks++
		}

		for _, counter := range []gonvml.NvLinkErrorCounter{
			gonvml.NVLINK_ERROR_DL_CRC_DATA,
			gonvml.NVLINK_ERROR_DL_CRC_FLIT,
		} {
			if val, r := device.GetNvLinkErrorCounter(link, counter); r == gonvml.SUCCESS {
				sig.NVLinkCRCErrors += val
			}
		}
	}
}
