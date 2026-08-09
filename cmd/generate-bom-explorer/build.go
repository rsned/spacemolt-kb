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
		doc.Recipes[id] = r
	}

	doc.Defaults = computeDefaults(recipes, doc.Recipes)

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
// Selection runs through bom.SelectRecipe over the FULL recipe set, packaging
// included, because SelectRecipe's own first filtering layer drops packaging.
// Going through it rather than reimplementing the choice is what guarantees
// the explorer opens on the same recipe path the static per-target pages show.
func computeDefaults(all, kept map[string]RecipeRec) map[string]string {
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
		chosen := bom.SelectRecipe(itemToRecipes, item)
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
