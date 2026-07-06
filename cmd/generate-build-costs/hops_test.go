package main

import (
	"database/sql"
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

func TestSystemResolver_canon(t *testing.T) {
	r := &systemResolver{
		ids:    map[string]bool{"krynn": true, "haven": true},
		byName: map[string]string{"Alpha Centauri": "alpha_centauri", "Krynn": "krynn"},
	}
	cases := []struct {
		in     string
		wantID string
		wantOK bool
	}{
		{"krynn", "krynn", true},                   // id hit
		{"Alpha Centauri", "alpha_centauri", true}, // name fallback
		{"Nowhere", "", false},                     // unresolved
	}
	for _, c := range cases {
		id, ok := r.canon(c.in)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("canon(%q) = (%q,%v), want (%q,%v)", c.in, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestStationSystems(t *testing.T) {
	r := &systemResolver{
		ids:    map[string]bool{"krynn": true},
		byName: map[string]string{"Alpha Centauri": "alpha_centauri"},
	}
	stations := []StationMeta{
		{ID: "krynn_hub", System: "krynn"},
		{ID: "ac_hub", System: "Alpha Centauri"},
		{ID: "ghost", System: "Nowhere"},
	}
	got, unresolved := stationSystems(stations, r)
	want := map[string]string{"krynn_hub": "krynn", "ac_hub": "alpha_centauri"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("stationSystems map = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(unresolved, []string{"ghost"}) {
		t.Errorf("unresolved = %v, want [ghost]", unresolved)
	}
}

func TestBfsHops(t *testing.T) {
	// a - b - c ; a - d ; e isolated
	adj := map[string][]string{
		"a": {"b", "d"}, "b": {"a", "c"}, "c": {"b"}, "d": {"a"}, "e": {},
	}
	got := bfsHops(adj, "a")
	want := map[string]int{"a": 0, "b": 1, "d": 1, "c": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bfsHops(a) = %v, want %v", got, want)
	}
	if _, ok := got["e"]; ok {
		t.Errorf("e should be unreachable from a")
	}
}

func TestStationHopDistAndPool(t *testing.T) {
	// systems: sA-sB-sC chain (sB has no station); station in sA, sC, and isolated sZ
	adj := map[string][]string{"sA": {"sB"}, "sB": {"sA", "sC"}, "sC": {"sB"}, "sZ": {}}
	stationSys := map[string]string{"A": "sA", "C": "sC", "Z": "sZ"}
	hd := stationHopDist(adj, stationSys)
	if hd["A"]["C"] != 2 { // A->sA->sB->sC = 2 jumps through empty sB
		t.Fatalf("hopDist A->C = %d, want 2", hd["A"]["C"])
	}
	if hd["A"]["A"] != 0 {
		t.Fatalf("hopDist A->A = %d, want 0", hd["A"]["A"])
	}
	if _, ok := hd["A"]["Z"]; ok {
		t.Fatalf("Z must be unreachable from A")
	}
	// pool membership (sorted, includes home)
	if got := poolMembers(hd, "A", 1); !reflect.DeepEqual(got, []string{"A"}) {
		t.Errorf("pool(A,1) = %v, want [A]", got) // C is 2 hops away
	}
	if got := poolMembers(hd, "A", 2); !reflect.DeepEqual(got, []string{"A", "C"}) {
		t.Errorf("pool(A,2) = %v, want [A C]", got)
	}
	if got := poolMembers(hd, "Z", 3); !reflect.DeepEqual(got, []string{"Z"}) {
		t.Errorf("pool(Z,3) = %v, want [Z]", got)
	}
}

// newHopsTestDB builds an in-memory knowledge DB with a systems table (id,
// name) and a connections table (from_system, to_system), matching the
// production schema queried by loadSystemResolver and loadConnections.
func newHopsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE systems (id TEXT PRIMARY KEY, name TEXT);
INSERT INTO systems VALUES ('krynn','Krynn');
INSERT INTO systems VALUES ('alpha_centauri','Alpha Centauri');
INSERT INTO systems VALUES ('haven','Haven');
CREATE TABLE connections (from_system TEXT, to_system TEXT, distance REAL);
INSERT INTO connections VALUES ('krynn','alpha_centauri',1.0);
INSERT INTO connections VALUES ('alpha_centauri','haven',1.0);
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestLoadSystemResolver(t *testing.T) {
	db := newHopsTestDB(t)
	r, err := loadSystemResolver(db)
	if err != nil {
		t.Fatalf("loadSystemResolver: %v", err)
	}
	if id, ok := r.canon("krynn"); !ok || id != "krynn" {
		t.Errorf("canon(krynn) = (%q,%v), want (krynn,true)", id, ok)
	}
	if id, ok := r.canon("Alpha Centauri"); !ok || id != "alpha_centauri" {
		t.Errorf("canon(Alpha Centauri) = (%q,%v), want (alpha_centauri,true)", id, ok)
	}
	if _, ok := r.canon("Nowhere"); ok {
		t.Errorf("canon(Nowhere) should be unresolved")
	}
}

func TestLoadConnections(t *testing.T) {
	db := newHopsTestDB(t)
	adj, err := loadConnections(db)
	if err != nil {
		t.Fatalf("loadConnections: %v", err)
	}
	want := map[string][]string{
		"krynn":          {"alpha_centauri"},
		"alpha_centauri": {"haven", "krynn"},
		"haven":          {"alpha_centauri"},
	}
	if !reflect.DeepEqual(adj, want) {
		t.Errorf("loadConnections = %v, want %v", adj, want)
	}
}

func TestPooledBook_UnionAndSort(t *testing.T) {
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 10, Qty: 5}, {Price: 20, Qty: 5}}}},
		"B": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 15, Qty: 5}}, "gold": {{Price: 99, Qty: 1}}}},
	}
	pb := pooledBook(books, []string{"A", "B"})
	// iron ladder is the union of both, re-sorted ascending by price.
	iron := pb.Sell["iron"]
	wantPrices := []float64{10, 15, 20}
	if len(iron) != 3 {
		t.Fatalf("iron ladder len = %d, want 3", len(iron))
	}
	for i, p := range wantPrices {
		if iron[i].Price != p {
			t.Errorf("iron[%d].Price = %v, want %v", i, iron[i].Price, p)
		}
	}
	// gold only exists at B.
	if len(pb.Sell["gold"]) != 1 || pb.Sell["gold"][0].Price != 99 {
		t.Errorf("gold ladder wrong: %v", pb.Sell["gold"])
	}
	// A cheapest-first walk over the pool picks the globally cheapest depth.
	w := pb.Walk("iron", 6) // 5@10 + 1@15 = 65
	if w.Cost != 65 || w.Shortfall != 0 {
		t.Errorf("pooled walk = %+v, want cost 65 shortfall 0", w)
	}
}

func TestPooledBooksForRadius(t *testing.T) {
	// A and C are 2 hops apart; radius 2 pools them, radius 1 does not.
	hopDist := map[string]map[string]int{
		"A": {"A": 0, "C": 2},
		"C": {"C": 0, "A": 2},
	}
	books := map[string]*buildcost.Book{
		"A": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 20, Qty: 5}}}},
		"C": {Sell: map[string]buildcost.Ladder{"iron": {{Price: 8, Qty: 5}}}},
	}
	stations := []StationMeta{{ID: "A"}, {ID: "C"}}
	r1 := pooledBooksForRadius(books, hopDist, stations, 1)
	if got := r1["A"].Walk("iron", 1).Cost; got != 20 {
		t.Errorf("radius1 A iron = %v, want 20 (local only)", got)
	}
	r2 := pooledBooksForRadius(books, hopDist, stations, 2)
	if got := r2["A"].Walk("iron", 1).Cost; got != 8 {
		t.Errorf("radius2 A iron = %v, want 8 (C is cheaper, within 2 hops)", got)
	}
}
