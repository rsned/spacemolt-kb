package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadShipCatalogParsesItemsWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_ships.json")
	body := `{"items":[
	  {"id":"prayer","name":"Prayer","class":"Freighter","category":"Commercial",
	   "faction":"outerrim","tier":1,"scale":1,"weapon_slots":0,"defense_slots":0,
	   "utility_slots":0,"cargo_capacity":540,"lore":"cargo containers welded to an engine"},
	  {"id":"comet","name":"Comet","class":"Liner","category":"Civilian",
	   "faction":"nebula","tier":4,"scale":4,"weapon_slots":0,"defense_slots":4,
	   "utility_slots":5,"cargo_capacity":40,"lore":"one class of service"}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	ships, err := loadShipCatalog(path)
	if err != nil {
		t.Fatalf("loadShipCatalog: %v", err)
	}
	if len(ships) != 2 {
		t.Fatalf("len = %d, want 2", len(ships))
	}
	if ships[0].ID != "prayer" || ships[0].CargoCapacity != 540 {
		t.Errorf("first ship = %+v", ships[0])
	}
}

func TestToStatsMapsSlotFields(t *testing.T) {
	c := catalogShip{
		ID: "magnate", Name: "Magnate", Class: "Command", Category: "Combat Support",
		Faction: "solarian", Tier: 4, Scale: 4,
		WeaponSlots: 3, DefenseSlots: 6, UtilitySlots: 5, CargoCapacity: 300,
	}
	s := toStats(c)

	if s.Weapon != 3 || s.Defense != 6 || s.Utility != 5 {
		t.Errorf("slots = %d/%d/%d, want 3/6/5", s.Weapon, s.Defense, s.Utility)
	}
	if s.ID != "magnate" || s.Name != "Magnate" || s.Faction != "solarian" {
		t.Errorf("identity fields wrong: %+v", s)
	}
	if s.Cargo != 300 {
		t.Errorf("Cargo = %d, want 300", s.Cargo)
	}
}

func TestLoadShipCatalogMissingFileErrors(t *testing.T) {
	if _, err := loadShipCatalog(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Errorf("expected an error for a missing catalog")
	}
}

func TestValidateCatalogRejectsEmptyID(t *testing.T) {
	ships := []catalogShip{
		{ID: "prayer", Name: "Prayer"},
		{ID: "", Name: "Nameless"},
	}
	err := validateCatalog(ships)
	if err == nil {
		t.Fatalf("expected an error for an empty id")
	}
	if !strings.Contains(err.Error(), "Nameless") {
		t.Errorf("error %q does not name the offending ship", err)
	}
}

func TestValidateCatalogRejectsDuplicateID(t *testing.T) {
	ships := []catalogShip{
		{ID: "comet", Name: "Comet"},
		{ID: "comet", Name: "Comet Mk II"},
	}
	err := validateCatalog(ships)
	if err == nil {
		t.Fatalf("expected an error for a duplicate id")
	}
	if !strings.Contains(err.Error(), "comet") {
		t.Errorf("error %q does not name the offending id", err)
	}
}

func TestValidateCatalogAcceptsCleanCatalog(t *testing.T) {
	ships := []catalogShip{
		{ID: "prayer", Name: "Prayer"},
		{ID: "comet", Name: "Comet"},
	}
	if err := validateCatalog(ships); err != nil {
		t.Errorf("unexpected error for a clean catalog: %v", err)
	}
}

func TestAppendLegacyShipsMergesOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy_ships.json")
	body := `[
	  {"id":"deeprock_harvester","name":"Deeprock Harvester","class":"Mining",
	   "category":"Discontinued","faction":"","tier":3,"scale":3,"weapon_slots":2,
	   "defense_slots":3,"utility_slots":6,"cargo_capacity":400,"legacy":true}
	]`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	base := []catalogShip{{ID: "comet", Name: "Comet"}}
	ships := appendLegacyShips(base, path)
	if len(ships) != 2 {
		t.Fatalf("len = %d, want 2 (catalog + overlay)", len(ships))
	}
	got := ships[1]
	if got.ID != "deeprock_harvester" || got.UtilitySlots != 6 || got.Class != "Mining" {
		t.Errorf("merged ship = %+v", got)
	}
	if !got.Legacy {
		t.Error("merged ship is not marked Legacy, so the contact sheet cannot flag it")
	}
}

// A checkout that has not run scripts/build_legacy.py must still render glyphs,
// the same tolerance kblegacy.Load and the items generator apply.
func TestAppendLegacyShipsMissingFileIsNotFatal(t *testing.T) {
	base := []catalogShip{{ID: "comet", Name: "Comet"}}
	ships := appendLegacyShips(base, filepath.Join(t.TempDir(), "absent.json"))
	if len(ships) != 1 || ships[0].ID != "comet" {
		t.Fatalf("ships = %+v, want the catalog unchanged", ships)
	}
}
