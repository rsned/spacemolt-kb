package main

import (
	"errors"
	"io/fs"
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
	// tank hull sized so a firing fleer (100/tick × up to 50 ticks = 5000)
	// would kill it well within the tick budget; 10000 was too tall to
	// discriminate a bug where the fleeing side fires anyway.
	tank := &StatBlock{Name: "tank", MaxHull: 1000, MaxShield: 0}
	cal := calFixed()
	cal.FleeTicksRequired = 1 << 30 // can never escape, and never fires: must stalemate
	cal.MaxTicks = 50
	rng := rand.New(rand.NewPCG(2, 2))
	if out := RunBattle(glass, tank, StanceFlee, StanceFire, cal, rng); out != OutStalemate {
		t.Errorf("fleeing unarmed-vs-unarmed = %s, want stalemate (fleeing side must not fire)", out)
	}
}

// Braced ships turtle up: 75% damage reduction, weapons down. Measured in
// the Haven fixture: 513 braced ticks across seven ships, zero shots fired.
func TestBraceDoesNotFire(t *testing.T) {
	glass := &StatBlock{Name: "glass", MaxHull: 10, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	// Hull sized so a firing bracer (100/tick) would kill inside the budget.
	tank := &StatBlock{Name: "tank", MaxHull: 1000, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 50
	rng := rand.New(rand.NewPCG(21, 21))
	if out := RunBattle(glass, tank, StanceBrace, StanceFire, cal, rng); out != OutStalemate {
		t.Errorf("braced armed vs unarmed = %s, want stalemate (braced side must not fire)", out)
	}
}

func TestFleeEscapes(t *testing.T) {
	a := &StatBlock{Name: "a", MaxHull: 100000, MaxShield: 0}
	b := &StatBlock{Name: "b", MaxHull: 100000, MaxShield: 0}
	cal := calFixed()
	cal.FleeTicksRequired = 1
	rng := rand.New(rand.NewPCG(3, 3))
	if out := RunBattle(a, b, StanceFlee, StanceFire, cal, rng); out != OutAFled {
		t.Errorf("guaranteed escape = %s, want A-fled", out)
	}
}

func TestBraceReducesDamage(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 300, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 4
	rng := rand.New(rand.NewPCG(4, 4))
	// 4 landed 100-dmg volleys: unbraced fire (skill 0, armor 0, so pre=100
	// lands unreduced) deals 100/tick and kills the 300-hull defender by
	// tick 3 (cumulative 300); braced fire is cut to 25/tick (100×0.25
	// brace_in_mult), landing 100 total over 4 ticks — defender survives.
	if out := RunBattle(att, def, StanceFire, StanceBrace, cal, rng); out != OutStalemate {
		t.Errorf("braced 300-hull vs 25/tick over 4 ticks = %s, want stalemate", out)
	}
}

func TestEvadeReducesDamage(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 300, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 4
	rng := rand.New(rand.NewPCG(5, 5))
	cal.EvadeAccuracyDebuff = 0 // isolate the damage mult from the accuracy debuff
	// Same shape as TestBraceReducesDamage: 4 landed 100-dmg volleys.
	// Unevaded fire deals 400 total, killing the 300-hull defender; evaded
	// fire is cut to 50/tick (100×0.5 evade_in_mult, DefaultCalibration),
	// landing 200 total over 4 ticks — defender survives.
	if out := RunBattle(att, def, StanceFire, StanceEvade, cal, rng); out != OutStalemate {
		t.Errorf("evading 300-hull vs 50/tick over 4 ticks = %s, want stalemate", out)
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
	// Values reflect the Phase-B measured calibration: brace_regen_mult
	// 2.5 (S4c, not the doc's 2), hit_chance 0.90 (measured engaged d0,
	// not 0.95), the rest measured/doc-backed.
	if c.BraceInMult != 0.25 || c.RegenHitDivisor != 3 || c.MaxTicks != 500 ||
		c.BraceRegenMult != 2.5 || c.EvadeAccuracyDebuff != 0.20 ||
		c.HitChanceA != 0.90 || c.HitChanceB != 0.90 ||
		c.FleeTicksRequired != 3 || c.StalemateTicks != 30 {
		t.Errorf("calibration = %+v, want measured/doc defaults", c)
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

// main.go's missing-calibration fallback checks errors.Is(err, fs.ErrNotExist)
// on the LoadCalibration error to fall back to DefaultCalibration(); this
// requires LoadCalibration to keep returning the unwrapped os.ReadFile error
// (not a fmt.Errorf-wrapped one that loses the errors.Is match).
func TestLoadCalibrationMissingFileIsNotExist(t *testing.T) {
	_, err := LoadCalibration("/nonexistent/path/does-not-exist.json")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("LoadCalibration missing file err = %v, want errors.Is(err, fs.ErrNotExist)", err)
	}
}

// skill.md stance table: evade cannot fire. Same discriminating shape as
// the brace/flee no-fire tests: an armed evader vs an unarmed tank must
// stalemate — any kill means the evading side fired.
func TestEvadeDoesNotFire(t *testing.T) {
	glass := &StatBlock{Name: "glass", MaxHull: 10, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	tank := &StatBlock{Name: "tank", MaxHull: 1000, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 50
	rng := rand.New(rand.NewPCG(31, 31))
	if out := RunBattle(glass, tank, StanceEvade, StanceFire, cal, rng); out != OutStalemate {
		t.Errorf("evading armed vs unarmed = %s, want stalemate (evading side must not fire)", out)
	}
}

// skill.md: evade applies -20% to enemy accuracy. With a full 1.0 debuff
// the attacker can never hit an evading target at all.
func TestEvadeAccuracyDebuff(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 100, MaxShield: 0}
	cal := calFixed()
	cal.EvadeAccuracyDebuff = 1.0
	rng := rand.New(rand.NewPCG(32, 32))
	if out := RunBattle(att, def, StanceFire, StanceEvade, cal, rng); out != OutStalemate {
		t.Errorf("un-hittable evader = %s, want stalemate", out)
	}
}

// skill.md: brace doubles shield regeneration. Attacker drains 24
// kinetic/tick into a 40-shield pool recharging 16/tick (divisor 1 for
// clean arithmetic, BraceInMult 1.0 to isolate regen). Braced: +32
// regen/tick beats the drain, shields never break, stalemate. Un-braced:
// net -8/tick empties the pool by tick 3, regen never restarts from zero,
// and 24 hull/tick kills the 120-hull defender well inside the window.
func TestBraceDoublesRegen(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 1000, MaxShield: 0, WeaponSkillPct: 0,
		Weapons: []Weapon{{Damage: 24, Type: "kinetic", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 120, MaxShield: 40, Recharge: 16}
	cal := calFixed()
	cal.BraceInMult = 1.0   // isolate regen from the damage mult
	cal.RegenHitDivisor = 1 // full regen on hit ticks, for clean arithmetic
	cal.StalemateTicks = 12
	rng := rand.New(rand.NewPCG(33, 33))
	out := RunBattle(att, def, StanceFire, StanceBrace, cal, rng)
	if out != OutStalemate {
		t.Errorf("braced regen-tank = %s, want stalemate", out)
	}
	outUnbraced := RunBattle(att, def, StanceFire, StanceFire, cal, rand.New(rand.NewPCG(33, 33)))
	if outUnbraced != OutAKill {
		t.Errorf("un-braced control = %s, want A-kill (shields must collapse without the 2x regen)", outUnbraced)
	}
}

// skill.md: 30 ticks with no kills is a stalemate draw. A kill that would
// land on tick 35 under the damage math must instead read stalemate at the
// default 30, and land when the threshold is raised.
func TestStalemateAtThirtyTicks(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 3500, MaxShield: 0} // dies on tick 35
	rng := rand.New(rand.NewPCG(34, 34))
	if out := RunBattle(att, def, StanceFire, StanceFire, calFixed(), rng); out != OutStalemate {
		t.Errorf("35-tick kill under 30-tick rule = %s, want stalemate", out)
	}
	cal := calFixed()
	cal.StalemateTicks = 40
	rng = rand.New(rand.NewPCG(34, 34))
	if out := RunBattle(att, def, StanceFire, StanceFire, cal, rng); out != OutAKill {
		t.Errorf("35-tick kill under 40-tick rule = %s, want A-kill", out)
	}
}
