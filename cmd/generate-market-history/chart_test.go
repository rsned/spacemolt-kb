package main

import (
	"strings"
	"testing"
)

func TestCandlestickSVG_UpDownBodies(t *testing.T) {
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 100}, // up
		{Day: "2026-06-22", Open: 14, High: 16, Low: 8, Close: 10, Volume: 40},  // down
	}
	out := string(candlestickSVG(cs))
	if !strings.Contains(out, "<svg") || !strings.Contains(out, "</svg>") {
		t.Fatal("not an svg")
	}
	if n := strings.Count(out, `class="body`); n != 2 {
		t.Errorf("want 2 bodies, got %d", n)
	}
	if !strings.Contains(out, `class="body up"`) || !strings.Contains(out, `class="body down"`) {
		t.Error("expected both up and down bodies")
	}
	// Price axis draws evenly-spaced horizontal gridlines with value labels.
	if n := strings.Count(out, `class="grid"`); n != priceTicks {
		t.Errorf("want %d gridlines, got %d", priceTicks, n)
	}
}

func TestCandlestickSVG_LogScaleForSkew(t *testing.T) {
	// A wide high/low ratio (1 → 42000) triggers the log10 price axis.
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 1, High: 1, Low: 1, Close: 1, Volume: 5},
		{Day: "2026-06-22", Open: 1, High: 42000, Low: 1, Close: 42000, Volume: 9},
	}
	out := string(candlestickSVG(cs))
	if !strings.Contains(out, ">log<") {
		t.Errorf("expected a log-scale indicator: %s", out)
	}
	if strings.Contains(out, "NaN") || strings.Contains(out, "Inf") {
		t.Errorf("numeric blowup on log scale: %s", out)
	}
	if n := strings.Count(out, `class="grid"`); n != priceTicks {
		t.Errorf("want %d gridlines, got %d", priceTicks, n)
	}
}

func TestCandlestickSVG_TightRangeStaysLinear(t *testing.T) {
	// A modest ratio (100 → 200) keeps the linear axis (no log indicator).
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 100, High: 120, Low: 100, Close: 110, Volume: 5},
		{Day: "2026-06-22", Open: 110, High: 200, Low: 105, Close: 190, Volume: 9},
	}
	out := string(candlestickSVG(cs))
	if strings.Contains(out, ">log<") {
		t.Errorf("tight-range chart should stay linear: %s", out)
	}
}

func TestPriceScale(t *testing.T) {
	// 10% headroom on each side of the data range.
	if lo, hi := priceScale(100, 200); lo != 90 || hi != 210 {
		t.Errorf("priceScale(100,200) = %v,%v want 90,210", lo, hi)
	}
	// Bottom clamps at zero when padding would go negative.
	if lo, hi := priceScale(1, 21); lo != 0 || hi != 23 {
		t.Errorf("priceScale(1,21) = %v,%v want 0,23", lo, hi)
	}
	// Flat series pads by 10% of the level.
	if lo, hi := priceScale(50, 50); lo != 45 || hi != 55 {
		t.Errorf("priceScale(50,50) = %v,%v want 45,55", lo, hi)
	}
	// Flat at zero falls back to a unit band, clamped at 0 below.
	if lo, hi := priceScale(0, 0); lo != 0 || hi != 1 {
		t.Errorf("priceScale(0,0) = %v,%v want 0,1", lo, hi)
	}
}

func TestCandlestickSVG_SingleCandle(t *testing.T) {
	out := string(candlestickSVG([]DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 50}}))
	if strings.Count(out, `class="body`) != 1 {
		t.Errorf("want 1 body: %s", out)
	}
}

func TestCandlestickSVG_FlatAndZeroVolume(t *testing.T) {
	// Flat prices (high==low everywhere) and zero volume must not panic or NaN.
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 5, High: 5, Low: 5, Close: 5, Volume: 0},
		{Day: "2026-06-22", Open: 5, High: 5, Low: 5, Close: 5, Volume: 0},
	}
	out := string(candlestickSVG(cs))
	if !strings.Contains(out, "<svg") {
		t.Fatal("not an svg")
	}
	if strings.Contains(out, "NaN") || strings.Contains(out, "Inf") {
		t.Errorf("numeric blowup: %s", out)
	}
}

func TestCandlestickSVG_Empty(t *testing.T) {
	out := string(candlestickSVG(nil))
	if !strings.Contains(out, "<svg") || !strings.Contains(out, "</svg>") {
		t.Errorf("empty input should still yield an svg: %s", out)
	}
}
