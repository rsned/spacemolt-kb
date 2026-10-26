package main

import "testing"

func TestLoadUpkeepConsumers(t *testing.T) {
	items := map[string]*Item{
		"catalytic_scrubber_cartridge": {ID: "catalytic_scrubber_cartridge"},
		"water_ice":                    {ID: "water_ice"},
		"fuel":                         {ID: "fuel"},
		"steel_plate":                  {ID: "steel_plate"},
	}
	facilities := map[string]*Facility{
		"life_support_system": {ID: "life_support_system", Name: "Life Support System", Category: "infrastructure",
			MaintenancePerCycle: []MaterialRef{{ItemID: "catalytic_scrubber_cartridge", Quantity: 2}}},
		"atmospheric_recycler": {ID: "atmospheric_recycler", Name: "Atmospheric Recycler", Category: "infrastructure",
			BuildMaterials:      []MaterialRef{{ItemID: "steel_plate", Quantity: 1200}},
			MaintenancePerCycle: []MaterialRef{{ItemID: "catalytic_scrubber_cartridge", Quantity: 1}, {ItemID: "water_ice", Quantity: 3}}},
		"bunker_fed_reactor": {ID: "bunker_fed_reactor", Name: "Bunker-Fed Reactor", Category: "infrastructure", MaintenanceFuel: 55},
		"ghost_consumer":     {ID: "ghost_consumer", Name: "Ghost", Category: "service", MaintenancePerCycle: []MaterialRef{{ItemID: "no_such_item", Quantity: 1}}},
	}
	loadUpkeepConsumers(facilities, items)

	got := items["catalytic_scrubber_cartridge"].ConsumedBy
	if len(got) != 2 || got[0].FacilityName != "Atmospheric Recycler" || got[0].Quantity != 1 ||
		got[1].FacilityName != "Life Support System" || got[1].Quantity != 2 || got[1].FacilityCategory != "infrastructure" {
		t.Errorf("scrubber consumers = %+v, want Atmospheric Recycler ×1 then Life Support System ×2", got)
	}
	if f := items["fuel"].ConsumedBy; len(f) != 1 || f[0].FacilityID != "bunker_fed_reactor" || f[0].Quantity != 55 {
		t.Errorf("fuel consumers = %+v, want the bunker reactor's maintenance_fuel 55", f)
	}
	if c := items["steel_plate"].ConsumedBy; len(c) != 0 {
		t.Errorf("a build material is not upkeep: steel_plate consumers = %+v", c)
	}
}
