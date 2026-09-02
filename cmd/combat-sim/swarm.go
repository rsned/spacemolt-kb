package main

import "math/rand/v2"

func hitChanceAt(dist int, cal *Calibration) float64 {
	if dist >= 0 && dist < len(cal.HitChanceByDistance) {
		return cal.HitChanceByDistance[dist]
	}
	return cal.HitChanceA // flat fallback
}

// volleyAt is the zone-aware volley: each weapon fires only when in reach
// (dist <= Reach), off cooldown, and not mid-reload. One hit roll per ship
// volley, hit chance from the distance table. Ammo is unlimited; emptying a
// magazine schedules a reload (see tickWeapons).
func volleyAt(att, tgt *SideState, dist int, cal *Calibration, rng *rand.Rand) VolleyOutcome {
	if att.Stance != StanceFire {
		return VolleyOutcome{}
	}
	var fired []int
	for i := range att.Stats.Weapons {
		w := att.Stats.Weapons[i]
		if dist > w.Reach || att.Cool[i] != 0 || att.Reload[i] != 0 || att.Ammo[i] == 0 {
			continue
		}
		fired = append(fired, i)
		att.Cool[i] = w.Cooldown
		if att.Ammo[i] > 0 {
			att.Ammo[i]--
			if att.Ammo[i] == 0 {
				// Empty magazine: schedule a reload. tickWeapons decrements
				// this at the END of the current tick and again each tick
				// after, refilling when it reaches 0 — so this must start
				// at 2 (not 1) to leave exactly one idle firing tick before
				// the weapon is usable again (see TestReloadCycle).
				att.Reload[i] = 2
			}
		}
	}
	if len(fired) == 0 {
		return VolleyOutcome{}
	}
	crits := make([]bool, len(fired))
	for i := range crits {
		crits[i] = rng.Float64() < float64(att.Stats.CritPct)/100
	}
	hc := hitChanceAt(dist, cal)
	if tgt.Stance == StanceEvade {
		hc = max(hc-cal.EvadeAccuracyDebuff, 0)
	}
	if rng.Float64() >= hc {
		return VolleyOutcome{}
	}
	return ResolveVolley(att.Stats, tgt, fired, crits, stanceInMult(tgt.Stance, cal), cal)
}

// tickWeapons advances cooldowns and reloads: a weapon whose reload timer
// reaches zero refills its full magazine (unlimited ammo).
func tickWeapons(s *SideState) {
	for i := range s.Cool {
		if s.Cool[i] > 0 {
			s.Cool[i]--
		}
		if s.Reload[i] > 0 {
			s.Reload[i]--
			if s.Reload[i] == 0 {
				s.Ammo[i] = s.Stats.Weapons[i].Magazine
			}
		}
	}
}
