package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPage(t *testing.T) {
	cat, cal := loadForTest(t)
	// Attackers = all 5 starters (matches production's columns, and what
	// buildHighEndData needs present to read m.cell(opus_magna, starterID)
	// for every starter); defenders = the 5 starters plus opus_magna, so
	// both buildOpusView and the Tier-0 rows in buildLowEndView have real
	// data to work with.
	defenders := append(append([]string{}, starterColumnIDs()...), opusMagnaID)
	m := BuildMatrix(cat, cal, starterColumnIDs(), defenders, 25000, 20, 4000)
	// High-End/Multi-Opus resolve straight from cat (see buildHighEndData,
	// buildMultiOpusData), independent of m's defender subset — small
	// runs/maxTicks here just keep the test fast, not because the subset
	// constrains them.
	fitPath := filepath.Join("..", "..", "data", "combat-sim", "fits", "high_end_opus_drone.json")
	highEnd := buildHighEndData(cat, cal, m, fitPath, 15, 4000, 2000)
	multiOpus := buildMultiOpusData(cat, cal, 15, 2000)
	html, err := RenderPage(m, highEnd, multiOpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<title", "smui.css", "Opus Magna", "no capital weapon bonus", "id=\"matrix\"",
		"High-End Setup", "Combat Drone", "damage reduction",
		"Multi-Opus Effect", "titans", "Dogpile", "Spread",
		"how easily most hulls fall", "id=\"ls-low-end\"",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	// The interactive data must be embedded for client-side sort/filter.
	if !strings.Contains(html, "MATRIX_DATA") {
		t.Fatal("embedded matrix JSON missing")
	}
}

// TestRenderPageNoHighEndOrMultiOpus asserts the page still renders (without
// crashing, and without the two callouts) when highEnd/multiOpus are nil —
// e.g. buildHighEndData/buildMultiOpusData returning nil for a hull that
// failed to resolve, or the fit file being unavailable.
func TestRenderPageNoHighEndOrMultiOpus(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "opus_magna"}, starterColumnIDs(), 25000, 20, 4000)
	html, err := RenderPage(m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"id=\"ls-high-end\"", "id=\"ls-multi-opus\""} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("page has %q with nil highEnd/multiOpus data", unwanted)
		}
	}
	if !strings.Contains(html, "id=\"ls-low-end\"") {
		t.Fatal("page missing low-end callout, which doesn't depend on highEnd/multiOpus")
	}
}

// TestLowEndTier0MirrorAxesAligned asserts the Tier-0 mirror table's row
// axis (Tier0Rows) and column axis (Tier0Cols) both follow
// tier0MirrorOrder — cobble, prospect, shard, theoria, threshold — so the
// self-vs-self diagonal lines up. This is independent of the main matrix
// table's own column order (starterColumnIDs' shard/prospect/cobble/
// theoria/threshold), which buildColViews still preserves unchanged.
func TestLowEndTier0MirrorAxesAligned(t *testing.T) {
	cat, cal := loadForTest(t)
	defenders := append(append([]string{}, starterColumnIDs()...), opusMagnaID)
	m := BuildMatrix(cat, cal, starterColumnIDs(), defenders, 25000, 20, 4000)
	cols := buildColViews(m.Columns)

	// The main table's column order is untouched by this change.
	wantMainOrder := []string{"shard", "prospect", "cobble", "theoria", "threshold"}
	for i, c := range cols {
		if c.ID != wantMainOrder[i] {
			t.Fatalf("main table column %d = %q, want %q (unchanged order)", i, c.ID, wantMainOrder[i])
		}
	}

	lowEnd := buildLowEndView(m, cols)
	if lowEnd == nil {
		t.Fatal("buildLowEndView = nil")
	}
	if len(lowEnd.Tier0Cols) != len(tier0MirrorOrder) {
		t.Fatalf("Tier0Cols = %d entries, want %d", len(lowEnd.Tier0Cols), len(tier0MirrorOrder))
	}
	for i, c := range lowEnd.Tier0Cols {
		if c.ID != tier0MirrorOrder[i] {
			t.Errorf("Tier0Cols[%d].ID = %q, want %q", i, c.ID, tier0MirrorOrder[i])
		}
	}
	if len(lowEnd.Tier0Rows) != len(tier0MirrorOrder) {
		t.Fatalf("Tier0Rows = %d entries, want %d", len(lowEnd.Tier0Rows), len(tier0MirrorOrder))
	}
	for i, r := range lowEnd.Tier0Rows {
		wantName := capitalize(tier0MirrorOrder[i]) // hull display names are the capitalized ids here
		if r.Name != wantName {
			t.Errorf("Tier0Rows[%d].Name = %q, want %q", i, r.Name, wantName)
		}
		if len(r.Cells) != len(tier0MirrorOrder) {
			t.Errorf("Tier0Rows[%d].Cells = %d entries, want %d", i, len(r.Cells), len(tier0MirrorOrder))
		}
	}
}

// TestBuildHighEndDataMissingOpusRow asserts the actual guard buildHighEndData
// documents: given a matrix subset whose defenders don't include opus_magna
// (so it has no row to source the stock crossover from), it returns nil
// rather than crashing or reading a zero-value stock N.
func TestBuildHighEndDataMissingOpusRow(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "opus_magna"}, starterColumnIDs(), 25000, 20, 4000)
	fitPath := filepath.Join("..", "..", "data", "combat-sim", "fits", "high_end_opus_drone.json")
	if got := buildHighEndData(cat, cal, m, fitPath, 15, 4000, 2000); got != nil {
		t.Fatalf("buildHighEndData = %+v, want nil (m has no opus_magna row)", got)
	}
}
