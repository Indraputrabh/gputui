package hints

import "github.com/indraputrabh/gputui/internal/hints/rules"

// defaultRules returns the built-in rule set, ordered roughly from
// "ground-truth driver signal" to "log-derived stability" so the TUI
// renders the most authoritative hints first when severities tie.
func defaultRules() []Rule {
	return []Rule{
		// Ground-truth NVML signals.
		&rules.ConfirmedThrottle{},
		&rules.GPUParked{},
		&rules.MemoryBandwidthBound{},
		&rules.PCIeLinkDegraded{},

		// Hardware-health rules (counters, NVLink topology, ECC).
		&rules.ThermalViolationOutlier{},
		&rules.NVLinkHealth{},
		&rules.ECCError{},

		// Workload-pattern rules (host-side correlation).
		&rules.CPUBound{},
		&rules.IOBound{},

		// Log-derived stability markers (Xid, OOM).
		&rules.Stability{},
	}
}
