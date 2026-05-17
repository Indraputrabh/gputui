package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Braille dot bit masks indexed by [column][row] within a character cell.
// Each braille char is 2 columns × 4 rows of dots.
var brailleDots = [2][4]rune{
	{0x01, 0x02, 0x04, 0x40},
	{0x08, 0x10, 0x20, 0x80},
}

type ringBuffer struct {
	data []float64
	head int
	size int
	cap  int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		data: make([]float64, capacity),
		cap:  capacity,
	}
}

func (r *ringBuffer) Push(v float64) {
	r.data[r.head] = v
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

func (r *ringBuffer) Values() []float64 {
	if r.size == 0 {
		return nil
	}
	vals := make([]float64, r.size)
	start := (r.head - r.size + r.cap) % r.cap
	for i := range r.size {
		vals[i] = r.data[(start+i)%r.cap]
	}
	return vals
}

func (r *ringBuffer) Last() float64 {
	if r.size == 0 {
		return 0
	}
	return r.data[(r.head-1+r.cap)%r.cap]
}

func (r *ringBuffer) LastN(n int) []float64 {
	vals := r.Values()
	if len(vals) <= n {
		return vals
	}
	return vals[len(vals)-n:]
}

// LastNInto writes the most recent up-to-n samples into dst (reusing its
// backing storage when possible) and returns the populated slice. This
// avoids per-render allocations on the chart hot path.
func (r *ringBuffer) LastNInto(dst []float64, n int) []float64 {
	if r.size == 0 || n <= 0 {
		return dst[:0]
	}
	count := r.size
	if count > n {
		count = n
	}
	if cap(dst) < count {
		dst = make([]float64, count)
	} else {
		dst = dst[:count]
	}
	// Oldest-first walk through the ring for the last `count` samples.
	start := (r.head - count + r.cap) % r.cap
	for i := 0; i < count; i++ {
		dst[i] = r.data[(start+i)%r.cap]
	}
	return dst
}

var (
	densityStyleHigh = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	densityStyleMid  = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	densityStyleLow  = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
)

// densityStyle returns a color based on value percentage (0.0–1.0).
// Cyan for low values, through green/yellow/orange, to red for high values.
// Styles are precomputed so rendering doesn't allocate lipgloss.Style values.
func densityStyle(pct float64) lipgloss.Style {
	switch {
	case pct > 0.9:
		return barCrit
	case pct > 0.7:
		return densityStyleHigh
	case pct > 0.5:
		return densityStyleMid
	case pct > 0.3:
		return barFill
	default:
		return densityStyleLow
	}
}

// renderChart draws a filled-area braille line chart with density coloring.
// Each braille character is colored based on the data value at that column,
// producing a heatmap effect (cyan→green→yellow→orange→red).
func renderChart(vals []float64, width, height int, maxVal float64) []string {
	if len(vals) == 0 || width < 2 || height < 1 || maxVal <= 0 {
		return nil
	}

	pxW := width * 2
	pxH := height * 4

	if len(vals) > pxW {
		vals = vals[len(vals)-pxW:]
	}

	grid := make([][]bool, pxH)
	for i := range grid {
		grid[i] = make([]bool, pxW)
	}

	colVal := make([]float64, pxW)
	startX := pxW - len(vals)
	for i, v := range vals {
		px := startX + i
		if px < 0 {
			continue
		}
		colVal[px] = v
		topY := pxH - 1 - int(v/maxVal*float64(pxH-1)+0.5)
		if topY < 0 {
			topY = 0
		}
		if topY >= pxH {
			topY = pxH - 1
		}
		for y := topY; y < pxH; y++ {
			grid[y][px] = true
		}
	}

	lines := make([]string, height)
	for cy := range height {
		var b strings.Builder
		for cx := range width {
			var code rune = 0x2800
			hasAny := false
			for dy := range 4 {
				for dx := range 2 {
					py := cy*4 + dy
					px := cx*2 + dx
					if py < pxH && px < pxW && grid[py][px] {
						code |= brailleDots[dx][dy]
						hasAny = true
					}
				}
			}
			if !hasAny {
				b.WriteRune(0x2800)
				continue
			}
			px1 := min(cx*2+1, pxW-1)
			v := max(colVal[cx*2], colVal[px1])
			b.WriteString(densityStyle(v / maxVal).Render(string(code)))
		}
		lines[cy] = b.String()
	}

	return lines
}
