// Command generate-build-costs renders the KB build-cost matrix: the live-market
// cost, feasibility, and margin of building every item and ship at every station.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
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
	stationCoverOut := flag.String("station-cover-out", "kb/did_you_know/stations_to_build.html", "output path for the station-cover did-you-know page; empty disables it")
	facilitiesOut := flag.String("facilities-out", "kb/build-costs/facilities", "output dir for facility build-cost pages; empty disables")
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
		v := matrixViability(matrices[radius])
		reach := []string{"the home station only", "stations within 1 jump", "stations within 2 jumps", "stations within 3 jumps"}[radius]
		summary := fmt.Sprintf("Of %d targets, %d%% are buildable as BoM, %d%% as recipe, %d%% by either — sourcing inputs from %s.",
			v.Total, v.Pct(v.BoM), v.Pct(v.Recipe), v.Pct(v.Either), reach)
		log.Printf("build-costs viability [%s]: %d%% BoM, %d%% recipe, %d%% either (of %d targets)",
			radiusLabels[radius], v.Pct(v.BoM), v.Pct(v.Recipe), v.Pct(v.Either), v.Total)
		must(renderIndex(*outDir, radiusFiles[radius], matrices[radius], headings[radius], summary, tabs), "render index "+radiusFiles[radius])
	}

	rowByID := make([]map[string]MatrixRow, 4)
	for radius := range 4 {
		idx := make(map[string]MatrixRow, len(matrices[radius].Rows))
		for _, row := range matrices[radius].Rows {
			idx[row.ID] = row
		}
		rowByID[radius] = idx
	}
	// Galaxy-wide (no distance limit) per-station sell depth, reused for both
	// the detail-page banners and the station-cover did-you-know page.
	depth := stationDepthFromBooks(books)
	stationIDs := make([]string, 0, len(stations))
	stationNames := make(map[string]string, len(stations))
	for _, st := range stations {
		stationIDs = append(stationIDs, st.ID)
		stationNames[st.ID] = st.Name
	}
	for _, row := range matrices[0].Rows {
		var hopRows [4]MatrixRow
		for radius := range 4 {
			hopRows[radius] = rowByID[radius][row.ID]
		}
		cover := galaxyCoverFor(targetByID[row.ID], depth, stationIDs, stationNames, itemNames)
		must(renderDetail(*outDir, row, stations, targetByID[row.ID], itemNames, categories, hopRows, cover), "render detail "+row.ID)
	}
	if *stationCoverOut != "" {
		page := buildStationCoverPage(targets, depth, stationIDs, stationNames, itemNames, categories)
		must(renderStationCover(*stationCoverOut, page), "render station cover")
		log.Printf("station-cover: %d buildable, %d single-station, %d unbuildable, hardest %s (%d) → %s",
			len(page.Buildable), page.SingleStation, page.UnbuildableCount, page.HardestID, page.MaxStations, *stationCoverOut)
	}

	if *facilitiesOut != "" {
		facRecs, err := loadFacilityCatalog(*catalogRoot)
		must(err, "load facility catalog")
		facBoM, err := loadFacilityBoM(craftDB)
		must(err, "load facility bom")
		recipeOut, err := loadRecipeOutputItem(craftDB)
		must(err, "load recipe outputs")
		gb := galaxyBook(books)
		fPages, fSummaries := buildFacilityPages(facRecs, facBoM, recipeOut, itemNames, categories, sellVWAP, gb)
		must(renderFacilitiesIndex(*facilitiesOut, fSummaries), "render facilities index")
		for _, p := range fPages {
			must(renderFacilityGroup(*facilitiesOut, p), "render facility group "+p.Group)
		}
		log.Printf("facility build-costs: %d facilities across %d groups → %s", len(facRecs), len(fPages), *facilitiesOut)
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
