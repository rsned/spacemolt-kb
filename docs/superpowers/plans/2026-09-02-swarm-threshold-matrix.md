# Swarm Threshold Matrix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Compute, for every catalog hull, the smallest swarm of each empire's starter ship that reliably destroys it — a ~375×5 matrix — and present it as an interactive `did_you_know` KB page, backed by a fast swarm-combat engine in `cmd/combat-sim` and validated against real multi-ship battles.

**Architecture:** Extend the Phase-B-calibrated `cmd/combat-sim` with (a) an any-tier `default_modules` resolver and weapon reach, (b) a distance/reach/reload-aware volley, (c) a general per-ship multi-ship engine (`RunMultiShip`) for heterogeneous battles and validation, (d) a fast homogeneous counting engine (`RunSwarm`) for the matrix, (e) a galloping+bisect crossover finder. A new `cmd/generate-last-stand` runs the matrix and renders the page.

**Tech Stack:** Go 1.24+ (`math/rand/v2`), stdlib only. Self-contained HTML+inline-JS page (no external libs), matching `kb/did_you_know/warp_simulator.html`.

**Spec:** `docs/superpowers/specs/2026-09-02-swarm-threshold-matrix-design.md` (read it; the plan argues from it)

## Global Constraints

- Go 1.24+; use `b.Loop()` in any benchmark; range-over-int where natural.
- `golangci-lint` clean (no new findings); `go test ./...` green after every task.
- Hermetic: read only committed `data/combat-sim/catalog/` snapshots and `data/battles/` fixtures; no network in the sim or its tests.
- **Stated assumptions, surfaced on the page:** no capital weapon bonus; unlimited ammo with a 1-tick reload per emptied magazine; default ammo only (base catalog damage/type, no exotic-round modifiers); `fire` stance both sides; one target per tick; d6→d0 approach closing 1 ring/tick, reach-gated.
- Reuse `data/combat-sim/calibration.json` verbatim — do **not** re-fit combat constants.
- Do not change the existing flat 1v1 `RunBattle` or its golden duel tests. The reload/reach/distance behavior lives only in the new swarm code path.
- Binaries → `bin/` (gitignored). Never `git add -A` blindly.
- All new package-level exported symbols in `package main` under `cmd/combat-sim`.

## File Structure

- Modify `cmd/combat-sim/loader.go` — add `DefaultModules []string` to `ShipDef`.
- Modify `cmd/combat-sim/resolver.go` — add `Reach int` to `Weapon`; populate reach in `Resolve`; add `ResolveHull`.
- Modify `cmd/combat-sim/engine.go` — add `Reload []int` to `SideState`; init in `NewSide`.
- Modify `cmd/combat-sim/battle.go` — add `HitChanceByDistance []float64` to `Calibration` + default.
- Create `cmd/combat-sim/swarm.go` — `hitChanceAt`, `volleyAt`, `tickWeapons`, `applyIdenticalVolleys`, `RunMultiShip`, `RunSwarm`, `Crossover`.
- Create `cmd/combat-sim/swarm_test.go` — engine + crossover unit tests.
- Create `cmd/combat-sim/swarm_validation_test.go` — real-battle golden replays.
- Modify `cmd/combat-sim/main.go` — `--swarm/--vs/--n-max/--runs/--swarm-json` mode.
- Create `cmd/generate-last-stand/main.go` (+ `render.go`, `render_test.go`) — matrix JSON + page.
- Create `data/combat-sim/last_stand_matrix.json` — committed matrix output.
- Create `kb/did_you_know/last_stand.html`; modify `kb/did_you_know/index.html`.
- Modify `cmd/combat-sim/README.md`; append errata to the spec.

---

### Task 1: Any-tier resolver + weapon reach

**Files:**
- Modify: `cmd/combat-sim/loader.go` (`ShipDef`)
- Modify: `cmd/combat-sim/resolver.go` (`Weapon`, `Resolve`, new `ResolveHull`)
- Test: `cmd/combat-sim/resolver_test.go`

**Interfaces:**
- Consumes: existing `Catalog`, `FitSpec`, `Resolve`.
- Produces: `Weapon.Reach int`; `ShipDef.DefaultModules []string`; `func ResolveHull(hullID string, cat *Catalog, allowCapital bool) (*StatBlock, error)` — builds a `FitSpec{Hull: hullID, Modules: hull.DefaultModules}` and resolves it, bypassing the tier≥5 guard when `allowCapital`. No capital weapon bonus is applied (identical damage math to `Resolve`).

- [ ] **Step 1: Write the failing test**

Add to `cmd/combat-sim/resolver_test.go`:

```go
func TestResolveHullCapital(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	// Opus Magna is tier 5: rejected as an attacker, allowed as a defender.
	if _, err := ResolveHull("opus_magna", cat, false); err == nil {
		t.Fatal("expected tier-5 rejection when allowCapital=false")
	}
	sb, err := ResolveHull("opus_magna", cat, true)
	if err != nil {
		t.Fatal(err)
	}
	if sb.MaxHull != 3000 || sb.MaxShield != 2400 {
		t.Fatalf("stock Opus Magna: hull=%d shield=%d want 3000/2400", sb.MaxHull, sb.MaxShield)
	}
	if len(sb.Weapons) != 8 {
		t.Fatalf("Opus Magna default_modules give 8 weapons, got %d", len(sb.Weapons))
	}
	// Reach must be populated from the catalog item (judgment_beam reach 4).
	var maxReach int
	for _, w := range sb.Weapons {
		if w.Reach > maxReach {
			maxReach = w.Reach
		}
	}
	if maxReach != 4 {
		t.Fatalf("max mount reach = %d, want 4", maxReach)
	}
}

func TestResolveStarterReach(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	sb, err := ResolveHull("shard", cat, false) // Crimson starter: 2× autocannon_i
	if err != nil {
		t.Fatal(err)
	}
	if len(sb.Weapons) != 2 {
		t.Fatalf("shard has 2 autocannons, got %d", len(sb.Weapons))
	}
	for _, w := range sb.Weapons {
		if w.Type != "kinetic" || w.Reach != 2 {
			t.Fatalf("autocannon_i: type=%s reach=%d want kinetic/2", w.Type, w.Reach)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestResolveHull|TestResolveStarterReach' -v`
Expected: FAIL (`ResolveHull` undefined; `Reach` field missing).

- [ ] **Step 3: Add `DefaultModules` to `ShipDef`**

In `cmd/combat-sim/loader.go`, add the field to `ShipDef`:

```go
	BaseArmor          int      `json:"base_armor"`
	DefaultModules     []string `json:"default_modules"`
```

- [ ] **Step 4: Add `Reach`, populate it, add `ResolveHull`**

In `cmd/combat-sim/resolver.go`, add `Reach int` to the `Weapon` struct, set it when appending weapons in `Resolve`:

```go
		if it.Slot == "weapon" && it.Damage > 0 {
			sb.Weapons = append(sb.Weapons, Weapon{
				Name: it.ID, Damage: it.Damage, Type: it.DamageType,
				Cooldown: it.Cooldown, Magazine: it.MagazineSize, Reach: it.Reach,
			})
		}
```

Refactor the tier guard so `Resolve` and `ResolveHull` share the body. Add:

```go
// ResolveHull resolves a hull's stock loadout (its default_modules). When
// allowCapital is true the tier>=5 guard is skipped so any warship can be a
// defender; NO capital weapon bonus is applied (stated assumption). Used for
// swarm defenders and starter attackers alike.
func ResolveHull(hullID string, cat *Catalog, allowCapital bool) (*StatBlock, error) {
	hull, ok := cat.Ships[hullID]
	if !ok {
		return nil, fmt.Errorf("unknown hull %q", hullID)
	}
	fit := &FitSpec{Name: hull.Name, Hull: hullID, Modules: hull.DefaultModules}
	return resolveFit(fit, cat, allowCapital)
}
```

Extract the current `Resolve` body into `resolveFit(fit *FitSpec, cat *Catalog, allowCapital bool)`, moving the guard to:

```go
	if hull.Tier >= 5 && !allowCapital {
		return nil, fmt.Errorf("hull %q: capital hulls unsupported as attacker in v1", fit.Hull)
	}
```

Keep the existing `func Resolve(fit *FitSpec, cat *Catalog) (*StatBlock, error) { return resolveFit(fit, cat, false) }` so current callers and tests are unchanged.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run 'TestResolveHull|TestResolveStarterReach|TestResolve' -v`
Expected: PASS (existing resolver tests still green).

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./cmd/combat-sim/...`

```bash
git add cmd/combat-sim/loader.go cmd/combat-sim/resolver.go cmd/combat-sim/resolver_test.go
git commit -m "combat-sim: any-tier default_modules resolver + weapon reach"
```

---

### Task 2: Calibration distance array + reach/distance/reload-aware volley

**Files:**
- Modify: `cmd/combat-sim/battle.go` (`Calibration`, `DefaultCalibration`)
- Modify: `cmd/combat-sim/engine.go` (`SideState`, `NewSide`)
- Create: `cmd/combat-sim/swarm.go` (`hitChanceAt`, `volleyAt`, `tickWeapons`)
- Test: `cmd/combat-sim/swarm_test.go`

**Interfaces:**
- Consumes: `SideState`, `ResolveVolley`, `stanceInMult`, `armorReduce`, `shieldEff`.
- Produces:
  - `Calibration.HitChanceByDistance []float64` (index = zone_distance 0..6).
  - `SideState.Reload []int` (per-weapon reload timer; 0 = ready).
  - `func hitChanceAt(dist int, cal *Calibration) float64`
  - `func volleyAt(att, tgt *SideState, dist int, cal *Calibration, rng *rand.Rand) VolleyOutcome` — like `volley` but gates each weapon on `dist <= Reach`, honors cooldown+reload, and uses `hitChanceAt(dist)`. One hit roll per ship-volley (unchanged model).
  - `func tickWeapons(s *SideState)` — decrement cooldowns, advance reload timers, refill magazine when a reload completes.

- [ ] **Step 1: Write the failing test**

Create `cmd/combat-sim/swarm_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestHitChanceAt|TestVolleyReachGate|TestReloadCycle' -v`
Expected: FAIL (symbols undefined).

- [ ] **Step 3: Add the calibration field**

In `cmd/combat-sim/battle.go`, add to `Calibration`:

```go
	HitChanceByDistance []float64 `json:"hit_chance_by_distance"`
```

and in `DefaultCalibration()` set it (matches calibration.json):

```go
		HitChanceByDistance: []float64{0.90, 0.80, 0.65, 0.50, 0.35, 0.22, 0.12},
```

- [ ] **Step 4: Add reload state to `SideState`**

In `cmd/combat-sim/engine.go`, add `Reload []int` to `SideState`, and in `NewSide` initialize it alongside `Cool`:

```go
	s.Ammo = make([]int, len(sb.Weapons))
	s.Cool = make([]int, len(sb.Weapons))
	s.Reload = make([]int, len(sb.Weapons))
```

- [ ] **Step 5: Implement `hitChanceAt`, `volleyAt`, `tickWeapons`**

Create `cmd/combat-sim/swarm.go`:

```go
package main

import "math/rand/v2"

const swarmStartDistance = 6 // battles open at Outer (d6)

func hitChanceAt(dist int, cal *Calibration) float64 {
	if dist >= 0 && dist < len(cal.HitChanceByDistance) {
		return cal.HitChanceByDistance[dist]
	}
	return cal.HitChanceA // flat fallback
}

// volleyAt is the zone-aware volley: each weapon fires only when in reach
// (dist <= Reach), off cooldown, and not mid-reload. One hit roll per ship
// volley, hit chance from the distance table. Ammo is unlimited; emptying a
// magazine schedules a 1-tick reload (see tickWeapons).
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
				att.Reload[i] = 1 // empty magazine → one idle reload tick
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
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run 'TestHitChanceAt|TestVolleyReachGate|TestReloadCycle|TestLoadCalibration' -v`
Expected: PASS. Then full package: `go test ./cmd/combat-sim/` PASS.

- [ ] **Step 7: Lint + commit**

Run: `golangci-lint run ./cmd/combat-sim/...`

```bash
git add cmd/combat-sim/battle.go cmd/combat-sim/engine.go cmd/combat-sim/swarm.go cmd/combat-sim/swarm_test.go
git commit -m "combat-sim: zone/reach/reload-aware volley + distance hit table"
```

---

### Task 3: General multi-ship engine (`RunMultiShip`)

**Files:**
- Modify: `cmd/combat-sim/swarm.go`
- Test: `cmd/combat-sim/swarm_test.go`

**Interfaces:**
- Consumes: `SideState`, `NewSide`, `volleyAt`, `regen`, `tickWeapons`.
- Produces:
  - `type Ship struct { Stats *StatBlock; Team int }`
  - `type MultiResult struct { WinningTeam int; Ticks int; KillsByTeam map[int]int }` (`WinningTeam == -1` = stalemate/timeout)
  - `func RunMultiShip(ships []Ship, cal *Calibration, maxTicks int, rng *rand.Rand) MultiResult`

**Engine rules (from spec §5.1–5.2):** all ships start at `swarmStartDistance` and close 1 ring/tick to 0 (shared scalar). Each ship targets one enemy per tick (lowest-index living enemy on another team — deterministic focus fire; a real melee resolves the same at the population level). All ships' volleys for the tick are resolved sequentially against the current defender state (shields deplete within the tick). A team wins when all other teams are dead; timeout/`maxTicks` → `WinningTeam = -1`.

- [ ] **Step 1: Write the failing test**

Add to `cmd/combat-sim/swarm_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestMultiShip' -v`
Expected: FAIL (`Ship`/`RunMultiShip` undefined).

- [ ] **Step 3: Implement `RunMultiShip`**

Add to `cmd/combat-sim/swarm.go`:

```go
type Ship struct {
	Stats *StatBlock
	Team  int
}

type MultiResult struct {
	WinningTeam int // -1 = stalemate/timeout
	Ticks       int
	KillsByTeam map[int]int
}

// RunMultiShip simulates a heterogeneous battle: every ship closes from
// Outer, fires at one enemy per tick, and a team wins when it is the only
// one left alive. Volleys within a tick resolve sequentially so shields
// deplete in order.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run 'TestMultiShip' -v`
Expected: PASS.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./cmd/combat-sim/...`

```bash
git add cmd/combat-sim/swarm.go cmd/combat-sim/swarm_test.go
git commit -m "combat-sim: general multi-ship battle engine (RunMultiShip)"
```

---

### Task 4: Fast homogeneous swarm engine (`RunSwarm`)

**Files:**
- Modify: `cmd/combat-sim/swarm.go`
- Test: `cmd/combat-sim/swarm_test.go`

**Interfaces:**
- Consumes: `SideState`, `NewSide`, `volleyAt` (for the focused attacker + defender), `regen`, `tickWeapons`, `ResolveVolley`, `armorReduce`, `shieldEff`.
- Produces:
  - `type SwarmResult struct { SwarmWin bool; Kills int; Ticks int }`
  - `func RunSwarm(attacker, defender *StatBlock, n int, cal *Calibration, maxTicks int, rng *rand.Rand) SwarmResult`
  - `func applyIdenticalVolleys(def *SideState, raw int, dmgType string, k int, cal *Calibration)` — O(1) application of `k` identical expected-damage volleys (closed-form shield depletion then bulk hull).

**Model (spec §5.3–5.4):** `n` identical attackers focus the lone defender. Track a healthy count + one focused attacker actually under the defender's guns. Per tick: (1) sample how many attackers land a volley on the defender and apply in closed form; (2) the defender's real `volleyAt` fires at the focused attacker (a real `SideState`, so cooldown/reload/reach are exact); a killed focus increments `Kills` and is replaced next tick from the healthy pool; (3) regen + cooldowns + close distance. Terminal: defender hull ≤ 0 → win; no attackers left → loss; timeout → loss.

The attacker landing count is `Binomial(aliveAttackers, p)` where `p = hitChanceAt(dist) × firingFraction`. `firingFraction` and per-landing `raw` come from `attackerVolleyProfile(attacker, dist, cal)` which sums the in-reach weapons' expected damage `Σ dmg_w · (1 + critPct/100 · 0.5)` and the steady-state ready fraction `Σ m_w/(m_w·c_w+1)` normalized so a single always-ready weapon → fraction 1. Single-type attackers only (starters are single-type); `raw` carries that one `dmgType`.

- [ ] **Step 1: Write the failing test**

Add to `cmd/combat-sim/swarm_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestSwarm(Monotonic|Agrees)' -v`
Expected: FAIL (`RunSwarm` undefined).

- [ ] **Step 3: Implement `applyIdenticalVolleys`, the volley profile, and `RunSwarm`**

Add to `cmd/combat-sim/swarm.go`:

```go
import (
	"math"
	"math/rand/v2"
)

type SwarmResult struct {
	SwarmWin bool
	Kills    int
	Ticks    int
}

// attackerVolleyProfile returns the expected combined raw damage of one
// landing volley at this distance, its single damage type, and the fraction
// of ticks an attacker is ready to fire (steady-state cooldown+reload).
func attackerVolleyProfile(sb *StatBlock, dist int) (raw int, dmgType string, firing float64) {
	critBoost := 1 + float64(sb.CritPct)/100*0.5
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
		expected += float64(w.Damage) * critBoost * f
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
// homogeneous cohort model (O(1) per tick regardless of n).
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
```

Note: the `import` block at the top of `swarm.go` must be merged (add `"math"`). Keep the file's single import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run 'TestSwarm' -v`
Expected: PASS. If `TestSwarmAgreesWithMultiShip` is outside tolerance, adjust `attackerVolleyProfile`/`applyIdenticalVolleys` until the cohort tracks the reference — the agreement test is the binding correctness gate, not the exact formula above.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./cmd/combat-sim/...`

```bash
git add cmd/combat-sim/swarm.go cmd/combat-sim/swarm_test.go
git commit -m "combat-sim: fast homogeneous swarm engine (RunSwarm) + cohort application"
```

---

### Task 5: Crossover finder (galloping + bisect)

**Files:**
- Modify: `cmd/combat-sim/swarm.go`
- Test: `cmd/combat-sim/swarm_test.go`

**Interfaces:**
- Consumes: `RunSwarm`.
- Produces:
  - `type CrossPoint struct { N int; PWin float64; MedianKills int }`
  - `type Crossing struct { N int; PWin float64; MedianKills int; Curve []CrossPoint }` — `N == 0` means no crossing ≤ `nMax` (∞).
  - `func Crossover(attacker, defender *StatBlock, cal *Calibration, nMax, runs, maxTicks int, seed uint64) Crossing` — galloping doubling to find a dominant `N` (`PWin > 0.5`), then bisect `[lo, hi]` for the smallest dominant `N`. Records probed points as the `Curve` (sorted, deduped).

- [ ] **Step 1: Write the failing test**

Add to `cmd/combat-sim/swarm_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestCrossover' -v`
Expected: FAIL (`Crossover` undefined).

- [ ] **Step 3: Implement `Crossover`**

Add to `cmd/combat-sim/swarm.go`:

```go
type CrossPoint struct {
	N           int     `json:"n"`
	PWin        float64 `json:"p_win"`
	MedianKills int     `json:"median_kills"`
}

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
	// Gallop: 1,2,4,... until dominant or past nMax.
	lo, hi := 0, 0
	for n := 1; ; n *= 2 {
		if n > nMax {
			hi = 0 // never dominated
			break
		}
		if probe(n).PWin > 0.5 {
			hi = n
			lo = n / 2 // last non-dominant power (0 when n==1)
			break
		}
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
```

Add small helpers `median([]int) int` (sort a copy, middle element, 0 for empty) and `sortedCurve(map[int]CrossPoint) []CrossPoint` (values sorted by `N`) to `swarm.go`. Import `"sort"` (or `slices`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run 'TestCrossover' -v`
Expected: PASS.

- [ ] **Step 5: Lint + full package test + commit**

Run: `golangci-lint run ./cmd/combat-sim/...` and `go test ./cmd/combat-sim/`

```bash
git add cmd/combat-sim/swarm.go cmd/combat-sim/swarm_test.go
git commit -m "combat-sim: galloping+bisect swarm crossover finder"
```

---

### Task 6: CLI swarm mode

**Files:**
- Modify: `cmd/combat-sim/main.go`
- Test: `cmd/combat-sim/swarm_test.go` (invoke the extracted helper directly)

**Interfaces:**
- Consumes: `LoadCatalog`, `LoadCalibration`, `ResolveHull`, `Crossover`.
- Produces: new flags `--swarm <hull>`, `--vs <hull>`, `--n-max` (default 25000), `--runs` (default 300), `--swarm-json <path>`; a helper `func runSwarmCLI(attackerID, defenderID string, cat *Catalog, cal *Calibration, nMax, runs, maxTicks int, seed uint64, jsonOut string, w io.Writer) error` that prints a one-line result and optionally writes the `Crossing` JSON.

- [ ] **Step 1: Write the failing test**

Add to `cmd/combat-sim/swarm_test.go`:

```go
func TestRunSwarmCLI(t *testing.T) {
	cat, err := LoadCatalog("../../data/combat-sim/catalog")
	if err != nil {
		t.Fatal(err)
	}
	var buf strings.Builder
	if err := runSwarmCLI("prospect", "opus_magna", cat, testCal(), 25000, 60, 4000, 42, "", &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "prospect") || !strings.Contains(out, "opus_magna") {
		t.Fatalf("summary missing ids: %q", out)
	}
}
```

(Add `"strings"` to the test imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run 'TestRunSwarmCLI' -v`
Expected: FAIL (`runSwarmCLI` undefined).

- [ ] **Step 3: Implement the flags and helper**

In `cmd/combat-sim/main.go` add the flags, and before the existing `--a/--b` path branch on `*swarm != ""`. Load the catalog (already done), load calibration, resolve attacker with `ResolveHull(id, cat, false)` and defender with `ResolveHull(id, cat, true)`, call `runSwarmCLI`, and return. Implement `runSwarmCLI` to print e.g.:

```
prospect swarm vs opus_magna: crossover N=<n> (P=<pwin>), titan kills <medianKills>   [or: N=∞ within 25000]
```

and, when `jsonOut != ""`, `json.MarshalIndent` the `Crossing` to that path. Use `maxTicks` from `cal.MaxTicks` unless a generous swarm cap is set; default the swarm cap to 4000 (constant `swarmMaxTicks = 4000`) so long grinds against capitals resolve.

- [ ] **Step 4: Run test + manual smoke**

Run: `go test ./cmd/combat-sim/ -run 'TestRunSwarmCLI' -v` → PASS
Run: `go run ./cmd/combat-sim --swarm prospect --vs opus_magna --runs 60`
Expected: one summary line with a finite N in the low thousands.

- [ ] **Step 5: Lint + commit**

Run: `golangci-lint run ./cmd/combat-sim/...`

```bash
git add cmd/combat-sim/main.go cmd/combat-sim/swarm_test.go
git commit -m "combat-sim: --swarm/--vs CLI crossover mode"
```

---

### Task 7: Matrix generator (`cmd/generate-last-stand`, JSON)

**Files:**
- Create: `cmd/generate-last-stand/main.go`
- Test: `cmd/generate-last-stand/main_test.go`
- Output: `data/combat-sim/last_stand_matrix.json`

**Interfaces:**
- Consumes (via the combat-sim package? No — `cmd/combat-sim` is `package main`): the generator is a **separate** binary and cannot import `cmd/combat-sim`. Therefore the crossover computation it needs must be reachable. Resolve this by having the generator shell out is NOT hermetic-friendly; instead **move the reusable engine into a library package** `pkg/swarmsim` OR keep the generator importing a small internal package.

**Decision (Ruling to record):** extract the swarm engine + resolver-from-catalog into a new library package `pkg/combatsim` that both `cmd/combat-sim` and `cmd/generate-last-stand` import. To keep this plan's earlier tasks stable, do the extraction as the first steps of THIS task: move `engine.go`, `resolver.go`, `loader.go`, `battle.go`, `table.go`, `swarm.go` types into `pkg/combatsim` with exported names, leaving `cmd/combat-sim` as a thin CLI over the package. If that refactor is too large for one task, the fallback is to duplicate only the small `Crossing`/matrix loop by having `cmd/generate-last-stand` invoke `combat-sim --swarm ... --swarm-json` per cell via `os/exec` against the built binary — but prefer the package extraction.

> Implementer: prefer the `pkg/combatsim` extraction. Verify `go test ./...` stays green after moving files (update package clause + qualify now-exported identifiers). Then the generator imports `pkg/combatsim`.

- [ ] **Step 1: Extract `pkg/combatsim`**

Move the combat model (loader, resolver, engine, battle, table, swarm) into `pkg/combatsim`, exporting the needed types/functions (`Catalog`, `LoadCatalog`, `ResolveHull`, `Calibration`, `LoadCalibration`, `DefaultCalibration`, `Crossover`, `Crossing`, `StatBlock`, `RunSwarm`, `RunMultiShip`, `Ship`). Reduce `cmd/combat-sim/*.go` to a CLI that imports the package. Keep all existing tests working (move engine/resolver/swarm tests into the package; keep CLI-level tests in `cmd/combat-sim`).

Run: `go test ./...` and `golangci-lint run ./...`
Expected: PASS/clean (pure move; no behavior change).

Commit:

```bash
git add -A pkg/combatsim cmd/combat-sim
git commit -m "combat-sim: extract reusable model into pkg/combatsim"
```

- [ ] **Step 2: Write the failing generator test**

Create `cmd/generate-last-stand/main_test.go`:

```go
package main

import "testing"

func TestBuildMatrixSubset(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "shard"}, []string{"axiom", "opus_magna"}, 25000, 40, 4000)
	if len(m.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(m.Rows))
	}
	// axiom (tier-1 fighter) falls to a small Prospect swarm; opus_magna needs many.
	axiom := m.cell("axiom", "prospect")
	opus := m.cell("opus_magna", "prospect")
	if axiom == 0 || axiom >= opus {
		t.Fatalf("axiom N=%d should be finite and < opus N=%d", axiom, opus)
	}
}
```

(`loadForTest` loads catalog + `testCal`-equivalent; `Matrix.cell(defID, atkID) int` returns the crossover N, 0 for ∞.)

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/generate-last-stand/ -run TestBuildMatrixSubset -v`
Expected: FAIL (undefined).

- [ ] **Step 4: Implement `BuildMatrix` + JSON writer**

Implement in `cmd/generate-last-stand/main.go`:
- `starterColumns()` → the 5 empire starter ids `["shard","prospect","cobble","theoria","threshold"]` with display metadata (empire, weapon summary, damage type).
- `BuildMatrix(cat, cal, defenderIDs, attackerIDs []string, nMax, runs, maxTicks int) Matrix` — for each defender × attacker, resolve (`ResolveHull(atk,false)`, `ResolveHull(def,true)`) and call `combatsim.Crossover`; skip defenders that fail to resolve (record a note). Parallelize across defenders with a worker pool (bounded by `GOMAXPROCS`); deterministic per-cell seed.
- `main()` flags: `-catalog`, `-calibration`, `-runs` (default 300), `-n-max` (default 25000), `-out` (matrix JSON, default `data/combat-sim/last_stand_matrix.json`), `-page` (HTML path, Task 8), `-limit` (0 = all defenders; >0 = first N, for quick runs). Default defenders = every hull in the catalog.
- Matrix JSON schema: `{ generated_utc, assumptions:[...], columns:[{id,name,empire,weapon,damage_type}], rows:[{ship_id,name,tier,class,cells:{<atkID>:{n,p_win,median_kills}|null}}] }`.

- [ ] **Step 5: Run test + generate a small matrix**

Run: `go test ./cmd/generate-last-stand/ -v` → PASS
Run: `go run ./cmd/generate-last-stand -limit 8 -runs 60 -out /tmp/lsm.json` and confirm well-formed JSON with 8 rows × 5 columns.

- [ ] **Step 6: Lint + commit** (do NOT commit the full matrix yet — Task 8 regenerates it with the page)

Run: `golangci-lint run ./cmd/generate-last-stand/...`

```bash
git add cmd/generate-last-stand/main.go cmd/generate-last-stand/main_test.go
git commit -m "generate-last-stand: swarm-threshold matrix builder"
```

---

### Task 8: Interactive KB page + index card + committed matrix

**Files:**
- Create: `cmd/generate-last-stand/render.go`, `cmd/generate-last-stand/render_test.go`
- Create: `kb/did_you_know/last_stand.html` (generated output, committed)
- Modify: `kb/did_you_know/index.html`
- Create: `data/combat-sim/last_stand_matrix.json` (committed, full run)

**Interfaces:**
- Consumes: the `Matrix` from Task 7.
- Produces: `func RenderPage(m Matrix) (string, error)` — a self-contained HTML string (inline `<style>`/`<script>`, `../smui.css` link, matching `warp_simulator.html`), and a `-page` flag wiring in `main()`.

- [ ] **Step 1: Write the failing render test**

Create `cmd/generate-last-stand/render_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestRenderPage(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "opus_magna"}, starterColumnIDs(), 25000, 20, 4000)
	html, err := RenderPage(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<title", "smui.css", "Opus Magna", "no capital weapon bonus", "id=\"matrix\""} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	// The interactive data must be embedded for client-side sort/filter.
	if !strings.Contains(html, "MATRIX_DATA") {
		t.Fatal("embedded matrix JSON missing")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-last-stand/ -run TestRenderPage -v`
Expected: FAIL (`RenderPage` undefined).

- [ ] **Step 3: Implement `RenderPage`**

Build the page with a `text/template` (HTML-escape ship names; embed the matrix as a `<script>const MATRIX_DATA = {...}</script>` JSON blob for client-side sort/filter/search). Sections:
- Header + the "you vs 1000 fifth-graders" framing; featured **Opus Magna** callout (its row, the 5 empire numbers, and "the titan killed N of you").
- The full sortable/filterable table: rows = ships (name, tier, class), 5 empire columns showing the crossover N (∞ rendered as `∞`); click a cell → drawer with the crossover curve (inline `<canvas>`, no libs) + median kills.
- Damage-type explainer (kinetic vs energy vs em vs the titan's shields/armor).
- **Assumptions** box listing every stated assumption from the spec.
- Filters: tier, class, empire-column min/max, name search.

Wire `-page` in `main()` to write `RenderPage(m)` to the given path.

- [ ] **Step 4: Add the index card**

In `kb/did_you_know/index.html`, add a card linking `last_stand.html` (match the existing card markup; title e.g. "Last Stand: Swarm vs Titan", one-line teaser).

- [ ] **Step 5: Run test + generate the real artifacts**

Run: `go test ./cmd/generate-last-stand/ -v` → PASS
Run the full matrix (this is the committed artifact):

```bash
go run ./cmd/generate-last-stand \
  -out data/combat-sim/last_stand_matrix.json \
  -page kb/did_you_know/last_stand.html -runs 300
```

Open `kb/did_you_know/last_stand.html` locally; verify the table renders, sort/filter/search work, a cell drawer shows a curve, and the Opus Magna row shows finite empire numbers (Crimson/Outer-Rim kinetic columns notably lower than the energy/em columns).

- [ ] **Step 6: Lint + commit**

Run: `golangci-lint run ./cmd/generate-last-stand/...`

```bash
git add cmd/generate-last-stand/render.go cmd/generate-last-stand/render_test.go \
  kb/did_you_know/last_stand.html kb/did_you_know/index.html \
  data/combat-sim/last_stand_matrix.json
git commit -m "did-you-know: Last Stand swarm-vs-titan matrix page"
```

---

### Task 9: Real-battle validation

**Files:**
- Create: `cmd/combat-sim/swarm_validation_test.go` (or in `pkg/combatsim` after extraction)
- Possibly add: a few small `data/battles/*.json` fixtures (already present: 3/4/5-participant)

**Interfaces:**
- Consumes: `RunMultiShip`, and a fixture loader that reads a battle JSON's participants into `[]Ship` (team = side, stats hand-mapped or resolved from logged modules).

- [ ] **Step 1: Write the validation test**

Using the committed multi-participant fixtures (`data/battles/b131fd5aae68420107dd20e93d15d3ba.json` = 5 participants across 4 sides; `509e1ef4...`, `b7847bbc...` = small), build `[]Ship` from each fixture's participants (map each participant's logged hull/modules to a `StatBlock`; reuse the hand-built blocks already in `engine_test.go` where a fixture is already characterized), run `RunMultiShip` many times, and assert the **predicted winning team matches the fixture's `winning_side`** a majority of the time, and median `Ticks` is within a generous band (e.g. 0.3×–3× the fixture `tick_count`). Skip participants that cannot be resolved, with a `t.Log`.

```go
func TestValidateAgainstRealBattles(t *testing.T) {
	// table of {fixture path, expected winning side, tick_count}; build ships,
	// run 200×, assert majority winner matches and duration is in-band.
}
```

- [ ] **Step 2: Run it**

Run: `go test ./... -run TestValidateAgainstRealBattles -v`
Expected: PASS. If a fixture's model is too rough to match, `t.Skip` it with a recorded reason (the engine targets population-level outcomes, not exact turn-by-turn replays) — but at least the clean small-N fixtures must pass.

- [ ] **Step 3: (Optional) export fresh 2v1 / 5v1 fixtures**

If the existing fixtures are insufficient, pull short non-wildlife 2v1/5v1 battle_ids from the current feed month and export:

```bash
# check no live session first: ps aux | grep -E "play_as|duel-runner"
bin/battle-export --agent craftsman-boss --battle <ids> --out-dir data/battles
```

Add the exported fixtures and extend the table. (Owner-gated: battle-export logs in as craftsman-boss and can disrupt a live session — only run when clear.)

- [ ] **Step 4: Commit**

```bash
git add cmd/combat-sim/swarm_validation_test.go data/battles/
git commit -m "combat-sim: validate multi-ship engine against real battles"
```

---

### Task 10: Docs + spec errata

**Files:**
- Modify: `cmd/combat-sim/README.md`
- Modify: `docs/superpowers/specs/2026-09-02-swarm-threshold-matrix-design.md` (errata)

- [ ] **Step 1: README**

Document the swarm mode: `--swarm/--vs/--n-max/--runs/--swarm-json`, the `generate-last-stand` binary and its flags, the matrix JSON schema, and the full assumptions list (no capital bonus; unlimited ammo + 1-tick reload; default ammo; one target/tick; d6→d0 reach-gated approach; `fire` stance; cohort firing-fraction approximation). Note the `pkg/combatsim` extraction.

- [ ] **Step 2: Spec errata**

Append an "Errata / as-built" section noting any deviations discovered during implementation (e.g. the `pkg/combatsim` extraction, the `swarmMaxTicks` cap value, any tolerance chosen for the cohort-vs-reference agreement test).

- [ ] **Step 3: Commit**

```bash
git add cmd/combat-sim/README.md docs/superpowers/specs/2026-09-02-swarm-threshold-matrix-design.md
git commit -m "docs: swarm threshold matrix — README + spec errata"
```

---

## Self-Review Notes

- **Spec coverage:** matrix (Tasks 7–8), cohort engine (Task 4), general engine + validation (Tasks 3, 9), crossover galloping+bisect (Task 5), capital resolve + no-bonus (Task 1), reach/distance/reload/default-ammo (Tasks 1–2), CLI (Task 6), page + assumptions box (Task 8), docs (Task 10). Future whole-catalog sweep is enabled by `BuildMatrix` taking arbitrary defender/attacker id lists.
- **Package boundary:** the generator cannot import `package main`; Task 7 Step 1 resolves this by extracting `pkg/combatsim`. This is the plan's one structural risk — it is sequenced first within Task 7 and gated by an unchanged-behavior `go test ./...`.
- **Type consistency:** `Crossing`/`CrossPoint` (Task 5) are consumed by the matrix (Task 7) and page (Task 8); `Ship`/`MultiResult` (Task 3) by validation (Task 9); `ResolveHull(id,cat,allowCapital)` (Task 1) used everywhere a hull is resolved.
- **Correctness gates over exact formulas:** the cohort model (Task 4) and its `attackerVolleyProfile`/`applyIdenticalVolleys` are specified concretely but the binding acceptance is `TestSwarmAgreesWithMultiShip` (±0.15) and the monotonic/floor test — the implementer tunes to pass these, not to match the sketch character-for-character.
