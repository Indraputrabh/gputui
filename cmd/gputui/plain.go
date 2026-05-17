package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/indraputrabh/gputui/internal/model"
)

func runPlain(p SnapshotProvider) error {
	snap, err := p.FetchSnapshot()
	if err != nil {
		return err
	}
	fmt.Print(renderPlain(snap))
	return nil
}

func renderPlain(snap model.Snapshot) string {
	var b strings.Builder
	hostname, _ := os.Hostname()
	b.WriteString(fmt.Sprintf("gputui  %s  %s\n\n", hostname, snap.TS.Format(time.RFC3339)))

	b.WriteString("GPU  Name                        Util   Mem%   VRAM                  Temp   Power          Clocks      State\n")
	b.WriteString("───  ──────────────────────────  ─────  ─────  ────────────────────  ─────  ─────────────  ──────────  ─────────────\n")
	for _, g := range snap.GPUs {
		vram := fmt.Sprintf("%d/%d MB", g.VRAMUsedMB, g.VRAMTotalMB)
		power := fmt.Sprintf("%.0f/%.0f W", g.PowerW, g.PowerLimitW)
		clocks := fmt.Sprintf("%d/%d MHz", g.ClocksMHz.Graphics, g.ClocksMHz.Mem)
		state := plainStateLabel(g)
		b.WriteString(fmt.Sprintf("%-3d  %-26s  %4.0f%%  %4.0f%%  %-20s  %3d°C  %-13s  %-10s  %s\n",
			g.Index, truncate(g.Name, 26), g.UtilPct, g.MemUtilPct, vram, g.TempC, power, clocks, state))
	}

	cpuTotal := snap.Node.CPUUser + snap.Node.CPUSys + snap.Node.CPUIowait
	memPct := float64(0)
	memUsed := uint64(0)
	if snap.Node.MemTotal > 0 {
		memUsed = snap.Node.MemTotal - snap.Node.MemAvailable
		memPct = float64(memUsed) / float64(snap.Node.MemTotal) * 100
	}
	loadStr := ""
	if len(snap.Node.LoadAvg) >= 3 {
		loadStr = fmt.Sprintf("  Load %.2f %.2f %.2f", snap.Node.LoadAvg[0], snap.Node.LoadAvg[1], snap.Node.LoadAvg[2])
	}
	b.WriteString(fmt.Sprintf("\nCPU %s %4.1f%%    Mem %s %s/%s%s\n",
		plainBar(cpuTotal, 100, 20), cpuTotal,
		plainBar(memPct, 100, 20), humanBytes(memUsed), humanBytes(snap.Node.MemTotal),
		loadStr))

	if len(snap.Procs) > 0 {
		b.WriteString("\nProcesses\n")
		b.WriteString("  PID      User              Cmd                                    GPU  VRAM     Util   CPU%  RSS\n")
		b.WriteString("  ───────  ────────────────  ─────────────────────────────────────  ───  ───────  ─────  ────  ────────\n")
		for _, p := range snap.Procs {
			b.WriteString(fmt.Sprintf("  %-7d  %-16s  %-37s  %3d  %4d MB  %4.0f%%  %3.0f%%  %4d MB\n",
				p.PID, p.User, p.Cmd, p.GPUIndex, p.VRAMMB, p.UtilPct, p.CPUPct, p.RSSMB))
		}
	}

	if len(snap.Hints) > 0 {
		b.WriteString("\nActive Hints\n")
		for _, h := range snap.Hints {
			b.WriteString(fmt.Sprintf("  [%s] %s (confidence=%.0f%%)\n",
				strings.ToUpper(h.Severity), h.Summary, h.Confidence*100))
			for _, e := range h.Evidence {
				b.WriteString(fmt.Sprintf("    - %s = %.1f%s", e.Metric, e.Value, e.Unit))
				if e.Threshold != 0 {
					b.WriteString(fmt.Sprintf(" (threshold: %.1f%s)", e.Threshold, e.Unit))
				}
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// plainStateLabel returns a compact plaintext label for the
// driver-reported state of a GPU (perf state, active throttle).
func plainStateLabel(g model.GPUStat) string {
	var parts []string
	if reasons := plainThrottleReasons(g.ThrottleReasons); reasons != "" {
		parts = append(parts, "THROTTLE:"+reasons)
	}
	if g.PerfState >= 8 && g.PerfState < 32 {
		parts = append(parts, fmt.Sprintf("P%d", g.PerfState))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func plainThrottleReasons(bits uint64) string {
	const (
		hwSlowdown            uint64 = 0x08
		hwThermal             uint64 = 0x40
		hwPowerBrake          uint64 = 0x80
		swThermal             uint64 = 0x20
		swPowerCap            uint64 = 0x04
		syncBoost             uint64 = 0x10
		applicationsClocksSet uint64 = 0x02
		gpuIdle               uint64 = 0x01
		displayClock          uint64 = 0x100
	)
	bits &^= gpuIdle | applicationsClocksSet | displayClock
	if bits == 0 {
		return ""
	}
	switch {
	case bits&hwThermal != 0:
		return "HW_THERMAL"
	case bits&hwPowerBrake != 0:
		return "HW_POWER_BRAKE"
	case bits&hwSlowdown != 0:
		return "HW_SLOWDOWN"
	case bits&swThermal != 0:
		return "SW_THERMAL"
	case bits&swPowerCap != 0:
		return "SW_POWER_CAP"
	case bits&syncBoost != 0:
		return "SYNC_BOOST"
	default:
		return fmt.Sprintf("0x%x", bits)
	}
}

func plainBar(val, maxVal float64, width int) string {
	filled := int(math.Round(val / maxVal * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("|", filled) + strings.Repeat(" ", width-filled) + "]"
}
