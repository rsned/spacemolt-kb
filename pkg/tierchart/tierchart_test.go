package tierchart

import (
	"reflect"
	"testing"
)

func miningTier(id, name string, value, power, cpu, pwr int) TierStats {
	return TierStats{
		ItemID:    id,
		ItemName:  name,
		Category:  "mining",
		BaseValue: value,
		Stats:     map[string]int{"mining_power": power, "cpu": cpu, "power": pwr},
	}
}

func TestBuildFamiliesGroupsAndSorts(t *testing.T) {
	stats := []TierStats{
		miningTier("mining_laser_iii", "Mining Laser III", 7900, 22, 6, 12),
		miningTier("mining_laser_i", "Mining Laser I", 1600, 5, 2, 5),
		miningTier("mining_laser_ii", "Mining Laser II", 2500, 12, 4, 8),
		// Singleton family must be dropped.
		miningTier("hyper_mining_laser_iii", "Hyper Mining Laser III", 33000, 35, 8, 16),
		// Non-tiered item must be ignored.
		{ItemID: "laser_focus_array", ItemName: "Laser Focus Array", Category: "component"},
	}

	families := BuildFamilies(stats)
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d: %+v", len(families), families)
	}

	fam := families[0]
	if fam.BaseName != "mining_laser" {
		t.Errorf("base name = %q, want mining_laser", fam.BaseName)
	}
	if fam.DisplayName != "Mining Laser" {
		t.Errorf("display name = %q, want Mining Laser", fam.DisplayName)
	}

	gotTiers := make([]string, len(fam.Tiers))
	for i, tier := range fam.Tiers {
		gotTiers[i] = tier.Tier
	}
	if want := []string{"I", "II", "III"}; !reflect.DeepEqual(gotTiers, want) {
		t.Errorf("tier order = %v, want %v", gotTiers, want)
	}
}

func TestColumnsRelevanceAndOrder(t *testing.T) {
	// Pulse laser family: damage, range, cpu, power, cooldown are non-zero.
	// mining_power is always zero and must be suppressed.
	stats := []TierStats{
		{
			ItemID: "pulse_laser_i", Category: "weapon", BaseValue: 2500, DamageType: "energy",
			Stats: map[string]int{"damage": 10, "range": 10, "cpu": 2, "power": 5, "cooldown": 1},
		},
		{
			ItemID: "pulse_laser_ii", Category: "weapon", BaseValue: 6900, DamageType: "energy",
			Stats: map[string]int{"damage": 18, "range": 12, "cpu": 3, "power": 8, "cooldown": 1},
		},
	}

	fam := BuildFamilies(stats)[0]
	cols := fam.Columns()
	want := []string{"damage", "range", "cpu", "power", "cooldown"}
	if !reflect.DeepEqual(cols, want) {
		t.Errorf("columns = %v, want %v", cols, want)
	}
}

func TestHasSkillReq(t *testing.T) {
	withSkill := []TierStats{
		{ItemID: "cloaking_device_i", Category: "utility", Stats: map[string]int{"cpu": 5}},
		{ItemID: "cloaking_device_ii", Category: "utility", SkillReq: "Stealth 3", Stats: map[string]int{"cpu": 8}},
	}
	if fam := BuildFamilies(withSkill)[0]; !fam.HasSkillReq() {
		t.Error("HasSkillReq() = false, want true when a tier has a requirement")
	}

	noSkill := []TierStats{
		{ItemID: "armor_plate_i", Category: "defense", Stats: map[string]int{"armor": 10}},
		{ItemID: "armor_plate_ii", Category: "defense", Stats: map[string]int{"armor": 20}},
	}
	if fam := BuildFamilies(noSkill)[0]; fam.HasSkillReq() {
		t.Error("HasSkillReq() = true, want false when no tier has a requirement")
	}
}

func TestDisplayDamageType(t *testing.T) {
	ts := TierStats{DamageType: "energy", Stats: map[string]int{"damage": 28, "cpu": 5}}
	if got := ts.Display("damage"); got != "28 energy" {
		t.Errorf("damage display = %q, want %q", got, "28 energy")
	}
	if got := ts.Display("cpu"); got != "5" {
		t.Errorf("cpu display = %q, want %q", got, "5")
	}
}
