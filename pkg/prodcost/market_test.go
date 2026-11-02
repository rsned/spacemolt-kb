package prodcost

import (
	"database/sql"
	"slices"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBuyNowWalksBook(t *testing.T) {
	m := Market{Asks: map[string][]Level{"ore": {{StationID: "a", Price: 5, Qty: 10}, {StationID: "b", Price: 8, Qty: 10}}}}
	avg, short, st := BuyNow(m, "ore", 15)
	if !near(avg, (50.0+40.0)/15) || short != 0 || st != "b" {
		t.Errorf("BuyNow = (%v, %v, %q), want (%v, 0, b)", avg, short, st, (50.0+40.0)/15)
	}
}

func TestBuyNowShortBook(t *testing.T) {
	m := Market{Asks: map[string][]Level{"ore": {{StationID: "a", Price: 5, Qty: 10}, {StationID: "b", Price: 8, Qty: 10}}}}
	avg, short, st := BuyNow(m, "ore", 30)
	if !near(avg, 130.0/20) || short != 10 || st != "b" {
		t.Errorf("BuyNow = (%v, %v, %q), want (6.5, 10, b)", avg, short, st)
	}
	if avg, short, st := BuyNow(m, "nothing", 4); avg != 0 || short != 4 || st != "" {
		t.Errorf("empty book = (%v, %v, %q)", avg, short, st)
	}
}

func openMem(t *testing.T, schema string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadMarketLatestCaptureAndSystems(t *testing.T) {
	mdb := openMem(t, `
CREATE TABLE market_orders (id INTEGER PRIMARY KEY, station_id TEXT, item_id TEXT, side TEXT,
	price_each REAL, quantity REAL, source TEXT, captured_at TEXT);
CREATE TABLE stations (station_id TEXT PRIMARY KEY, system_id TEXT);
INSERT INTO stations VALUES ('sol_central','sol'), ('grand_exchange','Haven');
INSERT INTO market_orders (station_id,item_id,side,price_each,quantity,source,captured_at) VALUES
	('sol_central','ore','sell',9,5,'','T1'),        -- stale capture: dropped
	('sol_central','ore','sell',7,5,'','T2'),
	('sol_central','ore','sell',6,2,'station','T2'),
	('grand_exchange','ore','sell',4,1,'worker','T1'),
	('sol_central','ore','buy',3,100,'station','T2'),
	('sol_central','ore','buy',3.5,10,'','T2'),       -- player bid: dropped
	('grand_exchange','ore','buy',2,50,'station','T1'),  -- stale
	('grand_exchange','ore','buy',2.5,40,'station','T3');
`)
	kdb := openMem(t, `
CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT);
CREATE TABLE pois (id TEXT PRIMARY KEY, system_id TEXT);
CREATE TABLE bases (id TEXT PRIMARY KEY, poi_id TEXT);
INSERT INTO systems VALUES ('sol','Sol'), ('haven','Haven'), ('cloverfield','Cloverfield');
INSERT INTO pois VALUES ('sol_central','sol'), ('862e','cloverfield');
INSERT INTO bases VALUES ('confederacy_central_command','sol_central'), ('c976','862e'), ('ghost','nowhere');
`)
	m, err := LoadMarket(mdb, kdb)
	if err != nil {
		t.Fatal(err)
	}
	wantAsks := []Level{{"grand_exchange", 4, 1}, {"sol_central", 6, 2}, {"sol_central", 7, 5}}
	if !slices.Equal(m.Asks["ore"], wantAsks) {
		t.Errorf("asks = %v, want %v", m.Asks["ore"], wantAsks)
	}
	wantBids := []Level{{"sol_central", 3, 100}, {"grand_exchange", 2.5, 40}}
	if !slices.Equal(m.Bids["ore"], wantBids) {
		t.Errorf("bids = %v, want %v", m.Bids["ore"], wantBids)
	}
	for st, sys := range map[string]string{"sol_central": "sol", "grand_exchange": "haven",
		"confederacy_central_command": "sol", "c976": "cloverfield"} {
		if m.StationSystem[st] != sys {
			t.Errorf("StationSystem[%s] = %q, want %q", st, m.StationSystem[st], sys)
		}
	}
	if _, ok := m.StationSystem["ghost"]; ok {
		t.Errorf("ghost should be unresolved")
	}
}
