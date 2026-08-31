package main

import "testing"

// Stat blocks are hand-built to match the fixtures' logged participants exactly
// (independent of the resolver, which Task 2 tests separately).
func artisEviction() *StatBlock { // 509e: 4× Pulse Laser III, skills W3+G3, target stats below
	return &StatBlock{Name: "artis509", WeaponSkillPct: 6, CritPct: 3,
		MaxHull: 480, MaxShield: 600, Recharge: 4, ArmorTotal: 25, FlatPct: 70, ShieldsSkill: 1,
		Weapons: []Weapon{{Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}}}
}

func molten509() *StatBlock { // 509e: Portfolio, PL II + PL I, W7+G10
	return &StatBlock{Name: "molten509", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 180, MaxShield: 130, Recharge: 2, ArmorTotal: 8, FlatPct: 0, ShieldsSkill: 3,
		Weapons: []Weapon{{Damage: 18, Type: "energy"}, {Damage: 10, Type: "energy"}}}
}

func moltenUnderwriter() *StatBlock { // b7847bbc: 2× Pulse Laser, W7+G10
	return &StatBlock{Name: "moltenUw", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 130, MaxShield: 95, Recharge: 2, ArmorTotal: 6, ShieldsSkill: 3,
		Weapons: []Weapon{{Damage: 10, Type: "energy"}, {Damage: 10, Type: "energy"}}}
}

func vera() *StatBlock { // b7847bbc: Autocannon, W0+G0
	return &StatBlock{Name: "vera", WeaponSkillPct: 0, CritPct: 0,
		MaxHull: 80, MaxShield: 35, Recharge: 1, ArmorTotal: 3, ShieldsSkill: 0,
		Weapons: []Weapon{{Damage: 8, Type: "kinetic"}}}
}

func side(sb *StatBlock, shield, hull int) *SideState {
	s := NewSide(sb, StanceFire)
	s.Shield, s.Hull = shield, hull
	return s
}

func all(n int) []int {
	idx := make([]int, n)
	for i := range n {
		idx[i] = i
	}
	return idx
}

func noCrit(n int) []bool { return make([]bool, n) }

type goldenCase struct {
	name   string
	att    *StatBlock
	tgt    *SideState
	crits  []bool
	wantSh int
	wantHl int
	hlTol  int // 0 = exact
}

func TestGoldenVolleys(t *testing.T) {
	// 7c044558 participants.
	moltenBroadaxe := &StatBlock{Name: "moltenBx", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 282, MaxShield: 28, Recharge: 2, ArmorTotal: 28, ShieldsSkill: 4,
		Weapons: []Weapon{{Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 28, Type: "kinetic"}}}
	artisSurvey := &StatBlock{Name: "artisSurvey", WeaponSkillPct: 6, CritPct: 3,
		MaxHull: 340, MaxShield: 400, Recharge: 9, ArmorTotal: 14, ShieldsSkill: 1,
		Weapons: []Weapon{{Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}}}

	cases := []goldenCase{
		// 509e tick 1744820: 118 → shres → 85 shield + 1 spill hull (net 85, recharge 2 → g 0).
		{"509A full-shield", artisEviction(), side(molten509(), 130, 136), noCrit(4), 85, 1, 0},
		// 509e tick 1744822 breakthrough: pool 45 consumes floor(45/.75)=60; 118−60=58; flat armor law (counted 6<12): 58−6=52.
		{"509B breakthrough", artisEviction(), side(molten509(), 45, 135), noCrit(4), 45, 52, 0},
		// 509e tick 1744823 kill cap: hull 83 remaining caps 112 → 83.
		{"509C kill cap", artisEviction(), side(molten509(), 0, 83), noCrit(4), 0, 83, 0},
		// 509e MoltenOne→Artis ×4: 32 → 31(spill) → 23 → flat70 → 6(spill); spills×0.3→0 hull. Actual drain 6; logged net 5 (g=1).
		{"509M flat70", molten509(), side(artisEviction(), 600, 480), noCrit(2), 6, 0, 0},
		// b7847 ticks 385/386: floor(23×.75)=17, no spill (Vera shres 0).
		{"NK1 full-shield", moltenUnderwriter(), side(vera(), 35, 80), noCrit(2), 17, 0, 0},
		// b7847 tick 387 crit breakthrough: raw 10+15=25 → 29; pool 1 consumes 1; 28−floor(3×.75)=26.
		{"NKB crit breakthrough", moltenUnderwriter(), side(vera(), 1, 80), []bool{false, true}, 1, 26, 0},
		// b7847 ticks 388/389 shields down: 23 − 2 = 21.
		{"NK4 shields down", moltenUnderwriter(), side(vera(), 0, 54), noCrit(2), 0, 21, 0},
		// b7847 tick 390 kill cap.
		{"NK6 kill cap", moltenUnderwriter(), side(vera(), 0, 12), noCrit(2), 0, 12, 0},
		// b7847 autocannon: 8 → shres spill (7.76) → drain 7 + 1 spill hull.
		{"NKA kinetic spill", vera(), side(moltenUnderwriter(), 95, 130), noCrit(1), 7, 1, 0},
		// 7c04 kinetic ×5: 81 → floor(80.19)=80 drain, no spill. Logged net 77 (g=3).
		{"7cK kinetic drain", moltenBroadaxe, side(artisSurvey, 400, 340), noCrit(4), 80, 0, 0},
		// 7c04 tick 961 crit breakthrough: raw 21+14+14+28=77 → 90; pool 15; 75 × (1−21/171) → 65.
		{"7cKB crit breakthrough", moltenBroadaxe, side(artisSurvey, 15, 340), []bool{true, false, false, false}, 15, 65, 0},
		// 7c04 tick 962 shields down: floor(81 × 150/171) = 71.
		{"7cKD shields down", moltenBroadaxe, side(artisSurvey, 0, 275), noCrit(4), 0, 71, 0},
		// 7c04 broadaxe rows: OPEN ±1 alternation (obs 41; 52/53) — tolerance 2.
		{"7cEB broadaxe breakthrough", artisSurvey, side(moltenBroadaxe, 8, 254), noCrit(2), 8, 41, 2},
		{"7cE broadaxe shields down", artisSurvey, side(moltenBroadaxe, 0, 213), noCrit(2), 0, 52, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveVolley(c.att, c.tgt, all(len(c.att.Weapons)), c.crits, 1.0, DefaultCalibration())
			if got.ShieldDrain != c.wantSh {
				t.Errorf("shield drain = %d, want %d", got.ShieldDrain, c.wantSh)
			}
			if d := got.HullDmg - c.wantHl; d < -c.hlTol || d > c.hlTol {
				t.Errorf("hull dmg = %d, want %d ±%d", got.HullDmg, c.wantHl, c.hlTol)
			}
		})
	}
}

// Void damage bypasses shields entirely (shieldEff["void"] == 0), so a
// full-shield target still takes hull damage straight through with no drain.
func TestVoidSkipsShields(t *testing.T) {
	att := &StatBlock{Name: "voidAtt", WeaponSkillPct: 0, CritPct: 0,
		Weapons: []Weapon{{Damage: 100, Type: "void"}}}
	tgt := side(&StatBlock{Name: "tgt", MaxHull: 100000, MaxShield: 500, ArmorTotal: 0, FlatPct: 0, ShieldsSkill: 0}, 500, 100000)
	got := ResolveVolley(att, tgt, all(1), noCrit(1), 1.0, DefaultCalibration())
	if got.ShieldDrain != 0 || got.HullDmg != 100 {
		t.Errorf("void volley = %+v, want shield drain 0 hull 100 (min-1/kill-cap not binding)", got)
	}
}
