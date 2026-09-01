package main

import (
	"strings"
	"testing"
)

func tableFits() (*StatBlock, *StatBlock) {
	big := &StatBlock{Name: "big", MaxHull: 1000, MaxShield: 200, Recharge: 2, WeaponSkillPct: 10, CritPct: 5,
		Weapons: []Weapon{{Damage: 50, Type: "energy", Cooldown: 1}}}
	small := &StatBlock{Name: "small", MaxHull: 100, MaxShield: 50, Recharge: 2, WeaponSkillPct: 5, CritPct: 5,
		Weapons: []Weapon{{Damage: 10, Type: "kinetic", Cooldown: 1}}}
	return big, small
}

func TestRunTableShape(t *testing.T) {
	a, b := tableFits()
	cells := RunTable(a, b, DefaultCalibration(), 50, 42)
	if len(cells) != 16 {
		t.Fatalf("cells = %d, want 16", len(cells))
	}
	for _, c := range cells {
		total := 0
		for _, n := range c.Counts {
			total += n
		}
		if total != 50 || c.Runs != 50 {
			t.Errorf("cell %s/%s: %d outcomes over %d runs, want 50/50", c.StanceA, c.StanceB, total, c.Runs)
		}
	}
	// fire-vs-fire: big should dominantly kill small.
	ff := cells[0]
	if ff.StanceA != StanceFire || ff.StanceB != StanceFire {
		t.Fatalf("cells[0] = %s/%s, want fire/fire", ff.StanceA, ff.StanceB)
	}
	if ff.Counts[OutAKill] < 45 {
		t.Errorf("fire/fire A-kill = %d/50, want ≥45 for a 10x fit advantage", ff.Counts[OutAKill])
	}
}

func TestRunTableDeterministic(t *testing.T) {
	a, b := tableFits()
	c1 := RunTable(a, b, DefaultCalibration(), 30, 7)
	c2 := RunTable(a, b, DefaultCalibration(), 30, 7)
	for i := range c1 {
		for k, v := range c1[i].Counts {
			if c2[i].Counts[k] != v {
				t.Fatalf("cell %d differs under same seed", i)
			}
		}
	}
}

func TestFormatTableFlagsAssumed(t *testing.T) {
	a, b := tableFits()
	cal := DefaultCalibration()
	cal.Assumed = append(cal.Assumed, "evade_in_mult") // mark evade cells for this test
	cells := RunTable(a, b, cal, 10, 1)
	out := FormatTable(a, b, cells, cal)
	if !strings.Contains(out, "*") || !strings.Contains(out, "evade_in_mult") {
		t.Errorf("table must flag ASSUMED-dependent cells and legend them:\n%s", out)
	}
	if !strings.Contains(out, "fire") || !strings.Contains(out, "brace") {
		t.Errorf("table missing stance headers:\n%s", out)
	}
}
