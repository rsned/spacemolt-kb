package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

func fixtureGlyphs() []renderedGlyph {
	return []renderedGlyph{
		{Stats: shipglyph.Stats{ID: "crowbar", Name: "Crowbar", Class: "Salvager", Faction: "crimson"},
			SVG: `<svg class="ship-glyph"><title>Crowbar</title></svg>`},
		{Stats: shipglyph.Stats{ID: "comet", Name: "Comet", Class: "Liner", Faction: "nebula"},
			SVG: `<svg class="ship-glyph"><title>Comet</title></svg>`},
		{Stats: shipglyph.Stats{ID: "war_wagon", Name: "War Wagon", Class: "Bulk Hauler", Faction: "crimson"},
			SVG: `<svg class="ship-glyph"><title>War Wagon</title></svg>`},
		{Stats: shipglyph.Stats{ID: "nofaction", Name: "No Faction", Class: "Cruiser", Faction: ""},
			SVG: `<svg class="ship-glyph"><title>No Faction</title></svg>`},
	}
}

func TestGroupByFactionIsSortedAndComplete(t *testing.T) {
	groups := groupByFaction(fixtureGlyphs())

	var total int
	for _, g := range groups {
		total += len(g.Glyphs)
		if g.Name == "" {
			t.Errorf("group has an empty display name")
		}
	}
	if total != 4 {
		t.Errorf("grouped %d glyphs, want 4", total)
	}

	// Deterministic ordering: same input must yield the same group order.
	again := groupByFaction(fixtureGlyphs())
	for i := range groups {
		if groups[i].Name != again[i].Name {
			t.Fatalf("group order is not deterministic at %d: %q vs %q",
				i, groups[i].Name, again[i].Name)
		}
	}
}

func TestWriteContactSheetProducesPage(t *testing.T) {
	dir := t.TempDir()
	if err := writeContactSheet(dir, fixtureGlyphs()); err != nil {
		t.Fatalf("writeContactSheet: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	page := string(data)

	for _, want := range []string{
		"<!DOCTYPE html>",
		`href="../../smui.css"`,
		`href="glyphs.css"`,
		`class="theme-toggle"`,
		"Crowbar",
		"War Wagon",
		`class="ship-glyph"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

func TestWriteContactSheetEscapesNames(t *testing.T) {
	dir := t.TempDir()
	glyphs := []renderedGlyph{{
		Stats: shipglyph.Stats{
			ID:      "x",
			Name:    `Ship <script>alert(1)</script>`,
			Class:   `Cruiser <img src=x onerror=alert(2)>`,
			Faction: "crimson",
		},
		SVG: `<svg class="ship-glyph"></svg>`,
	}}
	if err := writeContactSheet(dir, glyphs); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Contains(string(data), "<script>alert(1)</script>") {
		t.Errorf("ship name was not HTML-escaped")
	}
	if strings.Contains(string(data), "<img src=x") {
		t.Errorf("ship class was not HTML-escaped")
	}
}

// Discontinued hulls are still flown, so they belong on the sheet -- but a
// reader must be able to tell them apart from what is still for sale.
func TestContactSheetMarksLegacyGlyphs(t *testing.T) {
	dir := t.TempDir()
	glyphs := append(fixtureGlyphs(), renderedGlyph{
		Stats:  shipglyph.Stats{ID: "excavator", Name: "Excavator", Class: "Mining", Faction: ""},
		Legacy: true,
		SVG:    `<svg class="ship-glyph"><title>Excavator</title></svg>`,
	})
	if err := writeContactSheet(dir, glyphs); err != nil {
		t.Fatalf("writeContactSheet: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	if !strings.Contains(page, "Excavator") {
		t.Error("legacy ship is missing from the contact sheet entirely")
	}
	if !strings.Contains(page, "glyph-legacy") {
		t.Error("legacy ship carries no marker class, so it reads as still for sale")
	}
	// The marker must be on the legacy cell only, not every cell.
	if n := strings.Count(page, "glyph-legacy"); n != 1 {
		t.Errorf("glyph-legacy appears %d times, want 1 (only the legacy cell)", n)
	}
}
