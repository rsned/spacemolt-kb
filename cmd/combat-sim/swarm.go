package main

import (
	"math"
	"math/rand/v2"
)

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

// SwarmResult is the outcome of a homogeneous-cohort swarm battle.
type SwarmResult struct {
	SwarmWin bool
	Kills    int
	Ticks    int
}

// attackerVolleyProfile returns the expected combined raw damage of one
// landing volley at this distance (after the attacker's weapon-skill
// multiplier, mirroring ResolveVolley's `pre` stage), its single damage
// type, and the fraction of ticks an attacker is ready to fire (steady-state
// cooldown+reload).
func attackerVolleyProfile(sb *StatBlock, dist int) (raw int, dmgType string, firing float64) {
	critBoost := 1 + float64(sb.CritPct)/100*0.5
	skillMult := float64(100+sb.WeaponSkillPct) / 100
	var expected, frac float64
	for _, w := range sb.Weapons {
		if dist > w.Reach {
			continue
		}
		dmgType = w.Type
		f := 1.0
		if w.Cooldown > 1 || w.Magazine > 0 {
			m := float64(w.Magazine)
			if w.Magazine <= 0 {
				m = math.Inf(1)
			}
			f = m / (m*float64(w.Cooldown) + 1)
		}
		expected += float64(w.Damage) * critBoost * skillMult * f
		frac = math.Max(frac, f) // ship fires if any weapon is ready
	}
	return int(math.Round(expected)), dmgType, frac
}

// applyIdenticalVolleys applies k identical expected-damage volleys of raw
// damage to def in closed form: deplete shield, then bulk hull. Mirrors the
// ResolveVolley staging (spills ignored — a measured-small effect) so the
// homogeneous engine stays O(1) per tick for huge k.
func applyIdenticalVolleys(def *SideState, raw int, dmgType string, k int, cal *Calibration) {
	if k <= 0 || raw <= 0 {
		return
	}
	if def.Shield > 0 && shieldEff[dmgType] > 0 {
		x1 := int(float64(raw) * (1 - float64(def.Stats.ShieldsSkill)/100))
		d2 := int(float64(x1) * shieldEff[dmgType])
		drain := int(float64(d2) * (1 - float64(def.Stats.FlatPct)/100))
		if drain < 1 {
			drain = 1
		}
		need := (def.Shield + drain - 1) / drain
		if need >= k {
			def.Shield = max(def.Shield-drain*k, 0)
			return
		}
		def.Shield = 0
		k -= need
	}
	def.Hull -= armorReduce(raw, def.Stats.ArmorTotal, dmgType, cal) * k
}

// binomial samples the number of successes in count trials at probability p.
func binomial(count int, p float64, rng *rand.Rand) int {
	if p <= 0 || count <= 0 {
		return 0
	}
	if p >= 1 {
		return count
	}
	k := 0
	for range count {
		if rng.Float64() < p {
			k++
		}
	}
	return k
}

// RunSwarm simulates n identical attackers vs one defender using the
// homogeneous cohort model (O(1) per tick regardless of n). n-1 attackers
// are tracked only as a headcount; one "focused" attacker is a real
// SideState taking the defender's exact per-ship volleys (cooldown, reload,
// and reach honored), and is replaced from the healthy pool when killed.
func RunSwarm(attacker, defender *StatBlock, n int, cal *Calibration, maxTicks int, rng *rand.Rand) SwarmResult {
	if n <= 0 {
		return SwarmResult{}
	}
	def := NewSide(defender, StanceFire)
	focus := NewSide(attacker, StanceFire)
	healthy := n - 1 // everyone except the one currently focused
	kills := 0
	dist := swarmStartDistance
	for tick := range maxTicks {
		aliveAttackers := healthy + 1 // focus is alive here
		raw, dmgType, firing := attackerVolleyProfile(attacker, dist)
		if raw > 0 {
			k := binomial(aliveAttackers, hitChanceAt(dist, cal)*firing, rng)
			applyIdenticalVolleys(def, raw, dmgType, k, cal)
			if def.Hull <= 0 {
				return SwarmResult{true, kills, tick + 1}
			}
		}
		// Defender fires at the one focused attacker (exact per-ship sim).
		focus.HitThisTick = false
		o := volleyAt(def, focus, dist, cal, rng)
		focus.Shield -= o.ShieldDrain
		focus.Hull -= o.HullDmg
		if focus.Hull <= 0 {
			kills++
			if healthy == 0 {
				return SwarmResult{false, kills, tick + 1} // last attacker gone
			}
			healthy--
			focus = NewSide(attacker, StanceFire) // draw a fresh healthy attacker
		}
		regen(def, cal)
		tickWeapons(def)
		tickWeapons(focus)
		dist = max(dist-1, 0)
	}
	return SwarmResult{false, kills, maxTicks} // defender survived
}
