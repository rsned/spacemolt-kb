package wildlife

import (
	"database/sql"
	"testing"
)

// openStagesTestDB builds on openTestDB's fixture (species belt_grazer,
// rainbow_leviathan, ghost) by adding wildlife_observations rows covering:
// a species with no observations at all (belt_grazer), a species with
// multiple life stages (rainbow_leviathan: juvenile + unstaged "adult"), a
// creature observed more than once so only its latest sighting should count
// (c2), and a species with a single stage whose creatures still differ in
// size (ghost).
func openStagesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	stmts := []string{
		`CREATE TABLE wildlife_observations (
			creature_id TEXT, game_tick INTEGER, observed_utc TEXT, species TEXT,
			name TEXT, stage TEXT, role TEXT, hull INTEGER, max_hull INTEGER,
			in_combat INTEGER, branded INTEGER, system_id TEXT, poi_id TEXT, agent_id TEXT)`,
		`INSERT INTO wildlife_observations VALUES
		 ('c1',10,'2026-10-14T00:00:00Z','rainbow_leviathan','','','predator',900,900,0,0,'sol','','bot'),
		 ('c2',10,'2026-10-14T01:00:00Z','rainbow_leviathan','','','predator',700,700,0,0,'sol','','bot'),
		 ('c2',20,'2026-10-15T00:00:00Z','rainbow_leviathan','','','predator',950,950,0,0,'sol','','bot'),
		 ('c3',10,'2026-10-16T00:00:00Z','rainbow_leviathan','','juvenile','predator',300,300,0,0,'sol','','bot'),
		 ('c4',10,'2026-10-16T00:00:00Z','rainbow_leviathan','','juvenile','predator',350,360,0,0,'sol','','bot'),
		 ('g1',5,'2026-10-14T02:00:00Z','ghost','','','scavenger',10,10,0,0,'sol','','bot'),
		 ('g2',5,'2026-10-14T03:00:00Z','ghost','','','scavenger',10,14,0,0,'sol','','bot'),
		 ('zzz',5,'2026-10-14T00:00:00Z','unknown_species','','','predator',10,10,0,0,'sol','','bot')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func TestLoadStages(t *testing.T) {
	g, err := Load(openStagesTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	index := make(map[string]*Species, len(g.Species))
	for i := range g.Species {
		index[g.Species[i].ID] = &g.Species[i]
	}

	rl := index["rainbow_leviathan"]
	if len(rl.Stages) != 2 {
		t.Fatalf("rainbow_leviathan stages = %+v", rl.Stages)
	}
	// Ordered by median ascending: juvenile (330) before adult (925).
	juv, adult := rl.Stages[0], rl.Stages[1]
	if juv.Stage != "juvenile" || juv.Creatures != 2 || juv.Min != 300 || juv.Max != 360 || juv.Median != 330 {
		t.Errorf("juvenile = %+v", juv)
	}
	// c2's latest sighting (tick 20, max_hull 950) wins over its earlier one
	// (tick 10, max_hull 700): unstaged ('') is reported as "adult".
	if adult.Stage != "adult" || adult.Creatures != 2 || adult.Min != 900 || adult.Max != 950 || adult.Median != 925 {
		t.Errorf("adult = %+v", adult)
	}
	if !rl.HasStageVariation() {
		t.Error("rainbow_leviathan: expected stage variation (multiple stages)")
	}
	if lo, hi := rl.HullRange(); lo != 300 || hi != 950 {
		t.Errorf("HullRange = %d, %d", lo, hi)
	}
	if rl.StagesSince != "2026-10-14T00:00:00Z" {
		t.Errorf("StagesSince = %q", rl.StagesSince)
	}

	// Single stage, but its creatures differ in size: variation holds even
	// with only one stage.
	ghost := index["ghost"]
	if len(ghost.Stages) != 1 || ghost.Stages[0].Stage != "adult" || ghost.Stages[0].Min != 10 || ghost.Stages[0].Max != 14 {
		t.Fatalf("ghost stages = %+v", ghost.Stages)
	}
	if !ghost.HasStageVariation() {
		t.Error("ghost: expected stage variation (min != max within its one stage)")
	}

	// No observation rows at all: no stage data, no variation, HullRange
	// falls back to MaxHull.
	bg := index["belt_grazer"]
	if len(bg.Stages) != 0 || bg.HasStageVariation() {
		t.Errorf("belt_grazer: expected no stage data, got %+v", bg.Stages)
	}
	if lo, hi := bg.HullRange(); lo != bg.MaxHull || hi != bg.MaxHull {
		t.Errorf("belt_grazer HullRange = %d, %d, want MaxHull %d", lo, hi, bg.MaxHull)
	}
}

// TestLoadStages_NoTable covers a knowledge DB that predates the
// wildlife_observations ledger: Load must succeed and simply leave every
// species without stage data, matching how other optional tables are
// handled.
func TestLoadStages_NoTable(t *testing.T) {
	g, err := Load(openTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range g.Species {
		if len(s.Stages) != 0 || s.HasStageVariation() {
			t.Errorf("%s: expected no stage data without the table, got %+v", s.ID, s.Stages)
		}
	}
}
