package jumpmap

import (
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"
)

func sampleReport() hyperjump.OriginReport {
	return hyperjump.OriginReport{
		System: "sol",
		Pairs: []hyperjump.Pair{
			{To: "alpha", Bearing: 0, Distance: 300, Reachable: true, DestHasStation: true},
			{To: "beta", Bearing: 90, Distance: 500, Reachable: false, DestHasStation: true},
			{To: "gamma", Bearing: 180, Distance: 200, Reachable: true, DestHasStation: false},
		},
	}
}

func TestRenderStationArrows(t *testing.T) {
	names := map[string]string{"alpha": "Alpha", "beta": "Beta", "gamma": "Gamma"}
	svg := RenderStationArrows(sampleReport(), names)

	if !strings.Contains(svg, "<svg") {
		t.Fatalf("output is not an SVG:\n%s", svg)
	}
	// Exactly two arrows: the two station destinations (gamma excluded).
	if n := strings.Count(svg, "jump-arrow"); n != 2 {
		t.Errorf("got %d arrows, want 2", n)
	}
	if !strings.Contains(svg, "direct") {
		t.Errorf("missing 'direct' class for reachable station")
	}
	if !strings.Contains(svg, "blocked") {
		t.Errorf("missing 'blocked' class for interrupted station")
	}
	// Station labels with headings present; non-station excluded.
	if !strings.Contains(svg, "Alpha") || !strings.Contains(svg, "0.0") {
		t.Errorf("missing Alpha label / heading")
	}
	if !strings.Contains(svg, "Beta") || !strings.Contains(svg, "90.0") {
		t.Errorf("missing Beta label / heading")
	}
	if strings.Contains(svg, "Gamma") {
		t.Errorf("non-station Gamma should not appear")
	}
}
