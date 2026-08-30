package wildlife

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	stmts := []string{
		`CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT, position_x REAL DEFAULT 0, position_y REAL DEFAULT 0, police_level INTEGER DEFAULT 0, empire TEXT, is_stronghold INTEGER DEFAULT 0, last_updated_tick INTEGER DEFAULT 0)`,
		`CREATE TABLE connections (from_system TEXT, to_system TEXT, distance INTEGER)`,
		`CREATE TABLE pois (id TEXT PRIMARY KEY, system_id TEXT, name TEXT, type TEXT)`,
		`CREATE TABLE wildlife_species (species TEXT PRIMARY KEY, name TEXT, role TEXT, max_hull INTEGER, max_shield INTEGER, danger TEXT, danger_scanned_utc TEXT DEFAULT '', habitats TEXT, first_seen_utc TEXT, last_seen_utc TEXT, scan_traits TEXT DEFAULT '', scan_revealed TEXT DEFAULT '', ranchable INTEGER DEFAULT 0)`,
		`CREATE TABLE wildlife_attacks (species TEXT, battle_id TEXT, weapon_name TEXT, damage_type TEXT, shot_kind TEXT, shots INTEGER, hits INTEGER, damage_total REAL, damage_min REAL, damage_max REAL, observed_utc TEXT)`,
		`CREATE TABLE wildlife_sightings (id INTEGER PRIMARY KEY, species TEXT, system_id TEXT, poi_id TEXT, source TEXT, observed_count INTEGER, abundance TEXT, ranched INTEGER DEFAULT 0, branded INTEGER DEFAULT 0, in_combat INTEGER DEFAULT 0, bloom_status TEXT DEFAULT '', bloom_intensity REAL DEFAULT 0, game_tick INTEGER, observed_utc TEXT, agent_id TEXT DEFAULT '', survey_power INTEGER DEFAULT 0)`,
		`CREATE TABLE wildlife_surveys (id INTEGER PRIMARY KEY, system_id TEXT, poi_id TEXT, poi_type TEXT, source TEXT, species_seen INTEGER, creatures_seen INTEGER, game_tick INTEGER, observed_utc TEXT)`,
		`CREATE TABLE wildlife_kills (creature_id TEXT, game_tick INTEGER, species TEXT, creature_name TEXT, role TEXT, max_hull INTEGER, system_id TEXT, poi_id TEXT, battle_id TEXT, duration_ticks INTEGER, damage_dealt INTEGER, damage_taken INTEGER, wreck_id TEXT, salvage_value INTEGER, carcass_read INTEGER, killed_utc TEXT, agent_id TEXT)`,
		`CREATE TABLE wildlife_kill_drops (creature_id TEXT, game_tick INTEGER, item_id TEXT, quantity REAL)`,
		`CREATE TABLE items (id TEXT PRIMARY KEY, name TEXT, category TEXT)`,
		`INSERT INTO systems (id,name,last_updated_tick) VALUES ('cursa','Cursa',5),('sol','Sol',5),('void','Void',0)`,
		`INSERT INTO pois VALUES ('cursa_belt','cursa','Cursa Belt','asteroid_belt'),('cursa_facets','cursa','Stray Facets','asteroid_belt'),('sol_cloud','sol','Sol Cloud','gas_cloud')`,
		`INSERT INTO wildlife_species VALUES ('belt_grazer','Belt-Grazer','grazer',60,0,'harmless prey','','asteroid_belt','2026-08-17T09:58:42Z','2026-08-29T09:02:10Z','harmless prey, ranchable stock','species,role',1),
		 ('rainbow_leviathan','Rainbow Leviathan','predator',2200,0,'hunts ships','','asteroid_belt','2026-08-17T23:00:00Z','2026-08-27T00:00:00Z','hunts ships','species',0),
		 ('ghost','Ghost','scavenger',10,0,'','','nebula','','','','',0)`,
		`INSERT INTO wildlife_attacks VALUES ('rainbow_leviathan','b1','Rainbow Leviathan (natural)','energy','beam',2,2,260,130,130,'2026-08-17T23:43:06Z'),
		 ('rainbow_leviathan','b2','Rainbow Leviathan (natural)','energy','beam',3,1,120,120,120,'2026-08-20T00:00:00Z')`,
		// belt_grazer in cursa: older system survey (8) then newer (6); POIs latest counts 4 + 2.
		`INSERT INTO wildlife_sightings (species,system_id,poi_id,source,observed_count,abundance,ranched,branded,bloom_status,game_tick,observed_utc) VALUES
		 ('belt_grazer','cursa','','survey_system',8,'moderate',0,0,'dormant',100,'2026-08-20T00:00:00Z'),
		 ('belt_grazer','cursa','','survey_system',6,'scarce',1,0,'dormant',200,'2026-08-29T00:00:00Z'),
		 ('belt_grazer','cursa','cursa_belt','get_nearby',5,'',0,0,'',150,'2026-08-25T00:00:00Z'),
		 ('belt_grazer','cursa','cursa_belt','get_nearby',4,'',1,0,'',210,'2026-08-29T00:01:00Z'),
		 ('belt_grazer','cursa','cursa_facets','get_nearby',2,'',0,0,'',210,'2026-08-29T00:02:00Z'),
		 ('belt_grazer','sol','sol_cloud','get_nearby',3,'',0,1,'rising',300,'2026-08-29T01:00:00Z'),
		 ('rainbow_leviathan','sol','','survey_system',1,'scarce',0,0,'',300,'2026-08-29T01:00:00Z')`,
		`INSERT INTO wildlife_surveys (system_id,poi_id,poi_type,source,species_seen,creatures_seen,game_tick,observed_utc) VALUES ('cursa','','','survey_system',1,6,200,''),('cursa','cursa_belt','asteroid_belt','get_nearby',1,4,210,''),('void','','','survey_system',0,0,50,'')`,
		`INSERT INTO wildlife_kills VALUES ('c1',400,'belt_grazer','Belt-Grazer','grazer',60,'cursa','cursa_belt','b9',12,60,5,'w1',150,1,'2026-08-29T02:00:00Z','bot')`,
		`INSERT INTO wildlife_kill_drops VALUES ('c1',400,'creature_carapace',2)`,
		`INSERT INTO items VALUES ('creature_carapace','Creature Carapace','material')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func TestLoad(t *testing.T) {
	g, err := Load(openTestDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Species) != 3 {
		t.Fatalf("species = %d", len(g.Species))
	}
	// Sorted by role (grazer, predator, scavenger) then name.
	if g.Species[0].ID != "belt_grazer" || g.Species[1].ID != "rainbow_leviathan" || g.Species[2].ID != "ghost" {
		t.Errorf("order = %v %v %v", g.Species[0].ID, g.Species[1].ID, g.Species[2].ID)
	}
	bg := g.Species[0]
	if bg.Name != "Belt-Grazer" || bg.Role != "grazer" || bg.MaxHull != 60 || !bg.Ranchable || bg.Habitats[0] != "asteroid_belt" {
		t.Errorf("belt_grazer = %+v", bg)
	}
	if len(bg.Places) != 2 {
		t.Fatalf("places = %+v", bg.Places)
	}
	// Cursa: latest system survey wins for the estimate; POIs carry their latest counts.
	c := bg.Places[0]
	if c.SystemID != "cursa" || c.SystemName != "Cursa" || c.Count != 6 || c.Abundance != "scarce" || !c.Ranched || c.Branded || c.LastTick != 210 {
		t.Errorf("cursa = %+v", c)
	}
	if len(c.POIs) != 2 || c.POIs[0].Name != "Cursa Belt" || c.POIs[0].Count != 4 || c.POIs[1].Name != "Stray Facets" || c.POIs[1].Count != 2 {
		t.Errorf("cursa pois = %+v", c.POIs)
	}
	// Sol: no system-level survey, so the estimate is the sum of POI counts.
	s := bg.Places[1]
	if s.SystemID != "sol" || s.Count != 3 || s.Abundance != "" || !s.Branded || s.Bloom != "rising" || len(s.POIs) != 1 {
		t.Errorf("sol = %+v", s)
	}
	if bg.EstimatedTotal() != 9 || bg.SystemCount() != 2 {
		t.Errorf("totals = %d / %d", bg.EstimatedTotal(), bg.SystemCount())
	}

	rl := g.Species[1]
	if len(rl.Attacks) != 1 {
		t.Fatalf("attacks = %+v", rl.Attacks)
	}
	a := rl.Attacks[0]
	if a.Weapon != "Rainbow Leviathan (natural)" || a.DamageType != "energy" || a.ShotKind != "beam" || a.Battles != 2 || a.Shots != 5 || a.Hits != 3 || a.DamageTotal != 380 || a.DamageMin != 120 || a.DamageMax != 130 {
		t.Errorf("attack = %+v", a)
	}
	if a.Accuracy() != 60 || a.DamagePerHit() != 126.67 {
		t.Errorf("accuracy %v / per hit %v", a.Accuracy(), a.DamagePerHit())
	}
	if len(g.Species[2].Places) != 0 || g.Species[2].Attacks != nil {
		t.Errorf("ghost should have no data: %+v", g.Species[2])
	}

	if len(bg.Kills) != 1 || bg.Kills[0].SystemName != "Cursa" || bg.Kills[0].POIName != "Cursa Belt" || bg.Kills[0].SalvageValue != 150 {
		t.Errorf("kills = %+v", bg.Kills)
	}
	if len(bg.Kills[0].Drops) != 1 || bg.Kills[0].Drops[0].ItemName != "Creature Carapace" || bg.Kills[0].Drops[0].ItemCategory != "material" || bg.Kills[0].Drops[0].Quantity != 2 {
		t.Errorf("drops = %+v", bg.Kills[0].Drops)
	}

	if g.Coverage.SystemsSurveyed != 2 || g.Coverage.PlacesSurveyed != 3 || g.Coverage.SystemsWithWildlife != 2 || g.Coverage.TotalSystems != 3 {
		t.Errorf("coverage = %+v", g.Coverage)
	}
	if g.EstimatedCreatures() != 10 {
		t.Errorf("estimated creatures = %d", g.EstimatedCreatures())
	}
	if len(g.MapSystems) != 3 || g.MapByID["cursa"] == nil {
		t.Errorf("map systems = %d", len(g.MapSystems))
	}
}

func TestLoad_DescriptionColumnOptional(t *testing.T) {
	db := openTestDB(t)
	// The test schema has no description column: Load must still work and
	// leave Description empty.
	g, err := Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if g.Species[0].Description != "" {
		t.Errorf("no column -> description should be empty, got %q", g.Species[0].Description)
	}
	// Once the knowledge DB gains the column, it is read.
	if _, err := db.Exec(`ALTER TABLE wildlife_species ADD COLUMN description TEXT NOT NULL DEFAULT ''`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE wildlife_species SET description='Combs copper dust.' WHERE species='belt_grazer'`); err != nil {
		t.Fatal(err)
	}
	g, err = Load(db)
	if err != nil {
		t.Fatal(err)
	}
	if g.Species[0].Description != "Combs copper dust." {
		t.Errorf("description = %q", g.Species[0].Description)
	}
}
