package marketmeta

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func newDB(t *testing.T, rows ...string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE market_orders (station_id TEXT, item_id TEXT, side TEXT, price_each REAL, quantity REAL, captured_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	for _, ts := range rows {
		if _, err := db.Exec(`INSERT INTO market_orders (captured_at) VALUES (?)`, ts); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestLatestCapture_FormatsMaxAsUTC(t *testing.T) {
	db := newDB(t, "2026-07-12T14:00:51Z", "2026-07-12T17:22:03Z", "2026-07-12T09:15:00Z")
	got, err := LatestCapture(db)
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-07-12 17:22 UTC"; got != want {
		t.Fatalf("LatestCapture = %q, want %q", got, want)
	}
}

func TestLatestCapture_EmptyWhenNoOrders(t *testing.T) {
	db := newDB(t)
	got, err := LatestCapture(db)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("LatestCapture = %q, want empty", got)
	}
}
