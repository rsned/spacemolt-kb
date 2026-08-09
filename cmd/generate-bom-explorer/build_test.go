package main

import (
	"encoding/json"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

// fixture returns a small but structurally complete world:
//
//	iron_ore (ore, leaf)
//	steel_plate  <- smelt_steel (5 iron_ore -> 2 steel_plate)
//	             <- cast_steel  (6 iron_ore -> 2 steel_plate)   [second recipe]
//	crate        <- wrap_crate (1 steel_plate -> 1 crate)       [packaging, dropped]
func fixture() (map[string]ItemRec, map[string]RecipeRec) {
	items := map[string]ItemRec{
		"iron_ore":    {Name: "Iron Ore", Category: "ore"},
		"steel_plate": {Name: "Steel Plate", Category: "refined"},
		"crate":       {Name: "Crate", Category: "misc"},
	}
	recipes := map[string]RecipeRec{
		"smelt_steel": {
			Name: "Smelt Steel", Category: "Refining",
			Inputs:  [][]any{{"iron_ore", 5}},
			Outputs: [][]any{{"steel_plate", 2}},
		},
		"cast_steel": {
			Name: "Cast Steel", Category: "Refining",
			Inputs:  [][]any{{"iron_ore", 6}},
			Outputs: [][]any{{"steel_plate", 2}},
		},
		"wrap_crate": {
			Name: "Wrap Crate", Category: "Logistics",
			Inputs:  [][]any{{"steel_plate", 1}},
			Outputs: [][]any{{"crate", 1}},
		},
	}
	return items, recipes
}

func TestBuildDocDropsPackagingRecipes(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	if _, ok := doc.Recipes["wrap_crate"]; ok {
		t.Error("wrap_crate present in doc.Recipes; packaging recipes must be dropped")
	}
	if _, ok := doc.Recipes["smelt_steel"]; !ok {
		t.Error("smelt_steel missing from doc.Recipes")
	}
	if len(doc.Recipes) != 2 {
		t.Errorf("len(doc.Recipes) = %d, want 2", len(doc.Recipes))
	}
}

func TestBuildDocKeepsAllItems(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	// Every item stays, including ones only a dropped packaging recipe made.
	// The page needs their display names for tables and node labels.
	if len(doc.Items) != 3 {
		t.Errorf("len(doc.Items) = %d, want 3", len(doc.Items))
	}
	if doc.Items["iron_ore"].Category != "ore" {
		t.Errorf("iron_ore category = %q, want ore", doc.Items["iron_ore"].Category)
	}
}

func TestBuildDocDefaultsOnlyForMultiRecipeItems(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	got, ok := doc.Defaults["steel_plate"]
	if !ok {
		t.Fatal("steel_plate missing from doc.Defaults; it has two recipes")
	}
	if got != "smelt_steel" && got != "cast_steel" {
		t.Errorf("default for steel_plate = %q, want one of smelt_steel/cast_steel", got)
	}
	// crate is produced only by the dropped packaging recipe -> no default.
	if _, ok := doc.Defaults["crate"]; ok {
		t.Error("crate present in doc.Defaults; its only recipe is packaging")
	}
	if len(doc.Defaults) != 1 {
		t.Errorf("len(doc.Defaults) = %d, want 1 (only steel_plate)", len(doc.Defaults))
	}
}

func TestBuildDocTargetsFromShipsAndFacilities(t *testing.T) {
	items, recipes := fixture()
	ships := []catalog.Ship{{
		ID: "absence", Name: "Absence",
		BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 5}},
	}}
	facs := []catalog.Facility{
		{ID: "depot", Name: "Depot", BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 8150.0}}},
		{ID: "empty_pad", Name: "Empty Pad"}, // no build_materials -> omitted
	}

	doc := BuildDoc(items, recipes, ships, facs)

	if len(doc.Targets) != 2 {
		t.Fatalf("len(doc.Targets) = %d, want 2 (empty_pad has no build_materials)", len(doc.Targets))
	}
	if doc.Targets["absence"].Type != "ship" {
		t.Errorf("absence type = %q, want ship", doc.Targets["absence"].Type)
	}
	if doc.Targets["depot"].Type != "facility" {
		t.Errorf("depot type = %q, want facility", doc.Targets["depot"].Type)
	}
	if _, ok := doc.Targets["empty_pad"]; ok {
		t.Error("empty_pad present; targets without build_materials must be omitted")
	}
}

func TestBuildDocConvertsFloatQuantitiesToInt(t *testing.T) {
	items, recipes := fixture()
	facs := []catalog.Facility{{
		ID: "depot", Name: "Depot",
		BuildMaterials: []catalog.Material{{ItemID: "steel_plate", Quantity: 8150.0}},
	}}

	doc := BuildDoc(items, recipes, nil, facs)

	raw, err := json.Marshal(doc.Targets["depot"].BuildMaterials)
	if err != nil {
		t.Fatal(err)
	}
	// Must serialise as an integer, not 8150.0 — the page does integer math.
	if string(raw) != `[["steel_plate",8150]]` {
		t.Errorf("build materials JSON = %s, want [[\"steel_plate\",8150]]", raw)
	}
}

func TestBuildDocSerialisesShortKeys(t *testing.T) {
	items, recipes := fixture()
	doc := BuildDoc(items, recipes, nil, nil)

	raw, err := json.Marshal(doc.Items["iron_ore"])
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"n":"Iron Ore","c":"ore"}` {
		t.Errorf("item JSON = %s, want {\"n\":\"Iron Ore\",\"c\":\"ore\"}", raw)
	}

	raw, err = json.Marshal(doc.Recipes["smelt_steel"])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"n":"Smelt Steel","c":"Refining","i":[["iron_ore",5]],"o":[["steel_plate",2]]}`
	if string(raw) != want {
		t.Errorf("recipe JSON = %s, want %s", raw, want)
	}
}

// TestBuildDocEmptyInputsOutputsSerialiseAsEmptyArrays covers recipes such as
// pack_package/unpack_package (zero rows in recipe_inputs and
// recipe_outputs) and onboard_*_fuel_synthesis (zero rows in
// recipe_outputs — they produce ship fuel via a separate column, not an
// item). loadRecipes leaves RecipeRec.Inputs/Outputs as their nil zero value
// when attachPairs never appends to them; a nil [][]any marshals as JSON
// null, not []. The page expects an iterable array for every recipe's i/o.
func TestBuildDocEmptyInputsOutputsSerialiseAsEmptyArrays(t *testing.T) {
	items, recipes := fixture()
	recipes["pack_package"] = RecipeRec{Name: "Pack Package", Category: "Logistics"}
	doc := BuildDoc(items, recipes, nil, nil)

	raw, err := json.Marshal(doc.Recipes["pack_package"])
	if err != nil {
		t.Fatal(err)
	}
	want := `{"n":"Pack Package","c":"Logistics","i":[],"o":[]}`
	if string(raw) != want {
		t.Errorf("recipe JSON = %s, want %s", raw, want)
	}
}
