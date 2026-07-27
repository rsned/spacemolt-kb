package galaxymap

import (
	"strings"
	"testing"
)

func sampleSystems() ([]*System, map[string]*System) {
	a := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
		Empire: "solarian", LastUpdatedTick: 100,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	b := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 100,
		Empire: "nebula", LastUpdatedTick: 100,
		Connections: []Connection{{SystemID: "sol", Distance: 10}},
	}
	return []*System{a, b}, map[string]*System{"sol": a, "vega": b}
}

func TestRenderFullVariantHasBlobsAndConnections(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		ShowEmpireBlobs: true,
		ShowConnections: true,
	})

	if !strings.Contains(svg, "<svg") {
		t.Fatalf("output is not an SVG:\n%s", svg)
	}
	if !strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("missing metaball blob filter")
	}
	if !strings.Contains(svg, "<line") {
		t.Errorf("missing connection lines")
	}
}

func TestRenderDotsOnlyVariantOmitsBlobsAndConnections(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		ShowEmpireBlobs: false,
		ShowConnections: false,
	})

	if strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("blob filter present with ShowEmpireBlobs=false")
	}
	if strings.Contains(svg, "goo-galaxy") {
		t.Errorf("blob filter id present with ShowEmpireBlobs=false")
	}
	if strings.Contains(svg, "<line") {
		t.Errorf("connection lines present with ShowConnections=false")
	}
	// Dots survive.
	if n := strings.Count(svg, "galaxy-sys-dot"); n != 2 {
		t.Errorf("got %d dots, want 2", n)
	}
}

func TestRenderHighlightClassesAppliedPerSystem(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{
		HighlightClasses: func(id string) []string {
			if id == "sol" {
				return []string{"r-iron-ore", "r-copper-ore"}
			}
			return nil
		},
	})

	if !strings.Contains(svg, `class="galaxy-sys-dot r-iron-ore r-copper-ore"`) {
		t.Errorf("sol missing highlight classes:\n%s", svg)
	}
	// vega got none, so it keeps the bare class.
	if !strings.Contains(svg, `class="galaxy-sys-dot"`) {
		t.Errorf("vega should have the bare dot class")
	}
}

func TestRenderNilHighlightClassesIsSafe(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{HighlightClasses: nil})

	if !strings.Contains(svg, `class="galaxy-sys-dot"`) {
		t.Errorf("nil HighlightClasses should still emit the base class")
	}
}

func TestRenderLinkPrefixHonored(t *testing.T) {
	explored, m := sampleSystems()
	svg := Render(explored, nil, m, Options{LinkPrefix: "../"})

	if !strings.Contains(svg, `href="../systems/sol/"`) {
		t.Errorf("LinkPrefix not applied:\n%s", svg)
	}
}

func TestRenderEmptyExploredReturnsPlaceholder(t *testing.T) {
	svg := Render(nil, nil, map[string]*System{}, Options{})

	if strings.Contains(svg, "<svg") {
		t.Errorf("expected placeholder text, got an SVG")
	}
	if !strings.Contains(svg, "No explored systems") {
		t.Errorf("expected the no-systems placeholder, got: %s", svg)
	}
}

// reachSample returns a three-system chain sol(0) - vega(1) - rigel(2).
func reachSample() ([]*System, map[string]*System) {
	a := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0, IsStronghold: true,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	b := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 0,
		Connections: []Connection{{SystemID: "sol", Distance: 10}, {SystemID: "rigel", Distance: 10}},
	}
	c := &System{
		ID: "rigel", Name: "Rigel", PositionX: 200, PositionY: 0,
		Connections: []Connection{{SystemID: "vega", Distance: 10}},
	}
	return []*System{a, b, c}, map[string]*System{"sol": a, "vega": b, "rigel": c}
}

func reachRadius(m map[string]int) func(string) int {
	return func(id string) int {
		if r, ok := m[id]; ok {
			return r
		}
		return -1
	}
}

func TestReachBlobEmitsActivationClassPerSystem(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
			Color:  "#e53e3e",
		},
	})

	if !strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("ReachBlob should emit the metaball filter")
	}
	for _, want := range []string{`class="rb-0"`, `class="rb-1"`, `class="rb-2"`} {
		if !strings.Contains(svg, want) {
			t.Errorf("missing %s in:\n%s", want, svg)
		}
	}
	if !strings.Contains(svg, "#e53e3e") {
		t.Errorf("blob fill color not applied")
	}
}

func TestReachBlobEdgeUsesMaxOfEndpoints(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
		},
	})

	// sol(0)-vega(1) activates at 1; vega(1)-rigel(2) activates at 2.
	// Circles contribute rb-0, rb-1, rb-2 once each, so each edge class
	// must appear exactly one time beyond its circle.
	if got := strings.Count(svg, `class="rb-1"`); got != 2 {
		t.Errorf("rb-1 count = %d, want 2 (one circle + one edge)", got)
	}
	if got := strings.Count(svg, `class="rb-2"`); got != 2 {
		t.Errorf("rb-2 count = %d, want 2 (one circle + one edge)", got)
	}
	if got := strings.Count(svg, `class="rb-0"`); got != 1 {
		t.Errorf("rb-0 count = %d, want 1 (circle only)", got)
	}
}

func TestReachBlobOmitsBeyondMax(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    1,
		},
	})

	if strings.Contains(svg, `class="rb-2"`) {
		t.Errorf("radius above Max should emit no blob geometry")
	}
	if !strings.Contains(svg, `class="rb-1"`) {
		t.Errorf("radius at Max should still be drawn")
	}
}

func TestReachBlobOmitsUnreachableSystems(t *testing.T) {
	explored, m := reachSample()
	// rigel is never in reach.
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1}),
			Max:    5,
		},
	})

	// Two circles (rb-0, rb-1) and one edge (rb-1). The vega-rigel edge
	// must be dropped because rigel is unreachable.
	if got := strings.Count(svg, `class="rb-`); got != 3 {
		t.Errorf("blob element count = %d, want 3", got)
	}
}

func TestReachBlobDoesNotRecolorSystemDots(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
			Color:  "#e53e3e",
		},
	})

	// vega has no empire and is not a stronghold, so its dot keeps the
	// grey default rather than picking up the blob color.
	if !strings.Contains(svg, `fill="#E8E8E8"`) {
		t.Errorf("system dots should keep the grey default fill")
	}
}

func TestRenderCapitalDotClosesAnchorExactlyOnce(t *testing.T) {
	sol := &System{
		ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
		Empire: "solarian", LastUpdatedTick: 100,
	}
	other := &System{
		ID: "vega", Name: "Vega", PositionX: 100, PositionY: 100,
		Empire: "nebula", LastUpdatedTick: 100,
	}
	explored := []*System{sol, other}
	m := map[string]*System{"sol": sol, "vega": other}

	svg := Render(explored, nil, m, Options{})

	if strings.Contains(svg, "</a></a>") {
		t.Errorf("doubled anchor close in output:\n%s", svg)
	}
	// One anchor open and one close per system.
	if open, close := strings.Count(svg, "<a href="), strings.Count(svg, "</a>"); open != close {
		t.Errorf("unbalanced anchors: %d open, %d close", open, close)
	}
}

func TestGroupBlobsDrawOnePerGroupWithOwnColor(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		GroupBlobs: &GroupBlobs{
			Group: func(id string) string {
				switch id {
				case "sol", "vega":
					return "alpha"
				default:
					return ""
				}
			},
			Groups: []GroupBlob{{Key: "alpha", Color: "#00CED1"}},
		},
	})

	if !strings.Contains(svg, `class="gb-alpha"`) {
		t.Errorf("missing group blob element in:\n%s", svg)
	}
	if !strings.Contains(svg, "#00CED1") {
		t.Errorf("group color not applied")
	}
	if !strings.Contains(svg, "feGaussianBlur") {
		t.Errorf("GroupBlobs alone should still emit the metaball filter")
	}
}

func TestGroupBlobsOnlyLinkSameGroupNeighbors(t *testing.T) {
	// sol-vega share a group; vega-rigel do not, so only one blob edge.
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		GroupBlobs: &GroupBlobs{
			Group: func(id string) string {
				if id == "sol" || id == "vega" {
					return "alpha"
				}
				return ""
			},
			Groups: []GroupBlob{{Key: "alpha", Color: "#00CED1"}},
		},
	})

	// Two circles (sol, vega) plus one edge (sol-vega) = 3 elements.
	if got := strings.Count(svg, "#00CED1"); got != 3 {
		t.Errorf("group element count = %d, want 3 (2 circles + 1 edge)", got)
	}
}

func TestGroupBlobsDrawnBeneathReachBlob(t *testing.T) {
	explored, m := reachSample()
	svg := Render(explored, nil, m, Options{
		GroupBlobs: &GroupBlobs{
			Group:  func(string) string { return "alpha" },
			Groups: []GroupBlob{{Key: "alpha", Color: "#00CED1"}},
		},
		ReachBlob: &ReachBlob{
			Radius: reachRadius(map[string]int{"sol": 0, "vega": 1, "rigel": 2}),
			Max:    2,
			Color:  "#E8E8E8",
		},
	})

	// SVG paints in document order, so the group blobs must come first
	// for the reach wash to read as layered over empire territory.
	gb := strings.Index(svg, `class="gb-alpha"`)
	rb := strings.Index(svg, `class="rb-`)
	if gb < 0 || rb < 0 {
		t.Fatalf("expected both layers; gb=%d rb=%d", gb, rb)
	}
	if gb > rb {
		t.Errorf("group blobs must precede the reach blob (gb=%d, rb=%d)", gb, rb)
	}
}

func TestGroupBlobsNilLeavesOutputUnchanged(t *testing.T) {
	explored, m := reachSample()
	with := Render(explored, nil, m, Options{ShowEmpireBlobs: true, GroupBlobs: nil})
	without := Render(explored, nil, m, Options{ShowEmpireBlobs: true})

	if with != without {
		t.Errorf("nil GroupBlobs must not change output")
	}
}
