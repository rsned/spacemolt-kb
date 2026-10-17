package main

import (
	"fmt"
	htmltpl "html/template"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/wildlife"
)

// SVG geometry (user units) for the per-species life-stage histogram.
const (
	stageHistW     = 640.0
	stageHistH     = 150.0
	stageHistLeft  = 8.0
	stageHistRight = 632.0
	stageHistAxisY = 118.0
	stageHistPlotH = 100.0
	stageHistBins  = 12 // target bin count; a species with a single hull value gets 1
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

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart stage-hist" role="img" aria-label="hull histogram by life stage">`,
		stageHistW, stageHistH)
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="grid"/>`,
		stageHistLeft, stageHistAxisY, stageHistRight, stageHistAxisY)

	slot := (stageHistRight - stageHistLeft) / float64(nBins)
	barW := slot * 0.82
	for i, bin := range counts {
		x := stageHistLeft + float64(i)*slot + (slot-barW)/2
		y := stageHistAxisY
		for si, c := range bin {
			if c == 0 {
				continue
			}
			h := float64(c) / float64(maxStack) * stageHistPlotH
			y -= h
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="%s"><title>%s: %d</title></rect>`,
				x, y, barW, h, stageBarColors[stageColorIndex(si)], htmltpl.HTMLEscapeString(stageLabel(s.Stages[si].Stage)), c)
		}
	}
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis">%d</text>`, stageHistLeft, stageHistAxisY+16, lo)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis" text-anchor="end">%d</text>`, stageHistRight, stageHistAxisY+16, hi)
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
