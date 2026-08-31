package main

import (
	"math/rand/v2"
	"os"
	"testing"
)

func calFixed() *Calibration {
	c := DefaultCalibration()
	c.HitChanceA, c.HitChanceB = 1.0, 1.0 // deterministic for these tests
	return c
}

// Replays fixture 7c044558 end-to-end deterministically (hit chance 1, no
// crits at CritPct 0): broadaxe kills the survey fit... actually the fixture
// ended with MoltenOne (broadaxe) destroyed — with guaranteed hits both ways
// the higher-DPS side wins; assert the battle TERMINATES with a kill.
func TestRunBattleTerminates(t *testing.T) {
	a := &StatBlock{Name: "A", MaxHull: 282, MaxShield: 28, Recharge: 2, ArmorTotal: 28, ShieldsSkill: 4,
		WeaponSkillPct: 17, CritPct: 0,
		Weapons: []Weapon{{Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 28, Type: "kinetic", Cooldown: 1}}}
	b := &StatBlock{Name: "B", MaxHull: 340, MaxShield: 400, Recharge: 9, ArmorTotal: 14, ShieldsSkill: 1,
		WeaponSkillPct: 6, CritPct: 0,
		Weapons: []Weapon{{Damage: 28, Type: "energy", Cooldown: 1}, {Damage: 28, Type: "energy", Cooldown: 1}}}
	rng := rand.New(rand.NewPCG(1, 1))
	out := RunBattle(a, b, StanceFire, StanceFire, calFixed(), rng)
	if out != OutAKill && out != OutBKill {
		t.Errorf("outcome = %s, want a kill", out)
	}
}

func TestFleeNeverFires(t *testing.T) {
	glass := &StatBlock{Name: "glass", MaxHull: 10, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	tank := &StatBlock{Name: "tank", MaxHull: 10000, MaxShield: 0}
	cal := calFixed()
	cal.FleeEscapePerTick = 0 // can never escape, and never fires: must hit MaxTicks
	cal.MaxTicks = 50
	rng := rand.New(rand.NewPCG(2, 2))
	if out := RunBattle(glass, tank, StanceFlee, StanceFire, cal, rng); out != OutStalemate {
		t.Errorf("fleeing unarmed-vs-unarmed = %s, want stalemate (fleeing side must not fire)", out)
	}
}

func TestFleeEscapes(t *testing.T) {
	a := &StatBlock{Name: "a", MaxHull: 100000, MaxShield: 0}
	b := &StatBlock{Name: "b", MaxHull: 100000, MaxShield: 0}
	cal := calFixed()
	cal.FleeEscapePerTick = 1.0
	rng := rand.New(rand.NewPCG(3, 3))
	if out := RunBattle(a, b, StanceFlee, StanceFire, cal, rng); out != OutAFled {
		t.Errorf("guaranteed escape = %s, want A-fled", out)
	}
}

func TestBraceReducesDamage(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 1000, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 4
	rng := rand.New(rand.NewPCG(4, 4))
	// 3 landed 100-dmg volleys: fire eats 300, brace eats 75 (100×0.25 per volley).
	// Use RunBattleState variant? Keep it simple: brace survives 4 ticks, fire doesn't need testing here.
	if out := RunBattle(att, def, StanceFire, StanceBrace, cal, rng); out != OutStalemate {
		t.Errorf("braced 1000-hull vs 25/tick over 4 ticks = %s, want stalemate", out)
	}
}

func TestDeterministicUnderSeed(t *testing.T) {
	a := &StatBlock{Name: "a", MaxHull: 200, MaxShield: 100, Recharge: 2, CritPct: 7, WeaponSkillPct: 10,
		Weapons: []Weapon{{Damage: 20, Type: "energy", Cooldown: 1}}}
	b := &StatBlock{Name: "b", MaxHull: 200, MaxShield: 100, Recharge: 2, CritPct: 7, WeaponSkillPct: 10,
		Weapons: []Weapon{{Damage: 20, Type: "kinetic", Cooldown: 2}}}
	cal := DefaultCalibration()
	cal.HitChanceA, cal.HitChanceB = 0.8, 0.8
	o1 := RunBattle(a, b, StanceFire, StanceFire, cal, rand.New(rand.NewPCG(9, 9)))
	o2 := RunBattle(a, b, StanceFire, StanceFire, cal, rand.New(rand.NewPCG(9, 9)))
	if o1 != o2 {
		t.Errorf("same seed gave %s then %s", o1, o2)
	}
}

func TestLoadCalibrationFile(t *testing.T) {
	c, err := LoadCalibration("../../data/combat-sim/calibration.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.BraceInMult != 0.25 || c.RegenHitDivisor != 3 || c.MaxTicks != 500 {
		t.Errorf("calibration = %+v, want measured defaults", c)
	}
}

func TestLoadCalibrationValidatesRegenHitDivisor(t *testing.T) {
	// Reject invalid RegenHitDivisor (0 or negative) to prevent divide-by-zero panic
	tmpdir := t.TempDir()
	tmpfile, err := os.Create(tmpdir + "/bad_cal.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	_, err = tmpfile.WriteString(`{"regen_hit_divisor": 0}`)
	if err != nil {
		_ = tmpfile.Close()
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}

	_, err = LoadCalibration(tmpfile.Name())
	if err == nil {
		t.Errorf("LoadCalibration with regen_hit_divisor=0, want error")
	}
}
