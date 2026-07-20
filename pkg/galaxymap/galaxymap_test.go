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
