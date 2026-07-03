package main

import (
	"database/sql"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/rsned/spacemolt-kb/pkg/bom"
)

// TestDiagnoseBoMCycles is an opt-in diagnostic that walks every item, ship,
// and facility through the BoM calculator individually and reports every
// target that triggers a cycle. Run with:
//
//	go test ./cmd/generate-items-kb -run TestDiagnoseBoMCycles -v -tags diagnose
//
// (No build tag is actually required; the -run filter is enough.) Skip in
// normal CI by checking for the production DB up front.
func TestDiagnoseBoMCycles(t *testing.T) {
	// Resolve repo paths relative to this package's directory so the test
	// works regardless of where it's invoked from.
	dbPath := "../../../spacemolt-crafting-server/database/crafting.db"
	catalogDir := findLatestCatalogDir("../../../spacemolt/data/game-api")

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Skipf("crafting db unavailable: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := db.Ping(); err != nil {
		t.Skipf("crafting db not reachable: %v", err)
	}

	items, err := loadItems(db)
	if err != nil {
		t.Fatalf("load items: %v", err)
	}
	recipes, err := loadRecipes(db)
	if err != nil {
		t.Fatalf("load recipes: %v", err)
	}

	shipCatalogPath := filepath.Join(catalogDir, "catalog_ships.json")
	ships, _ := loadShipCatalog(shipCatalogPath)
	facilities, _ := loadFacilities(catalogDir)

	bomItems := make(map[string]*bom.Item, len(items))
	for id, it := range items {
		bomItems[id] = &bom.Item{ID: it.ID, Name: it.Name, Category: it.Category}
	}
	bomRecipes := make(map[string]*bom.Recipe, len(recipes))
	for id, r := range recipes {
		ins := make([]bom.RecipeItem, len(r.Inputs))
		for i, in := range r.Inputs {
			ins[i] = bom.RecipeItem{ItemID: in.ItemID, Quantity: in.Quantity}
		}
		outs := make([]bom.RecipeItem, len(r.Outputs))
		for i, out := range r.Outputs {
			outs[i] = bom.RecipeItem{ItemID: out.ItemID, Quantity: out.Quantity}
		}
		bomRecipes[id] = &bom.Recipe{ID: r.ID, Inputs: ins, Outputs: outs}
	}

	calc, err := bom.NewCalculator(db, bomRecipes, bomItems)
	if err != nil {
		t.Fatalf("new calculator: %v", err)
	}

	type cycleHit struct {
		Target string
		Cause  string // first cycling input encountered
		Cycle  string
	}
	var hits []cycleHit

	// Items
	for itemID, item := range items {
		if item.Category == "ore" || item.Category == "material" {
			continue
		}
		if _, err := calc.Calculate(itemID, 1); err != nil && strings.Contains(err.Error(), "circular dependency") {
			hits = append(hits, cycleHit{
				Target: "item:" + itemID,
				Cause:  itemID,
				Cycle:  extractCycle(err.Error()),
			})
		}
	}

	// Ships
	for _, ship := range ships {
		for _, mat := range ship.BuildMaterials {
			if _, err := calc.Calculate(mat.ItemID, mat.Quantity); err != nil && strings.Contains(err.Error(), "circular dependency") {
				hits = append(hits, cycleHit{
					Target: "ship:" + ship.ID,
					Cause:  mat.ItemID,
					Cycle:  extractCycle(err.Error()),
				})
				break // one cycle per ship is enough to flag it
			}
		}
	}

	// Facilities
	for _, fac := range facilities {
		for _, mat := range fac.BuildMaterials {
			if _, err := calc.Calculate(mat.ItemID, mat.Quantity); err != nil && strings.Contains(err.Error(), "circular dependency") {
				hits = append(hits, cycleHit{
					Target: "facility:" + fac.ID,
					Cause:  mat.ItemID,
					Cycle:  extractCycle(err.Error()),
				})
				break
			}
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Cycle != hits[j].Cycle {
			return hits[i].Cycle < hits[j].Cycle
		}
		return hits[i].Target < hits[j].Target
	})

	cycleCounts := map[string]int{}
	for _, h := range hits {
		cycleCounts[h.Cycle]++
	}

	// Breakdown by target type and by entry-point material.
	byType := map[string]int{}
	byCause := map[string]int{}
	directItems := []string{}
	for _, h := range hits {
		typ := strings.SplitN(h.Target, ":", 2)[0]
		byType[typ]++
		byCause[h.Cause]++
		if typ == "item" {
			directItems = append(directItems, strings.TrimPrefix(h.Target, "item:"))
		}
	}

	t.Logf("=== Total %d targets hit a cycle ===", len(hits))
	t.Logf("=== %d distinct cycle signatures ===", len(cycleCounts))
	t.Logf("")

	t.Logf("By target type:")
	for typ, n := range byType {
		t.Logf("  %-10s %d", typ, n)
	}
	t.Logf("")

	t.Logf("By entry-point input material (item the calculator was asked to resolve):")
	type causeRow struct {
		Cause string
		N     int
	}
	var causes []causeRow
	for c, n := range byCause {
		causes = append(causes, causeRow{c, n})
	}
	sort.Slice(causes, func(i, j int) bool { return causes[i].N > causes[j].N })
	for _, c := range causes {
		t.Logf("  %-40s %d", c.Cause, c.N)
	}
	t.Logf("")

	t.Logf("Items that ARE the cycle (themselves cyclic, %d):", len(directItems))
	sort.Strings(directItems)
	for _, id := range directItems {
		t.Logf("  - %s", id)
	}
	t.Logf("")

	// Sample ships and a handful of facilities to confirm what's downstream.
	var sampleShips, sampleFacs []string
	for _, h := range hits {
		switch {
		case strings.HasPrefix(h.Target, "ship:"):
			if len(sampleShips) < 20 {
				sampleShips = append(sampleShips, h.Target+" (via "+h.Cause+")")
			}
		case strings.HasPrefix(h.Target, "facility:"):
			if len(sampleFacs) < 5 {
				sampleFacs = append(sampleFacs, h.Target+" (via "+h.Cause+")")
			}
		}
	}
	t.Logf("Sample ships hit (up to 20):")
	for _, s := range sampleShips {
		t.Logf("  %s", s)
	}
	t.Logf("Sample facilities hit (up to 5):")
	for _, s := range sampleFacs {
		t.Logf("  %s", s)
	}
}

func extractCycle(errMsg string) string {
	const marker = "circular dependency detected in BoM calculation: "
	if _, after, ok := strings.Cut(errMsg, marker); ok {
		return after
	}
	return errMsg
}
