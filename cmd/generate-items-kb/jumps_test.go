package main

import (
	"bytes"
	"fmt"
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

	reports, names, hsys := buildJumpReports(systems)
	if _, ok := reports["sol"]; !ok {
		t.Fatalf("no report for sol")
	}
	data := buildJumpPageData(systems[0], reports, names, hsys)

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
	reports, names, hsys := buildJumpReports(systems)
	data := buildJumpPageData(systems[0], reports, names, hsys)

	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()

	// All three tables (Direct + Stations + Heading Sweep) are sortable.
	if n := strings.Count(out, `table class="sortable"`); n != 3 {
		t.Errorf("got %d sortable tables, want 3", n)
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

func TestJumpPageHeadingSweep(t *testing.T) {
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0,
			POIs: []SystemPOI{{ID: "p1", Name: "Dock", Type: "station"}}},
		{ID: "alpha", Name: "Alpha", PositionX: 300, PositionY: 0,
			POIs: []SystemPOI{{ID: "p2", Name: "Port", Type: "station"}}},
		{ID: "beta", Name: "Beta", PositionX: 150, PositionY: 0},
	}
	reports, names, hsys := buildJumpReports(systems)
	data := buildJumpPageData(systems[0], reports, names, hsys)

	if len(data.Sweep) == 0 {
		t.Fatalf("expected heading sweep ranges")
	}
	// Sweep covers all 360 whole-degree headings exactly once.
	total := 0
	for _, r := range data.Sweep {
		total += r.Width
	}
	if total != 360 {
		t.Errorf("sweep covers %d degrees, want 360", total)
	}
	// Beta (closest on the +X line) is a landing target; names are resolved.
	foundBeta := false
	for _, r := range data.Sweep {
		if r.LandsAt == "Beta" {
			foundBeta = true
		}
	}
	if !foundBeta {
		t.Errorf("expected Beta as a landing target in the sweep")
	}

	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	if !strings.Contains(buf.String(), "Heading Sweep") {
		t.Errorf("jump page missing Heading Sweep section")
	}
}

func TestFigcaptionUsesCappedCoverage(t *testing.T) {
	// A system that is 99.987% blocked with one gap must not read 100.0%.
	data := JumpPageData{
		System:          &System{Name: "Alpha Centauri"},
		Coverage:        99.98689,
		CoverageDisplay: 99.9,
		GapCount:        1,
	}
	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "100.0% of headings blocked") {
		t.Errorf("figcaption must not read 100.0%% blocked while a gap exists")
	}
	if !strings.Contains(out, "99.9% of headings blocked") {
		t.Errorf("figcaption should show capped 99.9%% blocked")
	}
}

func TestToleranceDecimals(t *testing.T) {
	// Decimal places = position of the tolerance's first significant digit.
	cases := map[float64]int{
		0.00030: 4, // 0.0003
		0.0012:  3,
		0.012:   2,
		0.12:    1,
		1.5:     0,
		21.012:  0,
	}
	for tol, want := range cases {
		if got := toleranceDecimals(tol); got != want {
			t.Errorf("toleranceDecimals(%g) = %d, want %d", tol, got, want)
		}
	}
}

func TestJumpPageAdaptiveHeadingText(t *testing.T) {
	// A tight tolerance quotes the heading to matching precision; a loose one
	// quotes whole degrees. Margins never drop below three decimals.
	data := JumpPageData{
		System: &System{Name: "Test"},
		Direct: []JumpRow{
			{ID: "a", Name: "Tight", Bearing: 191.05112, Distance: 2994, Ticks: 300, Duration: "50:00", Margin: 0.0003, BearingText: "191.0511", MarginText: "0.0003", Reachable: true},
			{ID: "b", Name: "Loose", Bearing: 47.6, Distance: 100, Ticks: 10, Duration: "1:40", Margin: 21.0, BearingText: "48", MarginText: "21.000", Reachable: true},
		},
	}
	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()
	for _, want := range []string{">191.0511°<", ">0.0003°<", ">48°<", ">21.000°<"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing adaptive cell %q", want)
		}
	}
}

func TestBuildJumpPageDataAdaptiveText(t *testing.T) {
	// BearingText/MarginText are populated for direct rows, heading precision
	// derived from the row's tolerance.
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0},
		{ID: "beta", Name: "Beta", PositionX: 150, PositionY: 0,
			POIs: []SystemPOI{{ID: "p", Name: "Dock", Type: "station"}}},
	}
	reports, names, hsys := buildJumpReports(systems)
	data := buildJumpPageData(systems[0], reports, names, hsys)
	if len(data.Direct) == 0 {
		t.Fatal("expected a direct row")
	}
	for _, row := range data.Direct {
		dp := toleranceDecimals(row.Margin)
		want := fmt.Sprintf("%.*f", dp, row.Bearing)
		if row.BearingText != want {
			t.Errorf("BearingText = %q, want %q (margin %g -> %d dp)", row.BearingText, want, row.Margin, dp)
		}
		if row.MarginText == "" {
			t.Errorf("MarginText not populated for direct row")
		}
	}
}

func TestJumpPageHasThemeToggleScript(t *testing.T) {
	// The header carries a #theme-toggle button; the page must also include the
	// script that wires it up, or the light/dark toggle does nothing.
	data := JumpPageData{System: &System{Name: "Test"}}
	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `id="theme-toggle"`) {
		t.Fatalf("jump page missing the theme-toggle button")
	}
	if !strings.Contains(out, "getElementById('theme-toggle')") {
		t.Errorf("jump page missing the theme-toggle wiring script")
	}
}

func TestTicksDuration(t *testing.T) {
	cases := map[int]string{0: "0:00", 1: "0:10", 6: "1:00", 15: "2:30", 349: "58:10"}
	for ticks, want := range cases {
		if got := ticksDuration(ticks); got != want {
			t.Errorf("ticksDuration(%d) = %q, want %q", ticks, got, want)
		}
	}
}

func TestSweepSingleDegreeRangeFormatting(t *testing.T) {
	data := JumpPageData{
		System: &System{Name: "Test"},
		Sweep: []SweepRow{
			{StartDeg: 0, EndDeg: 0, Width: 1, LandsAt: "Ross 248", LandsAtID: "ross_248", Distance: 3482, Ticks: 349},
			{StartDeg: 6, EndDeg: 9, Width: 4, LandsAt: "Wolf", LandsAtID: "wolf", Distance: 800, Ticks: 80},
		},
	}
	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "0°–0°") {
		t.Errorf("single-degree range should render as '0°', not '0°–0°'")
	}
	if !strings.Contains(out, ">0°<") {
		t.Errorf("expected single-degree range rendered as '0°'")
	}
	if !strings.Contains(out, "6°–9°") {
		t.Errorf("expected multi-degree range rendered as '6°–9°'")
	}
}

func TestJumpPageTravelColumn(t *testing.T) {
	systems := []*System{
		{ID: "sol", Name: "Sol", PositionX: 0, PositionY: 0},
		{ID: "beta", Name: "Beta", PositionX: 150, PositionY: 0,
			POIs: []SystemPOI{{ID: "p", Name: "Dock", Type: "station"}}},
	}
	reports, names, hsys := buildJumpReports(systems)
	data := buildJumpPageData(systems[0], reports, names, hsys)

	// beta at 150 GU -> ceil(150/10) = 15 ticks.
	if len(data.Direct) == 0 || data.Direct[0].Ticks != 15 {
		t.Fatalf("Direct[0].Ticks = %v, want 15 (%+v)", data.Direct, data.Direct)
	}
	if len(data.Stations) == 0 || data.Stations[0].Ticks != 15 {
		t.Errorf("Stations[0].Ticks = %v, want 15", data.Stations)
	}

	tmpl := htmltpl.Must(htmltpl.New("jump").Parse(jumpDetailTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute: %v", err)
	}
	// A 'Travel (ticks)' header on all three jump-page tables.
	if n := strings.Count(buf.String(), ">Travel (ticks)<"); n != 3 {
		t.Errorf("got %d 'Travel (ticks)' headers, want 3", n)
	}
	// Value shows ticks plus mm:ss: 15 ticks -> 150s -> 2:30.
	if !strings.Contains(buf.String(), "15 (2:30)") {
		t.Errorf("missing 'ticks (mm:ss)' value in rows")
	}
}
