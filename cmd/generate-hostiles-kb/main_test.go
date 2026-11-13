package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// testDB is an in-memory database shaped like the game client's hostile tables.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		`CREATE TABLE systems (id TEXT, name TEXT)`,
		`CREATE TABLE hostile_types (hostile_type TEXT PRIMARY KEY, kind TEXT, class TEXT, name TEXT, max_hull INTEGER, max_shield INTEGER, first_seen_utc TEXT, last_seen_utc TEXT)`,
		`CREATE TABLE hostile_sightings (id INTEGER PRIMARY KEY, hostile_id TEXT, hostile_type TEXT, system_id TEXT, poi_id TEXT, hull INTEGER, shield INTEGER, in_battle INTEGER, game_tick INTEGER, observed_utc TEXT, agent_id TEXT)`,
		`CREATE TABLE hostile_attacks (hostile_type TEXT, battle_id TEXT, weapon_name TEXT, damage_type TEXT, shot_kind TEXT, shots INTEGER, hits INTEGER, damage_total REAL, damage_min REAL, damage_max REAL, observed_utc TEXT)`,
		`INSERT INTO systems VALUES ('gloaming','Gloaming')`,
		`INSERT INTO hostile_types VALUES ('repeater_pane','repeater','pane','Repeater Pane',300,200,'2026-11-12T22:14:12Z','2026-11-12T22:14:50Z')`,
		// Monolith: no type row yet (stats not captured), known from sightings and a battle.
		`INSERT INTO hostile_sightings (hostile_id,hostile_type,system_id,in_battle,game_tick,observed_utc) VALUES
			('m1','repeater_monolith','gloaming',1,100,'2026-11-12T22:14:12Z'),
			('m2','repeater_monolith','gloaming',1,100,'2026-11-12T22:14:12Z'),
			('m1','repeater_monolith','gloaming',0,101,'2026-11-12T22:14:50Z'),
			('p1','repeater_pane','gloaming',1,100,'2026-11-12T22:14:12Z')`,
		`INSERT INTO hostile_attacks VALUES
			('repeater_monolith','b1','Heavy Pulse Laser','energy','beam',32,24,4120,78,305,''),
			('repeater_spire','b1','Focused Beam II','energy','beam',12,8,160,17,23,'')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return db
}

func TestLoadTypes(t *testing.T) {
	types, err := loadTypes(testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, ty := range types {
		ids = append(ids, ty.ID)
	}
	// Pane has captured stats (500) so leads; then by hardest hit: Monolith 305, Spire 23.
	if got := strings.Join(ids, ","); got != "repeater_pane,repeater_monolith,repeater_spire" {
		t.Fatalf("order = %s", got)
	}
	m := types[1]
	if m.Name != "Repeater Monolith" || m.Units != 2 || m.Sightings != 3 || len(m.Systems) != 1 {
		t.Fatalf("monolith = %+v", m)
	}
	if s := m.Systems[0]; s.SystemName != "Gloaming" || s.MaxAtOnce != 2 || s.BattlePct() != 66 {
		t.Errorf("monolith in gloaming = %+v (battle %d%%)", s, s.BattlePct())
	}
	if m.Battles != 1 || m.HardestHit != 305 || m.Weapons[0].HitPct() != 75 {
		t.Errorf("monolith combat = battles %d hardest %v hit %d%%", m.Battles, m.HardestHit, m.Weapons[0].HitPct())
	}
	if types[2].FirstSeen != "" {
		t.Error("spire is known from a battle only")
	}
}

func TestWritePages(t *testing.T) {
	types, err := loadTypes(testDB(t))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := writePages(dir, types); err != nil {
		t.Fatal(err)
	}
	read := func(n string) string {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	f := read("frontier.html")
	for _, want := range []string{"Repeater Monolith", "Heavy Pulse Laser", "75%", "78–305", `href="../systems/gloaming/"`, `id="hz-type"`} {
		if !strings.Contains(f, want) {
			t.Errorf("frontier lacks %q", want)
		}
	}
	if !strings.Contains(read("pirates.html"), "No pirate data has been recorded yet") {
		t.Error("pirates page should say nothing is recorded")
	}
	if idx := read("index.html"); !strings.Contains(idx, "3 types") || !strings.Contains(idx, "nothing recorded yet") {
		t.Errorf("index summaries wrong")
	}
}
