package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	pack, err := BuildHullPack(rep, dir, scales)
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}

	// One entry per distinct class, not per participant.
	if len(pack) != 4 {
		t.Fatalf("pack has %d entries, want 4 (dirk, vigil, anamnesis, station)", len(pack))
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

func TestBuildHullPackDefaultsScaleToOneWhenTheCatalogIsSilent(t *testing.T) {
	dir := t.TempDir()
	writeFootprint(t, dir, "dirk", 628)

	rep := Replay{Participants: []Participant{{ShipClass: "dirk", Kind: "player"}}}

	pack, err := BuildHullPack(rep, dir, map[string]int{}) // catalog knows nothing
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
