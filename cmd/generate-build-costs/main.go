// Command generate-build-costs renders the KB build-cost matrix: the live-market
// cost, feasibility, and margin of building every item and ship at every station.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/buildcost"
	_ "modernc.org/sqlite"
)

func openRO(path string) (*sql.DB, error) {
	return sql.Open("sqlite", "file:"+path+"?mode=ro")
}

func main() {
	craftPath := flag.String("crafting", "../../spacemolt-crafting-server/database/crafting.db", "crafting DB")
	marketPath := flag.String("market", "../spacemolt/data/market.db", "market DB")
	knowledgePath := flag.String("knowledge", "../spacemolt-knowledge.db", "knowledge DB")
	catalogRoot := flag.String("catalog", "../spacemolt/data/game-api", "game-api catalog root")
	outDir := flag.String("out", "kb/build-costs", "output directory")
	capMult := flag.Float64("price-cap-mult", 100, "drop sell orders priced above this multiple of an item's typical ask (VWAP); 0 = no cap")
	flag.Parse()

	craftDB, err := openRO(*craftPath)
	must(err, "open crafting")
	defer func() { _ = craftDB.Close() }()
	marketDB, err := openRO(*marketPath)
	must(err, "open market")
	defer func() { _ = marketDB.Close() }()
	knowledgeDB, err := openRO(*knowledgePath)
	must(err, "open knowledge")
	defer func() { _ = knowledgeDB.Close() }()

	ships, catalogPrice, err := loadShipCatalog(*catalogRoot)
	must(err, "load ships")
	itemNames, categories, err := loadItemMeta(craftDB)
	must(err, "load item meta")

	sellVWAP, err := loadSellVWAP(marketDB)
	must(err, "load sell vwap")
	books, dropped, err := loadBooks(marketDB, sellVWAP, *capMult)
	must(err, "load books")
	if *capMult > 0 {
		log.Printf("build-costs: dropped %d outlier sell orders (> %.0fx typical ask)", dropped, *capMult)
	} else {
		log.Printf("build-costs: outlier price cap disabled")
	}
	stations, err := loadStations(marketDB, knowledgeDB)
	must(err, "load stations")
	resolver, err := loadSystemResolver(knowledgeDB)
	must(err, "load system resolver")
	adj, err := loadConnections(knowledgeDB)
	must(err, "load connections")
	stationSys, unresolved := stationSystems(stations, resolver)
	if len(unresolved) > 0 {
		log.Printf("build-costs: %d station(s) had unresolved systems (treated as isolated): %v", len(unresolved), unresolved)
	}
	hopDist := stationHopDist(adj, stationSys)
	listings, err := loadShipListings(knowledgeDB)
	must(err, "load ship listings")
	targets, names, err := loadTargets(craftDB, ships, itemNames)
	must(err, "load targets")

	targetByID := make(map[string]buildcost.Target, len(targets))
	for _, t := range targets {
		targetByID[t.ID] = t
	}

	// Ships also contribute their category for the filter.
	for _, s := range ships {
		if categories[s.ID] == "" {
			categories[s.ID] = s.Class
		}
	}
	// Merge display names (item names + ship names) for rows.
	for id, n := range names {
		if itemNames[id] == "" {
			itemNames[id] = n
		}
	}

	// Four hop radii: 0 (local) = index.html, then +1/+2/+3.
	radiusFiles := []string{"index.html", "hop-1.html", "hop-2.html", "hop-3.html"}
	radiusLabels := []string{"Local", "+1", "+2", "+3"}
	headings := []string{"Build Cost Matrix (Local)", "Build Cost Matrix (+1 hop)", "Build Cost Matrix (+2 hops)", "Build Cost Matrix (+3 hops)"}

	matrices := make([]Matrix, 4)
	for radius := range 4 {
		costBooks := books
		if radius > 0 {
			costBooks = pooledBooksForRadius(books, hopDist, stations, radius)
		}
		matrices[radius] = BuildMatrix(targets, costBooks, books, stations, names, categories, listings, catalogPrice)
	}
	for radius := range 4 {
		tabs := make([]radiusTab, 4)
		for j := range 4 {
			tabs[j] = radiusTab{Label: radiusLabels[j], File: radiusFiles[j], Active: j == radius}
		}
		must(renderIndex(*outDir, radiusFiles[radius], matrices[radius], headings[radius], tabs), "render index "+radiusFiles[radius])
	}

	for _, row := range matrices[0].Rows {
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories), "render detail "+row.ID)
	}
	log.Printf("build-costs: %d rows × %d stations × 4 radii → %s", len(matrices[0].Rows), len(matrices[0].Stations), *outDir)
}

func must(err error, ctx string) {
	if err != nil {
		log.Fatalf("%s: %v", ctx, err)
	}
}

// loadItemMeta returns id->name and id->category for catalog items in crafting.db.
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

// loadShipCatalog reads the latest catalog_ships.json, returning the trimmed ship
// list and an id->price map.
func loadShipCatalog(root string) ([]Ship, map[string]int, error) {
	dir, err := findLatestCatalogDir(root)
	if err != nil {
		return nil, nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "catalog_ships.json"))
	if err != nil {
		return nil, nil, err
	}
	var doc struct {
		Items []Ship `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	price := map[string]int{}
	for _, s := range doc.Items {
		price[s.ID] = s.Price
	}
	return doc.Items, price, nil
}

// findLatestCatalogDir returns the most recently modified subdirectory of root.
func findLatestCatalogDir(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var best string
	var bestMod int64
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().UnixNano(); mt > bestMod {
			bestMod, best = mt, filepath.Join(root, e.Name())
		}
	}
	if best == "" {
		return "", os.ErrNotExist
	}
	return best, nil
}
