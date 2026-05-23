package main

import (
	"bytes"
	htmltpl "html/template"
	"strings"
	"testing"
)

func TestJumpPageRenders(t *testing.T) {
	// sol(0) - beta(150) - alpha(300) collinear on +X. beta sits between sol and
	// alpha, so sol->alpha is interrupted. sol and alpha have stations.
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
			POIs: []SystemPOI{{ID: "p1", Name: "Dock", Type: "station"}}},
		{ID: "alpha", Name: "Alpha", PositionX: 300, PositionY: 0,
			POIs: []SystemPOI{{ID: "p2", Name: "Port", Type: "station"}}},
		{ID: "beta", Name: "Beta", PositionX: 150, PositionY: 0},
	}

	reports, names := buildJumpReports(systems)
	if _, ok := reports["sol"]; !ok {
		t.Fatalf("no report for sol")
	}
	data := buildJumpPageData(systems[0], reports, names)

	// beta is a reachable direct (non-station) connection from sol.
	if len(data.Direct) == 0 {
		t.Errorf("expected at least one direct connection")
	}
	// alpha is a station destination, interrupted by beta.
	var alpha *JumpRow
	for i := range data.Stations {
		if data.Stations[i].ID == "alpha" {
			alpha = &data.Stations[i]
		}
	}
	if alpha == nil {
		t.Fatalf("alpha (station) missing from station destinations")
	}
	if alpha.Reachable {
		t.Errorf("sol->alpha should be interrupted (beta is between)")
	}

	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Pathfinder Jump Routes", "Alpha", "Beta", "<svg", "jumpmap-arrows", "jumpmap-wheel"} {
		if !strings.Contains(out, want) {
			t.Errorf("jump page missing %q", want)
		}
	}
}

func TestJumpPageSortableColumns(t *testing.T) {
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
			POIs: []SystemPOI{{ID: "p1", Name: "Dock", Type: "station"}}},
		{ID: "alpha", Name: "Alpha", PositionX: 300, PositionY: 0,
			POIs: []SystemPOI{{ID: "p2", Name: "Port", Type: "station"}}},
		{ID: "beta", Name: "Beta", PositionX: 150, PositionY: 0},
	}
	reports, names := buildJumpReports(systems)
	data := buildJumpPageData(systems[0], reports, names)

	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()

	// Both tables (Direct + Stations) are sortable.
	if n := strings.Count(out, `table class="sortable"`); n != 2 {
		t.Errorf("got %d sortable tables, want 2", n)
	}
	// System, Heading, and Distance headers are sortable.
	for _, want := range []string{
		`class="sortable">System`,
		`class="sortable" style="text-align:right">Heading`,
		`class="sortable" style="text-align:right">Distance`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing sortable header: %q", want)
		}
	}
	// Numeric cells carry data-sort for correct numeric ordering.
	if !strings.Contains(out, "data-sort=") {
		t.Errorf("missing data-sort attributes on numeric cells")
	}
	// The sort behavior script is included.
	if !strings.Contains(out, "table.sortable") {
		t.Errorf("missing sort script")
	}
}
