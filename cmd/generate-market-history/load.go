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

// notForSalePrice is the game's "not for sale" sentinel ask (also seen as the
// round 1000000 sibling). market_ohlcv folds resting order-book quantity —
// including these sentinels — into a bucket's high/close/vwap, so a
// sentinel-touched bucket would otherwise put a 999999 wick (and, when it is the
// day's last bucket, a 999999 close) on the candle. Excluding buckets at or
// above it is safe: the highest genuine sell high in the data is
// pathfinder_drive at ~773k, well clear of this cutoff.
const notForSalePrice = 999999

// loadDailyCandles streams every sell-side market_ohlcv row and folds the
// hourly buckets into per-item daily candles pooled across all stations.
// Buckets contaminated by the notForSalePrice sentinel (see above) are excluded.
// Rows arrive grouped by item and chronologically within item (bucket, then
// station_id), so day bucketing and first/last selection are a linear fold.
// Each returned slice is in ascending-day order.
func loadDailyCandles(db *sql.DB) (map[string][]DailyCandle, error) {
	rows, err := db.Query(`SELECT item_id, station_id, bucket_utc,
		open_price, high_price, low_price, close_price, volume
		FROM market_ohlcv
		WHERE side = 'sell' AND high_price < ?
		ORDER BY item_id, bucket_utc, station_id`, notForSalePrice)
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
