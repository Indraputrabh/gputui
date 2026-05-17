package rules

import (
	"fmt"
	"sort"

	"github.com/indraputrabh/gputui/internal/model"
)

// NVLinkHealth fires when:
//   - NVLink lanes are inactive on a single GPU compared to the fleet
//     median (real outlier vs. documented topology), OR
//   - CRC error counters are actively accumulating between samples
//     (the counter can be non-zero from boot; a stale non-zero value
//     is not treated as an active problem).
//
// Severity is `warning` -- the operator may have a documented
// asymmetric NVLink topology, so we don't dump-shout `critical` for
// what may be expected hardware. Production NVLink failures show up
// downstream (NCCL hangs, GPU-to-GPU bandwidth tests). This hint is a
// hint, not a paging alert.
type NVLinkHealth struct {
	prevCRC map[int]uint64
}

// DependsOnHealthOnly tells the evaluator this rule is deterministic in
// HealthSignals: when the signals match the previous tick, the CRC delta
// is zero and no new hints are produced, so cached output is replayable.
func (r *NVLinkHealth) DependsOnHealthOnly() bool { return true }

func (r *NVLinkHealth) Evaluate(snap model.Snapshot) []model.Hint {
	if r.prevCRC == nil {
		r.prevCRC = make(map[int]uint64)
	}

	// Compute fleet median active/total ratio so we can distinguish
	// a single-GPU outlier (real degraded link) from a uniformly
	// asymmetric topology (documented architecture, not a problem).
	fleetMedian := nvlinkFleetMedianRatio(snap.HealthSignals)
	hasFleet := len(snap.HealthSignals) >= 2

	var hints []model.Hint
	for _, sig := range snap.HealthSignals {
		if sig.NVLinkTotalLinks == 0 {
			continue
		}

		ratio := float64(sig.NVLinkActiveLinks) / float64(sig.NVLinkTotalLinks)
		inactive := sig.NVLinkTotalLinks - sig.NVLinkActiveLinks

		prev, hasPrev := r.prevCRC[sig.Index]
		r.prevCRC[sig.Index] = sig.NVLinkCRCErrors
		crcDelta := uint64(0)
		if hasPrev && sig.NVLinkCRCErrors > prev {
			crcDelta = sig.NVLinkCRCErrors - prev
		}

		// "Outlier" = strictly fewer active links than the fleet
		// median. With <2 GPUs there's no fleet baseline, so we
		// fall back to flagging any inactive links.
		isLinkOutlier := inactive > 0 && (!hasFleet || ratio < fleetMedian-1e-9)

		if !isLinkOutlier && crcDelta == 0 {
			continue
		}

		summary := ""
		switch {
		case isLinkOutlier && crcDelta > 0:
			summary = fmt.Sprintf(
				"GPU%d has %d/%d NVLink lanes active vs fleet median %.0f/%d, plus %d CRC errors this sample -- possible cabling issue",
				sig.Index, sig.NVLinkActiveLinks, sig.NVLinkTotalLinks,
				fleetMedian*float64(sig.NVLinkTotalLinks), sig.NVLinkTotalLinks, crcDelta)
		case isLinkOutlier:
			summary = fmt.Sprintf(
				"GPU%d has %d/%d NVLink lanes active vs fleet median %.0f/%d -- check for cabling or topology mismatch",
				sig.Index, sig.NVLinkActiveLinks, sig.NVLinkTotalLinks,
				fleetMedian*float64(sig.NVLinkTotalLinks), sig.NVLinkTotalLinks)
		default:
			summary = fmt.Sprintf("GPU%d NVLink CRC errors accumulating (+%d this sample)", sig.Index, crcDelta)
		}

		hints = append(hints, model.Hint{
			Name:       "nvlink-health",
			Category:   "hardware_health",
			Severity:   "warning",
			Confidence: 0.80,
			Summary:    summary,
			Evidence: []model.Evidence{
				{Metric: "nvlink_total_links", Value: float64(sig.NVLinkTotalLinks), Unit: "links"},
				{Metric: "nvlink_active_links", Value: float64(sig.NVLinkActiveLinks), Unit: "links"},
				{Metric: "nvlink_crc_errors_delta", Value: float64(crcDelta), Unit: "errors/sample"},
				{Metric: "nvlink_crc_errors_total", Value: float64(sig.NVLinkCRCErrors), Unit: "errors"},
			},
		})
	}
	return hints
}

func nvlinkFleetMedianRatio(sigs []model.GPUHealthSignal) float64 {
	ratios := make([]float64, 0, len(sigs))
	for _, sig := range sigs {
		if sig.NVLinkTotalLinks == 0 {
			continue
		}
		ratios = append(ratios, float64(sig.NVLinkActiveLinks)/float64(sig.NVLinkTotalLinks))
	}
	if len(ratios) == 0 {
		return 1.0
	}
	sort.Float64s(ratios)
	n := len(ratios)
	if n%2 == 0 {
		return (ratios[n/2-1] + ratios[n/2]) / 2.0
	}
	return ratios[n/2]
}
