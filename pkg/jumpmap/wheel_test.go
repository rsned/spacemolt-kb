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
