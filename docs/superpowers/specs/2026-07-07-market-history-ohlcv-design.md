# Market History (OHLCV) Pages — Design

**Date:** 2026-07-07
**Status:** Approved (pending spec review)

## Summary

Add a "Market History" KB section that visualizes each traded item's price history as
a **daily candlestick + volume chart**, built from the collected `market_ohlcv`
history. Charts are **galaxy-wide, sell-side (ask)**, aggregated to **daily** candles.
Pages mirror the items / build-costs sections: a landing index of the 12 market
categories, then one page per category listing its items (most-traded first), each
with a self-contained inline-SVG chart and a one-line stat summary.

A new binary `cmd/generate-market-history` produces the section, structured like the
existing `cmd/generate-build-costs`.

## Background & data

`market.db` table `market_ohlcv` holds hourly OHLCV per `(station_id, item_id, side,
bucket_utc)`:

```
station_id, item_id, side ('buy'|'sell'), bucket_utc (hourly, e.g. 2026-06-21T15:00:00Z),
open_price, high_price, low_price, close_price, volume, trade_count, vwap
```

- ~2.18M rows; 601 distinct traded items; window 2026-06-21 → 2026-07-07 (~16 days),
  hourly buckets (sparse — not every hour has trades).
- All 601 traded items resolve to a market category via `crafting.db` `items.category`.
  Category item counts (traded): component 132, refined 103, weapon 65, utility 63,
  ore 52, ammo 46, consumable 45, defense 43, mining 10, contraband 8, drone 5,
  material 2 (12 categories).

## Daily galaxy-wide candle (definition)

For one item, sell-side only, group all stations' hourly buckets by **UTC calendar
day**. For each day:

- `high` = max `high_price` across that day's buckets (all stations).
- `low` = min `low_price`.
- `volume` = Σ `volume`.
- `open` = `open_price` of the **chronologically-first** bucket that day (earliest
  `bucket_utc`; ties across stations broken by `station_id`).
- `close` = `close_price` of the **chronologically-last** bucket that day.

Pooling stations means a wide high–low range reflects real cross-station price
dispersion, not a single venue. This definition is stated so the aggregation is
unambiguous. Days with no sell-side trades for the item simply have no candle (the
x-axis is the sequence of days that *do* have data, not a padded calendar).

## Window summary (per item)

Over all the item's daily candles: `firstOpen` (open of the earliest day), `lastClose`
(close of the latest day), `high` (max), `low` (min), `totalVolume` (Σ), `days`
(candle count), and `changePct` = (lastClose − firstOpen) / firstOpen × 100 (0 when
firstOpen is 0 or only one day).

## Pages & navigation

Mirrors `kb/items` / `kb/build-costs/facilities` (index → per-category pages):

- `kb/market/index.html` — landing. A card per category (name · #items · total volume)
  plus a legend explaining the chart (galaxy-wide sell-side daily OHLC; green up / red
  down; volume strip; the ~16-day window). Cards sorted by category name.
- `kb/market/<category>/index.html` — one page per category. Items sorted by
  `totalVolume` descending (most-traded first). Each item is an anchored
  `<section id="<item_id>">` with:
  - Heading: item name linking to `kb/items/<category>/<id>.html`, plus a one-line
    stat: `last <close> · <±Δ%> · H <high> / L <low> · vol <total> · <days>d`.
  - The candlestick + volume SVG chart.

Category directory names reuse the item market-category vocabulary (same as `kb/items`
subdirectories).

## Charting

A pure function renders one item's chart as self-contained inline SVG (no external
assets), dark theme consistent with the build-cost pages:

```
candlestickSVG(candles []DailyCandle, opts chartOpts) template.HTML
```

- **Price panel (top):** one candle per day. Body spans `open`→`close`; a thin wick
  spans `low`→`high`. Up days (`close ≥ open`) use the "up" hue, down days the "down"
  hue. Y-axis labeled with the price min and max (2–3 ticks); a faint baseline grid.
- **Volume panel (bottom, ~25% height):** one bar per day, height ∝ `volume` /
  maxVolume, same up/down color as its candle.
- **X-axis:** sparse date labels (first, last, and a couple between).
- Fixed viewBox; scales responsively (`width:100%`). Deterministic output (no
  randomness, no timestamps baked in).
- **Colors:** follow the dataviz skill — an up/down pair that is colorblind-safe and
  legible on the dark KB background; encode direction with more than hue alone where
  feasible (up/down are also positionally obvious from body direction).
- **Edge cases:** a single candle renders one centered candle + one bar; a flat series
  (all prices equal, high==low) renders zero-height bodies as a 1px line at the level
  without divide-by-zero; zero total volume renders an empty (flat) volume strip.

Rendering detail (axis math, SVG string building) lives in a `market` package or a
chart file in the command package, kept separate from data loading so it is unit
testable in isolation.

## Pipeline / implementation

New binary `cmd/generate-market-history`, structured like `generate-build-costs`:

- `main.go` — flags: `-market` (market.db), `-crafting` (crafting.db, for item
  names/categories), `-out` (default `kb/market`). Loads item meta, streams candles,
  builds pages, renders.
- `load.go` — `loadItemMeta(craftDB)` (id→name, id→category, reusing the same query
  shape as build-costs); `loadDailyCandles(marketDB)` streams sell-side rows
  `ORDER BY item_id, bucket_utc, station_id` and folds them into `map[itemID][]DailyCandle`
  in one pass (rows arrive grouped by item and chronological within item, so day
  bucketing and first/last are a linear fold).
- `build.go` — window summaries; group items by category; sort within category by
  totalVolume desc; assemble the landing + per-category view models.
- `chart.go` — `candlestickSVG` and its axis/scale helpers.
- `render.go` + `templates/*.tmpl` — index and per-category templates (embedded via
  `//go:embed`), styled like the build-cost pages.

Items with sell-side history but that resolve to no category are bucketed under
`other` (kept, not dropped); a startup log notes any such count. All 601 items are
expected to have a category, so `other` should normally be empty.

## Testing

- **Candle folding:** hourly rows across two stations and two days fold into two daily
  candles with correct open (first chronological), close (last), high (max), low (min),
  volume (sum); station tie-break by id is deterministic.
- **Window summary:** changePct, high/low, totalVolume, day count; single-day and
  zero-open guards.
- **candlestickSVG:** emits the right number of candle elements and volume bars;
  up vs down days get distinct classes/colors; single-candle, flat-series (high==low),
  and zero-volume inputs produce valid SVG without panicking or dividing by zero;
  output contains the price min/max axis labels.
- **Grouping / sort:** items land in their category; within a category they are ordered
  by totalVolume desc; category cards carry correct counts and volume totals.
- **Render:** a category page emits one anchored section per item with the item-page
  link and the stat line; the landing page emits a card per category and the legend.

## Out of scope (v1)

- Buy-side (bid) series; per-station charts; hourly granularity; hover/tooltip
  interactivity; a VWAP overlay line; zoom/pan; cross-item comparison.
- Committing generated HTML is a separate regeneration step (per the KB regen runbook),
  not part of the feature branch.
