package rules

import (
	"fmt"
	"strings"

	"github.com/indraputrabh/gputui/internal/model"
)

// Stability surfaces log-derived stability markers (NVIDIA Xid errors,
// Linux OOM kills) parsed by the dmesg collector. The rule keeps the
// presentation honest: Xid codes mean different things (some hardware,
// some driver, some software), and an OOM kill could be a container
// limit or actual host exhaustion -- we surface the evidence and let
// the operator decide rather than guess.
type Stability struct{}

func (r *Stability) Evaluate(snap model.Snapshot) []model.Hint {
	var hints []model.Hint

	for _, m := range snap.Markers {
		switch m.Kind {
		case "xid":
			hints = append(hints, model.Hint{
				Name:       "gpu-xid-error",
				Category:   "stability",
				Severity:   "critical",
				Confidence: 1.0,
				Summary: fmt.Sprintf(
					"NVIDIA Xid event: %s -- consult the NVIDIA Xid reference (codes range across hardware, driver, and software causes)",
					strings.TrimSpace(m.Msg)),
				Evidence: []model.Evidence{
					{Metric: "xid_event", Msg: m.Msg},
					{Metric: "pci_slot", Msg: m.Extra},
				},
			})

		case "oom":
			hints = append(hints, model.Hint{
				Name:       "host-oom-kill",
				Category:   "stability",
				Severity:   "critical",
				Confidence: 1.0,
				Summary: fmt.Sprintf(
					"Linux OOM killer fired: %s -- could be a cgroup/container limit or host memory exhaustion (check %s)",
					strings.TrimSpace(m.Msg), strings.TrimSpace(m.Extra)),
				Evidence: []model.Evidence{
					{Metric: "oom_event", Msg: m.Msg},
					{Metric: "process_or_cgroup", Msg: m.Extra},
				},
			})
		}
	}

	return hints
}
