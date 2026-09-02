package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
)

type Calibration struct {
	HitChanceA          float64   `json:"hit_chance_a"`
	HitChanceB          float64   `json:"hit_chance_b"`
	BraceInMult         float64   `json:"brace_in_mult"`
	BraceRegenMult      float64   `json:"brace_regen_mult"`
	EvadeInMult         float64   `json:"evade_in_mult"`
	EvadeAccuracyDebuff float64   `json:"evade_accuracy_debuff"`
	FleeTicksRequired   int       `json:"flee_ticks_required"`
	RegenHitDivisor     int       `json:"regen_hit_divisor"`
	RegenFromZero       bool      `json:"regen_from_zero"`
	ArmorLaw            string    `json:"armor_law"`
	ArmorLawCrossover   float64   `json:"armor_law_crossover"`
	StalemateTicks      int       `json:"stalemate_ticks"`
	MaxTicks            int       `json:"max_ticks"`
	Assumed             []string  `json:"assumed"`
	HitChanceByDistance []float64 `json:"hit_chance_by_distance"`
}

func DefaultCalibration() *Calibration {
	// Stance behavior per skill.md "Combat & Battle System" (2026-09-01):
	// brace 25% taken + 2x shield regen; evade 50% taken, -20% enemy
	// accuracy, cannot fire; flee escapes after 3 consecutive flee ticks
	// (base; speed/Tactics/tackle modifiers not modeled). Stalemate is 30
	// ticks with no kills.
	return &Calibration{HitChanceA: 0.95, HitChanceB: 0.95, BraceInMult: 0.25,
		BraceRegenMult: 2, EvadeInMult: 0.5, EvadeAccuracyDebuff: 0.20,
		FleeTicksRequired: 3, RegenHitDivisor: 3,
		ArmorLaw: "auto", ArmorLawCrossover: 12, StalemateTicks: 30, MaxTicks: 500,
		Assumed:             []string{"regen_from_zero"},
		HitChanceByDistance: []float64{0.90, 0.80, 0.65, 0.50, 0.35, 0.22, 0.12}}
}

func LoadCalibration(path string) (*Calibration, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := DefaultCalibration()
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.RegenHitDivisor <= 0 {
		return nil, fmt.Errorf("%s: regen_hit_divisor must be > 0, got %d", path, c.RegenHitDivisor)
	}
	if c.FleeTicksRequired <= 0 {
		return nil, fmt.Errorf("%s: flee_ticks_required must be > 0, got %d", path, c.FleeTicksRequired)
	}
	if c.StalemateTicks <= 0 {
		return nil, fmt.Errorf("%s: stalemate_ticks must be > 0, got %d", path, c.StalemateTicks)
	}
	return c, nil
}

type Outcome string

const (
	OutAKill     Outcome = "A-kill"
	OutBKill     Outcome = "B-kill"
	OutAFled     Outcome = "A-fled"
	OutBFled     Outcome = "B-fled"
	OutMutual    Outcome = "mutual"
	OutStalemate Outcome = "stalemate"
)

func stanceInMult(s Stance, cal *Calibration) float64 {
	switch s {
	case StanceBrace:
		return cal.BraceInMult
	case StanceEvade:
		return cal.EvadeInMult
	default:
		return 1.0
	}
}

// volley rolls and resolves one side's attack; returns damage to apply.
func volley(att, tgt *SideState, hitChance float64, cal *Calibration, rng *rand.Rand) VolleyOutcome {
	// Only the fire stance fires. Brace and flee are measured (Haven
	// fixture: 513 braced ticks / 1763 flee ticks, zero shots); evade is
	// per skill.md's stance table ("Can Fire: No") — zero evade ticks
	// exist in any exported log to measure against.
	if att.Stance != StanceFire {
		return VolleyOutcome{}
	}
	var fired []int
	for i := range att.Stats.Weapons {
		if att.Cool[i] == 0 && att.Ammo[i] != 0 {
			fired = append(fired, i)
			att.Cool[i] = att.Stats.Weapons[i].Cooldown
			if att.Ammo[i] > 0 {
				att.Ammo[i]--
			}
		}
	}
	if len(fired) == 0 {
		return VolleyOutcome{}
	}
	crits := make([]bool, len(fired))
	for i := range crits { // crits roll regardless of hit (measured); a miss discards them
		crits[i] = rng.Float64() < float64(att.Stats.CritPct)/100
	}
	// Evading targets debuff the attacker's accuracy (skill.md: -20%),
	// on top of taking only half damage from volleys that still land.
	if tgt.Stance == StanceEvade {
		hitChance = max(hitChance-cal.EvadeAccuracyDebuff, 0)
	}
	if rng.Float64() >= hitChance {
		return VolleyOutcome{}
	}
	return ResolveVolley(att.Stats, tgt, fired, crits, stanceInMult(tgt.Stance, cal), cal)
}

func regen(s *SideState, cal *Calibration) {
	if s.Shield == 0 && !cal.RegenFromZero {
		return
	}
	r := s.Stats.Recharge
	if s.Stance == StanceBrace { // skill.md: brace doubles shield regen
		r = int(float64(r) * cal.BraceRegenMult)
	}
	if s.HitThisTick {
		r = r / cal.RegenHitDivisor
	}
	s.Shield = min(s.Shield+r, s.Stats.MaxShield)
}

// RunBattle simulates one 1v1 battle to a terminal outcome.
func RunBattle(a, b *StatBlock, sa, sb Stance, cal *Calibration, rng *rand.Rand) Outcome {
	A, B := NewSide(a, sa), NewSide(b, sb)
	fleeA, fleeB := 0, 0
	for tick := range cal.MaxTicks {
		A.HitThisTick, B.HitThisTick = false, false
		outA := volley(A, B, cal.HitChanceA, cal, rng) // A attacks B
		outB := volley(B, A, cal.HitChanceB, cal, rng) // B attacks A
		B.Shield -= outA.ShieldDrain
		B.Hull -= outA.HullDmg
		B.HitThisTick = outA.ShieldDrain > 0 || outA.HullDmg > 0
		A.Shield -= outB.ShieldDrain
		A.Hull -= outB.HullDmg
		A.HitThisTick = outB.ShieldDrain > 0 || outB.HullDmg > 0
		switch {
		case A.Hull <= 0 && B.Hull <= 0:
			return OutMutual
		case B.Hull <= 0:
			return OutAKill
		case A.Hull <= 0:
			return OutBKill
		}
		// Flee is a deterministic counter (skill.md): escape after
		// FleeTicksRequired consecutive flee ticks (stances are fixed per
		// run, so every survived tick counts). Ordering artifact: A is
		// checked first, so when both flee at the same requirement A
		// escapes — someone has to be first.
		if A.Stance == StanceFlee {
			fleeA++
			if fleeA >= cal.FleeTicksRequired {
				return OutAFled
			}
		}
		if B.Stance == StanceFlee {
			fleeB++
			if fleeB >= cal.FleeTicksRequired {
				return OutBFled
			}
		}
		// Stalemate rule (skill.md): 30 ticks with no kills is a draw. In a
		// 1v1 any kill ends the battle, so reaching the threshold IS the
		// no-kill condition. MaxTicks remains a hard safety bound.
		if tick+1 >= cal.StalemateTicks {
			return OutStalemate
		}
		regen(A, cal)
		regen(B, cal)
		for i := range A.Cool {
			A.Cool[i] = max(A.Cool[i]-1, 0)
		}
		for i := range B.Cool {
			B.Cool[i] = max(B.Cool[i]-1, 0)
		}
	}
	return OutStalemate
}
