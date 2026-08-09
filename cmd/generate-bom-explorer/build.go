package main

import (
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/bom"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// ItemRec is one entry of the "items" map: display name and catalog category.
type ItemRec struct {
	Name     string `json:"n"`
	Category string `json:"c"`
}

// RecipeRec is one entry of the "recipes" map. Inputs and Outputs are
// [item_id, quantity] pairs, kept as [][]any so they serialise as compact
// arrays rather than objects — the file ships ~750 of these.
type RecipeRec struct {
	Name     string  `json:"n"`
	Category string  `json:"c"`
	Inputs   [][]any `json:"i"`
	Outputs  [][]any `json:"o"`
}

// TargetRec is one entry of the "targets" map: a ship or facility. These are
// sinks — they consume items but nothing consumes them — so they only ever
// occupy the rightmost column of the graph.
type TargetRec struct {
	Name           string  `json:"n"`
	Type           string  `json:"t"` // "ship" or "facility"
	BuildMaterials [][]any `json:"bm"`
}

// Doc is the generated recipe-graph.json document.
type Doc struct {
	Items    map[string]ItemRec   `json:"items"`
	Recipes  map[string]RecipeRec `json:"recipes"`
	Targets  map[string]TargetRec `json:"targets"`
	Defaults map[string]string    `json:"defaults"`
}

// isPackaging reports whether a recipe id is one of the wrap_/unwrap_ pairs
// that form X <-> contained_X cycles in the source data. They are never a
// legitimate production path, and dropping them here makes those cycles
// unreachable from the page rather than something it must filter.
func isPackaging(recipeID string) bool {
	return strings.HasPrefix(recipeID, "wrap_") || strings.HasPrefix(recipeID, "unwrap_")
}

// BuildDoc transforms loaded crafting rows and snapshot catalogs into the
// document written to recipe-graph.json.
func BuildDoc(items map[string]ItemRec, recipes map[string]RecipeRec, ships []catalog.Ship, facs []catalog.Facility) Doc {
	doc := Doc{
		Items:    items,
		Recipes:  make(map[string]RecipeRec, len(recipes)),
		Targets:  make(map[string]TargetRec, len(ships)+len(facs)),
		Defaults: make(map[string]string),
	}

	for id, r := range recipes {
		if isPackaging(id) {
			continue
		}
		// A recipe with zero DB rows in recipe_inputs or recipe_outputs (e.g.
		// pack_package/unpack_package, or onboard_*_fuel_synthesis which
		// produces fuel via a separate column rather than an item output)
		// leaves these nil. A nil [][]any marshals as JSON null; the page
		// expects an iterable array for every recipe's i/o.
		if r.Inputs == nil {
			r.Inputs = [][]any{}
		}
		if r.Outputs == nil {
			r.Outputs = [][]any{}
		}
		doc.Recipes[id] = r
	}

	doc.Defaults = computeDefaults(items, recipes, doc.Recipes)

	for _, s := range ships {
		if len(s.BuildMaterials) == 0 {
			continue
		}
		doc.Targets[s.ID] = TargetRec{Name: s.Name, Type: "ship", BuildMaterials: materialPairs(s.BuildMaterials)}
	}
	for _, f := range facs {
		if len(f.BuildMaterials) == 0 {
			continue
		}
		doc.Targets[f.ID] = TargetRec{Name: f.Name, Type: "facility", BuildMaterials: materialPairs(f.BuildMaterials)}
	}

	return doc
}

// materialPairs converts catalog materials to [item_id, quantity] pairs,
// truncating the float quantities the facility catalog uses to int so the
// page does integer arithmetic throughout. Order follows the catalog.
func materialPairs(ms []catalog.Material) [][]any {
	out := make([][]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, []any{m.ItemID, int(m.Quantity)})
	}
	return out
}

// computeDefaults returns the default recipe for each item that more than one
// non-packaging recipe produces.
//
// Selection runs through bom.SelectRecipeSourceable over the FULL recipe set,
// packaging included, because selection's own first filtering layer drops
// packaging. Unlike the static per-target build-cost pages, it prefers
// candidates by STRUCTURAL obtainability (ore/material, or built from
// obtainable inputs) rather than live market availability — no market.db
// read, so regeneration needs only crafting.db and stays sub-second.
//
// This deliberately does not always match the static pages' recipe choice:
// those resolve ties with live market data, which can prefer a recipe that
// happens to route through a mob-drop item (e.g. gold_bar via
// gilded_chitin_smelting) over a mineable one (mint_gold_bar, from gold_ore)
// merely because the drop was for sale at capture time. For a page whose job
// is "what do I gather", the structurally-obtainable path is the better
// default. See docs/superpowers/specs/2026-08-08-bom-explorer-design.md,
// "Cross-check against the existing tables".
func computeDefaults(items map[string]ItemRec, all, kept map[string]RecipeRec) map[string]string {
	bomRecipes := make(map[string]*bom.Recipe, len(all))
	for id, r := range all {
		bomRecipes[id] = &bom.Recipe{ID: id, Inputs: toRecipeItems(r.Inputs), Outputs: toRecipeItems(r.Outputs)}
	}
	itemToRecipes, err := bom.BuildRecipeMaps(bomRecipes)
	if err != nil {
		// BuildRecipeMaps only errors on malformed input, which cannot happen
		// for rows read straight out of the schema-constrained tables.
		panic("bom.BuildRecipeMaps: " + err.Error())
	}

	obtainable := computeObtainable(items, itemToRecipes)

	// Count producers among the KEPT recipes: an item made only by packaging
	// has no choice to offer.
	producers := map[string][]string{}
	for id, r := range kept {
		for _, o := range r.Outputs {
			item, _ := o[0].(string)
			producers[item] = append(producers[item], id)
		}
	}

	defaults := make(map[string]string)
	for item, ids := range producers {
		if len(ids) < 2 {
			continue
		}
		chosen := bom.SelectRecipeSourceable(itemToRecipes, item, obtainable)
		if chosen == nil {
			// No structural preference; fall back to the lexically smallest id
			// so regeneration stays deterministic.
			sort.Strings(ids)
			defaults[item] = ids[0]
			continue
		}
		defaults[item] = chosen.ID
	}
	return defaults
}

// computeObtainable returns the structural, no-market-data fixed point of
// "obtainable": an item is obtainable if its catalog category is ore or
// material, or some non-packaging recipe produces it entirely from
// obtainable inputs. It is the "available" set fed to bom.ComputeSourceable,
// with the flattener's own terminal rule (isTerminal in crosscheck_test.go,
// pkg/bom.Calculator.isTerminal) as the matching terminal predicate, so a
// dead-end item — no recipe produces it and it isn't ore/material — never
// becomes obtainable no matter how many other items reference it.
func computeObtainable(items map[string]ItemRec, itemToRecipes map[string][]*bom.Recipe) map[string]bool {
	raw := make(map[string]bool, len(items))
	for id, it := range items {
		if it.Category == "ore" || it.Category == "material" {
			raw[id] = true
		}
	}
	isTerminal := func(id string) bool {
		if it, known := items[id]; known && (it.Category == "ore" || it.Category == "material") {
			return true
		}
		return len(itemToRecipes[id]) == 0
	}
	return bom.ComputeSourceable(itemToRecipes, raw, isTerminal)
}

// toRecipeItems converts [item_id, quantity] pairs to pkg/bom's representation.
func toRecipeItems(pairs [][]any) []bom.RecipeItem {
	out := make([]bom.RecipeItem, 0, len(pairs))
	for _, p := range pairs {
		id, _ := p[0].(string)
		qty, _ := p[1].(int)
		out = append(out, bom.RecipeItem{ItemID: id, Quantity: qty})
	}
	return out
}

// MultiYieldItems returns the items whose default recipe yields more than one
// unit per batch. Any chain containing one of these produces different totals
// under whole-batch rounding than the per-unit arithmetic the static
// bill_of_materials table uses — expected, and worth reporting on each run so
// the divergence is never silent.
func MultiYieldItems(doc Doc) []string {
	var out []string
	for item, recipeID := range doc.Defaults {
		for _, o := range doc.Recipes[recipeID].Outputs {
			id, _ := o[0].(string)
			qty, _ := o[1].(int)
			if id == item && qty > 1 {
				out = append(out, item)
			}
		}
	}
	sort.Strings(out)
	return out
}

// RecipeChoiceDivergence reports how many committed bill_of_materials targets
// reach a different SET of base items than the explorer's structural
// defaults do. A different base-item set means some recipe choice differs
// somewhere on the chain — the deliberate consequence of computeDefaults
// preferring structurally-obtainable inputs over the committed table's
// market-driven picks (see the doc comment on computeDefaults). Targets that
// reach the same set of base items but different quantities are the separate,
// expected multi-yield batching divergence MultiYieldItems already reports,
// and are not counted here.
func RecipeChoiceDivergence(doc Doc, committed map[string]map[string]int) int {
	count := 0
	for target, want := range committed {
		leaves, ok := flattenLeaves(doc, target)
		if !ok {
			continue
		}
		if !sameKeys(leaves, want) {
			count++
		}
	}
	return count
}

// flattenLeaves expands one unit of target through doc.Defaults, returning
// the set of base item ids reached. Recipe yield only determines how many
// units are needed, never which items are visited, so this is valid for
// comparing which base items a chain reaches even though it ignores the
// explorer's whole-batch rounding — the same terminal rule as
// crosscheck_test.go's flattenSingleYield decides where the walk stops.
func flattenLeaves(doc Doc, target string) (map[string]bool, bool) {
	producers := map[string][]string{}
	for id, r := range doc.Recipes {
		for _, o := range r.Outputs {
			item, _ := o[0].(string)
			producers[item] = append(producers[item], id)
		}
	}
	for _, ids := range producers {
		sort.Strings(ids)
	}

	leaves := map[string]bool{}
	ok := true

	var walk func(id string, depth int, stack map[string]bool)
	walk = func(id string, depth int, stack map[string]bool) {
		if depth > 32 || stack[id] {
			ok = false
			return
		}
		item, known := doc.Items[id]
		if known && (item.Category == "ore" || item.Category == "material") {
			leaves[id] = true
			return
		}
		ids := producers[id]
		if len(ids) == 0 {
			leaves[id] = true
			return
		}
		chosen := doc.Defaults[id]
		if chosen == "" {
			chosen = ids[0]
		}
		recipe := doc.Recipes[chosen]
		next := map[string]bool{id: true}
		for k := range stack {
			next[k] = true
		}
		for _, in := range recipe.Inputs {
			iid, _ := in[0].(string)
			walk(iid, depth+1, next)
		}
	}

	if len(producers[target]) == 0 {
		return nil, false
	}
	walk(target, 0, map[string]bool{})
	return leaves, ok
}

// sameKeys reports whether a and b's key sets are identical, ignoring values.
func sameKeys(a map[string]bool, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}
