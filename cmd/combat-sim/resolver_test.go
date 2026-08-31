package main

import (
	"strings"
	"testing"
)

// The two real fits from battle fixture 7c044558 — resolver output must match
// the participant stat blocks logged by the server.
func broadaxeFit() *FitSpec {
	return &FitSpec{
		Name: "MoltenOne", Hull: "broadaxe",
		Modules: []string{"autocannon_ii", "autocannon_ii", "autocannon_ii", "flak_cannon_ii", "armor_plate_ii"},
		Skills:  map[string]int{"weapons": 7, "gunnery": 10, "shields": 4},
	}
}

func surveyFit() *FitSpec {
	return &FitSpec{
		Name: "Artis", Hull: "survey_vessel",
		Modules: []string{"pulse_laser_iii", "pulse_laser_iii", "shield_booster_iv", "shield_recharger_ii"},
		Skills:  map[string]int{"weapons": 3, "gunnery": 3, "shields": 1},
	}
}

func TestResolveBroadaxe(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Resolve(broadaxeFit(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if sb.MaxHull != 282 || sb.MaxShield != 28 || sb.Recharge != 2 {
		t.Errorf("pools = hull %d shield %d recharge %d, want 282/28/2 (no capacity skill multipliers)", sb.MaxHull, sb.MaxShield, sb.Recharge)
	}
	if sb.ArmorTotal != 28 { // (18+10) × (1 + 0×0.01)
		t.Errorf("ArmorTotal = %v, want 28", sb.ArmorTotal)
	}
	if sb.WeaponSkillPct != 17 || sb.CritPct != 7 || sb.ShieldsSkill != 4 {
		t.Errorf("skills = wsp %d crit %d shres %d, want 17/7/4", sb.WeaponSkillPct, sb.CritPct, sb.ShieldsSkill)
	}
	if len(sb.Weapons) != 4 || sb.Weapons[3].Damage != 28 || sb.Weapons[0].Magazine != 650 {
		t.Errorf("weapons = %+v, want 4 weapons, flak 28, autocannon magazine 650", sb.Weapons)
	}
}

func TestResolveSurvey(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Resolve(surveyFit(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if sb.MaxShield != 400 || sb.Recharge != 9 || sb.ArmorTotal != 14 {
		t.Errorf("shield %d recharge %d armor %v, want 400/9/14", sb.MaxShield, sb.Recharge, sb.ArmorTotal)
	}
}

func TestResolveErrors(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(&FitSpec{Hull: "nope"}, cat); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown hull error = %v, want mention of id", err)
	}
	if _, err := Resolve(&FitSpec{Hull: "broadaxe", Modules: []string{"nope"}}, cat); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown module error = %v, want mention of id", err)
	}
	if _, err := Resolve(&FitSpec{Hull: "opus_magna"}, cat); err == nil || !strings.Contains(err.Error(), "capital") {
		t.Errorf("capital hull error = %v, want capital refusal", err)
	}
}

// Mixed damage types on one fit are unsupported in v1 (ResolveVolley assumes
// a single dmgType per volley) — the resolver must refuse them outright
// rather than silently resolving as one type.
func TestResolveMixedDamageTypeRefused(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	// pulse_laser_iii is energy, autocannon_ii is kinetic; broadaxe has
	// enough weapon slots for both in practice.
	fit := &FitSpec{Hull: "broadaxe", Modules: []string{"pulse_laser_iii", "autocannon_ii"}}
	if _, err := Resolve(fit, cat); err == nil || !strings.Contains(err.Error(), "mixed-damage-type") {
		t.Errorf("mixed-damage-type error = %v, want mixed-damage-type refusal", err)
	}
}

// A damage type outside the six known types must be refused, not silently
// treated as void-like (shieldEff/armorMult are undefined for it, so it
// would zero-value to shield-bypassing behavior).
func TestResolveUnknownDamageTypeRefused(t *testing.T) {
	cat := &Catalog{
		Ships: map[string]*ShipDef{"testhull": {ID: "testhull", BaseHull: 100}},
		Items: map[string]*ItemDef{
			"mystery_gun": {ID: "mystery_gun", Slot: "weapon", Damage: 10, DamageType: "plasma"},
		},
	}
	fit := &FitSpec{Hull: "testhull", Modules: []string{"mystery_gun"}}
	if _, err := Resolve(fit, cat); err == nil || !strings.Contains(err.Error(), "unknown damage type") {
		t.Errorf("unknown damage type error = %v, want unknown damage type refusal", err)
	}
}
