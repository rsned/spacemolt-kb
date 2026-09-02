package main

import (
	"math"
	"math/rand/v2"
	"sort"
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
			drain = 1 // avoid an infinite/divide-by-zero "need" below when heavy FlatPct floors drain to 0
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

// binomialExactMax is the largest trial count sampled by rolling every
// trial. Above it, binomial switches to a Poisson or normal approximation
// so per-tick cost stays bounded (O(1)) regardless of attacker headcount —
// a plain Bernoulli loop over n attackers would make each tick O(n),
// defeating the point of the homogeneous cohort model at n in the tens of
// thousands.
const binomialExactMax = 30

// deMoivreLaplaceGate is the minimum np (and n(1-p)) for the normal
// approximation to Binomial(n,p) to be trustworthy (rule of thumb: both
// well above ~9, e.g. Feller). Below it, a swarm large enough to skip exact
// Bernoulli trials but firing at a low hit chance (long range: d5=0.22,
// d6=0.12) or landing hits almost every time would get a biased normal
// sample — e.g. count=31,p=0.02 (np=0.62) sampled +13% high on the mean
// with the normal approximation and diverged sharply on P(k=0) (43.9% vs
// the true 53.6%). Poisson is the correct small-p (or small-(1-p), by
// symmetry on misses) limit and is used instead in that regime.
const deMoivreLaplaceGate = 9.0

// binomial samples the number of successes in count trials at probability
// p. Below binomialExactMax it rolls each trial exactly. Above it: when np
// or n(1-p) is small (deMoivreLaplaceGate), it uses the Poisson limit
// (successes, or misses by symmetry) instead of the normal approximation,
// which is biased in that corner; otherwise a mean/stddev normal
// approximation clamped to [0, count]. All three paths are O(1) in count.
func binomial(count int, p float64, rng *rand.Rand) int {
	if p <= 0 || count <= 0 {
		return 0
	}
	if p >= 1 {
		return count
	}
	if count <= binomialExactMax {
		k := 0
		for range count {
			if rng.Float64() < p {
				k++
			}
		}
		return k
	}
	np := float64(count) * p
	nq := float64(count) * (1 - p)
	switch {
	case np < deMoivreLaplaceGate:
		return min(poisson(np, rng), count)
	case nq < deMoivreLaplaceGate:
		return count - min(poisson(nq, rng), count)
	default:
		sd := math.Sqrt(np * nq)
		k := int(math.Round(np + sd*rng.NormFloat64()))
		return min(max(k, 0), count)
	}
}

// poisson samples from a Poisson distribution with mean lambda via Knuth's
// algorithm. Cost is O(1) in expectation for the small, bounded lambda
// values (< deMoivreLaplaceGate) binomial calls this with.
func poisson(lambda float64, rng *rand.Rand) int {
	if lambda <= 0 {
		return 0
	}
	l := math.Exp(-lambda)
	k := 0
	prod := 1.0
	for {
		prod *= rng.Float64()
		if prod <= l {
			return k
		}
		k++
	}
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
		def.HitThisTick = false
		raw, dmgType, firing := attackerVolleyProfile(attacker, dist)
		if raw > 0 {
			k := binomial(aliveAttackers, hitChanceAt(dist, cal)*firing, rng)
			applyIdenticalVolleys(def, raw, dmgType, k, cal)
			def.HitThisTick = k > 0
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

// CrossPoint is one probed swarm size and its measured outcome.
type CrossPoint struct {
	N           int     `json:"n"`
	PWin        float64 `json:"p_win"`
	MedianKills int     `json:"median_kills"`
}

// Crossing is the result of a Crossover search: the smallest dominant
// swarm size (N == 0 meaning no crossing was found within nMax, i.e. ∞)
// plus every probed point along the way.
type Crossing struct {
	N           int          `json:"n"` // 0 = no crossing within nMax (∞)
	PWin        float64      `json:"p_win"`
	MedianKills int          `json:"median_kills"`
	Curve       []CrossPoint `json:"curve"`
}

// Crossover finds the smallest swarm size whose win rate exceeds 0.5 via
// exponential doubling then bisection. Win-rate is monotonic in n, so this
// visits ~2·log2(N*) sizes.
func Crossover(attacker, defender *StatBlock, cal *Calibration, nMax, runs, maxTicks int, seed uint64) Crossing {
	curve := map[int]CrossPoint{}
	probe := func(n int) CrossPoint {
		if p, ok := curve[n]; ok {
			return p
		}
		wins, kills := 0, make([]int, 0, runs)
		for s := range uint64(runs) {
			rng := rand.New(rand.NewPCG(seed+s+1, uint64(n)*2654435761))
			r := RunSwarm(attacker, defender, n, cal, maxTicks, rng)
			if r.SwarmWin {
				wins++
			}
			kills = append(kills, r.Kills)
		}
		p := CrossPoint{N: n, PWin: float64(wins) / float64(runs), MedianKills: median(kills)}
		curve[n] = p
		return p
	}
	// Gallop: 1,2,4,... until dominant or past nMax. nMax need not be a
	// power of two, so before concluding ∞ on overflow, probe nMax itself
	// — the true crossover can land strictly between the largest probed
	// power of two and nMax (e.g. nMax=41: powers 1..32 probed
	// non-dominant, 64 overflows, but 41 itself may dominate).
	lo, hi := 0, 0
	lastPow := 0 // largest power of two probed so far (non-dominant), 0 initially
	for n := 1; ; n *= 2 {
		if n > nMax {
			hi = 0 // never dominated, provisionally
			if nMax >= 1 && probe(nMax).PWin > 0.5 {
				hi = nMax
				lo = lastPow
			}
			break
		}
		if probe(n).PWin > 0.5 {
			hi = n
			lo = lastPow // last non-dominant power (0 when n==1)
			break
		}
		lastPow = n
	}
	res := Crossing{Curve: sortedCurve(curve)}
	if hi == 0 {
		return res // ∞
	}
	// Bisect (lo dominated? no; hi dominates) for the smallest dominant n.
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if probe(mid).PWin > 0.5 {
			hi = mid
		} else {
			lo = mid
		}
	}
	p := probe(hi)
	res.N, res.PWin, res.MedianKills = p.N, p.PWin, p.MedianKills
	res.Curve = sortedCurve(curve)
	return res
}

// median returns the middle element of a sorted copy of vals (0 for an
// empty slice).
func median(vals []int) int {
	if len(vals) == 0 {
		return 0
	}
	cp := make([]int, len(vals))
	copy(cp, vals)
	sort.Ints(cp)
	return cp[len(cp)/2]
}

// sortedCurve returns the probed points sorted by N.
func sortedCurve(curve map[int]CrossPoint) []CrossPoint {
	out := make([]CrossPoint, 0, len(curve))
	for _, p := range curve {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].N < out[j].N })
	return out
}
