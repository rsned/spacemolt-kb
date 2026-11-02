// Command generate-production-costs renders the KB production-cost section
// (what each item costs to make, by every route, from public data only) and
// a private local page with market prices, opportunities and our own lines.
package main

import (
	"database/sql"
	"flag"
	"log"
	"maps"
	"path/filepath"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/galaxyhops"
	"github.com/rsned/spacemolt-kb/pkg/prodcost"
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

func main() {
	catalogRoot := flag.String("catalog", "../spacemolt/data/game-api", "game-api catalog root")
	knowledgePath := flag.String("knowledge", "spacemolt-knowledge.db", "knowledge DB")
	marketPath := flag.String("market", "../spacemolt/data/market.db", "market DB (private page only)")
	outDir := flag.String("out", "kb/production-costs", "public output directory")
	privateOut := flag.String("private-out", "private/production-costs", "private output directory (gitignored)")
	ourTag := flag.String("our-tag", "CRFT", "our faction tag, for the private page")
	shipSpeed := flag.Int("ship-speed", 3, "hauler speed, for the private page")
	hold := flag.Int("hold", 2000, "hauler hold size, for the private page")
	home := flag.String("home", "cloverfield", "haul origin system id, for the private page")
	flag.Parse()

	kdb, err := openRO(*knowledgePath)
	must(err, "open knowledge")
	defer func() { _ = kdb.Close() }()

	// Public world: no CRFT lines, nothing skipped.
	w, err := prodcost.LoadWorld(*catalogRoot, kdb, "")
	must(err, "load public world")
	// Our own faction's lines are private even where public_facilities lists
	// them, so they stay off the public pages.
	w.Lines = slices.DeleteFunc(w.Lines, func(l prodcost.Line) bool { return l.FactionTag == *ourTag })
	costs := prodcost.Compute(w, prodcost.Options{})

	var ages DataAges
	if dir, err := catalog.FindLatestDir(*catalogRoot); err == nil {
		ages.Catalog = filepath.Base(dir)
	}
	must(kdb.QueryRow(`SELECT COALESCE(MAX(last_seen_utc), '') FROM public_facilities`).Scan(&ages.Facilities), "facilities age")

	must(renderPublic(*outDir, w, costs, ages), "render public")
	log.Printf("production-costs: wrote %d items to %s", len(costs), *outDir)

	// Private page: our own lines, the market and haul settings; recipes that
	// consume energy crystals (reserved) are left out.
	pw, err := prodcost.LoadWorld(*catalogRoot, kdb, *ourTag)
	must(err, "load private world")
	pcosts := prodcost.Compute(pw, prodcost.Options{SkipInputs: map[string]bool{"energy_crystal": true}})
	mdb, err := openRO(*marketPath)
	must(err, "open market")
	defer func() { _ = mdb.Close() }()
	m, err := prodcost.LoadMarket(mdb, kdb)
	must(err, "load market")
	adj, err := galaxyhops.LoadConnections(kdb)
	must(err, "load connections")
	hp := prodcost.HaulParams{Home: *home, JumpTicks: float64(max(1, 7-*shipSpeed)), Hold: float64(*hold),
		Hops: map[string]map[string]int{}}
	for _, sys := range append(slices.Collect(maps.Values(m.StationSystem)), *home) {
		if _, ok := hp.Hops[sys]; !ok {
			hp.Hops[sys] = galaxyhops.BFSHops(adj, sys)
		}
	}
	must(mdb.QueryRow(`SELECT COALESCE(MAX(captured_at), '') FROM market_orders`).Scan(&ages.Market), "market age")
	opps := prodcost.Opportunities(pcosts, pw, m, hp)
	must(renderPrivate(*privateOut, pw, pcosts, m, hp, opps, ages), "render private")
	log.Printf("production-costs: wrote private page (%d items, %d opportunities, %d uncosted bid levels) to %s",
		len(pcosts), len(opps), prodcost.UncostedBids(pcosts, m), *privateOut)
}
