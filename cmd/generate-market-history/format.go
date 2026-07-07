package main

import (
	"fmt"
	"strconv"
)

// fmtCompact renders a value as a compact string: B/M carry one decimal, K and
// bare are whole. Thresholds are nudged below each round boundary so a value
// that would round UP to "1000" in the lower unit promotes to the next unit
// (e.g. 999,999 -> "1.0M", not "1000K").
func fmtCompact(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	var s string
	switch {
	case v >= 999_950_000:
		s = strconv.FormatFloat(v/1e9, 'f', 1, 64) + "B"
	case v >= 999_500:
		s = strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case v >= 999.5:
		s = strconv.FormatFloat(v/1e3, 'f', 0, 64) + "K"
	default:
		s = strconv.FormatFloat(v, 'f', 0, 64)
	}
	if neg {
		return "-" + s
	}
	return s
}

// fmtPct renders a signed percentage with one decimal ("+50.0%", "-3.1%").
func fmtPct(p float64) string {
	return fmt.Sprintf("%+.1f%%", p)
}
