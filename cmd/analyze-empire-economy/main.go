// Command analyze-empire-economy produces a Markdown report on component
// popularity, per-product empire self-sufficiency, and per-empire resource
// scarcity, using the crafting and knowledge databases plus the ship catalog.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/bom"
	_ "modernc.org/sqlite"
)

const (
	defaultCraftingDB  = "../../spacemolt-crafting-server/database/crafting.db"
	defaultKnowledgeDB = "../spacemolt-knowledge.db"
	defaultCatalogBase = "../spacemolt/data/game-api"
	defaultOutPath     = "docs/analysis/empire-economy.md"
)

// empires is the closed set of named empires this report covers. Unaligned
// systems (empire="") are excluded from every per-empire computation.
var empires = []string{"crimson", "nebula", "outerrim", "solarian", "voidborn"}

func main() {
	craftingDBPath := flag.String("crafting-db", defaultCraftingDB, "path to crafting.db")
	knowledgeDBPath := flag.String("knowledge-db", defaultKnowledgeDB, "path to spacemolt-knowledge.db")
	catalogBase := flag.String("catalog", defaultCatalogBase, "path to game-api catalog base dir; latest YYYYMMDD subdir is used")
	outPath := flag.String("out", defaultOutPath, "output markdown path")
	flag.Parse()

	catalogDir := findLatestCatalogDir(*catalogBase)
	log.Printf("Catalog dir: %s", catalogDir)

	craftingDB, err := sql.Open("sqlite", *craftingDBPath)
	if err != nil {
		log.Fatalf("open crafting db: %v", err)
	}
	defer func() { _ = craftingDB.Close() }()

	knowDB, err := sql.Open("sqlite", *knowledgeDBPath)
	if err != nil {
		log.Fatalf("open knowledge db: %v", err)
	}
	defer func() { _ = knowDB.Close() }()

	items, err := loadItems(craftingDB)
	if err != nil {
		log.Fatalf("load items: %v", err)
	}
	recipes, err := loadRecipes(craftingDB)
	if err != nil {
		log.Fatalf("load recipes: %v", err)
	}
	ships, err := loadShips(filepath.Join(catalogDir, "catalog_ships.json"))
	if err != nil {
		log.Fatalf("load ships: %v", err)
	}
	log.Printf("Loaded %d items, %d recipes, %d ships", len(items), len(recipes), len(ships))

	bomItems, bomRecipes := buildBomMaps(items, recipes)
	calc, err := bom.NewCalculator(nil, bomRecipes, bomItems)
	if err != nil {
		log.Fatalf("init bom calculator: %v", err)
	}

	products, err := computeProducts(calc, items, ships)
	if err != nil {
		log.Fatalf("compute products: %v", err)
	}
	log.Printf("Computed BoM for %d products", len(products))

	empMap, galaxy, err := loadEmpireResources(knowDB)
	if err != nil {
		log.Fatalf("load empire resources: %v", err)
	}

	// Sections emit their Markdown body and contribute one summary bullet for
	// the executive summary at the top.
	var summary []string
	var body strings.Builder

	if s := writePopularity(&body, items, recipes, products); s != "" {
		summary = append(summary, s)
	}
	soloByEmpire, soloProducts := writeSelfSufficiency(&body, products, empMap)
	if s := summarizeSelfSufficiency(products, soloByEmpire); s != "" {
		summary = append(summary, s)
	}
	easeByEmpireProduct := writeResourceSheets(&body, items, products, empMap, galaxy)
	if s := summarizeResources(empMap, galaxy); s != "" {
		summary = append(summary, s)
	}
	if s := writeSSI(&body, products, soloByEmpire, soloProducts, easeByEmpireProduct); s != "" {
		summary = append(summary, s)
	}

	finalDoc := assembleDoc(summary, body.String())

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
	if err := os.WriteFile(*outPath, []byte(finalDoc), 0o644); err != nil {
		log.Fatalf("write output: %v", err)
	}

	fmt.Printf("Wrote %s\n", *outPath)
	for _, bullet := range summary {
		fmt.Printf("  • %s\n", bullet)
	}
}

// assembleDoc stitches the executive summary and section bodies into the
// final report. The top of the file includes a generated-at timestamp so
// stale reports are obvious.
func assembleDoc(summaryBullets []string, body string) string {
	var sb strings.Builder
	sb.WriteString("# Empire Economy Analysis\n\n")
	fmt.Fprintf(&sb, "_Generated %s_\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"))
	sb.WriteString("## Executive Summary\n\n")
	if len(summaryBullets) == 0 {
		sb.WriteString("_(no findings)_\n\n")
	}
	for _, b := range summaryBullets {
		fmt.Fprintf(&sb, "- %s\n", b)
	}
	sb.WriteString("\n---\n\n")
	sb.WriteString(body)
	sb.WriteString("\n---\n\n")
	sb.WriteString("## Appendix: Methodology\n\n")
	sb.WriteString("- **Products analyzed**: every craftable item (excluding raw ores/materials) plus every ship in the catalog. Items whose BoM bottoms out on something other than a base ore/material (e.g. salvage-only drops) are skipped.\n")
	sb.WriteString("- **Base materials**: items in category `ore` or `material` — the terminals of `pkg/bom`'s recipe-resolution tree.\n")
	sb.WriteString("- **Empire scope**: only the five named empires (crimson, nebula, outerrim, solarian, voidborn). POIs in unaligned systems are excluded from per-empire totals (but counted in the galaxy denominator).\n")
	sb.WriteString("- **Presence**: an empire \"has\" a resource if at least one POI in its systems lists richness > 0 for it.\n")
	sb.WriteString("- **Galaxy share**: `sum(richness in empire's POIs) / sum(richness in all named-empire POIs)`. Verdict bands: ≥40% Dominant, 15–40% Sufficient, 1–15% Scarce, 0 Missing.\n")
	sb.WriteString("- **Ease score** for empire E building product P: the minimum galaxy share across P's base materials in E — bottleneck thinking.\n")
	sb.WriteString("- **SSI** (Self-Sufficiency Index, 0–100): `0.5 * (fraction of products E can solo-build) * 100 + 0.5 * (mean ease across those products) * 100`.\n")
	return sb.String()
}
