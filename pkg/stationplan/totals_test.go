package stationplan

import (
	"math"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/catalog"
)

func m(id string, q float64) catalog.Material { return catalog.Material{ItemID: id, Quantity: q} }

// testFacilities carries the real 2026-10-18 catalog values for the station
// build order, plus a power-drawing fixture.
func testFacilities() map[string]catalog.Facility {
	fs := []catalog.Facility{
		{ID: "locker_bay", Category: "service", BuildCost: 500000,
			BuildMaterials:    []catalog.Material{m("steel_plate", 1600)},
			MaintenanceInputs: []catalog.Material{m("steel_plate", 1), m("flex_polymer", 1)}},
		{ID: "faction_workshop", Category: "service", BuildCost: 500000,
			BuildMaterials:    []catalog.Material{m("steel_plate", 800), m("circuit_board", 400)},
			MaintenanceInputs: []catalog.Material{m("circuit_board", 1), m("focused_crystal", 1)}},
		{ID: "atmospheric_recycler", Category: "infrastructure", BuildCost: 600000,
			BuildMaterials: []catalog.Material{m("steel_plate", 1200), m("water_recycler", 200)},
			MaintenanceInputs: []catalog.Material{m("sunspindle", 1), m("water_ice", 3), m("purified_water", 1),
				m("oxygen_gas", 2), m("catalytic_scrubber_cartridge", 1)},
			LifeSupportSupply: 12},
		{ID: "bunker_fed_reactor", Category: "infrastructure", BuildCost: 500000, MaintenanceFuel: 55, PowerSupply: 12},
		{ID: "stellar_collector_grid", Category: "infrastructure", BuildCost: 900000,
			MaintenanceInputs: []catalog.Material{m("circuit_board", 2)}, PowerSupply: 20},
		{ID: "power_hog", Category: "production", PowerDraw: 6, LifeSupportDraw: 1},
	}
	out := map[string]catalog.Facility{}
	for _, f := range fs {
		out[f.ID] = f
	}
	return out
}

func TestPerDay(t *testing.T) {
	cases := []struct {
		perCycle, cycleTicks int
		want                 float64
	}{
		{1, 3000, 2.88}, // station maintenance cycle (player-reported ~3,000 ticks)
		{3, 3000, 8.64},
		{1, 100, 86.4}, // rent cycle
		{55, 100, 4752},
	}
	for _, c := range cases {
		if got := PerDay(c.perCycle, c.cycleTicks); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("PerDay(%d, %d) = %v, want %v", c.perCycle, c.cycleTicks, got, c.want)
		}
	}
	if got := CyclesPerDay(3000); math.Abs(got-2.88) > 1e-9 {
		t.Errorf("CyclesPerDay(3000) = %v, want 2.88", got)
	}
}

func TestAccumulate_StationBuildOrder(t *testing.T) {
	tot, err := Accumulate(testFacilities(), []StepInput{
		{ExtraCredits: 5000000},
		{FacilityID: "locker_bay"},
		{FacilityID: "faction_workshop"},
		{FacilityID: "atmospheric_recycler"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tot) != 4 {
		t.Fatalf("got %d totals, want 4", len(tot))
	}
	last := tot[3]
	if last.Credits != 6600000 {
		t.Errorf("Credits = %d, want 6,600,000", last.Credits)
	}
	wantMat := map[string]int{"steel_plate": 3600, "circuit_board": 400, "water_recycler": 200}
	for k, v := range wantMat {
		if last.Materials[k] != v {
			t.Errorf("Materials[%s] = %d, want %d", k, last.Materials[k], v)
		}
	}
	wantUp := map[string]int{"steel_plate": 1, "flex_polymer": 1, "circuit_board": 1, "focused_crystal": 1,
		"sunspindle": 1, "water_ice": 3, "purified_water": 1, "oxygen_gas": 2, "catalytic_scrubber_cartridge": 1}
	if len(last.UpkeepPerCycle) != len(wantUp) {
		t.Errorf("UpkeepPerCycle = %v", last.UpkeepPerCycle)
	}
	for k, v := range wantUp {
		if last.UpkeepPerCycle[k] != v {
			t.Errorf("UpkeepPerCycle[%s] = %d, want %d", k, last.UpkeepPerCycle[k], v)
		}
	}
	if last.LifeSupportSupply != 12 {
		t.Errorf("LifeSupportSupply = %d, want 12", last.LifeSupportSupply)
	}
	if tot[0].Credits != 5000000 || len(tot[0].UpkeepPerCycle) != 0 {
		t.Errorf("step 0 = %+v", tot[0])
	}
}

func TestAccumulate_NonFacilityStepCarriesTotals(t *testing.T) {
	tot, err := Accumulate(testFacilities(), []StepInput{{FacilityID: "locker_bay"}, {}})
	if err != nil {
		t.Fatal(err)
	}
	if tot[1].Credits != tot[0].Credits || tot[1].UpkeepPerCycle["flex_polymer"] != 1 || tot[1].Materials["steel_plate"] != 1600 {
		t.Errorf("non-facility step changed totals: %+v vs %+v", tot[1], tot[0])
	}
}

func TestAccumulate_FuelIsAnItem(t *testing.T) {
	tot, err := Accumulate(testFacilities(), []StepInput{{FacilityID: "bunker_fed_reactor"}})
	if err != nil {
		t.Fatal(err)
	}
	if tot[0].UpkeepPerCycle["fuel"] != 55 || tot[0].PowerSupply != 12 {
		t.Errorf("reactor totals = %+v", tot[0])
	}
}

func TestAccumulate_DeficitFlag(t *testing.T) {
	tot, err := Accumulate(testFacilities(), []StepInput{{FacilityID: "power_hog"}, {FacilityID: "stellar_collector_grid"}})
	if err != nil {
		t.Fatal(err)
	}
	if !tot[0].PowerDeficit() || !tot[0].LifeSupportDeficit() {
		t.Errorf("step 0 should be in deficit: %+v", tot[0])
	}
	if tot[1].PowerDeficit() {
		t.Errorf("collector grid should cover the draw: %+v", tot[1])
	}
}

func TestAccumulate_UnknownFacility(t *testing.T) {
	_, err := Accumulate(testFacilities(), []StepInput{{FacilityID: "no_such_facility"}})
	if err == nil || !strings.Contains(err.Error(), "no_such_facility") {
		t.Errorf("err = %v", err)
	}
}

func TestAccumulate_TotalsIndependent(t *testing.T) {
	tot, err := Accumulate(testFacilities(), []StepInput{{FacilityID: "locker_bay"}, {FacilityID: "faction_workshop"}})
	if err != nil {
		t.Fatal(err)
	}
	tot[0].Materials["steel_plate"] = 0
	tot[0].UpkeepPerCycle["flex_polymer"] = 0
	if tot[1].Materials["steel_plate"] != 2400 || tot[1].UpkeepPerCycle["flex_polymer"] != 1 {
		t.Errorf("totals share maps: %+v", tot[1])
	}
}
