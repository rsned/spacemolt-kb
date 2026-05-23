package jumpmap

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"
)

func TestRenderCoverageWheel_withGap(t *testing.T) {
	r := hyperjump.OriginReport{
		System:      "x",
		CoveragePct: 0.9,
		Gaps:        []hyperjump.Gap{{StartDeg: 30, EndDeg: 60, WidthDeg: 30, CenterDeg: 45}},
	}
	svg := RenderCoverageWheel(r)

	if !strings.Contains(svg, "<svg") {
		t.Fatalf("not an SVG:\n%s", svg)
	}
	if strings.Count(svg, "wheel-gap") != 1 {
		t.Errorf("expected exactly one gap wedge")
	}
	// Boundary headings of the gap are labeled.
	if !strings.Contains(svg, "30.0") || !strings.Contains(svg, "60.0") {
		t.Errorf("missing gap boundary labels 30.0 / 60.0")
	}
	// Center shows blocked coverage.
	if !strings.Contains(svg, "90.0") {
		t.Errorf("missing blocked-coverage center text")
	}
}

func TestRenderCoverageWheel_nearlySealed(t *testing.T) {
	// Alpha Centauri-like: 99.987% blocked with one tiny gap must not read 100.0%.
	r := hyperjump.OriginReport{
		System:      "alpha_centauri",
		CoveragePct: 0.9998689,
		Gaps:        []hyperjump.Gap{{StartDeg: 219.68, EndDeg: 219.73, WidthDeg: 0.047, CenterDeg: 219.7}},
	}
	svg := RenderCoverageWheel(r)
	if strings.Contains(svg, "100.0% blocked") {
		t.Errorf("nearly-sealed system must not display 100.0%% blocked while a gap exists")
	}
	if !strings.Contains(svg, "99.9% blocked") {
		t.Errorf("expected capped 99.9%% blocked, got:\n%s", svg)
	}
	if strings.Contains(svg, "0.0% open") {
		t.Errorf("open figure must not read 0.0%% open while a gap exists")
	}
}

func TestRenderCoverageWheel_noGap(t *testing.T) {
	r := hyperjump.OriginReport{System: "grumium", CoveragePct: 1.0, Gaps: nil}
	svg := RenderCoverageWheel(r)

	if strings.Contains(svg, "wheel-gap") {
		t.Errorf("fully enclosed system should have no gap wedges")
	}
	if !strings.Contains(svg, "No escape") {
		t.Errorf("expected 'No escape' center text for fully enclosed system")
	}
}
