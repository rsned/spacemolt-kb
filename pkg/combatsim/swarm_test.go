package combatsim

import (
	"math"
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
	tgt := newSide(&StatBlock{MaxHull: 1000, MaxShield: 0}, StanceFire)
	rng := rand.New(rand.NewPCG(1, 1))
	att := newSide(sb, StanceFire)
	if o := volleyAt(att, tgt, 3, testCal(), rng); o != (volleyOutcome{}) {
		t.Fatalf("out-of-reach volley fired: %+v", o)
	}
	// At d2 with hit chance 0.65 it eventually lands; force many rolls.
	landed := false
	for range 50 {
		att = newSide(sb, StanceFire)
		if o := volleyAt(att, tgt, 2, testCal(), rng); o.HullDmg > 0 {
			landed = true
			break
		}
	}
	if !landed {
		t.Fatal("in-reach weapon never landed in 50 tries")
	}
}

// TestSwarmAgreesWithMultiShipLargeN covers the n>binomialExactMax sampler
// path (Poisson/normal in binomial), which TestSwarmAgreesWithMultiShip's
// n=3,6,10 never exercises (those stay on the exact-Bernoulli path). Without
// this, the sampler that actually runs in the matrix at scale has zero
// accuracy coverage against the RunMultiShip reference.
func TestSwarmAgreesWithMultiShipLargeN(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	def := starter(t, cat, "axiom") // small tier-1 defender, quick battles
	cal := testCal()
	const runs = 100
	for _, n := range []int{40, 70, 100} {
		swarm := swarmWinRate(pro, def, n, runs, cal)
		wins := 0
		for s := range uint64(runs) {
			rng := rand.New(rand.NewPCG(s+1, 8181))
			ships := []Ship{{def, 1}}
			for range n {
				ships = append(ships, Ship{pro, 0})
			}
			if RunMultiShip(ships, cal, 4000, rng).WinningTeam == 0 {
				wins++
			}
		}
		ref := float64(wins) / runs
		if diff := swarm - ref; diff > 0.15 || diff < -0.15 {
			t.Fatalf("n=%d: RunSwarm %.2f vs RunMultiShip %.2f (>0.15 apart)", n, swarm, ref)
		}
	}
}

// TestSwarmAgreesWithMultiShipLowFiring covers an attacker whose only
// weapon has a low steady-state firing fraction f (em_disruptor_i: cd 2,
// mag 40 -> f~0.494), the case that exposed the f^2 double-count bug:
// attackerVolleyProfile folded f into expected damage AND RunSwarm folded
// it again into the binomial hit probability, so cohort DPS scaled with f^2
// instead of f (invisible for beam/high-uptime weapons where f~1, which is
// why the prospect-only coverage above never caught it).
//
// n=83 vs opus_magna is deliberately just past the very steep crossover
// this matchup has (win rate rises from ~0% to ~99% across n~=76..86); at
// the crossover itself (n~=80-82) RunSwarm and RunMultiShip can disagree by
// more than the +/-0.15 tolerance even at large run counts (a real
// staggered-fraction approximation artifact, not sampling noise) -- n=83 is
// past that knife-edge with both win rates still meaningfully non-trivial
// (neither saturated at 0 nor 1) and stable across run counts.
func TestSwarmAgreesWithMultiShipLowFiring(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	thr := starter(t, cat, "threshold")
	opus, err := ResolveHull("opus_magna", cat, true)
	if err != nil {
		t.Fatal(err)
	}
	cal := testCal()
	const n, runs = 83, 200
	swarm := swarmWinRate(thr, opus, n, runs, cal)
	wins := 0
	for s := range uint64(runs) {
		rng := rand.New(rand.NewPCG(s+1, 777))
		ships := []Ship{{opus, 1}}
		for range n {
			ships = append(ships, Ship{thr, 0})
		}
		if RunMultiShip(ships, cal, 4000, rng).WinningTeam == 0 {
			wins++
		}
	}
	ref := float64(wins) / runs
	if diff := swarm - ref; diff > 0.15 || diff < -0.15 {
		t.Fatalf("n=%d: RunSwarm %.2f vs RunMultiShip %.2f (>0.15 apart)", n, swarm, ref)
	}
}

// TestBinomialLowNPAccuracy directly checks binomial's Poisson-fallback
// corner (count > binomialExactMax but np, or n(1-p), below
// deMoivreLaplaceGate — reachable in the matrix via long-range low hit
// chances, e.g. d5=0.22/d6=0.12, once n exceeds the exact-trial threshold).
// A normal approximation is measurably biased here (previously: sampled
// mean +13% high, P(k=0) 43.9% vs a true ~53.5%); Poisson should track the
// true mean and P(k=0)/P(k=count) closely.
func TestBinomialLowNPAccuracy(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	const count, p = 31, 0.02 // np = 0.62, well below deMoivreLaplaceGate
	const runs = 100000
	trueMean := float64(count) * p
	truePZero := math.Pow(1-p, count) // exact Binomial(count,p) P(k=0)
	sum, zeros := 0, 0
	for range runs {
		k := binomial(count, p, rng)
		sum += k
		if k == 0 {
			zeros++
		}
	}
	mean := float64(sum) / runs
	pZero := float64(zeros) / runs
	if math.Abs(mean-trueMean) > 0.05*trueMean {
		t.Fatalf("mean=%.4f, want within 5%% of true mean %.4f", mean, trueMean)
	}
	if math.Abs(pZero-truePZero) > 0.05 {
		t.Fatalf("P(k=0)=%.4f, want within 0.05 of true %.4f", pZero, truePZero)
	}

	// Mirrored corner: n(1-p) small (high hit chance, e.g. close range).
	const count2, p2 = 31, 0.98
	trueMean2 := float64(count2) * p2
	truePFull := math.Pow(p2, count2) // exact Binomial(count2,p2) P(k=count2)
	sum2, full2 := 0, 0
	for range runs {
		k := binomial(count2, p2, rng)
		sum2 += k
		if k == count2 {
			full2++
		}
	}
	mean2 := float64(sum2) / runs
	pFull := float64(full2) / runs
	if math.Abs(mean2-trueMean2) > 0.05*trueMean2 {
		t.Fatalf("mean=%.4f, want within 5%% of true mean %.4f", mean2, trueMean2)
	}
	if math.Abs(pFull-truePFull) > 0.05 {
		t.Fatalf("P(k=count)=%.4f, want within 0.05 of true %.4f", pFull, truePFull)
	}
}

func starter(t *testing.T, cat *Catalog, id string) *StatBlock {
	t.Helper()
	sb, err := ResolveHull(id, cat, false)
	if err != nil {
		t.Fatal(err)
	}
	return sb
}

func TestMultiShipFocusFireWins(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	// 6 Prospects vs 1 Prospect: the six must win, and lose few.
	ships := []Ship{{pro, 1}}
	for range 6 {
		ships = append(ships, Ship{pro, 0})
	}
	wins := 0
	for s := range uint64(40) {
		rng := rand.New(rand.NewPCG(s+1, 99))
		if RunMultiShip(ships, testCal(), 500, rng).WinningTeam == 0 {
			wins++
		}
	}
	if wins < 36 { // 6v1 should be near-certain
		t.Fatalf("6v1 swarm won %d/40, expected >=36", wins)
	}
}

func TestMultiShipSoloDuel(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	// 1v1 identical Prospects: someone wins or it times out; must not panic
	// and must terminate.
	rng := rand.New(rand.NewPCG(7, 7))
	r := RunMultiShip([]Ship{{pro, 0}, {pro, 1}}, testCal(), 500, rng)
	if r.Ticks == 0 {
		t.Fatal("battle ran zero ticks")
	}
}

// TestMultiShipNoWithinTickTargetCycling is a dev-confirmed rule: a ship's
// target is locked at the START of a tick (lowest-index living enemy at
// that moment). If that target dies partway through the tick — killed by an
// earlier shooter's volley in the SAME tick — later shooters whose locked
// target was that same ship do NOT retarget to the next-lowest survivor;
// their shot is simply lost.
//
// Setup: 3 unarmed attackers (team 0, indices 0-2, so they never fire and
// can't confound the result) vs 2 one-shot-kill defenders (team 1, indices
// 3-4). Hit chance is forced to 1.0 (deterministic — no RNG dependence).
// Both defenders compute their tick-start target independently and land on
// the SAME ship: attacker index 0, the lowest-index living team-0 ship at
// the moment targets are snapshotted.
//
// Firing proceeds in ship-index order: defender 3 fires first and kills
// attacker 0. Defender 4 fires next with its target STILL locked to
// attacker 0 (computed once, at tick start) — but attacker 0 is now dead,
// so defender 4's shot is void.
//
// Under the OLD (buggy) per-shot target computation, defender 4 would
// re-scan for "the lowest-index living enemy" at the MOMENT it fires, find
// attacker 0 already dead, and retarget to attacker 1 — landing a second
// kill in the same tick. That is exactly the within-tick cycling the devs
// say does not happen; this test would see KillsByTeam[1] == 2 under that
// code, not 1.
func TestMultiShipNoWithinTickTargetCycling(t *testing.T) {
	atk := &StatBlock{Name: "unarmed-hulk", MaxHull: 10, MaxShield: 0} // no Weapons: never fires
	def := &StatBlock{Name: "one-shot", MaxHull: 1000, MaxShield: 0,
		Weapons: []Weapon{{Damage: 1000, Type: "kinetic", Cooldown: 1, Magazine: 500, Reach: 6}}}
	ships := []Ship{{atk, 0}, {atk, 0}, {atk, 0}, {def, 1}, {def, 1}}
	cal := testCal()
	cal.HitChanceByDistance = []float64{1, 1, 1, 1, 1, 1, 1} // always hit: fully deterministic
	rng := rand.New(rand.NewPCG(1, 1))

	r := RunMultiShip(ships, cal, 1, rng)

	if r.Ticks != 1 {
		t.Fatalf("Ticks = %d, want 1", r.Ticks)
	}
	if r.WinningTeam != -1 {
		t.Fatalf("WinningTeam = %d, want -1 (both teams still have survivors after 1 tick)", r.WinningTeam)
	}
	if got := r.KillsByTeam[1]; got != 1 {
		t.Fatalf("KillsByTeam[1] = %d, want exactly 1 (defender 4's shot at the "+
			"already-dead attacker 0 must be lost, not redirected to attacker 1)", got)
	}
	if got := r.KillsByTeam[0]; got != 0 {
		t.Fatalf("KillsByTeam[0] = %d, want 0 (attackers are unarmed)", got)
	}
}

// TestMultiShipLockedTargetStillLandsWhenSurviving is the positive
// counterpart to TestMultiShipNoWithinTickTargetCycling: when a ship's
// locked target does NOT die mid-tick, the tick-start target lock must not
// interfere with an ordinary landed hit. A single attacker vs a single
// unarmed defender has no other ship to retarget to either way, so this
// isolates "does the lock still deliver a normal hit" from the cycling
// question.
//
// The defender's hull is exactly two hits' worth of damage: it survives
// tick 1 (proving that hit landed — a lost/no-op shot would leave the
// defender at full health and this battle would never end) and dies on
// tick 2 (proving the second hit landed too, right on schedule).
func TestMultiShipLockedTargetStillLandsWhenSurviving(t *testing.T) {
	attacker := &StatBlock{Name: "striker", MaxHull: 50, MaxShield: 0,
		Weapons: []Weapon{{Damage: 5, Type: "kinetic", Cooldown: 1, Magazine: 500, Reach: 6}}}
	target := &StatBlock{Name: "punching-bag", MaxHull: 10, MaxShield: 0} // no Weapons: never fires back
	ships := []Ship{{attacker, 0}, {target, 1}}
	cal := testCal()
	cal.HitChanceByDistance = []float64{1, 1, 1, 1, 1, 1, 1} // always hit: fully deterministic
	rng := rand.New(rand.NewPCG(2, 2))

	r := RunMultiShip(ships, cal, 10, rng)

	if r.WinningTeam != 0 {
		t.Fatalf("WinningTeam = %d, want 0 (attacker survives, unarmed defender does not)", r.WinningTeam)
	}
	if r.Ticks != 2 {
		t.Fatalf("Ticks = %d, want exactly 2 (5dmg/hit needs 2 landed hits to down a 10-hull target; "+
			"a dropped tick-1 hit would push this to 3+ ticks)", r.Ticks)
	}
	if got := r.KillsByTeam[0]; got != 1 {
		t.Fatalf("KillsByTeam[0] = %d, want 1", got)
	}
}

func swarmWinRate(attacker, defender *StatBlock, n, runs int, cal *Calibration) float64 {
	wins := 0
	for s := range uint64(runs) {
		rng := rand.New(rand.NewPCG(s+1, 12345))
		if RunSwarm(attacker, defender, n, cal, 4000, rng).SwarmWin {
			wins++
		}
	}
	return float64(wins) / float64(runs)
}

func TestSwarmMonotonicAndFloor(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	opus, err := ResolveHull("opus_magna", cat, true)
	if err != nil {
		t.Fatal(err)
	}
	cal := testCal()
	// Floor: a lone Prospect cannot beat the Opus Magna (its chip < 25/tick
	// regen, and it dies in one volley).
	if r := swarmWinRate(pro, opus, 1, 40, cal); r > 0.0 {
		t.Fatalf("1 Prospect beat Opus Magna %.0f%% of the time, want 0", r*100)
	}
	// Monotonic: win rate never decreases across a rising ladder.
	prev := -1.0
	for _, n := range []int{50, 200, 800, 3000} {
		r := swarmWinRate(pro, opus, n, 40, cal)
		if r < prev-0.05 {
			t.Fatalf("win rate dropped at n=%d: %.2f < %.2f", n, r, prev)
		}
		prev = r
	}
	// A large enough swarm dominates.
	if r := swarmWinRate(pro, opus, 25000, 40, cal); r < 0.9 {
		t.Fatalf("25000 Prospects won only %.0f%%, want >=90%%", r*100)
	}
}

func TestSwarmAgreesWithMultiShip(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	def := starter(t, cat, "axiom") // small tier-1 defender, quick battles
	cal := testCal()
	for _, n := range []int{3, 6, 10} {
		swarm := swarmWinRate(pro, def, n, 200, cal)
		// Reference: RunMultiShip with n identical attackers vs 1 defender.
		wins := 0
		for s := range uint64(200) {
			rng := rand.New(rand.NewPCG(s+1, 777))
			ships := []Ship{{def, 1}}
			for range n {
				ships = append(ships, Ship{pro, 0})
			}
			if RunMultiShip(ships, cal, 4000, rng).WinningTeam == 0 {
				wins++
			}
		}
		ref := float64(wins) / 200
		if diff := swarm - ref; diff > 0.15 || diff < -0.15 {
			t.Fatalf("n=%d: RunSwarm %.2f vs RunMultiShip %.2f (>0.15 apart)", n, swarm, ref)
		}
	}
}

func TestReloadCycle(t *testing.T) {
	// mag 2, cd 1: fires t0,t1 then must reload for 1 idle tick before t3.
	sb := &StatBlock{Name: "m2", Weapons: []Weapon{{Damage: 5, Type: "kinetic", Cooldown: 1, Magazine: 2, Reach: 6}}}
	att := newSide(sb, StanceFire)
	tgt := newSide(&StatBlock{MaxHull: 1000}, StanceFire)
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

func TestCrossoverGallopBisect(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	weak := starter(t, cat, "prospect") // Prospect vs Prospect: crossover is tiny
	c := Crossover(pro, weak, testCal(), 25000, 120, 4000, 42)
	if c.N < 1 || c.N > 8 {
		t.Fatalf("Prospect-vs-Prospect crossover N=%d, expected small (1..8)", c.N)
	}
	if c.PWin <= 0.5 {
		t.Fatalf("crossover PWin=%.2f must exceed 0.5", c.PWin)
	}
	// Just below the crossover must NOT dominate (defines "smallest").
	if c.N > 1 {
		below := swarmWinRate(pro, weak, c.N-1, 200, testCal())
		if below > 0.5 {
			t.Fatalf("n=%d already dominates (%.2f); crossover not minimal", c.N-1, below)
		}
	}
	if len(c.Curve) == 0 {
		t.Fatal("curve not recorded")
	}
}

func TestCrossoverInfinite(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	opus, err := ResolveHull("opus_magna", cat, true)
	if err != nil {
		t.Fatal(err)
	}
	// A tiny cap cannot beat a dreadnought → N==0 (∞).
	c := Crossover(pro, opus, testCal(), 4, 60, 4000, 7)
	if c.N != 0 {
		t.Fatalf("expected ∞ (N==0) under n-max 4, got N=%d", c.N)
	}
}

// TestCrossoverNMaxNotPowerOfTwo guards the gallop overflow branch: the
// smallest dominant N can land strictly between a probed power of two and
// nMax when nMax itself is not a power of two. The doubling loop must probe
// nMax before concluding ∞ — otherwise a true, finite crossover just above
// the largest probed power of two gets misreported as unbeatable (N==0).
//
// This is self-adjusting: it first learns the matchup's true crossover C
// with a generous nMax (so gallop finds dominance well before overflowing,
// unaffected by the bug under test), then re-runs with nMax pinned to
// exactly C. If C happens to be a power of two the second call is a no-op
// check; for this matchup/seed C==41, which lands in the (32, 64] window
// and exercises the overflow-probe branch.
func TestCrossoverNMaxNotPowerOfTwo(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	pro := starter(t, cat, "prospect")
	opus, err := ResolveHull("opus_magna", cat, true)
	if err != nil {
		t.Fatal(err)
	}
	learned := Crossover(pro, opus, testCal(), 200000, 60, 4000, 7)
	if learned.N == 0 {
		t.Fatal("expected a finite crossover with a generous nMax")
	}
	pinned := Crossover(pro, opus, testCal(), learned.N, 60, 4000, 7)
	if pinned.N != learned.N {
		t.Fatalf("nMax=%d (== true crossover): got N=%d, want N=%d (not ∞)", learned.N, pinned.N, learned.N)
	}
}
