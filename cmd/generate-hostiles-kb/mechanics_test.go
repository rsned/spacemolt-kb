package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A two-tick log: the player lands 47 and then 285 on a Spire, which
// reflects 30% of each as EM, retreats once, and holds full shield.
const spireLog = `[{"entries":[
 {"tick":1,
  "snapshots":[
   {"player_id":"p1","ship_class":"axiomata","zone":"engaged","shield":900,"max_shield":950},
   {"player_id":"repeater_aa","ship_class":"spire","zone":"outer","shield":2400,"max_shield":2400}],
  "zone_moves":[{"player_id":"repeater_aa","reason":"retreat"}],
  "attacks":[
   {"attacker_id":"p1","target_id":"repeater_aa","hit_success":true,"damage_type":"energy","raw_damage":94,"landed_damage":47,"type_resist_pct":50,"weapons":[{}]},
   {"attacker_id":"repeater_aa","target_id":"p1","hit_success":true,"damage_type":"em","raw_damage":14,"weapons":null},
   {"attacker_id":"repeater_aa","target_id":"p1","hit_chance":0.6,"hit_success":true,"damage_type":"energy","zone_distance":3,"raw_damage":100,"weapons":[{}]}]},
 {"tick":2,
  "snapshots":[
   {"player_id":"p1","ship_class":"axiomata","zone":"engaged","shield":800,"max_shield":950},
   {"player_id":"repeater_aa","ship_class":"spire","zone":"outer","shield":2400,"max_shield":2400}],
  "attacks":[
   {"attacker_id":"p1","target_id":"repeater_aa","hit_success":true,"damage_type":"energy","landed_damage":285,"type_resist_pct":50,"weapons":[{}]},
   {"attacker_id":"repeater_aa","target_id":"p1","hit_success":true,"damage_type":"em","raw_damage":85,"weapons":null}],
  "kills":[{"killer_id":"p1","victim_id":"repeater_aa"}]}
]}]`

func TestLoadMechanics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b1.raw.json"), []byte(spireLog), 0o644); err != nil {
		t.Fatal(err)
	}
	mech, n, err := loadMechanics(dir)
	if err != nil || n != 1 {
		t.Fatalf("loadMechanics: n=%d err=%v", n, err)
	}
	m := mech["repeater_spire"]
	if m == nil {
		t.Fatalf("no repeater_spire in %v", mech)
	}
	if _, ok := mech["repeater_axiomata"]; ok {
		t.Error("player ship filed as a hostile")
	}
	if m.Reflect == nil || m.Reflect.Pct != 30 || m.Reflect.Matches != 2 || m.Reflect.DamageType != "em" {
		t.Errorf("reflect = %+v, want 30%% em, 2 matches", m.Reflect)
	}
	if len(m.Resists) != 1 || m.Resists[0] != (Resist{DamageType: "energy", Pct: 50, Hits: 2}) {
		t.Errorf("resists = %+v", m.Resists)
	}
	if m.Retreats != 1 || m.OuterPct != 100 || m.MinShieldPct != 100 || m.HitsTaken != 2 || m.Destroyed != 1 || m.Logs != 1 {
		t.Errorf("mechanics = %+v", m)
	}
	if len(m.HitByRange) != 1 || m.HitByRange[0] != (RangeHit{Distance: 3, Volleys: 1, MeanPct: 60}) {
		t.Errorf("hit by range = %+v", m.HitByRange)
	}
}

func TestLoadMechanicsMissingDir(t *testing.T) {
	mech, n, err := loadMechanics(filepath.Join(t.TempDir(), "none"))
	if err != nil || n != 0 || len(mech) != 0 {
		t.Errorf("missing dir: mech=%v n=%d err=%v", mech, n, err)
	}
}

func TestAttachMechanicsNamesReflection(t *testing.T) {
	ts := []*HostileType{{ID: "repeater_spire", Weapons: []Weapon{{Name: "Focused Beam II", DamageType: "energy"}, {DamageType: "em", ShotKind: "beam"}}}}
	attachMechanics(ts, map[string]*Mechanics{"repeater_spire": {Reflect: &Reflection{DamageType: "em", Pct: 30}}})
	if w := ts[0].Weapons[1]; w.Name != "Reflection" || w.ShotKind != "reflected" {
		t.Errorf("weaponless em row = %+v", w)
	}
	if ts[0].Weapons[0].Name != "Focused Beam II" {
		t.Error("real weapon renamed")
	}
}
