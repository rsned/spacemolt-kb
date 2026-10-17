package main

import (
	"fmt"
	htmltpl "html/template"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/wildlife"
)

// SVG geometry (user units) for the per-species life-stage histogram. The
// left margin holds the count axis and its label, the bottom margin the hull
// ticks and the axis title.
const (
	stageHistW     = 640.0
	stageHistH     = 196.0
	stageHistLeft  = 52.0
	stageHistRight = 628.0
	stageHistTop   = 18.0
	stageHistAxisY = 150.0
	stageHistBins  = 20 // target bin count; a species with a single hull value gets 1
	stageHistMajor = 5  // label every fifth bin edge; the rest get minor tick marks
)

// stageBarColors assigns a fill class to each life stage in ascending
// median-hull order (youngest/smallest first). A fifth-plus stage repeats
// the last color rather than running out.
var stageBarColors = []string{"stage-bar-0", "stage-bar-1", "stage-bar-2", "stage-bar-3"}

func stageColorIndex(i int) int {
	if i >= len(stageBarColors) {
		return len(stageBarColors) - 1
	}
	return i
}

// stageLabel renders a stage name for display; StageStats.Stage is already
// "adult" for the game's unstaged "" default.
func stageLabel(stage string) string {
	if stage == "" {
		return "Adult"
	}
	return strings.ToUpper(stage[:1]) + stage[1:]
}

// stageHistogramSVG renders a self-contained inline SVG histogram of each
// creature's max_hull at its latest sighting, binned across the species'
// full hull range and stacked/colored by life stage, plus a legend and a
// caption. Returns "" when the species has no stage data. No JS, no
// external assets.
func stageHistogramSVG(s wildlife.Species) htmltpl.HTML {
	if len(s.Stages) == 0 {
		return ""
	}
	lo, hi := s.HullRange()

	nBins := stageHistBins
	binW := 0.0
	if hi > lo {
		binW = float64(hi-lo) / float64(nBins)
	} else {
		nBins = 1
	}

	counts := make([][]int, nBins)
	for i := range counts {
		counts[i] = make([]int, len(s.Stages))
	}
	total := 0
	for si, st := range s.Stages {
		for _, h := range st.HullValues {
			total++
			idx := 0
			if binW > 0 {
				idx = int(float64(h-lo) / binW)
				idx = max(0, min(idx, nBins-1))
			}
			counts[idx][si]++
		}
	}
	maxStack := 1
	for _, bin := range counts {
		sum := 0
		for _, c := range bin {
			sum += c
		}
		maxStack = max(maxStack, sum)
	}

	plotH := stageHistAxisY - stageHistTop
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart stage-hist" role="img" aria-label="Histogram of creature counts by max hull, coloured by life stage">`,
		stageHistW, stageHistH)

	// count axis: integer ticks with gridlines, at most five steps
	step := max(1, (maxStack+4)/5)
	top := ((maxStack + step - 1) / step) * step
	if step > 1 && top <= 40 { // minor marks on the counts the labels skip
		for v := 1; v < top; v++ {
			if v%step == 0 {
				continue
			}
			y := stageHistAxisY - float64(v)/float64(top)*plotH
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="axis-line minor"/>`, stageHistLeft-3, y, stageHistLeft, y)
		}
	}
	for v := 0; v <= top; v += step {
		y := stageHistAxisY - float64(v)/float64(top)*plotH
		cls := "grid"
		if v == 0 {
			cls = "axis-line"
		}
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="%s"/>`, stageHistLeft, y, stageHistRight, y, cls)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis" text-anchor="end" dominant-baseline="middle">%d</text>`, stageHistLeft-6, y, v)
	}
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="axis-line"/>`, stageHistLeft, stageHistTop, stageHistLeft, stageHistAxisY)
	midY := stageHistTop + plotH/2
	fmt.Fprintf(&b, `<text x="14" y="%.1f" class="axis-title" text-anchor="middle" transform="rotate(-90 14 %.1f)">Creatures</text>`, midY, midY)

	slot := (stageHistRight - stageHistLeft) / float64(nBins)
	barW := slot * 0.82
	for i, bin := range counts {
		x := stageHistLeft + float64(i)*slot + (slot-barW)/2
		y := stageHistAxisY
		for si, c := range bin {
			if c == 0 {
				continue
			}
			h := float64(c) / float64(top) * plotH
			y -= h
			binLo := lo + int(float64(i)*binW)
			binHi := lo + int(float64(i+1)*binW)
			if nBins == 1 {
				binLo, binHi = lo, hi
			}
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="%s"><title>%s, hull %d–%d: %d</title></rect>`,
				x, y, barW, h, stageBarColors[stageColorIndex(si)], htmltpl.HTMLEscapeString(stageLabel(s.Stages[si].Stage)), binLo, binHi, c)
		}
	}

	// hull axis: a tick at every bin edge, labelled on every stageHistMajor-th
	// edge and at both range ends, so each labelled span splits into equal steps
	for t := 0; t <= nBins; t++ {
		x := stageHistLeft + float64(t)*slot
		major := t%stageHistMajor == 0 || t == nBins
		if !major {
			fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="axis-line minor"/>`, x, stageHistAxisY, x, stageHistAxisY+3)
			continue
		}
		v := lo + int(float64(t)*binW)
		if t == nBins {
			v = hi
		}
		anchor := "middle"
		switch t {
		case 0:
			anchor = "start"
		case nBins:
			anchor = "end"
		}
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="axis-line"/>`, x, stageHistAxisY, x, stageHistAxisY+6)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis" text-anchor="%s">%d</text>`, x, stageHistAxisY+18, anchor, v)
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis-title" text-anchor="middle">Max hull (HP)</text>`,
		(stageHistLeft+stageHistRight)/2, stageHistAxisY+38)
	b.WriteString(`</svg>`)

	b.WriteString(`<div class="stage-legend">`)
	for i, st := range s.Stages {
		fmt.Fprintf(&b, `<span class="stage-legend-item"><i class="stage-swatch stage-bg-%d"></i>%s (%d)</span>`,
			stageColorIndex(i), htmltpl.HTMLEscapeString(stageLabel(st.Stage)), st.Creatures)
	}
	b.WriteString(`</div>`)

	since := ""
	if t, err := time.Parse(time.RFC3339, s.StagesSince); err == nil {
		since = ", since " + t.Format("2006-01-02")
	}
	fmt.Fprintf(&b, `<p class="stage-hist-caption text-muted">Max hull of each individual creature at its latest sighting, by life stage (n = %d creatures%s).</p>`,
		total, since)

	return htmltpl.HTML(b.String()) //nolint:gosec // generated internally from trusted DB data
}
