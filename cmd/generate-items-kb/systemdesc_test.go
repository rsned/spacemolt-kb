package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMapSystemDescriptions(t *testing.T) {
	dir := t.TempDir()
	js := `{"systems":[{"system_id":"sol","description":"  Home.  "},{"system_id":"void","description":""}],"total_count":2}`
	if err := os.WriteFile(filepath.Join(dir, "get_map.json"), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mapSystemDescriptions(dir)
	if len(got) != 1 || got["sol"] != "Home." {
		t.Fatalf("descriptions %q, want only sol=Home.", got)
	}
	if mapSystemDescriptions(t.TempDir()) != nil {
		t.Fatal("missing get_map.json should yield nil")
	}
}

// A system agents have visited but the map scrape leaves out is uncharted
// (v0.687: uncharted frontier systems are excluded from maps and search).
func TestMarkUncharted(t *testing.T) {
	a, b := &System{ID: "sol"}, &System{ID: "beyond"}
	markUncharted([]*System{a, b}, map[string]bool{"sol": true})
	if a.Uncharted || !b.Uncharted {
		t.Fatalf("sol=%v beyond=%v", a.Uncharted, b.Uncharted)
	}
	// With no map scrape at all, nothing can be judged uncharted.
	c := &System{ID: "beyond"}
	markUncharted([]*System{c}, nil)
	if c.Uncharted {
		t.Fatal("marked uncharted without a map")
	}
}

// Uncharted systems are gathered into a Frontier group listed after the empires.
func TestFrontierGroup(t *testing.T) {
	sys := []*System{{ID: "sol", Name: "Sol"}, {ID: "bristle", Name: "Bristle", Uncharted: true},
		{ID: "ash_pearl", Name: "Ash Pearl", Uncharted: true}}
	g, ok := frontierGroup(sys)
	if !ok || g.Name != "Frontier" || g.ID != "frontier" || len(g.Systems) != 2 || g.Systems[0].ID != "ash_pearl" {
		t.Fatalf("group = %+v ok=%v", g, ok)
	}
	if _, ok := frontierGroup(sys[:1]); ok {
		t.Fatal("no uncharted systems should mean no Frontier group")
	}
}

// An uncharted system at (0,0) takes its estimated position, flagged
// approximate; with no estimate it is flagged as having no position. Charted
// systems (Sol really is at the origin) are untouched.
func TestApplyFrontierPositions(t *testing.T) {
	sol := &System{ID: "sol"}
	ln := &System{ID: "long_night", Uncharted: true}
	cl := &System{ID: "cold_lens", Uncharted: true}
	applyFrontierPositions([]*System{sol, ln, cl}, map[string]frontierEstimate{
		"long_night": {X: -6334, Y: 4615}, "sol": {X: 9, Y: 9},
	})
	if sol.PositionX != 0 || sol.PositionApprox || sol.PositionUnknown {
		t.Errorf("sol = %+v", sol)
	}
	if ln.PositionX != -6334 || ln.PositionY != 4615 || !ln.PositionApprox || ln.PositionUnknown {
		t.Errorf("long_night = %+v", ln)
	}
	if !cl.PositionUnknown || cl.PositionApprox {
		t.Errorf("cold_lens = %+v", cl)
	}

	// After the estimates are backfilled into the database the position is
	// no longer (0,0), but it is still an estimate: keep the marker.
	bf := &System{ID: "bristle", Uncharted: true, PositionX: -5054, PositionY: 5493}
	applyFrontierPositions([]*System{bf}, map[string]frontierEstimate{"bristle": {X: -5054, Y: 5493}})
	if !bf.PositionApprox || bf.PositionUnknown {
		t.Errorf("backfilled bristle = %+v, want approximate", bf)
	}
}

func TestLoadFrontierEstimates(t *testing.T) {
	est := loadFrontierEstimates("../../data/frontier_positions.json")
	if e, ok := est["bristle"]; !ok || e.X != -5054 || e.Y != 5493 {
		t.Fatalf("bristle = %+v (ok=%v)", e, ok)
	}
	if loadFrontierEstimates("/nonexistent.json") != nil {
		t.Error("missing file should give nil")
	}
}

// Uncharted systems stay out of star records (persisted to the database):
// their positions are estimates at best.
func TestUnchartedSkippedByStars(t *testing.T) {
	sys := []*System{{ID: "sol", Name: "Sol"}, {ID: "bristle", Name: "Bristle", Uncharted: true, PositionApprox: true}}
	if recs := starRecords(sys); len(recs) != 1 {
		t.Errorf("star records = %d", len(recs))
	}
}
