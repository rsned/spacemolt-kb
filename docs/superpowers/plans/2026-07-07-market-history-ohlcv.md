# Market History (OHLCV) Pages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `kb/market/` KB section — a category index plus per-category pages of galaxy-wide sell-side daily candlestick + volume charts — built from the collected `market_ohlcv` history by a new `cmd/generate-market-history` binary.

**Architecture:** A standalone Go generator structured like `cmd/generate-build-costs`. `load.go` streams sell-side hourly OHLCV rows and folds them into per-item daily candles. `build.go` summarizes each item's window and groups items into per-category view models. `chart.go` renders one item's candles as a self-contained inline SVG. `render.go` + embedded templates emit the landing index and one page per category. `main.go` wires flags → load → build → render.

**Tech Stack:** Go 1.25, `database/sql` + `modernc.org/sqlite` (read-only), `html/template`, `//go:embed`. No new dependencies.

## Global Constraints

- Module path: `github.com/rsned/spacemolt-kb`; new package is `package main` in `cmd/generate-market-history/`.
- Go 1.24+ idioms: `range` over integers, `b.Loop()` in any benchmark. Code must pass `golangci-lint` with no new findings.
- Charts are **galaxy-wide** (all stations pooled), **sell-side only** (`side='sell'`), aggregated to **daily** (UTC calendar day) candles.
- Daily candle fold (per item, per UTC day): `high`=max high_price, `low`=min low_price, `volume`=Σ volume, `open`=open_price of the chronologically-first bucket (earliest `bucket_utc`, ties broken by `station_id`), `close`=close_price of the chronologically-last bucket.
- `changePct` = (lastClose − firstOpen) / firstOpen × 100, and is **0** when there is only one day or firstOpen is 0.
- Charts are self-contained inline SVG — no external assets, no scripts, deterministic (no `Date.now`/`Math.random`/timestamps). Up days (`close ≥ open`) use the "up" hue, down days the "down" hue, applied via CSS classes `up`/`down`.
- Category directory names reuse the item market-category vocabulary (`ammo component consumable contraband defense drone material mining ore refined utility weapon`); items with an empty category bucket under `other`.
- Do NOT commit generated HTML under `kb/market/`; regeneration is a separate step per the KB regen runbook. Commit only source (Go, templates, tests).
- Colors follow the existing KB dark palette: bg `#0d1117`, panel `#161b22`, borders `#30363d`/`#21262d`, text `#c9d1d9`, muted `#8b949e`, accent `#e3b341`, link `#58a6ff`, up `#3fb950`, down `#f85149`.

---

### Task 1: Daily candle data model + loader

**Files:**
- Create: `cmd/generate-market-history/load.go`
- Test: `cmd/generate-market-history/load_test.go`

**Interfaces:**
- Consumes: an open read-only `*sql.DB` on `market.db`.
- Produces:
  - `type DailyCandle struct { Day string; Open, High, Low, Close, Volume float64 }` — `Day` is `"YYYY-MM-DD"`.
  - `func loadDailyCandles(db *sql.DB) (map[string][]DailyCandle, error)` — keyed by `item_id`; each slice is in ascending-day order.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-market-history/ -run TestLoadDailyCandles -v`
Expected: FAIL — `undefined: loadDailyCandles` / `undefined: DailyCandle` (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `cmd/generate-market-history/load.go`:

```go
package main

import "database/sql"

// DailyCandle is one galaxy-wide sell-side daily OHLC bar for an item.
type DailyCandle struct {
	Day    string // "YYYY-MM-DD" (UTC calendar day)
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// loadDailyCandles streams every sell-side market_ohlcv row and folds the
// hourly buckets into per-item daily candles pooled across all stations.
// Rows arrive grouped by item and chronologically within item (bucket, then
// station_id), so day bucketing and first/last selection are a linear fold.
// Each returned slice is in ascending-day order.
func loadDailyCandles(db *sql.DB) (map[string][]DailyCandle, error) {
	rows, err := db.Query(`SELECT item_id, station_id, bucket_utc,
		open_price, high_price, low_price, close_price, volume
		FROM market_ohlcv
		WHERE side = 'sell'
		ORDER BY item_id, bucket_utc, station_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := map[string][]DailyCandle{}
	var (
		curItem, curDay string
		cur             DailyCandle
		have            bool
	)
	flush := func() {
		if have {
			out[curItem] = append(out[curItem], cur)
			have = false
		}
	}
	for rows.Next() {
		var item, station, bucket string
		var o, h, l, c, vol float64
		if err := rows.Scan(&item, &station, &bucket, &o, &h, &l, &c, &vol); err != nil {
			return nil, err
		}
		day := bucket
		if len(bucket) >= 10 {
			day = bucket[:10]
		}
		if !have || item != curItem || day != curDay {
			flush()
			curItem, curDay = item, day
			cur = DailyCandle{Day: day, Open: o, High: h, Low: l, Close: c, Volume: vol}
			have = true
			continue
		}
		if h > cur.High {
			cur.High = h
		}
		if l < cur.Low {
			cur.Low = l
		}
		cur.Close = c // chronologically-last bucket wins
		cur.Volume += vol
	}
	flush()
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-market-history/ -run TestLoadDailyCandles -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-market-history/load.go cmd/generate-market-history/load_test.go
git commit -m "feat(market-history): daily candle loader folding sell-side OHLCV"
```

---

### Task 2: Formatters, window summary, and page grouping

**Files:**
- Create: `cmd/generate-market-history/format.go`
- Create: `cmd/generate-market-history/build.go`
- Test: `cmd/generate-market-history/format_test.go`
- Test: `cmd/generate-market-history/build_test.go`

**Interfaces:**
- Consumes: `DailyCandle` (Task 1); `names, categories map[string]string` (id→display name, id→category).
- Produces:
  - `func fmtCompact(v float64) string` — compact money/volume ("1.0M", "999", "1.2K").
  - `func fmtPct(p float64) string` — signed one-decimal percent ("+50.0%", "-3.1%").
  - `type WindowSummary struct { ItemID string; FirstOpen, LastClose, High, Low, TotalVolume float64; Days int; ChangePct float64 }`.
  - `func summarize(cs []DailyCandle) WindowSummary`.
  - `type ItemVM struct { ID, Name, Category, ItemHref, Stat string; Candles []DailyCandle; Chart template.HTML; Summary WindowSummary }`.
  - `type CategoryPage struct { Category string; Items []ItemVM }`.
  - `type CategoryCard struct { Category, Href, VolStr string; Count int }`.
  - `func buildPages(candles map[string][]DailyCandle, names, categories map[string]string) ([]CategoryPage, []CategoryCard)`.

- [ ] **Step 1: Write the failing formatter test**

```go
package main

import "testing"

func TestFmtCompact(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{999.5, "1K"},
		{1500, "2K"},
		{999_500, "1.0M"},
		{999_999, "1.0M"},
		{12_300_000, "12.3M"},
		{999_999_999, "1.0B"},
		{-57_700_000, "-57.7M"},
	}
	for _, c := range cases {
		if got := fmtCompact(c.in); got != c.want {
			t.Errorf("fmtCompact(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFmtPct(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{50, "+50.0%"},
		{-3.14, "-3.1%"},
		{0, "+0.0%"},
	}
	for _, c := range cases {
		if got := fmtPct(c.in); got != c.want {
			t.Errorf("fmtPct(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-market-history/ -run 'TestFmtCompact|TestFmtPct' -v`
Expected: FAIL — `undefined: fmtCompact` / `undefined: fmtPct`.

- [ ] **Step 3: Write the formatters**

Create `cmd/generate-market-history/format.go`:

```go
package main

import (
	"fmt"
	"strconv"
)

// fmtCompact renders a value as a compact string: B/M carry one decimal, K and
// bare are whole. Thresholds are nudged below each round boundary so a value
// that would round UP to "1000" in the lower unit promotes to the next unit
// (e.g. 999,999 -> "1.0M", not "1000K").
func fmtCompact(v float64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	var s string
	switch {
	case v >= 999_950_000:
		s = strconv.FormatFloat(v/1e9, 'f', 1, 64) + "B"
	case v >= 999_500:
		s = strconv.FormatFloat(v/1e6, 'f', 1, 64) + "M"
	case v >= 999.5:
		s = strconv.FormatFloat(v/1e3, 'f', 0, 64) + "K"
	default:
		s = strconv.FormatFloat(v, 'f', 0, 64)
	}
	if neg {
		return "-" + s
	}
	return s
}

// fmtPct renders a signed percentage with one decimal ("+50.0%", "-3.1%").
func fmtPct(p float64) string {
	return fmt.Sprintf("%+.1f%%", p)
}
```

- [ ] **Step 4: Run to verify formatters pass**

Run: `go test ./cmd/generate-market-history/ -run 'TestFmtCompact|TestFmtPct' -v`
Expected: PASS.

- [ ] **Step 5: Write the failing build test**

Create `cmd/generate-market-history/build_test.go`:

```go
package main

import "testing"

func TestSummarize(t *testing.T) {
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 150},
		{Day: "2026-06-22", Open: 14, High: 16, Low: 13, Close: 15, Volume: 80},
	}
	s := summarize(cs)
	if s.FirstOpen != 10 || s.LastClose != 15 || s.High != 16 || s.Low != 9 {
		t.Errorf("OHLC = %+v", s)
	}
	if s.TotalVolume != 230 || s.Days != 2 {
		t.Errorf("volume/days = %+v", s)
	}
	if s.ChangePct != 50 {
		t.Errorf("changePct = %v, want 50", s.ChangePct)
	}
}

func TestSummarize_SingleDayZeroChange(t *testing.T) {
	cs := []DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 12, Volume: 40}}
	s := summarize(cs)
	if s.Days != 1 || s.ChangePct != 0 {
		t.Errorf("single-day summary = %+v, want Days=1 ChangePct=0", s)
	}
}

func TestBuildPages_GroupSortAndCards(t *testing.T) {
	candles := map[string][]DailyCandle{
		// ore: two items, different total volume.
		"ore_iron": {{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 100}},
		"ore_gold": {{Day: "2026-06-21", Open: 50, High: 55, Low: 48, Close: 52, Volume: 300}},
		// weapon: one item.
		"laser": {{Day: "2026-06-21", Open: 5, High: 6, Low: 5, Close: 6, Volume: 20}},
		// no category -> "other".
		"mystery": {{Day: "2026-06-21", Open: 1, High: 2, Low: 1, Close: 2, Volume: 5}},
		// empty candle slice must be skipped.
		"ghost": {},
	}
	names := map[string]string{"ore_iron": "Iron Ore", "ore_gold": "Gold Ore", "laser": "Laser"}
	categories := map[string]string{"ore_iron": "ore", "ore_gold": "ore", "laser": "weapon"}

	pages, cards := buildPages(candles, names, categories)

	// Cards sorted by category name (bytewise): "ore" < "other" < "weapon".
	if len(cards) != 3 {
		t.Fatalf("want 3 cards, got %d: %+v", len(cards), cards)
	}
	if cards[0].Category != "ore" || cards[1].Category != "other" || cards[2].Category != "weapon" {
		t.Fatalf("card order = %v/%v/%v", cards[0].Category, cards[1].Category, cards[2].Category)
	}
	if cards[0].Count != 2 || cards[0].VolStr != "400" {
		t.Errorf("ore card = %+v, want Count=2 VolStr=400", cards[0])
	}

	// Find the ore page; items sorted by total volume desc -> gold(300) before iron(100).
	var ore CategoryPage
	for _, p := range pages {
		if p.Category == "ore" {
			ore = p
		}
	}
	if len(ore.Items) != 2 || ore.Items[0].ID != "ore_gold" || ore.Items[1].ID != "ore_iron" {
		t.Fatalf("ore items order = %+v", ore.Items)
	}
	// Name falls back to id when missing; href points at the items page.
	if ore.Items[0].Name != "Gold Ore" || ore.Items[0].ItemHref != "../../items/ore/ore_gold.html" {
		t.Errorf("gold item = %+v", ore.Items[0])
	}
	// Unknown category item lands under "other" with id as name.
	var other CategoryPage
	for _, p := range pages {
		if p.Category == "other" {
			other = p
		}
	}
	if len(other.Items) != 1 || other.Items[0].Name != "mystery" {
		t.Errorf("other page = %+v", other.Items)
	}
}
```

Note: every fixture item above is single-day, so their `ChangePct` is 0 — the stat string is asserted separately on a two-day item in `TestFmtStat` (Step 8).

- [ ] **Step 6: Run to verify it fails**

Run: `go test ./cmd/generate-market-history/ -run 'TestSummarize|TestBuildPages' -v`
Expected: FAIL — `undefined: summarize` / `undefined: buildPages`.

- [ ] **Step 7: Write build.go**

Create `cmd/generate-market-history/build.go`:

```go
package main

import (
	"fmt"
	"html/template"
	"sort"
)

// WindowSummary is the whole-window rollup of one item's daily candles.
type WindowSummary struct {
	ItemID      string
	FirstOpen   float64
	LastClose   float64
	High        float64
	Low         float64
	TotalVolume float64
	Days        int
	ChangePct   float64
}

// summarize rolls a day-ordered candle slice into a WindowSummary. cs must be
// non-empty.
func summarize(cs []DailyCandle) WindowSummary {
	s := WindowSummary{
		FirstOpen: cs[0].Open,
		High:      cs[0].High,
		Low:       cs[0].Low,
		Days:      len(cs),
	}
	for _, c := range cs {
		if c.High > s.High {
			s.High = c.High
		}
		if c.Low < s.Low {
			s.Low = c.Low
		}
		s.TotalVolume += c.Volume
	}
	s.LastClose = cs[len(cs)-1].Close
	if s.Days > 1 && s.FirstOpen != 0 {
		s.ChangePct = (s.LastClose - s.FirstOpen) / s.FirstOpen * 100
	}
	return s
}

// fmtStat renders the one-line stat shown under an item heading.
func fmtStat(s WindowSummary) string {
	return fmt.Sprintf("last %s · %s · H %s / L %s · vol %s · %dd",
		fmtCompact(s.LastClose), fmtPct(s.ChangePct),
		fmtCompact(s.High), fmtCompact(s.Low), fmtCompact(s.TotalVolume), s.Days)
}

// ItemVM is one item's view model on a category page.
type ItemVM struct {
	ID       string
	Name     string
	Category string
	ItemHref string
	Stat     string
	Candles  []DailyCandle
	Chart    template.HTML // filled at render time
	Summary  WindowSummary
}

// CategoryPage is one category's page: its items sorted most-traded first.
type CategoryPage struct {
	Category string
	Items    []ItemVM
}

// CategoryCard is one landing-page card.
type CategoryCard struct {
	Category string
	Href     string
	VolStr   string
	Count    int
}

// buildPages groups items by category, sorts items within each category by
// total window volume (descending, ties by id), and builds the landing cards.
// Items with no candles are skipped; items with an empty category bucket under
// "other"; missing names fall back to the item id. Chart HTML is left empty
// (filled during rendering).
func buildPages(candles map[string][]DailyCandle, names, categories map[string]string) ([]CategoryPage, []CategoryCard) {
	byCat := map[string][]ItemVM{}
	for id, cs := range candles {
		if len(cs) == 0 {
			continue
		}
		cat := categories[id]
		if cat == "" {
			cat = "other"
		}
		name := names[id]
		if name == "" {
			name = id
		}
		s := summarize(cs)
		s.ItemID = id
		byCat[cat] = append(byCat[cat], ItemVM{
			ID:       id,
			Name:     name,
			Category: cat,
			ItemHref: fmt.Sprintf("../../items/%s/%s.html", cat, id),
			Stat:     fmtStat(s),
			Candles:  cs,
			Summary:  s,
		})
	}

	cats := make([]string, 0, len(byCat))
	for cat := range byCat {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	var pages []CategoryPage
	var cards []CategoryCard
	for _, cat := range cats {
		items := byCat[cat]
		sort.Slice(items, func(i, j int) bool {
			if items[i].Summary.TotalVolume != items[j].Summary.TotalVolume {
				return items[i].Summary.TotalVolume > items[j].Summary.TotalVolume
			}
			return items[i].ID < items[j].ID
		})
		var tot float64
		for _, it := range items {
			tot += it.Summary.TotalVolume
		}
		pages = append(pages, CategoryPage{Category: cat, Items: items})
		cards = append(cards, CategoryCard{
			Category: cat,
			Href:     cat + "/",
			VolStr:   fmtCompact(tot),
			Count:    len(items),
		})
	}
	return pages, cards
}
```

- [ ] **Step 8: Add the stat-string assertion to the build test**

Append to `build_test.go` a two-day item so `ChangePct` is exercised in the stat:

```go
func TestFmtStat(t *testing.T) {
	s := summarize([]DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 150},
		{Day: "2026-06-22", Open: 14, High: 16, Low: 13, Close: 15, Volume: 80},
	})
	got := fmtStat(s)
	want := "last 15 · +50.0% · H 16 / L 9 · vol 230 · 2d"
	if got != want {
		t.Errorf("fmtStat = %q, want %q", got, want)
	}
}
```

- [ ] **Step 9: Run all Task-2 tests**

Run: `go test ./cmd/generate-market-history/ -run 'TestFmt|TestSummarize|TestBuildPages' -v`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add cmd/generate-market-history/format.go cmd/generate-market-history/build.go \
        cmd/generate-market-history/format_test.go cmd/generate-market-history/build_test.go
git commit -m "feat(market-history): window summary, stat line, category grouping"
```

---

### Task 3: Candlestick + volume SVG chart

**Files:**
- Create: `cmd/generate-market-history/chart.go`
- Test: `cmd/generate-market-history/chart_test.go`

**Interfaces:**
- Consumes: `DailyCandle` (Task 1); `fmtCompact` (Task 2).
- Produces: `func candlestickSVG(candles []DailyCandle) template.HTML`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"strings"
	"testing"
)

func TestCandlestickSVG_UpDownBodies(t *testing.T) {
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 10, High: 15, Low: 9, Close: 14, Volume: 100}, // up
		{Day: "2026-06-22", Open: 14, High: 16, Low: 8, Close: 10, Volume: 40},  // down
	}
	out := string(candlestickSVG(cs))
	if !strings.Contains(out, "<svg") || !strings.Contains(out, "</svg>") {
		t.Fatal("not an svg")
	}
	if n := strings.Count(out, `class="body`); n != 2 {
		t.Errorf("want 2 bodies, got %d", n)
	}
	if !strings.Contains(out, `class="body up"`) || !strings.Contains(out, `class="body down"`) {
		t.Error("expected both up and down bodies")
	}
	// Price axis labels use the compact formatter for the max (16) and min (8).
	if !strings.Contains(out, ">16<") || !strings.Contains(out, ">8<") {
		t.Errorf("missing axis labels: %s", out)
	}
}

func TestCandlestickSVG_SingleCandle(t *testing.T) {
	out := string(candlestickSVG([]DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 50}}))
	if strings.Count(out, `class="body`) != 1 {
		t.Errorf("want 1 body: %s", out)
	}
}

func TestCandlestickSVG_FlatAndZeroVolume(t *testing.T) {
	// Flat prices (high==low everywhere) and zero volume must not panic or NaN.
	cs := []DailyCandle{
		{Day: "2026-06-21", Open: 5, High: 5, Low: 5, Close: 5, Volume: 0},
		{Day: "2026-06-22", Open: 5, High: 5, Low: 5, Close: 5, Volume: 0},
	}
	out := string(candlestickSVG(cs))
	if !strings.Contains(out, "<svg") {
		t.Fatal("not an svg")
	}
	if strings.Contains(out, "NaN") || strings.Contains(out, "Inf") {
		t.Errorf("numeric blowup: %s", out)
	}
}

func TestCandlestickSVG_Empty(t *testing.T) {
	out := string(candlestickSVG(nil))
	if !strings.Contains(out, "<svg") || !strings.Contains(out, "</svg>") {
		t.Errorf("empty input should still yield an svg: %s", out)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-market-history/ -run TestCandlestickSVG -v`
Expected: FAIL — `undefined: candlestickSVG`.

- [ ] **Step 3: Write chart.go**

```go
package main

import (
	"fmt"
	"html/template"
	"strings"
)

// SVG geometry (user units). viewBox is fixed; the element scales to 100% width.
const (
	chartW   = 720.0
	chartVBH = 260.0
	priceTop = 10.0
	priceH   = 170.0
	volTop   = 195.0
	volH     = 40.0
	plotLeft = 44.0
	plotRt   = 710.0
	xLabelY  = 252.0
)

// candlestickSVG renders one item's galaxy-wide sell-side daily OHLC as a
// self-contained inline SVG: a price panel of candles above a volume panel.
// Up days (close >= open) get the "up" class, down days "down"; colors live in
// the page stylesheet. Output is deterministic.
func candlestickSVG(candles []DailyCandle) template.HTML {
	if len(candles) == 0 {
		return template.HTML(fmt.Sprintf(
			`<svg viewBox="0 0 %.0f %.0f" class="chart" role="img" aria-label="no data"></svg>`,
			chartW, chartVBH))
	}

	pmin, pmax := candles[0].Low, candles[0].High
	var vmax float64
	for _, c := range candles {
		if c.Low < pmin {
			pmin = c.Low
		}
		if c.High > pmax {
			pmax = c.High
		}
		if c.Volume > vmax {
			vmax = c.Volume
		}
	}
	prange := pmax - pmin
	yFor := func(p float64) float64 {
		if prange == 0 {
			return priceTop + priceH/2
		}
		return priceTop + (pmax-p)/prange*priceH
	}

	slot := (plotRt - plotLeft) / float64(len(candles))
	bodyW := slot * 0.6
	if bodyW > 14 {
		bodyW = 14
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %.0f %.0f" class="chart" role="img" aria-label="daily price and volume">`, chartW, chartVBH)
	// Price axis: max at top, min at bottom, baseline grid line.
	fmt.Fprintf(&b, `<text x="4" y="%.1f" class="axis">%s</text>`, priceTop+8, fmtCompact(pmax))
	fmt.Fprintf(&b, `<text x="4" y="%.1f" class="axis">%s</text>`, priceTop+priceH, fmtCompact(pmin))
	fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="grid"/>`, plotLeft, priceTop+priceH, plotRt, priceTop+priceH)

	for i, c := range candles {
		cx := plotLeft + (float64(i)+0.5)*slot
		cls := "up"
		if c.Close < c.Open {
			cls = "down"
		}
		// Wick: high to low.
		fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="wick %s"/>`,
			cx, yFor(c.High), cx, yFor(c.Low), cls)
		// Body: open to close, min 1px tall.
		top, bot := yFor(c.Open), yFor(c.Close)
		if bot < top {
			top, bot = bot, top
		}
		h := bot - top
		if h < 1 {
			h = 1
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="body %s"/>`,
			cx-bodyW/2, top, bodyW, h, cls)
		// Volume bar.
		vh := 0.0
		if vmax > 0 {
			vh = c.Volume / vmax * volH
		}
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="vol %s"/>`,
			cx-bodyW/2, volTop+volH-vh, bodyW, vh, cls)
	}

	for _, i := range axisLabelIndices(len(candles)) {
		cx := plotLeft + (float64(i)+0.5)*slot
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" class="axis" text-anchor="middle">%s</text>`,
			cx, xLabelY, candles[i].Day[5:]) // MM-DD
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// axisLabelIndices picks up to four evenly spaced candle indices for x labels.
func axisLabelIndices(n int) []int {
	if n <= 0 {
		return nil
	}
	if n <= 4 {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	return []int{0, n / 3, 2 * n / 3, n - 1}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-market-history/ -run TestCandlestickSVG -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-market-history/chart.go cmd/generate-market-history/chart_test.go
git commit -m "feat(market-history): candlestick + volume inline SVG chart"
```

---

### Task 4: Templates + renderers

**Files:**
- Create: `cmd/generate-market-history/render.go`
- Create: `cmd/generate-market-history/templates/market-index.html.tmpl`
- Create: `cmd/generate-market-history/templates/market-category.html.tmpl`
- Test: `cmd/generate-market-history/render_test.go`

**Interfaces:**
- Consumes: `CategoryCard`, `CategoryPage`, `ItemVM` (Task 2); `candlestickSVG` (Task 3).
- Produces:
  - `func renderMarketIndex(outDir string, cards []CategoryCard) error` — writes `outDir/index.html`.
  - `func renderCategoryPage(outDir string, page CategoryPage) error` — fills each item's `Chart` via `candlestickSVG`, writes `outDir/<category>/index.html`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderCategoryPage(t *testing.T) {
	dir := t.TempDir()
	page := CategoryPage{
		Category: "ore",
		Items: []ItemVM{{
			ID:       "ore_iron",
			Name:     "Iron Ore",
			Category: "ore",
			ItemHref: "../../items/ore/ore_iron.html",
			Stat:     "last 11 · +0.0% · H 12 / L 9 · vol 100 · 1d",
			Candles:  []DailyCandle{{Day: "2026-06-21", Open: 10, High: 12, Low: 9, Close: 11, Volume: 100}},
		}},
	}
	if err := renderCategoryPage(dir, page); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ore", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `id="ore_iron"`) {
		t.Error("missing item anchor")
	}
	if !strings.Contains(html, `../../items/ore/ore_iron.html`) {
		t.Error("missing item-page link")
	}
	if !strings.Contains(html, "last 11 ·") {
		t.Error("missing stat line")
	}
	if !strings.Contains(html, "<svg") {
		t.Error("missing chart svg")
	}
}

func TestRenderMarketIndex(t *testing.T) {
	dir := t.TempDir()
	cards := []CategoryCard{
		{Category: "ore", Href: "ore/", VolStr: "400", Count: 2},
		{Category: "weapon", Href: "weapon/", VolStr: "20", Count: 1},
	}
	if err := renderMarketIndex(dir, cards); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	if !strings.Contains(html, `href="ore/"`) || !strings.Contains(html, `href="weapon/"`) {
		t.Error("missing category cards")
	}
	if !strings.Contains(html, "sell-side") {
		t.Error("missing legend text")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-market-history/ -run TestRender -v`
Expected: FAIL — `undefined: renderCategoryPage` / `undefined: renderMarketIndex`.

- [ ] **Step 3: Write the index template**

Create `cmd/generate-market-history/templates/market-index.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Market History — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 a{color:#58a6ff} h1{margin:.3rem 0} h2{margin:1.6rem 0 .3rem;font-size:1.15rem}
 .legend{font-size:.85rem;color:#8b949e;max-width:80ch}
 .cards{display:flex;flex-wrap:wrap;gap:.6rem;margin-top:1rem}
 .card{display:block;min-width:8rem;background:#161b22;border:1px solid #30363d;border-radius:6px;padding:.7rem .9rem;color:#c9d1d9;text-decoration:none}
 .card:hover{border-color:#58a6ff}
 .card .n{font-size:1.5rem;color:#e3b341;line-height:1}
 .card .g{text-transform:capitalize;margin-top:.2rem}
 .card .v{font-size:.8rem;color:#8b949e;margin-top:.15rem}
</style>
</head>
<body>
<p><a href="../">← Spacemolt KB</a></p>
<h1>Market History</h1>
<p class="legend">Daily price history for every traded item, aggregated <strong>galaxy-wide</strong> from the <strong>sell-side</strong> (ask) order book. Each chart is a daily <strong>candlestick</strong> panel (green up, red down; body spans open→close, wick spans low→high) over a <strong>volume</strong> strip, across the collected ~16-day window. Pick a category to browse its items, most-traded first.</p>
<div class="cards">
{{range .Cards}}<a class="card" href="{{.Href}}"><div class="n">{{.Count}}</div><div class="g">{{.Category}}</div><div class="v">vol {{.VolStr}}</div></a>
{{end}}</div>
</body>
</html>
```

- [ ] **Step 4: Write the category template**

Create `cmd/generate-market-history/templates/market-category.html.tmpl`:

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Category}} — Market History — Spacemolt KB</title>
<style>
 body{font-family:system-ui,sans-serif;margin:0;padding:1rem;background:#0d1117;color:#c9d1d9}
 a{color:#58a6ff} h1{margin:.3rem 0;text-transform:capitalize} h2{margin:0 0 .2rem;font-size:1rem}
 .legend{font-size:.85rem;color:#8b949e;max-width:80ch}
 .item{background:#161b22;border:1px solid #30363d;border-radius:6px;padding:.7rem .9rem;margin:1rem 0;max-width:760px}
 .stat{font-size:.8rem;color:#8b949e;margin:.1rem 0 .4rem}
 .chart{width:100%;height:auto;display:block;background:#0d1117;border-radius:4px}
 .chart .body.up,.chart .wick.up,.chart .vol.up{fill:#3fb950;stroke:#3fb950}
 .chart .body.down,.chart .wick.down,.chart .vol.down{fill:#f85149;stroke:#f85149}
 .chart .wick{stroke-width:1}
 .chart .axis{fill:#8b949e;font-size:10px}
 .chart .grid{stroke:#21262d;stroke-width:1}
</style>
</head>
<body>
<p><a href="../">← Market History</a></p>
<h1>{{.Category}}</h1>
<p class="legend">Galaxy-wide sell-side daily candlesticks with a volume strip, most-traded first. Green = up day (close ≥ open), red = down.</p>
{{range .Items}}<section class="item" id="{{.ID}}">
<h2><a href="{{.ItemHref}}">{{.Name}}</a></h2>
<div class="stat">{{.Stat}}</div>
{{.Chart}}
</section>
{{end}}</body>
</html>
```

- [ ] **Step 5: Write render.go**

```go
package main

import (
	"embed"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl
var tmplFS embed.FS

// renderMarketIndex writes the landing page: a legend and one card per category.
func renderMarketIndex(outDir string, cards []CategoryCard) error {
	t, err := template.ParseFS(tmplFS, "templates/market-index.html.tmpl")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, map[string]any{"Cards": cards})
}

// renderCategoryPage writes outDir/<category>/index.html, rendering each item's
// candlestick chart inline.
func renderCategoryPage(outDir string, page CategoryPage) error {
	for i := range page.Items {
		page.Items[i].Chart = candlestickSVG(page.Items[i].Candles)
	}
	t, err := template.ParseFS(tmplFS, "templates/market-category.html.tmpl")
	if err != nil {
		return err
	}
	dir := filepath.Join(outDir, page.Category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return t.Execute(f, page)
}
```

- [ ] **Step 6: Run to verify it passes**

Run: `go test ./cmd/generate-market-history/ -run TestRender -v`
Expected: PASS (both).

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-market-history/render.go cmd/generate-market-history/render_test.go \
        cmd/generate-market-history/templates/
git commit -m "feat(market-history): index + category templates and renderers"
```

---

### Task 5: main.go wiring + smoke run

**Files:**
- Create: `cmd/generate-market-history/main.go`

**Interfaces:**
- Consumes: `loadDailyCandles` (Task 1), `buildPages` (Task 2), `renderMarketIndex`/`renderCategoryPage` (Task 4).
- Produces: the `generate-market-history` binary. No new exported API.

- [ ] **Step 1: Write main.go**

```go
// Command generate-market-history renders the KB market-history section: a
// category index plus per-category pages of galaxy-wide sell-side daily
// candlestick + volume charts, built from the collected market_ohlcv history.
package main

import (
	"database/sql"
	"flag"
	"log"

	_ "modernc.org/sqlite"
)

func openRO(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

func must(err error, ctx string) {
	if err != nil {
		log.Fatalf("%s: %v", ctx, err)
	}
}

// loadItemMeta returns id->name and id->category for catalog items.
func loadItemMeta(craftDB *sql.DB) (names, categories map[string]string, err error) {
	names, categories = map[string]string{}, map[string]string{}
	rows, err := craftDB.Query(`SELECT id, name, COALESCE(category,'') FROM items`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, nil, err
		}
		names[id], categories[id] = name, cat
	}
	return names, categories, rows.Err()
}

func main() {
	marketPath := flag.String("market", "../spacemolt/data/market.db", "market DB")
	craftPath := flag.String("crafting", "../../spacemolt-crafting-server/database/crafting.db", "crafting DB")
	outDir := flag.String("out", "kb/market", "output directory")
	flag.Parse()

	marketDB, err := openRO(*marketPath)
	must(err, "open market")
	defer func() { _ = marketDB.Close() }()
	craftDB, err := openRO(*craftPath)
	must(err, "open crafting")
	defer func() { _ = craftDB.Close() }()

	names, categories, err := loadItemMeta(craftDB)
	must(err, "load item meta")

	candles, err := loadDailyCandles(marketDB)
	must(err, "load daily candles")

	pages, cards := buildPages(candles, names, categories)

	// Warn about any items that fell through to the "other" bucket.
	for _, c := range cards {
		if c.Category == "other" {
			log.Printf("market-history: %d item(s) had no category (bucketed under 'other')", c.Count)
		}
	}

	must(renderMarketIndex(*outDir, cards), "render index")
	for _, p := range pages {
		must(renderCategoryPage(*outDir, p), "render category "+p.Category)
	}
	log.Printf("market-history: %d items across %d categories → %s", len(candles), len(pages), *outDir)
}
```

- [ ] **Step 2: Build the binary**

Run: `go build ./cmd/generate-market-history/`
Expected: builds with no output.

- [ ] **Step 3: Run the full package test suite + vet**

Run: `go test ./cmd/generate-market-history/ && go vet ./cmd/generate-market-history/`
Expected: `ok` and no vet findings.

- [ ] **Step 4: Smoke run against the real databases**

Run: `go run ./cmd/generate-market-history/ -out /tmp/mkt-smoke`
Expected: a final log line like `market-history: <~601> items across <~12> categories → /tmp/mkt-smoke`, and `/tmp/mkt-smoke/index.html` plus per-category `index.html` files exist. Spot-check one category page opens with candlestick `<svg>` content. (This output is scratch — do not commit it.)

- [ ] **Step 5: Lint**

Run the `golangci-lint` tool over `cmd/generate-market-history/`. Expected: no new findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-market-history/main.go
git commit -m "feat(market-history): wire generator binary (flags, load, render)"
```

---

## Notes for the implementer

- `market.db` is ~6 GB and lives at `../spacemolt/data/market.db` relative to the repo root; `crafting.db` at `../../spacemolt-crafting-server/database/crafting.db`. Both are opened read-only. The smoke run (Task 5) is the only step that touches the real data; all unit tests use in-memory fixtures.
- Do not commit any generated HTML under `kb/market/` or the scratch `/tmp/mkt-smoke` output — regeneration is a separate step per the KB regeneration runbook.
- `bucket_utc` is RFC3339 with a `Z` suffix (e.g. `2026-06-21T15:00:00Z`); the UTC day is exactly the first 10 characters, so no time parsing is needed.
- Keep `fmtCompact` byte-for-byte consistent with the intent described (B/M one decimal, K/bare whole, boundary-nudged) — the boundary test cases in Task 2 guard it.
```
