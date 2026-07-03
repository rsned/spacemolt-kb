package main

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/rsned/spacemolt-kb/pkg/bom"
)

// TestBOMIntegration_RoundTrip exercises the cmd-level pipeline end-to-end:
// Migrate → CalculateAll → loadBoMFromDB → in-memory struct attachment. It
// uses an in-memory SQLite with a small handcrafted dataset rather than the
// production crafting DB, so it runs in CI and pins the loadBoMFromDB
// behavior alongside the calculator.
func TestBOMIntegration_RoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if err := bom.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Tiny dataset:
	//   iron_ore (ore, base)
	//   steel_plate: recipe consumes 5 iron_ore -> outputs 2 steel_plate
	//     -> 1 steel_plate needs ceil(5/2) = 3 iron_ore
	//   durasteel_plate: recipe consumes 4 steel_plate + 2 tungsten_ore
	//                    -> outputs 2 durasteel_plate
	//     -> 1 durasteel_plate needs ceil(4/2)=2 steel_plate => 2*3=6 iron_ore,
	//        plus ceil(2/2)=1 tungsten_ore (tungsten_ore is unknown -> base)
	//
	// Wraps to drive the cmd-level types into the bom-level types.
	cmdItems := map[string]*Item{
		"iron_ore":        {ID: "iron_ore", Name: "Iron Ore", Category: "ore"},
		"steel_plate":     {ID: "steel_plate", Name: "Steel Plate", Category: "refined"},
		"durasteel_plate": {ID: "durasteel_plate", Name: "Durasteel Plate", Category: "component"},
	}
	bomItems := make(map[string]*bom.Item, len(cmdItems))
	for id, it := range cmdItems {
		bomItems[id] = &bom.Item{ID: it.ID, Name: it.Name, Category: it.Category}
	}

	bomRecipes := map[string]*bom.Recipe{
		"refine_steel": {
			ID:      "refine_steel",
			Inputs:  []bom.RecipeItem{{ItemID: "iron_ore", Quantity: 5}},
			Outputs: []bom.RecipeItem{{ItemID: "steel_plate", Quantity: 2}},
		},
		"forge_durasteel": {
			ID: "forge_durasteel",
			Inputs: []bom.RecipeItem{
				{ItemID: "steel_plate", Quantity: 4},
				{ItemID: "tungsten_ore", Quantity: 2},
			},
			Outputs: []bom.RecipeItem{{ItemID: "durasteel_plate", Quantity: 2}},
		},
	}

	calc, err := bom.NewCalculator(db, bomRecipes, bomItems)
	if err != nil {
		t.Fatalf("new calculator: %v", err)
	}

	bomShips := map[string]*bom.Ship{
		"test_ship": {
			ID:   "test_ship",
			Name: "Test Ship",
			BuildMaterials: []bom.ShipBuildRef{
				{ItemID: "durasteel_plate", Quantity: 3},
			},
		},
	}
	bomFacilities := map[string]*bom.Facility{
		"test_facility": {
			ID:   "test_facility",
			Name: "Test Facility",
			BuildMaterials: []bom.FacilityMaterial{
				{ItemID: "steel_plate", Quantity: 10},
			},
		},
	}

	if err := calc.CalculateAll(bomItems, bomShips, bomFacilities); err != nil {
		t.Fatalf("calculate all: %v", err)
	}

	// Round-trip through loadBoMFromDB onto cmd-level structs.
	cmdShips := []*Ship{{ID: "test_ship", Name: "Test Ship"}}
	cmdFacilities := []*Facility{{ID: "test_facility", Name: "Test Facility"}}

	if err := loadBoMFromDB(db, cmdItems, cmdShips, cmdFacilities); err != nil {
		t.Fatalf("loadBoMFromDB: %v", err)
	}

	// Assert per-1-unit item BoM (ceil arithmetic).
	steel := cmdItems["steel_plate"].BoM
	if steel == nil {
		t.Fatal("steel_plate.BoM is nil")
	}
	if got := materialQty(steel, "iron_ore"); got != 3 {
		t.Errorf("steel_plate iron_ore: got %d, want 3 (ceil(5/2))", got)
	}

	dura := cmdItems["durasteel_plate"].BoM
	if dura == nil {
		t.Fatal("durasteel_plate.BoM is nil")
	}
	if got := materialQty(dura, "iron_ore"); got != 6 {
		t.Errorf("durasteel_plate iron_ore: got %d, want 6", got)
	}
	if got := materialQty(dura, "tungsten_ore"); got != 1 {
		t.Errorf("durasteel_plate tungsten_ore: got %d, want 1 (unknown -> base)", got)
	}

	// Ship: 3 durasteel_plate -> 18 iron_ore, 3 tungsten_ore.
	ship := cmdShips[0].BoM
	if ship == nil {
		t.Fatal("test_ship.BoM is nil")
	}
	if got := materialQty(ship, "iron_ore"); got != 18 {
		t.Errorf("test_ship iron_ore: got %d, want 18", got)
	}
	if got := materialQty(ship, "tungsten_ore"); got != 3 {
		t.Errorf("test_ship tungsten_ore: got %d, want 3", got)
	}

	// Facility: 10 steel_plate -> 30 iron_ore.
	fac := cmdFacilities[0].BoM
	if fac == nil {
		t.Fatal("test_facility.BoM is nil")
	}
	if got := materialQty(fac, "iron_ore"); got != 30 {
		t.Errorf("test_facility iron_ore: got %d, want 30", got)
	}

	// JSON output sanity check.
	want := `[{"item_id":"iron_ore","quantity":3}]`
	if got := steel.JSON(); got != want {
		t.Errorf("steel_plate JSON: got %q, want %q", got, want)
	}
}

func materialQty(r *bom.BoMResult, itemID string) int {
	for _, m := range r.BaseMaterials {
		if m.ItemID == itemID {
			return m.Quantity
		}
	}
	return -1
}
