package main

import (
	"math/rand/v2"
	"testing"
)

func testCal() *Calibration {
	c := DefaultCalibration()
	c.HitChanceByDistance = []float64{0.90, 0.80, 0.65, 0.50, 0.35, 0.22, 0.12}
	return c
}

func TestHitChanceAt(t *testing.T) {
	cal := testCal()
	for d, want := range map[int]float64{0: 0.90, 4: 0.35, 6: 0.12} {
		if got := hitChanceAt(d, cal); got != want {
			t.Fatalf("d%d: got %v want %v", d, got, want)
		}
	}
	// Out-of-range falls back to the flat engaged value.
	if got := hitChanceAt(9, cal); got != cal.HitChanceA {
		t.Fatalf("oob: got %v want %v", got, cal.HitChanceA)
	}
}

func TestVolleyReachGate(t *testing.T) {
	// autocannon reach 2: silent at d6..d3, fires at d2.
	sb := &StatBlock{Name: "ac", Weapons: []Weapon{{Damage: 8, Type: "kinetic", Cooldown: 1, Magazine: 500, Reach: 2}}}
	tgt := NewSide(&StatBlock{MaxHull: 1000, MaxShield: 0}, StanceFire)
	rng := rand.New(rand.NewPCG(1, 1))
	att := NewSide(sb, StanceFire)
	if o := volleyAt(att, tgt, 3, testCal(), rng); o != (VolleyOutcome{}) {
		t.Fatalf("out-of-reach volley fired: %+v", o)
	}
	// At d2 with hit chance 0.65 it eventually lands; force many rolls.
	landed := false
	for range 50 {
		att = NewSide(sb, StanceFire)
		if o := volleyAt(att, tgt, 2, testCal(), rng); o.HullDmg > 0 {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatal("in-reach weapon never landed in 50 tries")
	}
}

func TestReloadCycle(t *testing.T) {
	// mag 2, cd 1: fires t0,t1 then must reload for 1 idle tick before t3.
	sb := &StatBlock{Name: "m2", Weapons: []Weapon{{Damage: 5, Type: "kinetic", Cooldown: 1, Magazine: 2, Reach: 6}}}
	att := NewSide(sb, StanceFire)
	tgt := NewSide(&StatBlock{MaxHull: 1000}, StanceFire)
	cal := testCal()
	cal.HitChanceByDistance = []float64{1, 1, 1, 1, 1, 1, 1} // always hit
	rng := rand.New(rand.NewPCG(2, 2))
	fired := make([]bool, 4)
	for tick := range 4 {
		o := volleyAt(att, tgt, 0, cal, rng)
		fired[tick] = o.HullDmg > 0
		tickWeapons(att)
	}
	// t0 fire, t1 fire, t2 reload (no fire), t3 fire.
	if !fired[0] || !fired[1] || fired[2] || !fired[3] {
		t.Fatalf("reload pattern = %v, want [true true false true]", fired)
	}
}
