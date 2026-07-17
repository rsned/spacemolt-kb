// Command generate-market-history renders the KB market-history section: a
// category index plus per-category pages of galaxy-wide sell-side daily
// candlestick + volume charts, built from the collected market_ohlcv history.
package main

import (
	"database/sql"
	"flag"
	"log"

	"github.com/rsned/spacemolt-kb/pkg/marketmeta"
	_ "modernc.org/sqlite"
)

// lastMarketUpdate is the freshness of the market data (latest order-book
// capture) for the "last updated" footer, shared across every rendered page.
var lastMarketUpdate string

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
	lastMarketUpdate, err = marketmeta.LatestCapture(marketDB)
	must(err, "load market capture time")
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
