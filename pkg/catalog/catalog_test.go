package catalog

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSnapshot(t *testing.T, root, name, ships, facilities string, mod time.Time) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if ships != "" {
		if err := os.WriteFile(filepath.Join(dir, "catalog_ships.json"), []byte(ships), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if facilities != "" {
		if err := os.WriteFile(filepath.Join(dir, "catalog_facilities.json"), []byte(facilities), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(dir, mod, mod); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestFindLatestDirPicksMostRecentlyModified(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now().Add(-1 * time.Hour)
	// "20260101" sorts after "previous" lexically but is older by mtime.
	writeSnapshot(t, root, "20260101", `{"items":[]}`, `{"items":[]}`, old)
	want := writeSnapshot(t, root, "previous", `{"items":[]}`, `{"items":[]}`, recent)

	got, err := FindLatestDir(root)
	if err != nil {
		t.Fatalf("FindLatestDir: %v", err)
	}
	if got != want {
		t.Errorf("FindLatestDir = %q, want %q", got, want)
	}
}

func TestFindLatestDirErrorsOnEmptyRoot(t *testing.T) {
	if _, err := FindLatestDir(t.TempDir()); err == nil {
		t.Fatal("FindLatestDir on empty root: want error, got nil")
	}
}

func TestLoadShips(t *testing.T) {
	root := t.TempDir()
	writeSnapshot(t, root, "20260807", `{"items":[
	  {"id":"absence","name":"Absence","class":"frigate","price":900,
	   "build_materials":[{"item_id":"steel_plate","quantity":5}]}
	]}`, `{"items":[]}`, time.Now())

	ships, err := LoadShips(root)
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	if len(ships) != 1 {
		t.Fatalf("len(ships) = %d, want 1", len(ships))
	}
	s := ships[0]
	if s.ID != "absence" || s.Name != "Absence" || s.Class != "frigate" || s.Price != 900 {
		t.Errorf("ship = %+v, want id/name/class/price absence/Absence/frigate/900", s)
	}
	if len(s.BuildMaterials) != 1 || s.BuildMaterials[0].ItemID != "steel_plate" || s.BuildMaterials[0].Quantity != 5 {
		t.Errorf("build materials = %+v, want [{steel_plate 5}]", s.BuildMaterials)
	}
}

func TestLoadFacilitiesKeepsFloatQuantities(t *testing.T) {
	root := t.TempDir()
	writeSnapshot(t, root, "20260807", `{"items":[]}`, `{"items":[
	  {"id":"depot","name":"Depot","category":"service","level":2,"recipe_id":"build_depot",
	   "build_materials":[{"item_id":"steel_plate","quantity":8150.0},{"item_id":"hot_cell","quantity":2.5}]}
	]}`, time.Now())

	facs, err := LoadFacilities(root)
	if err != nil {
		t.Fatalf("LoadFacilities: %v", err)
	}
	if len(facs) != 1 {
		t.Fatalf("len(facilities) = %d, want 1", len(facs))
	}
	f := facs[0]
	if f.ID != "depot" || f.Name != "Depot" || f.Category != "service" || f.Level != 2 || f.RecipeID != "build_depot" {
		t.Errorf("facility = %+v, want depot/Depot/service/2/build_depot", f)
	}
	if len(f.BuildMaterials) != 2 || f.BuildMaterials[1].Quantity != 2.5 {
		t.Errorf("build materials = %+v, want second quantity 2.5 preserved", f.BuildMaterials)
	}
}

func writeCatalogFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "20261018")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLoadFacilities_UpkeepFields(t *testing.T) {
	root := writeCatalogFiles(t, map[string]string{"catalog_facilities.json": `{"items":[
		{"id":"atmospheric_recycler","name":"Atmospheric Recycler","category":"infrastructure","level":1,
		 "description":"Air.","build_cost":600000,"build_time":150,
		 "build_materials":[{"item_id":"steel_plate","quantity":1200},{"item_id":"water_recycler","quantity":200}],
		 "maintenance_inputs":[{"item_id":"sunspindle","quantity":1},{"item_id":"water_ice","quantity":3},{"item_id":"purified_water","quantity":1},{"item_id":"oxygen_gas","quantity":2},{"item_id":"catalytic_scrubber_cartridge","quantity":1}],
		 "life_support_supply":12},
		{"id":"bunker_fed_reactor","name":"Bunker-Fed Reactor","category":"infrastructure","level":1,
		 "build_cost":500000,"maintenance_fuel":55,"power_supply":12},
		{"id":"scriptorium","name":"Scriptorium","category":"production","level":1,
		 "power_draw":6,"life_support_draw":1,"service_type":"crafting"}]}`})
	facs, err := LoadFacilities(root)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Facility{}
	for _, f := range facs {
		by[f.ID] = f
	}
	r := by["atmospheric_recycler"]
	if r.BuildCost != 600000 || r.BuildTime != 150 || r.LifeSupportSupply != 12 || r.Description != "Air." {
		t.Errorf("recycler = %+v", r)
	}
	if len(r.MaintenanceInputs) != 5 || r.MaintenanceInputs[1].ItemID != "water_ice" || r.MaintenanceInputs[1].Quantity != 3 {
		t.Errorf("recycler upkeep = %+v", r.MaintenanceInputs)
	}
	b := by["bunker_fed_reactor"]
	if b.MaintenanceFuel != 55 || b.PowerSupply != 12 {
		t.Errorf("reactor = %+v", b)
	}
	s := by["scriptorium"]
	if s.PowerDraw != 6 || s.LifeSupportDraw != 1 || s.ServiceType != "crafting" {
		t.Errorf("scriptorium = %+v", s)
	}
}

func TestLoadItems(t *testing.T) {
	root := writeCatalogFiles(t, map[string]string{"catalog_items.json": `{"items":[
		{"id":"flex_polymer","name":"Flex Polymer","category":"refined"}]}`})
	items, err := LoadItems(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0] != (Item{ID: "flex_polymer", Name: "Flex Polymer", Category: "refined"}) {
		t.Errorf("items = %+v", items)
	}
}
