package main

import (
	"database/sql"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
)

// StationMeta is a station column: its id, display name, system, and empire.
type StationMeta struct {
	ID     string
	Name   string
	System string
	Empire string
}

// notForSalePrice is the game's "not for sale" sentinel ask (also seen as the
// round 1000000 sibling). Contrary to an earlier assumption, market_ohlcv folds
// resting order-book quantity — including these sentinels — into volume/vwap, so
// any bucket whose high touched the sentinel has a wildly inflated vwap (e.g.
// iron_ore's sell VWAP reads 382cr against a ~2cr live ask). Excluding buckets
// at or above it is a self-contained filter: the highest genuine sell high in
// the data is pathfinder_drive at ~773k, well clear of this cutoff.
const notForSalePrice = 999999

// loadSellVWAP returns each item's sell-side (ask) volume-weighted average price
// from market_ohlcv, excluding any bucket contaminated by the notForSalePrice
// sentinel (see above). Items with no clean sell-side history are absent from
// the map (so an item only ever listed at the sentinel falls back to the
// order-book median in loadBooks rather than reporting a bogus ~1M average).
func loadSellVWAP(marketDB *sql.DB) (map[string]float64, error) {
	rows, err := marketDB.Query(`
		SELECT item_id, SUM(vwap*volume) AS num, SUM(volume) AS den
		FROM market_ohlcv
		WHERE side='sell' AND high_price < ?
		GROUP BY item_id
		HAVING den > 0`, notForSalePrice)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	ref := map[string]float64{}
	for rows.Next() {
		var id string
		var num, den float64
		if err := rows.Scan(&id, &num, &den); err != nil {
			return nil, err
		}
		if den > 0 {
			ref[id] = num / den
		}
	}
	return ref, rows.Err()
}

// median returns a robust "typical" value from a slice (0 if empty). It sorts a
// copy and, for even lengths, returns the lower-middle element rather than
// averaging the two central values. This resists a high outlier polluting the
// result in tiny even-length pools (e.g. {40, 999999} → 40, not ~500k), which
// matters because this is a fallback reference for the outlier cap itself.
func median(in []float64) float64 {
	if len(in) == 0 {
		return 0
	}
	s := append([]float64(nil), in...)
	sort.Float64s(s)
	return s[(len(s)-1)/2]
}

// loadBooks builds each station's current order book from its most recent
// snapshot (MAX(captured_at) per station). Sell ladders are sorted ascending by
// price; BestBuy holds the highest resting buy price per item.
//
// sellVWAP is the per-item sell-side VWAP reference (from loadSellVWAP); items
// missing from it fall back to the median of their own current order-book sell
// prices. When capMult > 0, any sell order priced above capMult*ref is dropped
// as an outlier (e.g. sentinel "not for sale" listings). The second return is
// the count of dropped sell orders.
func loadBooks(marketDB *sql.DB, sellVWAP map[string]float64, capMult float64) (map[string]*buildcost.Book, int, error) {
	rows, err := marketDB.Query(`
WITH latest AS (
  SELECT station_id, MAX(captured_at) AS cap FROM market_orders GROUP BY station_id
)
SELECT o.station_id, o.item_id, o.side, o.price_each, o.quantity
FROM market_orders o
JOIN latest l ON o.station_id = l.station_id AND o.captured_at = l.cap`)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	books := map[string]*buildcost.Book{}
	for rows.Next() {
		var st, item, side string
		var price, qty float64
		if err := rows.Scan(&st, &item, &side, &price, &qty); err != nil {
			return nil, 0, err
		}
		b := books[st]
		if b == nil {
			b = &buildcost.Book{Sell: map[string]buildcost.Ladder{}, BestBuy: map[string]float64{}}
			books[st] = b
		}
		switch side {
		case "sell":
			b.Sell[item] = append(b.Sell[item], buildcost.Order{Price: price, Qty: qty})
		case "buy":
			if price > b.BestBuy[item] {
				b.BestBuy[item] = price
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for _, b := range books {
		for item := range b.Sell {
			l := b.Sell[item]
			sort.Slice(l, func(i, j int) bool { return l[i].Price < l[j].Price })
			b.Sell[item] = l
		}
	}

	dropped := 0
	if capMult > 0 {
		// Gather every item's sell prices across all stations for the median fallback.
		allSell := map[string][]float64{}
		for _, b := range books {
			for item, ladder := range b.Sell {
				for _, o := range ladder {
					allSell[item] = append(allSell[item], o.Price)
				}
			}
		}
		refFor := func(item string) float64 {
			if v, ok := sellVWAP[item]; ok && v > 0 {
				return v
			}
			return median(allSell[item])
		}
		for _, b := range books {
			for item, ladder := range b.Sell {
				ref := refFor(item)
				if ref <= 0 {
					continue
				}
				capVal := capMult * ref
				kept := ladder[:0]
				for _, o := range ladder {
					if o.Price > capVal {
						dropped++
						continue
					}
					kept = append(kept, o)
				}
				b.Sell[item] = kept
			}
		}
	}
	return books, dropped, nil
}

// loadStations returns station columns joined to their empire via the knowledge
// DB systems table (station.system_id -> systems.id -> systems.empire).
func loadStations(marketDB, knowledgeDB *sql.DB) ([]StationMeta, error) {
	empire := map[string]string{}
	erows, err := knowledgeDB.Query(`SELECT id, COALESCE(empire,''), police_level, is_stronghold FROM systems`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = erows.Close() }()
	for erows.Next() {
		var id, emp string
		var police int
		var stronghold bool
		if err := erows.Scan(&id, &emp, &police, &stronghold); err != nil {
			return nil, err
		}
		// A lawless system (no police, non-stronghold) is not under empire control;
		// its empire tag is nearest-empire drift, so treat it as independent space.
		// Mirrors the empire-footprint sweep in cmd/generate-items-kb.
		if police == 0 && !stronghold {
			emp = ""
		}
		empire[id] = emp
	}
	if err := erows.Err(); err != nil {
		return nil, err
	}

	// Canonical station display name + system id, keyed by station/POI id. Preferred
	// over market.db, whose station_name can be a raw slug or hash and whose
	// system_id can be differently cased ("Dheneb" vs "dheneb") — both observed in
	// live scrapes. The knowledge DB POI is the same source the system pages render.
	type poiMeta struct{ name, system string }
	poi := map[string]poiMeta{}
	prows, err := knowledgeDB.Query(`SELECT id, COALESCE(name,''), COALESCE(system_id,'') FROM pois`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = prows.Close() }()
	for prows.Next() {
		var id, name, system string
		if err := prows.Scan(&id, &name, &system); err != nil {
			return nil, err
		}
		poi[id] = poiMeta{name: name, system: system}
	}
	if err := prows.Err(); err != nil {
		return nil, err
	}

	rows, err := marketDB.Query(`SELECT station_id, station_name, system_id, system_name FROM stations`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []StationMeta
	for rows.Next() {
		var m StationMeta
		var sysName string
		if err := rows.Scan(&m.ID, &m.Name, &m.System, &sysName); err != nil {
			return nil, err
		}
		if pm, ok := poi[m.ID]; ok {
			if pm.name != "" {
				m.Name = pm.name
			}
			if pm.system != "" {
				m.System = pm.system
			}
		}
		m.Empire = empire[m.System]
		if m.Empire == "" {
			m.Empire = "Independent"
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// itemMargin derives an item's finished-good margin at a station: ask = cheapest
// sell (first ladder entry), bid = highest buy.
func itemMargin(book *buildcost.Book, finishedID string) buildcost.Margin {
	var m buildcost.Margin
	if book == nil {
		return m
	}
	if l := book.Sell[finishedID]; len(l) > 0 {
		m.FinishedAsk, m.HasAsk = l[0].Price, true
	}
	if bid, ok := book.BestBuy[finishedID]; ok && bid > 0 {
		m.FinishedBid, m.HasBid = bid, true
	}
	return m
}

// normalizeShipKey lowercases and strips a leading empire prefix so a catalog
// ship id (e.g. "outerrim_cobble") and a ship_listings class_id ("cobble") map to
// the same key. Both the full and prefix-stripped forms are indexed by callers.
func normalizeShipKey(s string) string {
	s = strings.ToLower(s)
	if i := strings.Index(s, "_"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// loadShipListings returns stationID -> shipKey -> minimum listed price, where
// shipKey is the normalized class_id. Only the cheapest listing per ship/station
// is kept (the ask you'd actually pay), and only among each station's most
// recent snapshot (MAX(captured_at) per station) — mirroring loadBooks so a
// future accumulation of historical rows can't surface a stale ask.
func loadShipListings(knowledgeDB *sql.DB) (map[string]map[string]float64, error) {
	rows, err := knowledgeDB.Query(`
WITH latest AS (
  SELECT station_id, MAX(captured_at) AS cap FROM ship_listings GROUP BY station_id
), current AS (
  SELECT s.station_id, s.class_id, s.price
  FROM ship_listings s JOIN latest l ON s.station_id = l.station_id AND s.captured_at = l.cap
)
SELECT station_id, class_id, MIN(price) FROM current GROUP BY station_id, class_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]map[string]float64{}
	for rows.Next() {
		var st, classID string
		var price float64
		if err := rows.Scan(&st, &classID, &price); err != nil {
			return nil, err
		}
		if out[st] == nil {
			out[st] = map[string]float64{}
		}
		out[st][normalizeShipKey(classID)] = price
	}
	return out, rows.Err()
}

// shipMargin returns a ship's finished-good margin at a station: ask from the
// per-station listing (matched by normalized ship id) with a catalog-price
// fallback; never a bid.
func shipMargin(listings map[string]map[string]float64, catalogPrice map[string]int, shipID, stationID string) buildcost.Margin {
	var m buildcost.Margin
	key := normalizeShipKey(shipID)
	if st := listings[stationID]; st != nil {
		if p, ok := st[key]; ok {
			m.FinishedAsk, m.HasAsk = p, true
			return m
		}
	}
	if p, ok := catalogPrice[shipID]; ok && p > 0 {
		m.FinishedAsk, m.HasAsk = float64(p), true
	}
	return m
}

// Ship is the minimal ship-catalog shape this generator needs.
type Ship struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Class string `json:"class"`
	Price int    `json:"price"`
}

// loadTargets builds the row set: every item and ship in bill_of_materials, with
// BoM requirements from that table and (items only) candidate recipes from
// recipe_inputs/recipe_outputs. The second return maps target id -> display name.
func loadTargets(craftDB *sql.DB, ships []Ship, itemNames map[string]string) ([]buildcost.Target, map[string]string, error) {
	// 1. BoM rows: target -> requirements, split by kind.
	type key struct{ id, kind string }
	bom := map[key][]buildcost.Requirement{}
	brows, err := craftDB.Query(`SELECT target_id, target_type, base_item_id, quantity
	                             FROM bill_of_materials WHERE target_type IN ('item','ship')
	                             ORDER BY target_id, base_item_id`)
	if err != nil {
		return nil, nil, err
	}
	for brows.Next() {
		var id, kind, base string
		var qty float64
		if err := brows.Scan(&id, &kind, &base, &qty); err != nil {
			_ = brows.Close()
			return nil, nil, err
		}
		k := key{id, kind}
		bom[k] = append(bom[k], buildcost.Requirement{ItemID: base, Qty: qty})
	}
	_ = brows.Close()
	if err := brows.Err(); err != nil {
		return nil, nil, err
	}

	// 2. Item recipes: output -> []Recipe (inputs + output qty).
	recipes := map[string][]buildcost.Recipe{}
	rrows, err := craftDB.Query(`
SELECT ro.item_id AS output, ri.recipe_id, ri.item_id AS input, ri.quantity, ro.quantity AS out_qty
FROM recipe_inputs ri JOIN recipe_outputs ro ON ri.recipe_id = ro.recipe_id
ORDER BY ro.item_id, ri.recipe_id, ri.item_id`)
	if err != nil {
		return nil, nil, err
	}
	// Accumulate inputs per (output, recipe_id).
	type rkey struct{ output, recipe string }
	acc := map[rkey]*buildcost.Recipe{}
	var order []rkey
	for rrows.Next() {
		var output, recipeID, input string
		var qty, outQty float64
		if err := rrows.Scan(&output, &recipeID, &input, &qty, &outQty); err != nil {
			_ = rrows.Close()
			return nil, nil, err
		}
		rk := rkey{output, recipeID}
		r := acc[rk]
		if r == nil {
			r = &buildcost.Recipe{ID: recipeID, OutputQty: outQty}
			acc[rk] = r
			order = append(order, rk)
		}
		r.Inputs = append(r.Inputs, buildcost.Requirement{ItemID: input, Qty: qty})
	}
	_ = rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, nil, err
	}
	for _, rk := range order {
		recipes[rk.output] = append(recipes[rk.output], *acc[rk])
	}

	// 3. Assemble targets.
	names := map[string]string{}
	var targets []buildcost.Target
	for _, s := range ships {
		names[s.ID] = s.Name
	}
	for k, reqs := range bom {
		t := buildcost.Target{ID: k.id, Kind: k.kind, BoM: reqs}
		if k.kind == "item" {
			t.Recipes = recipes[k.id]
			if n, ok := itemNames[k.id]; ok {
				names[k.id] = n
			} else {
				names[k.id] = k.id
			}
		} else { // ship
			t.RecipeNA = "sub-assemblies not market-traded"
			if _, ok := names[k.id]; !ok {
				names[k.id] = k.id
			}
		}
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })
	return targets, names, nil
}
