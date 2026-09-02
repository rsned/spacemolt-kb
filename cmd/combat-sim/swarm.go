package main

import "math/rand/v2"

// swarmStartDistance is the shared ring both sides close from, one ring per
// tick, in a multi-ship battle (spec §5.1).
const swarmStartDistance = 6

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

// Ship is one participant's fit plus its team assignment for RunMultiShip.
type Ship struct {
	Stats *StatBlock
	Team  int
}

// MultiResult is the outcome of a multi-ship battle.
type MultiResult struct {
	WinningTeam int // -1 = stalemate/timeout
	Ticks       int
	KillsByTeam map[int]int
}

// RunMultiShip simulates a heterogeneous battle: every ship closes from
// swarmStartDistance, fires at one enemy per tick, and a team wins when it
// is the only one left alive. Volleys within a tick resolve sequentially so
// shields deplete in order.
func RunMultiShip(ships []Ship, cal *Calibration, maxTicks int, rng *rand.Rand) MultiResult {
	n := len(ships)
	side := make([]*SideState, n)
	team := make([]int, n)
	alive := make([]bool, n)
	for i, s := range ships {
		side[i] = NewSide(s.Stats, StanceFire)
		team[i] = s.Team
		alive[i] = true
	}
	kills := map[int]int{}
	dist := swarmStartDistance
	for tick := range maxTicks {
		for i := range side {
			side[i].HitThisTick = false
		}
		// Each living ship fires at the lowest-index living enemy.
		for i := range side {
			if !alive[i] {
				continue
			}
			tgt := -1
			for j := range side {
				if alive[j] && team[j] != team[i] {
					tgt = j
					break
				}
			}
			if tgt == -1 {
				continue
			}
			o := volleyAt(side[i], side[tgt], dist, cal, rng)
			side[tgt].Shield -= o.ShieldDrain
			side[tgt].Hull -= o.HullDmg
			if o.ShieldDrain > 0 || o.HullDmg > 0 {
				side[tgt].HitThisTick = true
			}
			if side[tgt].Hull <= 0 {
				alive[tgt] = false
				kills[team[i]]++
			}
		}
		if w := soleTeam(team, alive); w != -2 {
			return MultiResult{WinningTeam: w, Ticks: tick + 1, KillsByTeam: kills}
		}
		for i := range side {
			if alive[i] {
				regen(side[i], cal)
				tickWeapons(side[i])
			}
		}
		dist = max(dist-1, 0)
	}
	return MultiResult{WinningTeam: -1, Ticks: maxTicks, KillsByTeam: kills}
}

// soleTeam returns the only team with living ships, -1 if none alive, or -2
// if more than one team is still alive.
func soleTeam(team []int, alive []bool) int {
	winner := -1
	for i, a := range alive {
		if !a {
			continue
		}
		if winner == -1 {
			winner = team[i]
		} else if team[i] != winner {
			return -2
		}
	}
	return winner
}
