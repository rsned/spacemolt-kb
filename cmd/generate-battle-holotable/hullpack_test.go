package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ftoa formats a float compactly for generated SVG test fixtures. It exists
// solely for tests, so it lives here rather than in hullpack.go.
func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// writeFootprint drops a minimal contract-compliant SVG into dir.
func writeFootprint(t *testing.T, dir, ship string, height float64) {
	t.Helper()
	svg := `<svg viewBox="0 0 1020 ` + ftoa(height) + `" data-ship="` + ship +
		`" data-aspect="` + ftoa(1000/(height-20)) + `" data-kb-match="verbatim">` +
		`<path d="M10 10L1010 10L1010 ` + ftoa(height-10) + `L10 ` + ftoa(height-10) + `Z"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, ship+".svg"), []byte(svg), 0o644); err != nil {
		t.Fatalf("write footprint: %v", err)
	}
}

func TestBuildHullPackResolvesArtStationsAndMisses(t *testing.T) {
	dir := t.TempDir()
	writeFootprint(t, dir, "dirk", 628)
	writeFootprint(t, dir, "vigil", 400)

	rep := Replay{
		BattleID: "test",
		Participants: []Participant{
			{PlayerID: "p1", ShipClass: "dirk", Kind: "player"},
			{PlayerID: "p2", ShipClass: "dirk", Kind: "player"},   // duplicate class
			{PlayerID: "p3", ShipClass: "vigil", Kind: "pirate"},
			{PlayerID: "p4", ShipClass: "anamnesis", Kind: "player"}, // no art
			{PlayerID: "p5", ShipClass: "", Kind: "station"},          // the station
		},
	}
	scales := map[string]int{"dirk": 2, "vigil": 4, "anamnesis": 3}

	pack, problems, err := BuildHullPack(rep, dir, scales)
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}

	// One entry per distinct class, not per participant.
	if len(pack) != 4 {
		t.Fatalf("pack has %d entries, want 4 (dirk, vigil, anamnesis, station)", len(pack))
	}

	if len(problems) != 0 {
		t.Errorf("problems = %v, want none — the fixture footprints satisfy the asset contract", problems)
	}

	dirk, ok := pack["dirk"]
	if !ok {
		t.Fatal("dirk missing from pack")
	}
	if dirk.Kind != "hull" {
		t.Errorf("dirk Kind = %q, want hull", dirk.Kind)
	}
	if dirk.D == "" {
		t.Error("dirk has no path data")
	}
	if dirk.Height != 628 {
		t.Errorf("dirk Height = %v, want 628", dirk.Height)
	}
	if dirk.Scale != 2 {
		t.Errorf("dirk Scale = %d, want 2", dirk.Scale)
	}

	miss, ok := pack["anamnesis"]
	if !ok {
		t.Fatal("anamnesis missing from pack; a class with no art must still get an entry")
	}
	if miss.Kind != "missing" {
		t.Errorf("anamnesis Kind = %q, want missing", miss.Kind)
	}
	if miss.D != "" {
		t.Error("anamnesis has path data but has no art file")
	}
	if miss.Scale != 3 {
		t.Errorf("anamnesis Scale = %d, want 3 — scale comes from the catalog, not the art", miss.Scale)
	}

	station, ok := pack[""]
	if !ok {
		t.Fatal("the station's empty ship_class must still get an entry")
	}
	if station.Kind != "station" {
		t.Errorf("station Kind = %q, want station", station.Kind)
	}
}

func TestBuildHullPackSurfacesFootprintCheckProblems(t *testing.T) {
	dir := t.TempDir()
	// A viewBox width off the contract's 1020: footprint.Parse accepts it (it
	// doesn't validate), but the pack still needs to render it — silently
	// mis-centred, since holotable.js hardcodes FOOTPRINT_WIDTH — so Check
	// must be the thing that reports the drift.
	svg := `<svg viewBox="0 0 999 628" data-ship="crooked" data-aspect="1.6447">` +
		`<path d="M10 10L989 10L989 618L10 618Z"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "crooked.svg"), []byte(svg), 0o644); err != nil {
		t.Fatalf("write footprint: %v", err)
	}

	rep := Replay{Participants: []Participant{{ShipClass: "crooked", Kind: "player"}}}

	pack, problems, err := BuildHullPack(rep, dir, map[string]int{})
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}
	// Parse does not validate, so the hull still renders...
	if pack["crooked"].Kind != "hull" {
		t.Errorf("crooked Kind = %q, want hull — Parse must not refuse a contract-violating asset", pack["crooked"].Kind)
	}
	// ...but the violation must be reported, not silently absorbed.
	probs := problems["crooked"]
	if len(probs) == 0 {
		t.Fatal("problems[\"crooked\"] is empty, want the viewBox-width violation reported")
	}
	found := false
	for _, p := range probs {
		if strings.Contains(p, "viewBox width") {
			found = true
		}
	}
	if !found {
		t.Errorf("problems[\"crooked\"] = %v, want a viewBox width complaint", probs)
	}
}

func TestBuildHullPackDefaultsScaleToOneWhenTheCatalogIsSilent(t *testing.T) {
	dir := t.TempDir()
	writeFootprint(t, dir, "dirk", 628)

	rep := Replay{Participants: []Participant{{ShipClass: "dirk", Kind: "player"}}}

	pack, _, err := BuildHullPack(rep, dir, map[string]int{}) // catalog knows nothing
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}
	if got := pack["dirk"].Scale; got != 1 {
		t.Errorf("Scale = %d, want 1 — an unknown scale must not render a zero-size hull", got)
	}
}

func TestRenderPageWiresTheDataFilesAndRenderer(t *testing.T) {
	rep := Replay{
		BattleID:   "a2619bbe328676445828b4e1007fe9aa",
		SystemName: "Node Beta",
		TickCount:  30,
	}
	got, err := RenderPage(rep)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	page := string(got)

	for _, want := range []string{
		"a2619bbe328676445828b4e1007fe9aa.json",       // the replay
		"a2619bbe328676445828b4e1007fe9aa-hulls.json", // the hull pack
		"holotable.js",                                // the renderer
		"Node Beta",                                   // the heading
		"<canvas",                                     // something to draw on
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not mention %q", want)
		}
	}

	// The renderer is data-only: the page must not inline ship data.
	if strings.Contains(page, "data-ship") {
		t.Error("page inlines SVG attributes; hull data belongs in the hull pack")
	}
}

func TestRenderPageEscapesTheBattleID(t *testing.T) {
	rep := Replay{BattleID: `"><script>alert(1)</script>`, SystemName: "x"}
	got, err := RenderPage(rep)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	if strings.Contains(string(got), "<script>alert(1)</script>") {
		t.Error("battle id was interpolated unescaped")
	}
}
