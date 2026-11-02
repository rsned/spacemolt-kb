package prodcost

import (
	"cmp"
	"database/sql"
	"fmt"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/galaxyhops"
)

// Level is one order-book level at one station.
type Level struct {
	StationID  string
	Price, Qty float64
}

// Market is the latest captured order book of every station.
type Market struct {
	// Asks is item -> marketbot-captured sell levels, price asc then station id.
	Asks map[string][]Level
	// Bids is item -> Station Manager buy levels (source='station'), price
	// desc then station id.
	Bids map[string][]Level
	// StationSystem maps both market station ids (market.db stations) and
	// facility-line base ids (knowledge DB bases.poi_id -> pois.system_id) to
	// canonical system ids. Unresolvable stations are absent.
	StationSystem map[string]string
}

// latestOrdersSQL reads only marketbot captures (source '' or 'station',
// which share one captured_at per capture); 'worker' rows are our agents'
// partial views and are ignored. For each (station, item, side) it keeps the
// rows at that key's latest marketbot captured_at: every ask, and the Station
// Manager (source='station') bids.
const latestOrdersSQL = `
WITH bot AS (
	SELECT station_id, item_id, side, price_each, quantity, source, captured_at
	FROM market_orders WHERE source IN ('', 'station')
), latest AS (
	SELECT item_id, side, station_id, MAX(captured_at) AS mc
	FROM bot GROUP BY item_id, side, station_id
)
SELECT o.station_id, o.item_id, o.side, o.price_each, o.quantity
FROM bot o
JOIN latest l ON o.item_id = l.item_id AND o.side = l.side
	AND o.station_id = l.station_id AND o.captured_at = l.mc
WHERE o.side = 'sell' OR o.source = 'station'`

// MarketAge returns the latest marketbot captured_at in mdb (market.db), or
// "" when there is none; worker rows are ignored, as in LoadMarket.
func MarketAge(mdb *sql.DB) (string, error) {
	var age string
	err := mdb.QueryRow(`SELECT COALESCE(MAX(captured_at), '') FROM market_orders WHERE source IN ('', 'station')`).Scan(&age)
	return age, err
}

// LoadMarket reads the latest books from mdb (market.db) and station systems
// from mdb's stations table and kdb's bases/pois, canonicalised through kdb's
// systems table.
func LoadMarket(mdb, kdb *sql.DB) (Market, error) {
	m := Market{Asks: map[string][]Level{}, Bids: map[string][]Level{}, StationSystem: map[string]string{}}
	res, err := galaxyhops.LoadSystemResolver(kdb)
	if err != nil {
		return Market{}, fmt.Errorf("load systems: %w", err)
	}

	rows, err := mdb.Query(latestOrdersSQL)
	if err != nil {
		return Market{}, fmt.Errorf("query market_orders: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var item, side string
		var l Level
		if err := rows.Scan(&l.StationID, &item, &side, &l.Price, &l.Qty); err != nil {
			return Market{}, fmt.Errorf("scan market_orders: %w", err)
		}
		if side == "sell" {
			m.Asks[item] = append(m.Asks[item], l)
		} else {
			m.Bids[item] = append(m.Bids[item], l)
		}
	}
	if err := rows.Err(); err != nil {
		return Market{}, fmt.Errorf("read market_orders: %w", err)
	}
	for _, ls := range m.Asks {
		slices.SortFunc(ls, func(a, b Level) int {
			return cmp.Or(cmp.Compare(a.Price, b.Price), cmp.Compare(a.StationID, b.StationID))
		})
	}
	for _, ls := range m.Bids {
		slices.SortFunc(ls, func(a, b Level) int {
			return cmp.Or(cmp.Compare(b.Price, a.Price), cmp.Compare(a.StationID, b.StationID))
		})
	}

	if err := addStationSystems(m.StationSystem, res, mdb, `SELECT station_id, system_id FROM stations`); err != nil {
		return Market{}, fmt.Errorf("market stations: %w", err)
	}
	if err := addStationSystems(m.StationSystem, res, kdb,
		`SELECT b.id, p.system_id FROM bases b JOIN pois p ON p.id = b.poi_id WHERE p.system_id IS NOT NULL`); err != nil {
		return Market{}, fmt.Errorf("base systems: %w", err)
	}
	return m, nil
}

// addStationSystems adds every (station, system) row of query whose system
// resolves, without overwriting an earlier entry.
func addStationSystems(dst map[string]string, res *galaxyhops.SystemResolver, db *sql.DB, query string) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var st, sys string
		if err := rows.Scan(&st, &sys); err != nil {
			return err
		}
		if id, ok := res.Canon(sys); ok {
			if _, seen := dst[st]; !seen {
				dst[st] = id
			}
		}
	}
	return rows.Err()
}

// BuyNow walks item's asks cheapest-first for qty units. avgPrice covers the
// filled part; short = qty - filled; station is the station of the costliest
// filled level (from which the haul home is counted), or "" if nothing fills.
func BuyNow(m Market, item string, qty float64) (avgPrice, short float64, station string) {
	var filled, spent float64
	for _, l := range m.Asks[item] {
		if filled >= qty {
			break
		}
		take := min(l.Qty, qty-filled)
		if take <= 0 {
			continue
		}
		filled += take
		spent += take * l.Price
		station = l.StationID
	}
	if filled > 0 {
		avgPrice = spent / filled
	}
	return avgPrice, qty - filled, station
}
