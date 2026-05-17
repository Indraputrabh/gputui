package main

import (
	"fmt"
	"strings"
)

// wordWrap splits s into lines of at most width visual columns, breaking on
// whitespace where possible. Words longer than width are hard-broken so a
// pathological summary still fits inside the popup. Width <= 0 returns s
// untouched as a single-element slice so callers don't have to special-case.
func wordWrap(s string, width int) []string {
	if width <= 0 || s == "" {
		return []string{s}
	}
	var lines []string
	for _, paragraph := range strings.Split(s, "\n") {
		if paragraph == "" {
			lines = append(lines, "")
			continue
		}
		var cur strings.Builder
		for _, word := range strings.Fields(paragraph) {
			if len(word) > width {
				if cur.Len() > 0 {
					lines = append(lines, cur.String())
					cur.Reset()
				}
				for len(word) > width {
					lines = append(lines, word[:width])
					word = word[width:]
				}
				cur.WriteString(word)
				continue
			}
			needed := len(word)
			if cur.Len() > 0 {
				needed++
			}
			if cur.Len()+needed > width {
				lines = append(lines, cur.String())
				cur.Reset()
				cur.WriteString(word)
				continue
			}
			if cur.Len() > 0 {
				cur.WriteByte(' ')
			}
			cur.WriteString(word)
		}
		if cur.Len() > 0 {
			lines = append(lines, cur.String())
		}
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func formatRSS(mb uint64) string {
	if mb >= 1024 {
		return fmt.Sprintf("%.1f GB", float64(mb)/1024)
	}
	return fmt.Sprintf("%d MB", mb)
}

func humanBytes(b uint64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
