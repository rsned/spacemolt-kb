package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// newMarketDB returns an in-memory market.db with just the market_ohlcv table.
func newMarketDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE market_ohlcv (
		station_id TEXT, item_id TEXT, side TEXT, bucket_utc TEXT,
		open_price REAL, high_price REAL, low_price REAL, close_price REAL,
		volume REAL, trade_count INTEGER, vwap REAL)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func insOHLCV(t *testing.T, db *sql.DB, station, item, side, bucket string, o, h, l, c, vol float64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO market_ohlcv
		(station_id,item_id,side,bucket_utc,open_price,high_price,low_price,close_price,volume,trade_count,vwap)
		VALUES (?,?,?,?,?,?,?,?,?,0,0)`, station, item, side, bucket, o, h, l, c, vol)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadDailyCandles_FoldsByDay(t *testing.T) {
	db := newMarketDB(t)
	defer func() { _ = db.Close() }()

	// ore_iron, day 2026-06-21: two stations/two buckets; day 2026-06-22: one bucket.
	insOHLCV(t, db, "A", "ore_iron", "sell", "2026-06-21T15:00:00Z", 10, 12, 9, 11, 100)
	insOHLCV(t, db, "B", "ore_iron", "sell", "2026-06-21T16:00:00Z", 11, 15, 10, 14, 50)
	insOHLCV(t, db, "A", "ore_iron", "sell", "2026-06-22T09:00:00Z", 14, 16, 13, 15, 80)
	// A buy row that must be ignored entirely.
	insOHLCV(t, db, "A", "ore_iron", "buy", "2026-06-21T15:00:00Z", 5, 5, 5, 5, 999)

	got, err := loadDailyCandles(db)
	if err != nil {
		t.Fatal(err)
	}
	cs := got["ore_iron"]
	if len(cs) != 2 {
		t.Fatalf("want 2 candles, got %d", len(cs))
	}
	d1 := cs[0]
	if d1.Day != "2026-06-21" || d1.Open != 10 || d1.Close != 14 || d1.High != 15 || d1.Low != 9 || d1.Volume != 150 {
		t.Errorf("day1 = %+v", d1)
	}
	d2 := cs[1]
	if d2.Day != "2026-06-22" || d2.Open != 14 || d2.Close != 15 || d2.High != 16 || d2.Low != 13 || d2.Volume != 80 {
		t.Errorf("day2 = %+v", d2)
	}
}

func TestLoadDailyCandles_ExcludesSentinelBuckets(t *testing.T) {
	db := newMarketDB(t)
	defer func() { _ = db.Close() }()

	// day 2026-06-21: one clean bucket plus a notForSalePrice-sentinel bucket
	// whose 999999 high/close and resting volume must be fully excluded.
	insOHLCV(t, db, "A", "ore_x", "sell", "2026-06-21T15:00:00Z", 10, 12, 9, 11, 100)
	insOHLCV(t, db, "A", "ore_x", "sell", "2026-06-21T16:00:00Z", 5, 999999, 5, 999999, 500)
	// ore_y: every bucket is sentinel, so it yields no candle at all.
	insOHLCV(t, db, "A", "ore_y", "sell", "2026-06-21T15:00:00Z", 5, 1000000, 5, 1000000, 42)

	got, err := loadDailyCandles(db)
	if err != nil {
		t.Fatal(err)
	}
	cs := got["ore_x"]
	if len(cs) != 1 {
		t.Fatalf("want 1 candle for ore_x, got %d", len(cs))
	}
	c := cs[0]
	if c.High != 12 || c.Close != 11 || c.Low != 9 || c.Volume != 100 {
		t.Errorf("candle contaminated by sentinel bucket: %+v", c)
	}
	if _, ok := got["ore_y"]; ok {
		t.Errorf("ore_y has only sentinel buckets, expected no candle, got %+v", got["ore_y"])
	}
}

func TestLoadDailyCandles_SameBucketTieBreakByStation(t *testing.T) {
	db := newMarketDB(t)
	defer func() { _ = db.Close() }()

	// Same bucket, two stations: open must come from station "A" (lower id),
	// close from station "B" (higher id) — deterministic tie-break.
	insOHLCV(t, db, "B", "ore_tie", "sell", "2026-06-23T12:00:00Z", 20, 22, 20, 21, 30)
	insOHLCV(t, db, "A", "ore_tie", "sell", "2026-06-23T12:00:00Z", 18, 19, 18, 19, 40)

	got, err := loadDailyCandles(db)
	if err != nil {
		t.Fatal(err)
	}
	cs := got["ore_tie"]
	if len(cs) != 1 {
		t.Fatalf("want 1 candle, got %d", len(cs))
	}
	c := cs[0]
	if c.Open != 18 || c.Close != 21 || c.High != 22 || c.Low != 18 || c.Volume != 70 {
		t.Errorf("candle = %+v", c)
	}
}
