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
