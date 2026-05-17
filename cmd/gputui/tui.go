package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/indraputrabh/gputui/internal/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type panel int

const (
	panelGPU panel = iota
	panelProc
	panelHints
	panelHistory
	panelCharts
	panelCount // sentinel for modulo cycling
)

type sortField int

const (
	sortByUtil sortField = iota
	sortByVRAM
	sortByPID
)

func (s sortField) label() string {
	switch s {
	case sortByUtil:
		return "Util%"
	case sortByVRAM:
		return "VRAM"
	case sortByPID:
		return "PID"
	default:
		return "?"
	}
}

type chartMetric int

const (
	metricUtil chartMetric = iota
	metricVRAM
	metricTemp
	metricPower
	metricClock
	metricCount // sentinel for modulo cycling
)

func (m chartMetric) label() string {
	switch m {
	case metricUtil:
		return "Util"
	case metricVRAM:
		return "VRAM"
	case metricTemp:
		return "Temp"
	case metricPower:
		return "Power"
	case metricClock:
		return "Clock"
	default:
		return "?"
	}
}

func (m chartMetric) maxVal() float64 {
	switch m {
	case metricClock:
		return 2500
	default:
		return 100
	}
}

var (
	zoomSamples = [3]int{60, 150, 300}
	zoomLabels  = [3]string{"2min", "5min", "10min"}
)

type gpuHistory struct {
	util     *ringBuffer
	vramPct  *ringBuffer
	temp     *ringBuffer
	powerPct *ringBuffer
	clockGfx *ringBuffer
}

func newGPUHistory(capacity int) *gpuHistory {
	return &gpuHistory{
		util:     newRingBuffer(capacity),
		vramPct:  newRingBuffer(capacity),
		temp:     newRingBuffer(capacity),
		powerPct: newRingBuffer(capacity),
		clockGfx: newRingBuffer(capacity),
	}
}

func (h *gpuHistory) forMetric(m chartMetric) *ringBuffer {
	switch m {
	case metricVRAM:
		return h.vramPct
	case metricTemp:
		return h.temp
	case metricPower:
		return h.powerPct
	case metricClock:
		return h.clockGfx
	default:
		return h.util
	}
}

// --- hint history ---------------------------------------------------------

type hintHistoryEntry struct {
	hint      model.Hint
	firstSeen time.Time
	lastSeen  time.Time
	count     int
	active    bool
}

// --- nvlink cache ---------------------------------------------------------

type nvlinkGPUInfo struct {
	index  int
	total  int
	active int
	status string // "ok", "crc", "down"
}

type gpuErrorStats struct {
	crcDeltaTotal    uint64
	eccCorrectable   uint64
	eccUncorrectable uint64
	remappedPending  bool
}

// --- session stats --------------------------------------------------------

type gpuSessionStats struct {
	peakTemp     int
	peakTempGPU  int
	peakPowerW   float64
	peakPowerGPU int
	sumUtil      float64
	minClockMHz  uint32
	minClockGPU  int
	count        int
	initialized  bool
}

// --- styles ---------------------------------------------------------------

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	headStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	warnStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	critStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	barFill    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	barHigh    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	barCrit    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	barEmpty   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	footStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hintWarn   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
	hintCrit   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("9"))
	sepStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selStyle   = lipgloss.NewStyle().Background(lipgloss.Color("240"))
	focusStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	pauseStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("11"))
)

// --- messages -------------------------------------------------------------

type tickMsg time.Time

type snapshotMsg struct {
	snap model.Snapshot
	err  error
}

// --- model ----------------------------------------------------------------

// Maximum number of distinct hints to keep in history before evicting
// the oldest inactive entry. Prevents unbounded memory growth in long
// sessions.
const maxHintHistory = 200

type tuiModel struct {
	provider SnapshotProvider
	refresh  time.Duration
	hostname string

	snapshot model.Snapshot
	err      error
	ready    bool
	width    int
	height   int
	quitting bool

	paused        bool
	showHelp      bool
	hintDetail    *model.Hint // non-nil renders the hint detail overlay
	activePanel   panel
	gpuCursor     int
	procCursor    int
	hintCursor    int
	historyCursor int
	sortField     sortField
	history       map[int]*gpuHistory
	chartZoom     int // 0=2min, 1=5min, 2=10min

	hintHistory  []hintHistoryEntry
	sessionStats gpuSessionStats
	prevCRC      map[int]uint64
	nvlinkCache  []nvlinkGPUInfo
	errorStats   map[int]*gpuErrorStats

	// Memoised sorted slices. Invalidated by bumping dataVersion when the
	// snapshot, sort field, or hint history changes.
	dataVersion    uint64
	procsVersion   uint64
	procsSorted    []model.ProcStat
	historyVersion uint64
	historySorted  []hintHistoryEntry
}

func newModel(p SnapshotProvider, refresh time.Duration, hostname string) tuiModel {
	return tuiModel{
		provider:   p,
		refresh:    refresh,
		hostname:   hostname,
		history:    make(map[int]*gpuHistory),
		chartZoom:  1, // default 5min
		prevCRC:    make(map[int]uint64),
		errorStats: make(map[int]*gpuErrorStats),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.fetchSnapshot, m.tick())
}

func (m tuiModel) tick() tea.Cmd {
	return tea.Tick(m.refresh, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m tuiModel) cursor() string {
	return "▸"
}

func (m tuiModel) fetchSnapshot() tea.Msg {
	snap, err := m.provider.FetchSnapshot()
	if err != nil {
		return snapshotMsg{err: err}
	}
	return snapshotMsg{snap: snap}
}

// --- update ---------------------------------------------------------------

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case snapshotMsg:
		if msg.err != nil {
			m.err = msg.err
			m.ready = false
		} else {
			m.snapshot = msg.snap
			m.err = nil
			m.ready = true
			m.pushHistory(msg.snap)
			m.mergeHintHistory(msg.snap)
			m.updateNVLink(msg.snap)
			if m.procCursor >= len(m.snapshot.Procs) {
				m.procCursor = max(0, len(m.snapshot.Procs)-1)
			}
			if m.gpuCursor >= len(m.snapshot.GPUs) {
				m.gpuCursor = max(0, len(m.snapshot.GPUs)-1)
			}
			m.dataVersion++
			m.refreshSortCaches()
		}

	case tickMsg:
		if m.paused {
			return m, m.tick()
		}
		return m, tea.Batch(m.fetchSnapshot, m.tick())
	}

	return m, nil
}

func (m tuiModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.hintDetail != nil {
		// Ctrl+C / q always quits, anything else just closes the popup.
		if s := msg.String(); s == "q" || s == "ctrl+c" {
			m.hintDetail = nil
			m.quitting = true
			return m, tea.Quit
		}
		m.hintDetail = nil
		return m, nil
	}
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.showHelp = true
	case "enter", "o":
		if h := m.cursoredHint(); h != nil {
			cp := *h
			m.hintDetail = &cp
		}
	case " ":
		m.paused = !m.paused
	case "tab":
		m.activePanel = (m.activePanel + 1) % panel(panelCount)
	case "g":
		m.activePanel = panelGPU
	case "p":
		m.activePanel = panelProc
	case "h":
		m.activePanel = panelHints
	case "H":
		m.activePanel = panelHistory
	case "c":
		m.activePanel = panelCharts
	case "s":
		m.sortField = (m.sortField + 1) % 3
		m.dataVersion++
		m.refreshSortCaches()
	case "z":
		m.chartZoom = (m.chartZoom + 1) % len(zoomSamples)
	case "j", "down":
		switch m.activePanel {
		case panelGPU:
			if len(m.snapshot.GPUs) > 0 {
				m.gpuCursor = min(m.gpuCursor+1, len(m.snapshot.GPUs)-1)
			}
		case panelProc:
			if len(m.snapshot.Procs) > 0 {
				m.procCursor = min(m.procCursor+1, len(m.snapshot.Procs)-1)
			}
		case panelHints:
			m.hintCursor++
		case panelHistory:
			m.historyCursor++
		}
	case "k", "up":
		switch m.activePanel {
		case panelGPU:
			m.gpuCursor = max(m.gpuCursor-1, 0)
		case panelProc:
			m.procCursor = max(m.procCursor-1, 0)
		case panelHints:
			m.hintCursor = max(m.hintCursor-1, 0)
		case panelHistory:
			m.historyCursor = max(m.historyCursor-1, 0)
		}
	}
	return m, nil
}

func (m *tuiModel) pushHistory(snap model.Snapshot) {
	for _, g := range snap.GPUs {
		h, ok := m.history[g.Index]
		if !ok {
			h = newGPUHistory(300)
			m.history[g.Index] = h
		}
		h.util.Push(g.UtilPct)

		vramPct := float64(0)
		if g.VRAMTotalMB > 0 {
			vramPct = float64(g.VRAMUsedMB) / float64(g.VRAMTotalMB) * 100
		}
		h.vramPct.Push(vramPct)

		h.temp.Push(float64(g.TempC))

		powerPct := float64(0)
		if g.PowerLimitW > 0 {
			powerPct = g.PowerW / g.PowerLimitW * 100
		}
		h.powerPct.Push(powerPct)

		h.clockGfx.Push(float64(g.ClocksMHz.Graphics))
	}
	m.updateSessionStats(snap)
}

func (m *tuiModel) mergeHintHistory(snap model.Snapshot) {
	for i := range m.hintHistory {
		m.hintHistory[i].active = false
	}

	// Pick the highest-severity hint per name so a critical version
	// is never shadowed by a warning that happens to come first.
	best := make(map[string]model.Hint)
	for _, h := range snap.Hints {
		prev, exists := best[h.Name]
		if !exists || severityRank(h.Severity) < severityRank(prev.Severity) {
			best[h.Name] = h
		}
	}

	for _, h := range best {
		found := false
		for i := range m.hintHistory {
			if m.hintHistory[i].hint.Name == h.Name {
				m.hintHistory[i].lastSeen = snap.TS
				m.hintHistory[i].count++
				m.hintHistory[i].active = true
				if severityRank(h.Severity) < severityRank(m.hintHistory[i].hint.Severity) {
					m.hintHistory[i].hint.Severity = h.Severity
					m.hintHistory[i].hint.Summary = h.Summary
				}
				found = true
				break
			}
		}
		if !found {
			m.hintHistory = append(m.hintHistory, hintHistoryEntry{
				hint:      h,
				firstSeen: snap.TS,
				lastSeen:  snap.TS,
				count:     1,
				active:    true,
			})
		}
	}

	m.capHintHistory()
}

// capHintHistory keeps at most maxHintHistory entries, evicting the
// oldest inactive entry first.
func (m *tuiModel) capHintHistory() {
	if len(m.hintHistory) <= maxHintHistory {
		return
	}

	// First pass: drop inactive entries in age order until within budget.
	for len(m.hintHistory) > maxHintHistory {
		dropIdx := -1
		oldest := time.Time{}
		for i := range m.hintHistory {
			if m.hintHistory[i].active {
				continue
			}
			if dropIdx < 0 || m.hintHistory[i].lastSeen.Before(oldest) {
				dropIdx = i
				oldest = m.hintHistory[i].lastSeen
			}
		}
		if dropIdx < 0 {
			// All active: drop the oldest active to stay bounded.
			for i := range m.hintHistory {
				if dropIdx < 0 || m.hintHistory[i].lastSeen.Before(oldest) {
					dropIdx = i
					oldest = m.hintHistory[i].lastSeen
				}
			}
		}
		m.hintHistory = append(m.hintHistory[:dropIdx], m.hintHistory[dropIdx+1:]...)
	}
}

// refreshSortCaches recomputes the memoised sorted procs/hint-history
// slices so View() helpers can return them in O(1).
func (m *tuiModel) refreshSortCaches() {
	procs := m.snapshot.Procs
	if cap(m.procsSorted) < len(procs) {
		m.procsSorted = make([]model.ProcStat, len(procs))
	} else {
		m.procsSorted = m.procsSorted[:len(procs)]
	}
	copy(m.procsSorted, procs)
	sortProcsBy(m.procsSorted, m.sortField)
	m.procsVersion = m.dataVersion

	hist := m.hintHistory
	if cap(m.historySorted) < len(hist) {
		m.historySorted = make([]hintHistoryEntry, len(hist))
	} else {
		m.historySorted = m.historySorted[:len(hist)]
	}
	copy(m.historySorted, hist)
	sortHintHistory(m.historySorted)
	m.historyVersion = m.dataVersion
}

func (m *tuiModel) updateSessionStats(snap model.Snapshot) {
	for _, g := range snap.GPUs {
		if !m.sessionStats.initialized {
			m.sessionStats = gpuSessionStats{
				peakTemp:     g.TempC,
				peakTempGPU:  g.Index,
				peakPowerW:   g.PowerW,
				peakPowerGPU: g.Index,
				minClockMHz:  g.ClocksMHz.Graphics,
				minClockGPU:  g.Index,
				initialized:  true,
			}
		}
		if g.TempC > m.sessionStats.peakTemp {
			m.sessionStats.peakTemp = g.TempC
			m.sessionStats.peakTempGPU = g.Index
		}
		if g.PowerW > m.sessionStats.peakPowerW {
			m.sessionStats.peakPowerW = g.PowerW
			m.sessionStats.peakPowerGPU = g.Index
		}
		if g.ClocksMHz.Graphics < m.sessionStats.minClockMHz {
			m.sessionStats.minClockMHz = g.ClocksMHz.Graphics
			m.sessionStats.minClockGPU = g.Index
		}
		m.sessionStats.sumUtil += g.UtilPct
		m.sessionStats.count++
	}
}

func (m *tuiModel) updateNVLink(snap model.Snapshot) {
	if len(snap.HealthSignals) == 0 {
		return // keep previous cache intact
	}

	hasAnyLinks := false
	for _, sig := range snap.HealthSignals {
		if sig.NVLinkTotalLinks > 0 {
			hasAnyLinks = true
			break
		}
	}
	if !hasAnyLinks {
		return
	}

	infos := make([]nvlinkGPUInfo, 0, len(snap.HealthSignals))
	for _, sig := range snap.HealthSignals {
		if sig.NVLinkTotalLinks == 0 {
			continue
		}
		inactive := sig.NVLinkTotalLinks - sig.NVLinkActiveLinks
		status := "ok"
		if inactive > 0 {
			status = "down"
		} else {
			prev, hasPrev := m.prevCRC[sig.Index]
			if hasPrev && sig.NVLinkCRCErrors > prev {
				status = "crc"
			}
		}
		infos = append(infos, nvlinkGPUInfo{
			index: sig.Index, total: sig.NVLinkTotalLinks,
			active: sig.NVLinkActiveLinks, status: status,
		})

		// Accumulate session-level error stats.
		es, ok := m.errorStats[sig.Index]
		if !ok {
			es = &gpuErrorStats{}
			m.errorStats[sig.Index] = es
		}
		prev, hasPrev := m.prevCRC[sig.Index]
		if hasPrev && sig.NVLinkCRCErrors > prev {
			es.crcDeltaTotal += sig.NVLinkCRCErrors - prev
		}
		es.eccCorrectable = sig.ECCCorrectableVolatile
		es.eccUncorrectable = sig.ECCUncorrectableVolatile
		if sig.RemappedRowsPending {
			es.remappedPending = true
		}
	}
	m.nvlinkCache = infos

	// Update prevCRC AFTER computing status so delta detection works.
	for _, sig := range snap.HealthSignals {
		m.prevCRC[sig.Index] = sig.NVLinkCRCErrors
	}
}

// --- view -----------------------------------------------------------------

// DEC private mode 2026: begin/end synchronized output. Terminals that
// support it (WezTerm, Kitty, iTerm2, foot, Alacritty >=0.13) buffer
// everything between BSU/ESU and paint the frame atomically, which
// eliminates the visible sweep when Bubble Tea writes a large View().
// Terminals that don't recognize these sequences ignore them silently.
const (
	syncBegin = "\x1b[?2026h"
	syncEnd   = "\x1b[?2026l"
)

// Panel visible-row caps adapt to terminal height so View() never
// outgrows the terminal. When that happens with alt-screen mode the
// top of the rendered string is clipped, which is a poor demo.
//
// Heuristic: keep all 8 GPUs visible at the top; trim lower panels
// (procs, hints, charts) before they push the GPU table off-screen.

// gpuPanelCap returns the GPU table row cap. Always 8 — the GPU table
// is the visual anchor, never trimmed; we'd rather drop charts.
func (m tuiModel) gpuPanelCap() int { return 8 }

// procPanelCap returns the process table row cap based on m.height.
// Mirrors the visible-rows knob in viewProcs. Aggressive trimming on
// small terminals to make room for the GPU history charts at the
// bottom — both the GPU table and the charts are visual anchors.
func (m tuiModel) procPanelCap() int {
	switch {
	case m.height <= 0:
		return 8
	case m.height < 40:
		return 2
	case m.height < 48:
		return 4
	case m.height < 60:
		return 6
	default:
		return 8
	}
}

// hintsPanelCap returns the active and history caps for the hints
// panel. The hints panel renders side-by-side, so the panel takes
// max(activeCap, historyCap) vertical rows.
func (m tuiModel) hintsPanelCap() (active, history int) {
	switch {
	case m.height <= 0:
		return 5, 8
	case m.height < 40:
		return 2, 2
	case m.height < 48:
		return 3, 3
	case m.height < 60:
		return 4, 5
	default:
		return 5, 8
	}
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	w := m.width
	if w < 40 {
		w = 80
	}

	if m.showHelp {
		return syncBegin + m.viewHelp(w) + syncEnd
	}

	if m.hintDetail != nil {
		return syncBegin + m.viewHintDetail(w) + syncEnd
	}

	var b strings.Builder
	b.WriteString(syncBegin)

	if !m.ready {
		status := "connecting..."
		if m.err != nil {
			status = fmt.Sprintf("error: %v", m.err)
		}
		b.WriteString(titleStyle.Render(" gputui "))
		b.WriteString("  ")
		b.WriteString(dimStyle.Render(status))
		b.WriteString("\n\n")
		b.WriteString(footStyle.Render("  q quit  •  ? help"))
		b.WriteString(syncEnd)
		return b.String()
	}

	snap := m.snapshot

	m.viewHeader(&b, snap, w)
	m.viewGPUs(&b, snap, w)
	m.viewHost(&b, snap, w)
	m.viewProcs(&b, snap, w)
	m.viewHints(&b, snap, w)
	m.viewCharts(&b, snap, w)
	m.viewFooter(&b, w)

	b.WriteString(syncEnd)
	return b.String()
}

func (m tuiModel) viewHeader(b *strings.Builder, snap model.Snapshot, w int) {
	titleText := " gputui "
	hostText := m.hostname
	tsText := snap.TS.Format("2006-01-02 15:04:05")
	right := hostText + "  " + tsText

	if m.paused {
		right = pauseStyle.Render(" PAUSED ") + "  " + right
	}

	padLen := w - lipgloss.Width(titleText) - lipgloss.Width(right)
	if padLen < 1 {
		padLen = 1
	}
	b.WriteString(titleStyle.Render(titleText))
	b.WriteString(strings.Repeat(" ", padLen))
	b.WriteString(dimStyle.Render(right))
	b.WriteString("\n")
}

func (m tuiModel) viewGPUs(b *strings.Builder, snap model.Snapshot, w int) {
	if m.activePanel == panelGPU {
		b.WriteString(renderFocusSep(w))
	} else {
		b.WriteString(renderSep(w))
	}

	// Build GPU table lines (left column).
	var gpuLines []string

	label := " GPUs"
	if m.activePanel == panelGPU {
		label = focusStyle.Render(label)
	} else {
		label = headStyle.Render(label)
	}
	gpuLines = append(gpuLines, label)

	nameW, barW := layoutWidths(w)

	hdr := fmt.Sprintf(" %-3s  %-*s  %-*s  %5s  %-15s  %5s  %-13s  %s",
		"GPU", nameW, "Name", barW+6, "Util", "VRAM%", "VRAM", "Temp", "Power", "Clocks")
	gpuLines = append(gpuLines, dimStyle.Render(truncate(hdr, w)))

	gpus := snap.GPUs
	maxVisible := min(len(gpus), 8)
	offset := 0
	if m.gpuCursor >= maxVisible {
		offset = m.gpuCursor - maxVisible + 1
	}
	end := min(offset+maxVisible, len(gpus))

	isFocused := m.activePanel == panelGPU

	for i := offset; i < end; i++ {
		g := gpus[i]
		utilBar := renderBar(g.UtilPct, 100, barW)
		utilPctStr := fmt.Sprintf("%4.0f%%", g.UtilPct)

		vramPct := float64(0)
		if g.VRAMTotalMB > 0 {
			vramPct = float64(g.VRAMUsedMB) / float64(g.VRAMTotalMB) * 100
		}
		vramPctStr := fmt.Sprintf("%4.0f%%", vramPct)
		vramStr := fmt.Sprintf("%d/%d MB", g.VRAMUsedMB, g.VRAMTotalMB)

		tempStr := fmt.Sprintf("%3d°C", g.TempC)
		if g.TempC > 80 {
			tempStr = critStyle.Render(tempStr)
		} else if g.TempC > 70 {
			tempStr = warnStyle.Render(tempStr)
		}

		powerStr := fmt.Sprintf("%.0f/%.0f W", g.PowerW, g.PowerLimitW)
		clocksStr := fmt.Sprintf("%d/%d", g.ClocksMHz.Graphics, g.ClocksMHz.Mem)

		prefix := " "
		if isFocused && i == m.gpuCursor {
			prefix = m.cursor()
		}

		line := fmt.Sprintf("%s%-3d  %-*s  %s %s  %s  %-15s  %s  %-13s  %s",
			prefix, g.Index, nameW, truncate(g.Name, nameW),
			utilBar, utilPctStr,
			vramPctStr, vramStr,
			tempStr, powerStr, clocksStr)

		// Append ground-truth state badges (throttle reasons, parked
		// P-state) so the operator sees the same authoritative
		// signals the hint engine fires on.
		badge := gpuStateBadges(g)
		if badge != "" {
			line = line + "  " + badge
		}

		if isFocused && i == m.gpuCursor {
			line = selStyle.Render(line)
		}
		gpuLines = append(gpuLines, line)
	}

	if len(gpus) > maxVisible {
		gpuLines = append(gpuLines, dimStyle.Render(fmt.Sprintf("   ↕ %d/%d GPUs (j/k to scroll)", m.gpuCursor+1, len(gpus))))
	}

	// Build session stats (right column) if enough width.
	statsLines := m.buildSessionStats()

	if len(statsLines) > 0 && w >= 100 {
		leftW := w/2 + w/4
		joined := joinColumns(gpuLines, statsLines, leftW, " │ ")
		for _, line := range joined {
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		for _, line := range gpuLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func (m tuiModel) buildSessionStats() []string {
	if !m.sessionStats.initialized {
		return nil
	}
	var lines []string
	lines = append(lines, headStyle.Render(" Session Stats"))
	lines = append(lines, fmt.Sprintf("  Peak Temp   %s %s",
		warnStyle.Render(fmt.Sprintf("%d°C", m.sessionStats.peakTemp)),
		dimStyle.Render(fmt.Sprintf("(GPU%d)", m.sessionStats.peakTempGPU))))
	lines = append(lines, fmt.Sprintf("  Peak Power  %s %s",
		warnStyle.Render(fmt.Sprintf("%.0fW", m.sessionStats.peakPowerW)),
		dimStyle.Render(fmt.Sprintf("(GPU%d)", m.sessionStats.peakPowerGPU))))

	avgUtil := float64(0)
	if m.sessionStats.count > 0 {
		avgUtil = m.sessionStats.sumUtil / float64(m.sessionStats.count)
	}
	lines = append(lines, fmt.Sprintf("  Avg Util    %s",
		barFill.Render(fmt.Sprintf("%.1f%%", avgUtil))))
	lines = append(lines, fmt.Sprintf("  Min Clock   %s %s",
		dimStyle.Render(fmt.Sprintf("%d MHz", m.sessionStats.minClockMHz)),
		dimStyle.Render(fmt.Sprintf("(GPU%d)", m.sessionStats.minClockGPU))))
	lines = append(lines, fmt.Sprintf("  Samples     %s",
		dimStyle.Render(fmt.Sprintf("%d", m.sessionStats.count))))
	return lines
}

func (m tuiModel) viewHost(b *strings.Builder, snap model.Snapshot, w int) {
	b.WriteString(renderSep(w))

	_, barW := layoutWidths(w)

	// Build host lines (left column).
	var hostLines []string
	hostLines = append(hostLines, headStyle.Render(" Host"))

	cpuTotal := snap.Node.CPUUser + snap.Node.CPUSys + snap.Node.CPUIowait
	cpuBar := renderBar(cpuTotal, 100, barW)
	cpuDetail := fmt.Sprintf("usr %.1f%%  sys %.1f%%  iow %.1f%%",
		snap.Node.CPUUser, snap.Node.CPUSys, snap.Node.CPUIowait)
	hostLines = append(hostLines, fmt.Sprintf("   CPU  %s %4.1f%%   %s",
		cpuBar, cpuTotal, dimStyle.Render(cpuDetail)))

	memUsed := uint64(0)
	memPct := float64(0)
	if snap.Node.MemTotal > 0 {
		memUsed = snap.Node.MemTotal - snap.Node.MemAvailable
		memPct = float64(memUsed) / float64(snap.Node.MemTotal) * 100
	}
	memBar := renderBar(memPct, 100, barW)
	hostLines = append(hostLines, fmt.Sprintf("   Mem  %s %4.1f%%   %s / %s",
		memBar, memPct, humanBytes(memUsed), humanBytes(snap.Node.MemTotal)))

	if snap.Node.SwapTotal > 0 && snap.Node.SwapUsed > 0 {
		swapPct := float64(snap.Node.SwapUsed) / float64(snap.Node.SwapTotal) * 100
		hostLines = append(hostLines, fmt.Sprintf("   Swap %s / %s (%.1f%%)",
			humanBytes(snap.Node.SwapUsed), humanBytes(snap.Node.SwapTotal), swapPct))
	}

	if len(snap.Node.LoadAvg) >= 3 {
		hostLines = append(hostLines, fmt.Sprintf("   Load %.2f %s  %.2f %s  %.2f %s",
			snap.Node.LoadAvg[0], dimStyle.Render("1m"),
			snap.Node.LoadAvg[1], dimStyle.Render("5m"),
			snap.Node.LoadAvg[2], dimStyle.Render("15m")))
	}

	// Build NVLink status (right column) from cached state.
	nvLines := m.buildNVLinkStatus()

	if len(nvLines) > 0 && w >= 100 {
		leftW := w/2 - 1
		joined := joinColumns(hostLines, nvLines, leftW, " │ ")
		for _, line := range joined {
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		for _, line := range hostLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func (m tuiModel) buildNVLinkStatus() []string {
	if len(m.nvlinkCache) == 0 {
		return nil
	}

	// Build link grid lines.
	var gridLines []string
	gridLines = append(gridLines, headStyle.Render(" NVLink Status"))

	perCol := (len(m.nvlinkCache) + 1) / 2

	for row := 0; row < perCol; row++ {
		var parts []string
		for col := 0; col < 2; col++ {
			idx := row + col*perCol
			if idx >= len(m.nvlinkCache) {
				continue
			}
			info := m.nvlinkCache[idx]
			indicator := barFill.Render("●")
			switch info.status {
			case "crc":
				indicator = warnStyle.Render("⚠")
			case "down":
				indicator = critStyle.Render("✗")
			}
			parts = append(parts, fmt.Sprintf("  GPU%d %d/%d %s",
				info.index, info.active, info.total, indicator))
		}
		gridLines = append(gridLines, strings.Join(parts, "  "))
	}

	// Build errors-seen column from session stats.
	errorLines := m.buildErrorsSeen()
	if len(errorLines) == 0 {
		return gridLines
	}

	gridW := 0
	for _, l := range gridLines {
		if lw := lipgloss.Width(l); lw > gridW {
			gridW = lw
		}
	}

	return joinColumns(gridLines, errorLines, gridW+2, "  ")
}

func (m tuiModel) buildErrorsSeen() []string {
	type errEntry struct {
		index int
		desc  string
		rank  int // 0=critical, 1=warning
	}

	var entries []errEntry
	for idx, es := range m.errorStats {
		if es.eccUncorrectable > 0 {
			entries = append(entries, errEntry{
				index: idx,
				desc:  fmt.Sprintf("ECC ×%d uncorr", es.eccUncorrectable),
				rank:  0,
			})
		}
		if es.remappedPending {
			entries = append(entries, errEntry{
				index: idx,
				desc:  "remap pending",
				rank:  0,
			})
		}
		if es.crcDeltaTotal > 0 {
			entries = append(entries, errEntry{
				index: idx,
				desc:  fmt.Sprintf("CRC +%d", es.crcDeltaTotal),
				rank:  1,
			})
		}
		if es.eccCorrectable > 0 {
			entries = append(entries, errEntry{
				index: idx,
				desc:  fmt.Sprintf("ECC ×%d corr", es.eccCorrectable),
				rank:  1,
			})
		}
	}

	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].rank != entries[j].rank {
			return entries[i].rank < entries[j].rank
		}
		return entries[i].index < entries[j].index
	})

	var lines []string
	lines = append(lines, headStyle.Render("Errors Seen"))
	for _, e := range entries {
		style := warnStyle
		if e.rank == 0 {
			style = critStyle
		}
		lines = append(lines, fmt.Sprintf(" GPU%d  %s", e.index, style.Render(e.desc)))
	}
	return lines
}

func (m tuiModel) viewProcs(b *strings.Builder, snap model.Snapshot, w int) {
	if len(snap.Procs) == 0 {
		return
	}

	label := " Processes"
	sortLabel := fmt.Sprintf("  [sort: %s]", m.sortField.label())
	if m.activePanel == panelProc {
		label = focusStyle.Render(label)
		sortLabel = focusStyle.Render(sortLabel)
		b.WriteString(renderFocusSep(w))
	} else {
		label = headStyle.Render(label)
		sortLabel = dimStyle.Render(sortLabel)
		b.WriteString(renderSep(w))
	}
	b.WriteString(label)
	b.WriteString(sortLabel)
	b.WriteString("\n")

	// Fixed-width columns: PID(7) + GPU(3) + VRAM(7) + Util(5) + CPU(5) + RSS(7) + padding
	userW := 16
	cmdW := w - userW - 60
	if cmdW < 16 {
		cmdW = 16
	}

	hdr := fmt.Sprintf("   %-7s  %-*s  %-*s  %3s  %7s  %5s  %5s  %7s",
		"PID", userW, "User", cmdW, "Cmd", "GPU", "VRAM", "Util", "CPU%", "RSS")
	b.WriteString(dimStyle.Render(hdr))
	b.WriteString("\n")

	procs := m.sortedProcs(snap.Procs)

	maxVisible := min(len(procs), m.procPanelCap())
	offset := 0
	if m.procCursor >= maxVisible {
		offset = m.procCursor - maxVisible + 1
	}
	end := min(offset+maxVisible, len(procs))

	isFocused := m.activePanel == panelProc

	for i := offset; i < end; i++ {
		p := procs[i]
		prefix := "  "
		if isFocused && i == m.procCursor {
			prefix = " " + m.cursor()
		}
		rssStr := formatRSS(p.RSSMB)
		line := fmt.Sprintf("%s %-7d  %-*s  %-*s  %3d  %4d MB  %4.0f%%  %4.0f%%  %7s",
			prefix, p.PID, userW, truncate(p.User, userW), cmdW, truncate(p.Cmd, cmdW),
			p.GPUIndex, p.VRAMMB, p.UtilPct, p.CPUPct, rssStr)
		if isFocused && i == m.procCursor {
			b.WriteString(selStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	if len(procs) > maxVisible {
		b.WriteString(dimStyle.Render(fmt.Sprintf("   ↕ %d/%d processes (j/k to scroll)", m.procCursor+1, len(procs))))
		b.WriteString("\n")
	}
}

// sortedProcs returns the memoised sorted process slice. Computation
// happens in Update whenever dataVersion changes (new snapshot or sort
// field change); here we just hand out the cached result.
func (m tuiModel) sortedProcs(procs []model.ProcStat) []model.ProcStat {
	if m.procsVersion == m.dataVersion && len(m.procsSorted) == len(procs) {
		return m.procsSorted
	}
	// Fall back to an eager sort if caches are stale (e.g. called before
	// Update had a chance to run). Avoids panics in edge cases.
	sorted := make([]model.ProcStat, len(procs))
	copy(sorted, procs)
	sortProcsBy(sorted, m.sortField)
	return sorted
}

func sortProcsBy(procs []model.ProcStat, field sortField) {
	sort.Slice(procs, func(i, j int) bool {
		switch field {
		case sortByVRAM:
			return procs[i].VRAMMB > procs[j].VRAMMB
		case sortByPID:
			return procs[i].PID < procs[j].PID
		default:
			return procs[i].UtilPct > procs[j].UtilPct
		}
	})
}

// groupedHint aggregates duplicate hints by name.
type groupedHint struct {
	hint  model.Hint
	count int
	ts    time.Time
}

// cursoredHint returns the hint currently selected by the user, depending on
// which panel is focused. Returns nil if the focused panel isn't a hints
// panel or there's nothing to point at.
func (m tuiModel) cursoredHint() *model.Hint {
	switch m.activePanel {
	case panelHints:
		groups := m.activeHintGroups(m.snapshot)
		if len(groups) == 0 {
			return nil
		}
		i := m.hintCursor
		if i < 0 {
			i = 0
		}
		if i >= len(groups) {
			i = len(groups) - 1
		}
		return &groups[i].hint
	case panelHistory:
		sorted := m.sortedHistory()
		if len(sorted) == 0 {
			return nil
		}
		i := m.historyCursor
		if i < 0 {
			i = 0
		}
		if i >= len(sorted) {
			i = len(sorted) - 1
		}
		return &sorted[i].hint
	}
	return nil
}

// activeHintGroups returns the same severity-sorted, name-grouped list of
// hints that viewHints renders. Used by both the renderer and the keypress
// handler so the cursor selection in the popup matches what the user sees.
func (m tuiModel) activeHintGroups(snap model.Snapshot) []*groupedHint {
	if len(snap.Hints) == 0 {
		return nil
	}
	groups := make(map[string]*groupedHint)
	var order []string
	for _, h := range snap.Hints {
		if g, ok := groups[h.Name]; ok {
			g.count++
			if severityRank(h.Severity) < severityRank(g.hint.Severity) {
				g.hint = h
			}
		} else {
			groups[h.Name] = &groupedHint{hint: h, count: 1, ts: snap.TS}
			order = append(order, h.Name)
		}
	}
	display := make([]*groupedHint, 0, len(order))
	for _, name := range order {
		display = append(display, groups[name])
	}
	sort.Slice(display, func(i, j int) bool {
		return severityRank(display[i].hint.Severity) < severityRank(display[j].hint.Severity)
	})
	return display
}

func (m tuiModel) viewHints(b *strings.Builder, snap model.Snapshot, w int) {
	isSideBySide := w >= 100
	leftW := w
	if isSideBySide {
		leftW = w/2 - 1
	}
	rightW := w - leftW - 3 // 3 for " │ "

	// --- Active Hints (left column) ---
	var leftLines []string

	activeLabel := " Active Hints"
	if m.activePanel == panelHints {
		activeLabel = focusStyle.Render(activeLabel)
	} else {
		activeLabel = headStyle.Render(activeLabel)
	}

	display := m.activeHintGroups(snap)
	if len(display) == 0 {
		leftLines = append(leftLines, activeLabel+dimStyle.Render("  — no active alerts"))
	} else {
		total := len(snap.Hints)
		unique := len(display)
		leftLines = append(leftLines, activeLabel+dimStyle.Render(fmt.Sprintf("  (%d unique, %d total)", unique, total)))

		activeCap, _ := m.hintsPanelCap()
		hc := min(m.hintCursor, max(0, len(display)-1))
		maxVisible := min(len(display), activeCap)
		offset := 0
		if hc >= maxVisible {
			offset = hc - maxVisible + 1
		}
		end := min(offset+maxVisible, len(display))
		isFocused := m.activePanel == panelHints

		// Reserve space for: prefix(3) + " " + tsStr(8) + "  " + badge(~10)
		// + " " + countStr(~12). Leave at least 10 chars for the summary
		// itself even on a very narrow column.
		const activeMetaW = 36
		for i := offset; i < end; i++ {
			g := display[i]
			badge := hintBadge(g.hint.Severity)
			tsStr := dimStyle.Render(g.ts.Format("15:04:05"))
			countStr := ""
			if g.count > 1 {
				countStr = dimStyle.Render(fmt.Sprintf(" (×%d GPUs)", g.count))
			}
			prefix := "  "
			if isFocused && i == hc {
				prefix = " " + m.cursor()
			}
			sumW := leftW - activeMetaW
			if sumW < 10 {
				sumW = 10
			}
			summary := truncate(g.hint.Summary, sumW)
			line := fmt.Sprintf("%s %s  %s %s%s", prefix, tsStr, badge, summary, countStr)
			if isFocused && i == hc {
				line = selStyle.Render(line)
			}
			leftLines = append(leftLines, line)
		}

		if len(display) > maxVisible {
			leftLines = append(leftLines, dimStyle.Render(fmt.Sprintf("   ↕ %d/%d hints (j/k to scroll)", hc+1, len(display))))
		}
	}

	// --- Hints History (right column) ---
	var rightLines []string

	histLabel := " Hints History"
	if m.activePanel == panelHistory {
		histLabel = focusStyle.Render(histLabel)
	} else {
		histLabel = headStyle.Render(histLabel)
	}

	if len(m.hintHistory) == 0 {
		rightLines = append(rightLines, histLabel+dimStyle.Render("  — empty"))
	} else {
		sorted := m.sortedHistory()
		rightLines = append(rightLines, histLabel+dimStyle.Render(fmt.Sprintf("  (%d total)", len(sorted))))

		_, historyCap := m.hintsPanelCap()
		hc := min(m.historyCursor, max(0, len(sorted)-1))
		maxVisible := min(len(sorted), historyCap)
		offset := 0
		if hc >= maxVisible {
			offset = hc - maxVisible + 1
		}
		end := min(offset+maxVisible, len(sorted))
		isFocused := m.activePanel == panelHistory

		for i := offset; i < end; i++ {
			entry := sorted[i]
			badge := hintBadge(entry.hint.Severity)
			tsStr := dimStyle.Render(entry.firstSeen.Format("01-02 15:04:05"))
			countStr := ""
			if entry.count > 1 {
				countStr = dimStyle.Render(fmt.Sprintf(" (×%d seen)", entry.count))
			}
			activeMarker := ""
			if entry.active {
				activeMarker = barFill.Render("▪") + " "
			}
			prefix := "  "
			if isFocused && i == hc {
				prefix = " " + m.cursor()
			}
			// Reserve space for: prefix(3) + " " + activeMarker(0-2) +
			// tsStr(14) + "  " + badge(~10) + " " + countStr(~12) ≈ 43.
			// Always truncate so neither side-by-side nor stacked mode
			// overflows the column boundary.
			const histMetaW = 43
			colW := rightW
			if !isSideBySide {
				colW = w
			}
			sumW := colW - histMetaW
			if sumW < 10 {
				sumW = 10
			}
			summary := truncate(entry.hint.Summary, sumW)
			line := fmt.Sprintf("%s %s%s  %s %s%s", prefix, activeMarker, tsStr, badge, summary, countStr)
			if isFocused && i == hc {
				line = selStyle.Render(line)
			}
			rightLines = append(rightLines, line)
		}

		if len(sorted) > maxVisible {
			rightLines = append(rightLines, dimStyle.Render(fmt.Sprintf("   ↕ %d/%d history (j/k to scroll)", hc+1, len(sorted))))
		}
	}

	// --- Compose output ---
	if m.activePanel == panelHints || m.activePanel == panelHistory {
		b.WriteString(renderFocusSep(w))
	} else {
		b.WriteString(renderSep(w))
	}

	if isSideBySide {
		lines := joinColumns(leftLines, rightLines, leftW, " │ ")
		for _, line := range lines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	} else {
		for _, line := range leftLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		for _, line := range rightLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

// gpuStateBadges returns a short coloured string summarising the
// most important driver-reported state: active throttle reasons and
// a parked P-state. Empty when there's nothing notable to surface.
func gpuStateBadges(g model.GPUStat) string {
	var parts []string
	if reasons := decodeTUIThrottle(g.ThrottleReasons); reasons != "" {
		parts = append(parts, hintCrit.Render(" THROTTLE: "+reasons+" "))
	}
	if g.PerfState >= 8 && g.PerfState < 32 {
		parts = append(parts, dimStyle.Render(fmt.Sprintf(" P%d ", g.PerfState)))
	}
	return strings.Join(parts, " ")
}

// decodeTUIThrottle decodes the NVML throttle bitmap into a short
// human-readable label. Mirrors rules.decodeThrottleReasons but kept
// inline here so the TUI doesn't import the rules package.
func decodeTUIThrottle(bits uint64) string {
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

func hintBadge(severity string) string {
	sev := strings.ToUpper(severity)
	badge := " " + sev + " "
	switch severity {
	case "critical":
		return hintCrit.Render(badge)
	case "warning":
		return hintWarn.Render(badge)
	default:
		return dimStyle.Render(badge)
	}
}

func severityRank(sev string) int {
	switch sev {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func (m tuiModel) sortedHistory() []hintHistoryEntry {
	if m.historyVersion == m.dataVersion && len(m.historySorted) == len(m.hintHistory) {
		return m.historySorted
	}
	sorted := make([]hintHistoryEntry, len(m.hintHistory))
	copy(sorted, m.hintHistory)
	sortHintHistory(sorted)
	return sorted
}

func sortHintHistory(entries []hintHistoryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		ri, rj := severityRank(entries[i].hint.Severity), severityRank(entries[j].hint.Severity)
		if ri != rj {
			return ri < rj
		}
		return entries[i].firstSeen.After(entries[j].firstSeen)
	})
}

func joinColumns(left, right []string, leftW int, sep string) []string {
	n := max(len(left), len(right))
	result := make([]string, n)
	for i := 0; i < n; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		lVis := lipgloss.Width(l)
		pad := leftW - lVis
		if pad < 0 {
			pad = 0
		}
		result[i] = l + strings.Repeat(" ", pad) + sep + r
	}
	return result
}

func (m tuiModel) viewCharts(b *strings.Builder, snap model.Snapshot, w int) {
	if len(m.history) == 0 {
		return
	}

	nGPU := len(snap.GPUs)
	if nGPU == 0 {
		return
	}

	// Estimate rows consumed by upstream sections so charts can fill the
	// rest. When the terminal is too short for at least 2 rows per GPU,
	// skip the chart panel entirely so the GPU table at the top stays
	// visible — losing the bottom panel is much better than losing the
	// top of a 70-row View() to alt-screen scrollback.
	gpuPanelRows := m.gpuPanelCap()
	procCap := m.procPanelCap()
	activeCap, historyCap := m.hintsPanelCap()

	overhead := 1 + // header
		3 + gpuPanelRows + // GPU section (sep + title + col_header + rows)
		5 + // host section
		2 // footer
	if len(snap.Procs) > 0 {
		overhead += 3 + min(len(snap.Procs), procCap) + 1
	}
	nHints := len(m.dedupHints(snap))
	nHistoryLines := min(len(m.hintHistory), historyCap)
	activeHintLines := 1
	if nHints > 0 {
		activeHintLines += min(nHints, activeCap) + 1
	}
	historyHintLines := 1
	if nHistoryLines > 0 {
		historyHintLines += nHistoryLines + 1
	}
	overhead += max(activeHintLines, historyHintLines) + 1
	overhead += 3 // chart header (sep + title + col_headers)

	termH := m.height
	if termH <= 0 {
		termH = 40 // pre-WindowSizeMsg fallback
	}
	chartContentRows := termH - overhead
	// Each braille row already encodes 4 vertical pixels, so a 1-row
	// chart per GPU still conveys a meaningful trend. Only skip the
	// panel entirely when even that doesn't fit.
	minPerGPU := 1
	if chartContentRows < nGPU*minPerGPU {
		return
	}

	if m.activePanel == panelCharts {
		b.WriteString(renderFocusSep(w))
	} else {
		b.WriteString(renderSep(w))
	}

	titleLabel := "GPU History"
	if m.activePanel == panelCharts {
		titleLabel = focusStyle.Render(" " + titleLabel)
	} else {
		titleLabel = headStyle.Render(" " + titleLabel)
	}
	zoomInfo := dimStyle.Render(fmt.Sprintf("  %s  z:zoom", zoomLabels[m.chartZoom]))
	b.WriteString(titleLabel)
	b.WriteString(zoomInfo)
	b.WriteString("\n")

	chartH := chartContentRows / nGPU
	if chartH < minPerGPU {
		chartH = minPerGPU
	}
	if chartH > 4 {
		chartH = 4 // cap so charts don't dominate when terminal is huge
	}

	nMetrics := int(metricCount)
	gpuLabelW := 6
	gapW := 1
	colW := (w - gpuLabelW - gapW*(nMetrics-1)) / nMetrics
	if colW < 4 {
		colW = 4
	}

	hdr := strings.Repeat(" ", gpuLabelW)
	for metric := chartMetric(0); metric < metricCount; metric++ {
		lbl := metric.label()
		colTotal := colW
		if int(metric) < nMetrics-1 {
			colTotal += gapW
		}
		pad := colTotal - len(lbl)
		if pad < 0 {
			pad = 0
		}
		hdr += lbl + strings.Repeat(" ", pad)
	}
	b.WriteString(dimStyle.Render(truncate(hdr, w)))
	b.WriteString("\n")

	maxSamples := zoomSamples[m.chartZoom]

	// Reuse a scratch slice across all metric charts rendered in this
	// frame. renderChart doesn't retain `vals`, so one buffer can feed
	// every GPU × metric pair.
	scratch := make([]float64, 0, maxSamples)

	for _, g := range snap.GPUs {
		h, ok := m.history[g.Index]
		if !ok {
			continue
		}

		var charts [5][]string
		for metric := chartMetric(0); metric < metricCount; metric++ {
			rb := h.forMetric(metric)
			scratch = rb.LastNInto(scratch, maxSamples)
			charts[int(metric)] = renderChart(scratch, colW, chartH, metric.maxVal())
		}

		for row := 0; row < chartH; row++ {
			if row == chartH/2 {
				label := fmt.Sprintf(" GPU%d ", g.Index)
				b.WriteString(headStyle.Render(label))
			} else {
				b.WriteString(strings.Repeat(" ", gpuLabelW))
			}
			for mi := 0; mi < nMetrics; mi++ {
				if charts[mi] != nil && row < len(charts[mi]) {
					b.WriteString(charts[mi][row])
				} else {
					b.WriteString(strings.Repeat(" ", colW))
				}
				if mi < nMetrics-1 {
					b.WriteString(strings.Repeat(" ", gapW))
				}
			}
			b.WriteString("\n")
		}
	}
}

// dedupHints returns the count of unique hint names (for layout calculation).
func (m tuiModel) dedupHints(snap model.Snapshot) []model.Hint {
	seen := make(map[string]bool)
	var result []model.Hint
	for _, h := range snap.Hints {
		if !seen[h.Name] {
			seen[h.Name] = true
			result = append(result, h)
		}
	}
	return result
}

func (m tuiModel) viewFooter(b *strings.Builder, w int) {
	b.WriteString(renderSep(w))
	foot := "  q quit  •  ? help  •  s sort  •  space pause  •  j/k scroll  •  tab focus  •  enter details  •  z zoom  •  H history"
	b.WriteString(footStyle.Render(foot))
	b.WriteString("\n")
}

func (m tuiModel) viewHelp(w int) string {
	var b strings.Builder

	title := titleStyle.Render(" gputui Help ")
	b.WriteString("\n")
	pad := max(0, (w-lipgloss.Width(title))/2)
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(title)
	b.WriteString("\n\n")

	keys := []struct{ key, desc string }{
		{"q / Ctrl+C", "Quit"},
		{"?", "Toggle this help"},
		{"space", "Pause / resume refresh"},
		{"j / Down", "Scroll down in focused panel"},
		{"k / Up", "Scroll up in focused panel"},
		{"s", "Cycle sort (Util% → VRAM → PID)"},
		{"Tab", "Cycle panel focus"},
		{"g / p / h / c", "Jump to GPU / proc / hints / charts"},
		{"H (shift)", "Jump to hints history"},
		{"Enter / o", "Open detail popup for selected hint"},
		{"z", "Cycle chart zoom (2m / 5m / 10m)"},
	}

	for _, k := range keys {
		b.WriteString(fmt.Sprintf("   %-14s  %s\n", k.key, k.desc))
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  Press any key to close"))
	b.WriteString("\n")

	return b.String()
}

// viewHintDetail renders a centered popup with the full text of the selected
// hint, severity badge, confidence, and any evidence rows attached by the
// rule. Long summaries are word-wrapped so nothing exceeds the popup width.
func (m tuiModel) viewHintDetail(w int) string {
	h := *m.hintDetail

	// Popup width: clamp to terminal width with sensible bounds. Inside the
	// border we lose 4 columns ("│ " + " │") for padding/border.
	innerW := w - 4 - 4 // 4 for outer margin, 4 for border+padding
	if innerW < 30 {
		innerW = 30
	}
	if innerW > 90 {
		innerW = 90
	}

	var body strings.Builder

	badge := hintBadge(h.Severity)
	body.WriteString(badge)
	body.WriteString("  ")
	body.WriteString(headStyle.Render(h.Name))
	body.WriteString("\n")
	meta := fmt.Sprintf("category: %s   confidence: %.2f", h.Category, h.Confidence)
	body.WriteString(dimStyle.Render(meta))
	body.WriteString("\n\n")

	body.WriteString(headStyle.Render("Summary"))
	body.WriteString("\n")
	for _, line := range wordWrap(h.Summary, innerW) {
		body.WriteString(line)
		body.WriteString("\n")
	}

	if len(h.Evidence) > 0 {
		body.WriteString("\n")
		body.WriteString(headStyle.Render("Evidence"))
		body.WriteString("\n")
		for _, ev := range h.Evidence {
			head := "  • " + ev.Metric
			if ev.Unit != "" {
				if ev.Threshold != 0 {
					head += fmt.Sprintf(" = %.2f %s (threshold %.2f %s)", ev.Value, ev.Unit, ev.Threshold, ev.Unit)
				} else {
					head += fmt.Sprintf(" = %.2f %s", ev.Value, ev.Unit)
				}
			} else if ev.Value != 0 || ev.Threshold != 0 {
				if ev.Threshold != 0 {
					head += fmt.Sprintf(" = %.2f (threshold %.2f)", ev.Value, ev.Threshold)
				} else {
					head += fmt.Sprintf(" = %.2f", ev.Value)
				}
			}
			body.WriteString(head)
			body.WriteString("\n")
			if ev.Msg != "" {
				for _, line := range wordWrap(ev.Msg, innerW-4) {
					body.WriteString("    ")
					body.WriteString(dimStyle.Render(line))
					body.WriteString("\n")
				}
			}
		}
	}

	body.WriteString("\n")
	body.WriteString(dimStyle.Render("Press any key to close"))

	popupStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("14")).
		Padding(0, 1).
		Width(innerW + 2)

	popup := popupStyle.Render(body.String())

	// Center the popup horizontally with leading newlines for breathing room.
	popupW := lipgloss.Width(popup)
	leftPad := max(0, (w-popupW)/2)
	pad := strings.Repeat(" ", leftPad)

	var out strings.Builder
	out.WriteString("\n")

	title := titleStyle.Render(" Hint Details ")
	titlePad := max(0, (w-lipgloss.Width(title))/2)
	out.WriteString(strings.Repeat(" ", titlePad))
	out.WriteString(title)
	out.WriteString("\n\n")

	for _, line := range strings.Split(popup, "\n") {
		out.WriteString(pad)
		out.WriteString(line)
		out.WriteString("\n")
	}
	return out.String()
}

// --- rendering helpers ----------------------------------------------------

func renderSep(w int) string {
	if w < 1 {
		w = 80
	}
	return sepStyle.Render(" "+strings.Repeat("─", w-2)) + "\n"
}

func renderFocusSep(w int) string {
	if w < 1 {
		w = 80
	}
	return focusStyle.Render(" "+strings.Repeat("─", w-2)) + "\n"
}

func renderBar(val, maxVal float64, width int) string {
	pct := val / maxVal
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(math.Round(pct * float64(width)))
	empty := width - filled

	style := barFill
	if pct > 0.9 {
		style = barCrit
	} else if pct > 0.7 {
		style = barHigh
	}
	return "[" + style.Render(strings.Repeat("█", filled)) + barEmpty.Render(strings.Repeat("░", empty)) + "]"
}

func layoutWidths(termW int) (nameW, barW int) {
	nameW = 26
	barW = 10
	if termW >= 120 {
		nameW = 30
		barW = 16
	} else if termW < 90 {
		nameW = 18
		barW = 8
	}
	return nameW, barW
}
