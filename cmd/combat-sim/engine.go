package main

import "math"

type Stance string

const (
	StanceFire  Stance = "fire"
	StanceBrace Stance = "brace"
	StanceEvade Stance = "evade"
	StanceFlee  Stance = "flee"
)

// Per-damage-type constants (measured; see spec).
var shieldEff = map[string]float64{
	"energy": 0.75, "kinetic": 1.0, "void": 0.0,
	"explosive": 1.0, "em": 1.0, "thermal": 1.0,
}

var armorMult = map[string]float64{
	"energy": 0.75, "kinetic": 1.5, "void": 1.5,
	"explosive": 1.0, "em": 1.0, "thermal": 0.25,
}

const armorK = 150.0         // half-saturation constant = max bare hull armor
const armorLawCrossover = 12 // counted-armor value splitting the two measured law ranges (OPEN)

var armorLaw = "auto" // Task 4 moves this into Calibration

type SideState struct {
	Stats        *StatBlock
	Hull, Shield int
	Ammo         []int
	Cool         []int
	Stance       Stance
	HitThisTick  bool
	Fled         bool
}

func NewSide(sb *StatBlock, stance Stance) *SideState {
	s := &SideState{Stats: sb, Hull: sb.MaxHull, Shield: sb.MaxShield, Stance: stance}
	s.Ammo = make([]int, len(sb.Weapons))
	s.Cool = make([]int, len(sb.Weapons))
	for i, w := range sb.Weapons {
		if w.Magazine > 0 {
			s.Ammo[i] = w.Magazine
		} else {
			s.Ammo[i] = -1
		}
	}
	return s
}

type VolleyOutcome struct{ ShieldDrain, HullDmg int }

// stageFloor floors v, reporting whether the truncated fraction spills a
// point to hull (measured threshold: frac >= 0.5).
func stageFloor(v float64) (int, int) {
	f := math.Floor(v)
	if v-f >= 0.5 {
		return int(f), 1
	}
	return int(f), 0
}

func armorReduce(hullIn int, armorTotal float64, dmgType string) int {
	if hullIn <= 0 {
		return 0
	}
	counted := armorTotal * armorMult[dmgType]
	var out int
	usePct := counted >= armorLawCrossover
	switch armorLaw {
	case "pct150":
		usePct = true
	case "flat75":
		usePct = false
	}
	if usePct {
		out = int(float64(hullIn) * (1 - counted/(counted+armorK)))
	} else {
		out = hullIn - int(armorTotal*0.75)
	}
	return max(out, 1) // min-1: every hit that reaches hull lands for at least 1
}

// ResolveVolley applies one LANDED volley. Pure: mutates nothing.
func ResolveVolley(att *StatBlock, tgt *SideState, fired []int, critFlags []bool, stanceInMult float64) VolleyOutcome {
	raw := 0
	dmgType := ""
	for i, wi := range fired {
		w := att.Weapons[wi]
		dmgType = w.Type // v1: single-type volleys (mixed types unsupported; resolver fits are single-type)
		d := w.Damage
		if critFlags[i] {
			d = d * 3 / 2 // floor(d × 1.5)
		}
		raw += d
	}
	pre := raw * (100 + att.WeaponSkillPct) / 100
	pre = int(float64(pre) * stanceInMult)
	var out VolleyOutcome
	hullIn := 0
	if tgt.Shield > 0 && shieldEff[dmgType] > 0 {
		spills := 0
		x1, s1 := stageFloor(float64(pre) * (1 - float64(tgt.Stats.ShieldsSkill)/100))
		d2, s2 := stageFloor(float64(x1) * shieldEff[dmgType])
		drain, s3 := stageFloor(float64(d2) * (1 - float64(tgt.Stats.FlatPct)/100))
		spills = s1 + s2 + s3
		if tgt.Shield >= drain {
			out.ShieldDrain = drain
			out.HullDmg = spills * (100 - tgt.Stats.FlatPct) / 100 // spill bypasses armor
			return out
		}
		// Breakthrough: no shields-skill term, no spills (measured).
		out.ShieldDrain = tgt.Shield
		hullIn = pre - int(float64(tgt.Shield)/shieldEff[dmgType])
	} else {
		hullIn = pre // shields down, or void (skips shields entirely)
	}
	out.HullDmg = min(armorReduce(hullIn, tgt.Stats.ArmorTotal, dmgType), tgt.Hull)
	return out
}
