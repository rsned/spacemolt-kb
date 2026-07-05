package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newMarketTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE market_orders (station_id TEXT, item_id TEXT, side TEXT, price_each REAL, quantity REAL, captured_at TEXT);
CREATE TABLE stations (station_id TEXT PRIMARY KEY, station_name TEXT, system_id TEXT, system_name TEXT);
INSERT INTO stations VALUES ('st1','Station One','sysA','System A');
-- stale snapshot (ignored) then fresh snapshot for st1
INSERT INTO market_orders VALUES ('st1','iron','sell',99,1,'2026-07-05T10:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','sell',10,5,'2026-07-05T16:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','sell',12,5,'2026-07-05T16:00:00Z');
INSERT INTO market_orders VALUES ('st1','iron','buy',8,3,'2026-07-05T16:00:00Z');
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadBooks_LatestSnapshotAndSortedLadder(t *testing.T) {
	db := newMarketTestDB(t)
	books, err := loadBooks(db)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := books["st1"]
	if !ok {
		t.Fatal("no book for st1")
	}
	// The stale 99-priced order must be excluded; ladder sorted ascending 10 then 12.
	if len(b.Sell["iron"]) != 2 || b.Sell["iron"][0].Price != 10 || b.Sell["iron"][1].Price != 12 {
		t.Fatalf("ladder: %+v", b.Sell["iron"])
	}
	if b.BestBuy["iron"] != 8 {
		t.Fatalf("best buy: %v want 8", b.BestBuy["iron"])
	}
}

func TestItemMargin(t *testing.T) {
	db := newMarketTestDB(t)
	books, _ := loadBooks(db)
	m := itemMargin(books["st1"], "iron")
	if !m.HasAsk || m.FinishedAsk != 10 || !m.HasBid || m.FinishedBid != 8 {
		t.Fatalf("margin: %+v", m)
	}
}

func TestItemMargin_NilBook(t *testing.T) {
	m := itemMargin(nil, "iron")
	if m.HasAsk || m.HasBid {
		t.Fatalf("expected zero-value margin for nil book, got %+v", m)
	}
}

func newKnowledgeTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE systems (id TEXT PRIMARY KEY, empire TEXT);
INSERT INTO systems VALUES ('sysA','Empire X');
CREATE TABLE ship_listings (station_id TEXT, class_id TEXT, price REAL);
INSERT INTO ship_listings VALUES ('st1','cobble',5000);
INSERT INTO ship_listings VALUES ('st1','cobble',4800);
INSERT INTO ship_listings VALUES ('st2','runner',9000);
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadStations_EmpireJoinAndFallback(t *testing.T) {
	marketDB := newMarketTestDB(t)
	// Add a second station whose system has no entry in the knowledge DB's
	// systems table, so it should fall back to "Independent".
	if _, err := marketDB.Exec(`INSERT INTO stations VALUES ('st2','Station Two','sysB','System B')`); err != nil {
		t.Fatal(err)
	}
	knowledgeDB := newKnowledgeTestDB(t)

	stations, err := loadStations(marketDB, knowledgeDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(stations) != 2 {
		t.Fatalf("expected 2 stations, got %d: %+v", len(stations), stations)
	}
	byID := map[string]StationMeta{}
	for _, s := range stations {
		byID[s.ID] = s
	}
	st1 := byID["st1"]
	if st1.Name != "Station One" || st1.System != "sysA" || st1.Empire != "Empire X" {
		t.Fatalf("st1 mismatch: %+v", st1)
	}
	st2 := byID["st2"]
	if st2.Empire != "Independent" {
		t.Fatalf("st2 expected fallback empire Independent, got %+v", st2)
	}
}

func TestLoadShipListings_MinPricePerStationAndKey(t *testing.T) {
	db := newKnowledgeTestDB(t)
	listings, err := loadShipListings(db)
	if err != nil {
		t.Fatal(err)
	}
	// class_id "cobble" has two listings at st1 (5000, 4800); min should win,
	// keyed under the normalized (unprefixed, since "cobble" has no underscore) key.
	if got := listings["st1"]["cobble"]; got != 4800 {
		t.Fatalf("st1/cobble = %v, want 4800", got)
	}
	if got := listings["st2"]["runner"]; got != 9000 {
		t.Fatalf("st2/runner = %v, want 9000", got)
	}
}

func TestNormalizeShipKey(t *testing.T) {
	cases := map[string]string{
		"outerrim_cobble":  "cobble",
		"COBBLE":           "cobble",
		"nexus_runner_mk2": "runner_mk2",
		"plain":            "plain",
	}
	for in, want := range cases {
		if got := normalizeShipKey(in); got != want {
			t.Errorf("normalizeShipKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShipMargin_ListingPreferredOverCatalog(t *testing.T) {
	listings := map[string]map[string]float64{
		"st1": {"cobble": 4800},
	}
	catalog := map[string]int{"outerrim_cobble": 6000}

	m := shipMargin(listings, catalog, "outerrim_cobble", "st1")
	if !m.HasAsk || m.FinishedAsk != 4800 || m.HasBid {
		t.Fatalf("expected listing price 4800 with no bid, got %+v", m)
	}
}

func TestShipMargin_CatalogFallbackWhenNoListing(t *testing.T) {
	listings := map[string]map[string]float64{}
	catalog := map[string]int{"outerrim_cobble": 6000}

	m := shipMargin(listings, catalog, "outerrim_cobble", "st1")
	if !m.HasAsk || m.FinishedAsk != 6000 || m.HasBid {
		t.Fatalf("expected catalog fallback 6000, got %+v", m)
	}
}

func TestShipMargin_NeitherAvailable(t *testing.T) {
	m := shipMargin(map[string]map[string]float64{}, map[string]int{}, "outerrim_cobble", "st1")
	if m.HasAsk || m.HasBid {
		t.Fatalf("expected no margin available, got %+v", m)
	}
}

func newCraftTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE bill_of_materials (target_id TEXT, target_type TEXT, base_item_id TEXT, quantity REAL);
INSERT INTO bill_of_materials VALUES ('widget','item','iron',2);
INSERT INTO bill_of_materials VALUES ('widget','item','copper',1);
INSERT INTO bill_of_materials VALUES ('scout','ship','iron',50);

CREATE TABLE recipe_inputs (recipe_id TEXT, item_id TEXT, quantity REAL);
CREATE TABLE recipe_outputs (recipe_id TEXT, item_id TEXT, quantity REAL);
INSERT INTO recipe_inputs VALUES ('r1','iron',2);
INSERT INTO recipe_inputs VALUES ('r1','copper',1);
INSERT INTO recipe_outputs VALUES ('r1','widget',1);
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadTargets_ItemsAndShips(t *testing.T) {
	db := newCraftTestDB(t)
	ships := []Ship{{ID: "scout", Name: "Scout Ship", Class: "frigate", Price: 12000}}
	itemNames := map[string]string{"widget": "Widget"}

	targets, names, err := loadTargets(db, ships, itemNames)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(targets), targets)
	}
	// Sorted ascending by ID: "scout" < "widget".
	if targets[0].ID != "scout" || targets[1].ID != "widget" {
		t.Fatalf("unexpected order: %+v", targets)
	}

	widget := targets[1]
	if widget.Kind != "item" {
		t.Fatalf("widget kind = %q, want item", widget.Kind)
	}
	if len(widget.BoM) != 2 {
		t.Fatalf("widget BoM = %+v, want 2 requirements", widget.BoM)
	}
	if len(widget.Recipes) != 1 || widget.Recipes[0].ID != "r1" || len(widget.Recipes[0].Inputs) != 2 {
		t.Fatalf("widget recipes = %+v", widget.Recipes)
	}
	if widget.RecipeNA != "" {
		t.Fatalf("widget RecipeNA should be empty, got %q", widget.RecipeNA)
	}

	scout := targets[0]
	if scout.Kind != "ship" {
		t.Fatalf("scout kind = %q, want ship", scout.Kind)
	}
	if scout.RecipeNA != "sub-assemblies not market-traded" {
		t.Fatalf("scout RecipeNA = %q", scout.RecipeNA)
	}
	if len(scout.BoM) != 1 || scout.BoM[0].ItemID != "iron" || scout.BoM[0].Qty != 50 {
		t.Fatalf("scout BoM = %+v", scout.BoM)
	}

	if names["widget"] != "Widget" {
		t.Fatalf("names[widget] = %q, want Widget", names["widget"])
	}
	if names["scout"] != "Scout Ship" {
		t.Fatalf("names[scout] = %q, want Scout Ship", names["scout"])
	}
}

func TestLoadTargets_ItemWithoutNameFallsBackToID(t *testing.T) {
	db := newCraftTestDB(t)
	targets, names, err := loadTargets(db, nil, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tg := range targets {
		if tg.ID == "widget" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected widget target, got %+v", targets)
	}
	if names["widget"] != "widget" {
		t.Fatalf("names[widget] = %q, want fallback to id", names["widget"])
	}
}
