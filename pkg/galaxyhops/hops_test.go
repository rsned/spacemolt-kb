package galaxyhops

import (
	"database/sql"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSystemResolverCanon(t *testing.T) {
	r := &SystemResolver{
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
		id, ok := r.Canon(c.in)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("Canon(%q) = (%q,%v), want (%q,%v)", c.in, id, ok, c.wantID, c.wantOK)
		}
	}
}

func TestNewSystemResolver(t *testing.T) {
	r := NewSystemResolver(map[string]string{"alpha_centauri": "Alpha Centauri"})
	if id, ok := r.Canon("Alpha Centauri"); !ok || id != "alpha_centauri" {
		t.Errorf("Canon(Alpha Centauri) = (%q,%v)", id, ok)
	}
	if id, ok := r.Canon("alpha_centauri"); !ok || id != "alpha_centauri" {
		t.Errorf("Canon(alpha_centauri) = (%q,%v)", id, ok)
	}
}

func TestBFSHops(t *testing.T) {
	// a - b - c ; a - d ; e isolated
	adj := map[string][]string{
		"a": {"b", "d"}, "b": {"a", "c"}, "c": {"b"}, "d": {"a"}, "e": {},
	}
	got := BFSHops(adj, "a")
	want := map[string]int{"a": 0, "b": 1, "d": 1, "c": 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BFSHops(a) = %v, want %v", got, want)
	}
	if _, ok := got["e"]; ok {
		t.Errorf("e should be unreachable from a")
	}
}

// newHopsTestDB builds an in-memory knowledge DB with a systems table (id,
// name) and a connections table (from_system, to_system), matching the
// production schema queried by LoadSystemResolver and LoadConnections.
func newHopsTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
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
	r, err := LoadSystemResolver(db)
	if err != nil {
		t.Fatalf("LoadSystemResolver: %v", err)
	}
	if id, ok := r.Canon("krynn"); !ok || id != "krynn" {
		t.Errorf("Canon(krynn) = (%q,%v), want (krynn,true)", id, ok)
	}
	if id, ok := r.Canon("Alpha Centauri"); !ok || id != "alpha_centauri" {
		t.Errorf("Canon(Alpha Centauri) = (%q,%v), want (alpha_centauri,true)", id, ok)
	}
	if _, ok := r.Canon("Nowhere"); ok {
		t.Errorf("Canon(Nowhere) should be unresolved")
	}
}

func TestLoadConnections(t *testing.T) {
	db := newHopsTestDB(t)
	adj, err := LoadConnections(db)
	if err != nil {
		t.Fatalf("LoadConnections: %v", err)
	}
	want := map[string][]string{
		"krynn":          {"alpha_centauri"},
		"alpha_centauri": {"haven", "krynn"},
		"haven":          {"alpha_centauri"},
	}
	if !reflect.DeepEqual(adj, want) {
		t.Errorf("LoadConnections = %v, want %v", adj, want)
	}
}
