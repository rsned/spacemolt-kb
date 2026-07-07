package main

import (
	"fmt"
	"html/template"
	"strings"
)

// SVG geometry (user units). viewBox is fixed; the element scales to 100% width.
const (
	chartW   = 720.0
	chartVBH = 260.0
	priceTop = 10.0
	priceH   = 170.0
	volTop   = 195.0
	volH     = 40.0
	plotLeft = 44.0
	plotRt   = 710.0
	xLabelY  = 252.0
)

// candlestickSVG renders one item's galaxy-wide sell-side daily OHLC as a
// self-contained inline SVG: a price panel of candles above a volume panel.
// Up days (close >= open) get the "up" class, down days "down"; colors live in
// the page stylesheet. Output is deterministic.
func candlestickSVG(candles []DailyCandle) template.HTML {
	if len(candles) == 0 {
		return template.HTML(fmt.Sprintf(
			`<svg viewBox="0 0 %.0f %.0f" class="chart" role="img" aria-label="no data"></svg>`,
			chartW, chartVBH))
	}

	pmin, pmax := candles[0].Low, candles[0].High
	var vmax float64
	for _, c := range candles {
		if c.Low < pmin {
			pmin = c.Low
		}
		if c.High > pmax {
			pmax = c.High
		}
		if c.Volume > vmax {
			vmax = c.Volume
		}
	}
	prange := pmax - pmin
	yFor := func(p float64) float64 {
		if prange == 0 {
			return priceTop + priceH/2
		}
		return priceTop + (pmax-p)/prange*priceH
	}

	slot := (plotRt - plotLeft) / float64(len(candles))
	bodyW := slot * 0.6
	if bodyW > 14 {
		bodyW = 14
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart" role="img" aria-label="daily price and volume">`, chartW, chartVBH)
	// Price axis: max at top, min at bottom, baseline grid line.
	fmt.Fprintf(&b, `<text x="4" y="%.1f" class="axis">%s</text>`, priceTop+8, fmtCompact(pmax))
	fmt.Fprintf(&b, `<text x="4" y="%.1f" class="axis">%s</text>`, priceTop+priceH, fmtCompact(pmin))
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="grid"/>`, plotLeft, priceTop+priceH, plotRt, priceTop+priceH)

	for i, c := range candles {
		cx := plotLeft + (float64(i)+0.5)*slot
		cls := "up"
		if c.Close < c.Open {
			cls = "down"
		}
		// Wick: high to low.
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="wick %s"/>`,
			cx, yFor(c.High), cx, yFor(c.Low), cls)
		// Body: open to close, min 1px tall.
		top, bot := yFor(c.Open), yFor(c.Close)
		if bot < top {
			top, bot = bot, top
		}
		h := bot - top
		if h < 1 {
			h = 1
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="body %s"/>`,
			cx-bodyW/2, top, bodyW, h, cls)
		// Volume bar.
		vh := 0.0
		if vmax > 0 {
			vh = c.Volume / vmax * volH
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="vol %s"/>`,
			cx-bodyW/2, volTop+volH-vh, bodyW, vh, cls)
	}

	for _, i := range axisLabelIndices(len(candles)) {
		cx := plotLeft + (float64(i)+0.5)*slot
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis" text-anchor="middle">%s</text>`,
			cx, xLabelY, candles[i].Day[5:]) // MM-DD
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// axisLabelIndices picks up to four evenly spaced candle indices for x labels.
func axisLabelIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	if n <= 4 {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	return []int{0, n / 3, 2 * n / 3, n - 1}
}
