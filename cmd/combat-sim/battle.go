package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
)

type Calibration struct {
	HitChanceA        float64  `json:"hit_chance_a"`
	HitChanceB        float64  `json:"hit_chance_b"`
	BraceInMult       float64  `json:"brace_in_mult"`
	EvadeInMult       float64  `json:"evade_in_mult"`
	FleeEscapePerTick float64  `json:"flee_escape_per_tick"`
	RegenHitDivisor   int      `json:"regen_hit_divisor"`
	RegenFromZero     bool     `json:"regen_from_zero"`
	ArmorLaw          string   `json:"armor_law"`
	ArmorLawCrossover float64  `json:"armor_law_crossover"`
	MaxTicks          int      `json:"max_ticks"`
	Assumed           []string `json:"assumed"`
}

func DefaultCalibration() *Calibration {
	return &Calibration{HitChanceA: 0.95, HitChanceB: 0.95, BraceInMult: 0.25,
		EvadeInMult: 0.5, FleeEscapePerTick: 0.25, RegenHitDivisor: 3,
		ArmorLaw: "auto", ArmorLawCrossover: 12, MaxTicks: 500,
		Assumed: []string{"evade_in_mult", "flee_escape_per_tick", "regen_from_zero"}}
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
	// Braced ships do not fire — measured: across the Haven fixture
	// (2a76e1a1), seven ships spent 513 braced ticks and fired zero shots.
	// Fleeing ships likewise (1763 flee ticks, zero shots).
	if att.Stance == StanceFlee || att.Stance == StanceBrace {
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
	if s.HitThisTick {
		r = r / cal.RegenHitDivisor
	}
	s.Shield = min(s.Shield+r, s.Stats.MaxShield)
}

// RunBattle simulates one 1v1 battle to a terminal outcome.
func RunBattle(a, b *StatBlock, sa, sb Stance, cal *Calibration, rng *rand.Rand) Outcome {
	A, B := NewSide(a, sa), NewSide(b, sb)
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
		if tick > 0 {
			// Deliberate ordering artifact: A's escape roll is checked before
			// B's, so under symmetric fleeing/pursuing parameters A escapes
			// slightly more often than B — someone has to roll first.
			if A.Stance == StanceFlee && rng.Float64() < cal.FleeEscapePerTick {
				return OutAFled
			}
			if B.Stance == StanceFlee && rng.Float64() < cal.FleeEscapePerTick {
				return OutBFled
			}
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
