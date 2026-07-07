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
	// Price axis labels use the compact formatter for the max (16) and min (8).
	if !strings.Contains(out, ">16<") || !strings.Contains(out, ">8<") {
		t.Errorf("missing axis labels: %s", out)
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
