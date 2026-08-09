// Command generate-bom-explorer writes the recipe graph that the interactive
// Bill of Materials explorer page loads. It reads only crafting.db and the
// newest game-API snapshot catalogs — no market data — so the output never
// goes stale against market captures.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
	_ "modernc.org/sqlite"
)

func main() {
	craftPath := flag.String("crafting", "crafting.db", "crafting DB")
	catalogRoot := flag.String("catalog", "data/snapshots", "game-api snapshot catalog root")
	out := flag.String("out", "kb/build-costs/recipe-graph.json", "output JSON path")
	flag.Parse()

	craftDB, err := sql.Open("sqlite", "file:"+*craftPath+"?mode=ro")
	must(err, "open crafting")
	defer func() { _ = craftDB.Close() }()

	items, err := loadItems(craftDB)
	must(err, "load items")
	recipes, err := loadRecipes(craftDB)
	must(err, "load recipes")
	ships, err := catalog.LoadShips(*catalogRoot)
	must(err, "load ships")
	facs, err := catalog.LoadFacilities(*catalogRoot)
	must(err, "load facilities")

	doc := BuildDoc(items, recipes, ships, facs)

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		must(err, "create output dir")
	}
	blob, err := json.Marshal(doc)
	must(err, "marshal")
	must(os.WriteFile(*out, blob, 0o644), "write")

	log.Printf("bom-explorer: %d items, %d recipes, %d targets, %d defaults, %d bytes → %s",
		len(doc.Items), len(doc.Recipes), len(doc.Targets), len(doc.Defaults), len(blob), *out)
}

// loadItems reads every row of the items table.
func loadItems(db *sql.DB) (map[string]ItemRec, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(category,'') FROM items`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]ItemRec{}
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, err
		}
		out[id] = ItemRec{Name: name, Category: cat}
	}
	return out, rows.Err()
}

// loadRecipes reads every recipe with its inputs and outputs. Input and output
// pairs are ordered by item id so regeneration is byte-identical run to run.
func loadRecipes(db *sql.DB) (map[string]RecipeRec, error) {
	rows, err := db.Query(`SELECT id, name, COALESCE(category,'') FROM recipes`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]RecipeRec{}
	for rows.Next() {
		var id, name, cat string
		if err := rows.Scan(&id, &name, &cat); err != nil {
			return nil, err
		}
		out[id] = RecipeRec{Name: name, Category: cat}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := attachPairs(db, `SELECT recipe_id, item_id, quantity FROM recipe_inputs ORDER BY recipe_id, item_id`,
		out, func(r *RecipeRec, p []any) { r.Inputs = append(r.Inputs, p) }); err != nil {
		return nil, err
	}
	return out, attachPairs(db, `SELECT recipe_id, item_id, quantity FROM recipe_outputs ORDER BY recipe_id, item_id`,
		out, func(r *RecipeRec, p []any) { r.Outputs = append(r.Outputs, p) })
}

func attachPairs(db *sql.DB, query string, out map[string]RecipeRec, add func(*RecipeRec, []any)) error {
	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var rid, iid string
		var qty int
		if err := rows.Scan(&rid, &iid, &qty); err != nil {
			return err
		}
		rec, ok := out[rid]
		if !ok {
			continue
		}
		add(&rec, []any{iid, qty})
		out[rid] = rec
	}
	return rows.Err()
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", what, err)
		os.Exit(1)
	}
}
