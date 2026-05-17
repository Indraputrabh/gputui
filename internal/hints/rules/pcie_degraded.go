package rules

import (
	"fmt"

	"github.com/indraputrabh/gputui/internal/model"
)

// PCIeLinkDegraded fires when NVML reports the PCIe link has
// renegotiated to a lower generation or width than the hardware
// supports. Reads nvmlDeviceGetCurrPcieLinkGeneration / Width and
// compares against the corresponding Max getters. Common causes:
// poorly seated card, damaged riser, BIOS pcie-link-speed override,
// or thermal protection on the slot.
type PCIeLinkDegraded struct{}

func (r *PCIeLinkDegraded) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint
	for _, g := range snap.GPUs {
		if g.PCIeGenMax == 0 || g.PCIeWidthMax == 0 {
			continue
		}

		genDegraded := g.PCIeGenCurrent > 0 && g.PCIeGenCurrent < g.PCIeGenMax
		widthDegraded := g.PCIeWidthCurrent > 0 && g.PCIeWidthCurrent < g.PCIeWidthMax
		if !genDegraded && !widthDegraded {
			continue
		}

		hints = append(hints, model.Hint{
			Name:       "pcie-link-degraded",
			Category:   "hardware_health",
			Severity:   "warning",
			Confidence: 0.95,
			Summary: fmt.Sprintf(
				"GPU%d PCIe link running at Gen%d x%d (max Gen%d x%d) -- bandwidth degraded, check seating and BIOS link-speed override",
				g.Index, g.PCIeGenCurrent, g.PCIeWidthCurrent, g.PCIeGenMax, g.PCIeWidthMax),
			Evidence: []model.Evidence{
				{Metric: "pcie_gen_current", Value: float64(g.PCIeGenCurrent), Threshold: float64(g.PCIeGenMax)},
				{Metric: "pcie_gen_max", Value: float64(g.PCIeGenMax)},
				{Metric: "pcie_width_current", Value: float64(g.PCIeWidthCurrent), Threshold: float64(g.PCIeWidthMax), Unit: "lanes"},
				{Metric: "pcie_width_max", Value: float64(g.PCIeWidthMax), Unit: "lanes"},
			},
		})
	}
	return hints
}
