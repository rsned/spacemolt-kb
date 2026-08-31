# Combat Simulator (cmd/combat-sim) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A hermetic Go CLI that Monte-Carlo simulates 1v1 combat between two ship fittings using the log-verified damage model and prints a 4×4 stance-pair outcome table.

**Architecture:** Single `main` package in `cmd/combat-sim/`: catalog loader (committed JSON snapshots) → fit resolver (StatBlock) → pure volley/mitigation core (golden-tested against three real battle fixtures) → tick engine with stances → Monte Carlo runner + table printer. Randomness = one hit roll per volley + one crit roll per weapon, all from one seeded RNG.

**Tech Stack:** Go 1.25 (module `github.com/rsned/spacemolt-kb`), stdlib only (`encoding/json`, `math/rand/v2`, `flag`). No DBs, no network.

**Spec:** `docs/superpowers/specs/2026-08-31-combat-sim-design.md`

## Global Constraints

- Go 1.25 idioms: range-over-int; `b.Loop()` in any benchmark (never `for i := 0; i < b.N; i++`).
- All new code passes `golangci-lint run ./cmd/combat-sim/...` with no new findings.
- Run `go build ./... && go test ./cmd/combat-sim/...` before every commit.
- Built binaries go in `bin/` (gitignored) — never the repo root.
- Match actual JSON field names from the snapshot files; never assume.
- Catalog inputs: `data/snapshots/latest/catalog_{ships,items,skills}.json` (each file is `{"items":[...]}`; `latest` is a symlink — open via the path given, do not resolve).
- Integer truncation everywhere the model says floor: use `int(x)` on non-negative floats or integer division; never `math.Round`.
- Every ASSUMED (unmeasured) constant lives in the calibration struct, never inline in engine code.

---

### Task 1: Catalog loader

**Files:**
- Create: `cmd/combat-sim/loader.go`
- Test: `cmd/combat-sim/loader_test.go`

**Interfaces:**
- Consumes: committed snapshot JSON only.
- Produces (used by Tasks 2, 6):

```go
type ShipDef struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Class              string `json:"class"`
	Tier               int    `json:"tier"`
	BaseHull           int    `json:"base_hull"`
	BaseShield         int    `json:"base_shield"`
	BaseShieldRecharge int    `json:"base_shield_recharge"`
	BaseArmor          int    `json:"base_armor"`
}

type ItemDef struct {
	ID                  string         `json:"id"`
	Type                string         `json:"type"`
	Slot                string         `json:"slot"`
	Damage              int            `json:"damage"`
	DamageType          string         `json:"damage_type"`
	Reach               int            `json:"reach"`
	Cooldown            int            `json:"cooldown"`
	MagazineSize        int            `json:"magazine_size"`
	ShieldBonus         int            `json:"shield_bonus"`
	ArmorBonus          int            `json:"armor_bonus"`
	DamageReduction     int            `json:"damage_reduction"`
	ShieldRechargeBonus int            `json:"shield_recharge_bonus"`
	RequiredSkills      map[string]int `json:"required_skills"`
}

type Catalog struct {
	Ships map[string]*ShipDef
	Items map[string]*ItemDef
}

func LoadCatalog(dir string) (*Catalog, error) // reads catalog_ships.json + catalog_items.json from dir
```

- [ ] **Step 1: Write the failing test**

`cmd/combat-sim/loader_test.go`:

```go
package main

import (
	"path/filepath"
	"testing"
)

func catalogDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "data", "snapshots", "latest")
}

func TestLoadCatalog(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	pl := cat.Items["pulse_laser_iii"]
	if pl == nil || pl.Damage != 28 || pl.DamageType != "energy" || pl.Cooldown != 1 {
		t.Errorf("pulse_laser_iii = %+v, want damage 28 energy cooldown 1", pl)
	}
	ac := cat.Items["autocannon_ii"]
	if ac == nil || ac.Damage != 14 || ac.MagazineSize != 650 {
		t.Errorf("autocannon_ii = %+v, want damage 14 magazine 650", ac)
	}
	as := cat.Items["adaptive_shield_iii"]
	if as == nil || as.DamageReduction != 35 || as.ShieldBonus != 200 {
		t.Errorf("adaptive_shield_iii = %+v, want damage_reduction 35 shield_bonus 200", as)
	}
	bx := cat.Ships["broadaxe"]
	if bx == nil || bx.BaseHull != 282 || bx.BaseShield != 28 || bx.BaseArmor != 18 || bx.BaseShieldRecharge != 2 {
		t.Errorf("broadaxe = %+v, want hull 282 shield 28 armor 18 recharge 2", bx)
	}
	if sv := cat.Ships["survey_vessel"]; sv == nil || sv.BaseArmor != 14 {
		t.Errorf("survey_vessel = %+v, want base_armor 14", sv)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/combat-sim/ -run TestLoadCatalog -v`
Expected: FAIL (compile error: `LoadCatalog` undefined).

- [ ] **Step 3: Write the loader**

`cmd/combat-sim/loader.go`:

```go
// Command combat-sim Monte-Carlo simulates 1v1 combat between two ship
// fittings using the battle-log-verified damage model. Hermetic: reads only
// committed catalog snapshots and small JSON input files.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func loadItems[T any](path string) ([]T, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Items []T `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return wrap.Items, nil
}

// LoadCatalog reads catalog_ships.json and catalog_items.json from dir.
func LoadCatalog(dir string) (*Catalog, error) {
	ships, err := loadItems[*ShipDef](filepath.Join(dir, "catalog_ships.json"))
	if err != nil {
		return nil, err
	}
	items, err := loadItems[*ItemDef](filepath.Join(dir, "catalog_items.json"))
	if err != nil {
		return nil, err
	}
	cat := &Catalog{Ships: map[string]*ShipDef{}, Items: map[string]*ItemDef{}}
	for _, s := range ships {
		cat.Ships[s.ID] = s
	}
	for _, it := range items {
		cat.Items[it.ID] = it
	}
	return cat, nil
}
```

Plus the `ShipDef`, `ItemDef`, `Catalog` type declarations from the Interfaces block above (same file).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/combat-sim/ -run TestLoadCatalog -v` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/combat-sim/loader.go cmd/combat-sim/loader_test.go
git commit -m "feat(combat-sim): hermetic catalog loader over committed snapshots"
```

---

### Task 2: Fit resolver

**Files:**
- Create: `cmd/combat-sim/resolver.go`
- Test: `cmd/combat-sim/resolver_test.go`

**Interfaces:**
- Consumes: `Catalog` from Task 1.
- Produces (used by Tasks 3–6):

```go
type FitSpec struct {
	Name    string         `json:"name"`
	Hull    string         `json:"hull"`
	Modules []string       `json:"modules"`
	Skills  map[string]int `json:"skills"` // keys: weapons, gunnery, shields, armor (missing = 0)
}

type Weapon struct {
	Name     string
	Damage   int
	Type     string // energy|kinetic|void|explosive|em|thermal
	Cooldown int
	Magazine int // 0 = no ammo tracking (beam weapons)
}

type StatBlock struct {
	Name           string
	MaxHull        int
	MaxShield      int
	Recharge       int
	ArmorTotal     float64 // (base_armor + Σ armor_bonus) × (1 + Armor×0.01)
	FlatPct        int     // Σ damage_reduction, capped 75
	ShieldsSkill   int
	WeaponSkillPct int // Weapons + Gunnery (v1: Gunnery applied to all types)
	CritPct        int // Weapons × 1
	Weapons        []Weapon
}

func Resolve(fit *FitSpec, cat *Catalog) (*StatBlock, error)
func LoadFit(path string) (*FitSpec, error)
```

Rules (all measured — see spec):
- NO capacity skill multipliers on hull/shield/recharge (stat blocks match catalog + modules exactly).
- Armor skill: ArmorTotal × (1 + Armor×0.01) ("armorEffectiveness 1%/level").
- FlatPct capped at 75. A module's `damage_reduction` and its `adaptive_resistance_N` special are ONE number — read only `damage_reduction`.
- Unknown hull or module id → error naming the id. Hull `Tier >= 5` → error `"capital hulls unsupported in v1 (capital weapon bonus unmodeled)"`.

- [ ] **Step 1: Write the failing test**

`cmd/combat-sim/resolver_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

// The two real fits from battle fixture 7c044558 — resolver output must match
// the participant stat blocks logged by the server.
func broadaxeFit() *FitSpec {
	return &FitSpec{
		Name: "MoltenOne", Hull: "broadaxe",
		Modules: []string{"autocannon_ii", "autocannon_ii", "autocannon_ii", "flak_cannon_ii", "armor_plate_ii"},
		Skills:  map[string]int{"weapons": 7, "gunnery": 10, "shields": 4},
	}
}

func surveyFit() *FitSpec {
	return &FitSpec{
		Name: "Artis", Hull: "survey_vessel",
		Modules: []string{"pulse_laser_iii", "pulse_laser_iii", "shield_booster_iv", "shield_recharger_ii"},
		Skills:  map[string]int{"weapons": 3, "gunnery": 3, "shields": 1},
	}
}

func TestResolveBroadaxe(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Resolve(broadaxeFit(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if sb.MaxHull != 282 || sb.MaxShield != 28 || sb.Recharge != 2 {
		t.Errorf("pools = hull %d shield %d recharge %d, want 282/28/2 (no capacity skill multipliers)", sb.MaxHull, sb.MaxShield, sb.Recharge)
	}
	if sb.ArmorTotal != 28 { // (18+10) × (1 + 0×0.01)
		t.Errorf("ArmorTotal = %v, want 28", sb.ArmorTotal)
	}
	if sb.WeaponSkillPct != 17 || sb.CritPct != 7 || sb.ShieldsSkill != 4 {
		t.Errorf("skills = wsp %d crit %d shres %d, want 17/7/4", sb.WeaponSkillPct, sb.CritPct, sb.ShieldsSkill)
	}
	if len(sb.Weapons) != 4 || sb.Weapons[3].Damage != 28 || sb.Weapons[0].Magazine != 650 {
		t.Errorf("weapons = %+v, want 4 weapons, flak 28, autocannon magazine 650", sb.Weapons)
	}
}

func TestResolveSurvey(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	sb, err := Resolve(surveyFit(), cat)
	if err != nil {
		t.Fatal(err)
	}
	if sb.MaxShield != 400 || sb.Recharge != 9 || sb.ArmorTotal != 14 {
		t.Errorf("shield %d recharge %d armor %v, want 400/9/14", sb.MaxShield, sb.Recharge, sb.ArmorTotal)
	}
}

func TestResolveErrors(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(&FitSpec{Hull: "nope"}, cat); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown hull error = %v, want mention of id", err)
	}
	if _, err := Resolve(&FitSpec{Hull: "broadaxe", Modules: []string{"nope"}}, cat); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("unknown module error = %v, want mention of id", err)
	}
	if _, err := Resolve(&FitSpec{Hull: "opus_magna"}, cat); err == nil || !strings.Contains(err.Error(), "capital") {
		t.Errorf("capital hull error = %v, want capital refusal", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/combat-sim/ -run TestResolve -v` — Expected: FAIL (Resolve undefined).

- [ ] **Step 3: Write the resolver**

`cmd/combat-sim/resolver.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadFit reads a fitting-spec JSON file.
func LoadFit(path string) (*FitSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f FitSpec
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}

// Resolve turns a fit + skills into the combat stat block. Measured rule: no
// capacity skill multipliers — server stat blocks equal catalog + modules.
func Resolve(fit *FitSpec, cat *Catalog) (*StatBlock, error) {
	hull, ok := cat.Ships[fit.Hull]
	if !ok {
		return nil, fmt.Errorf("unknown hull %q", fit.Hull)
	}
	if hull.Tier >= 5 {
		return nil, fmt.Errorf("hull %q: capital hulls unsupported in v1 (capital weapon bonus unmodeled)", fit.Hull)
	}
	sk := func(name string) int { return fit.Skills[name] }
	sb := &StatBlock{
		Name:           fit.Name,
		MaxHull:        hull.BaseHull,
		MaxShield:      hull.BaseShield,
		Recharge:       hull.BaseShieldRecharge,
		ShieldsSkill:   sk("shields"),
		WeaponSkillPct: sk("weapons") + sk("gunnery"),
		CritPct:        sk("weapons"),
	}
	armor := hull.BaseArmor
	flat := 0
	for _, id := range fit.Modules {
		it, ok := cat.Items[id]
		if !ok {
			return nil, fmt.Errorf("unknown module %q", id)
		}
		sb.MaxShield += it.ShieldBonus
		sb.Recharge += it.ShieldRechargeBonus
		armor += it.ArmorBonus
		flat += it.DamageReduction
		if it.Slot == "weapon" && it.Damage > 0 {
			sb.Weapons = append(sb.Weapons, Weapon{
				Name: it.ID, Damage: it.Damage, Type: it.DamageType,
				Cooldown: it.Cooldown, Magazine: it.MagazineSize,
			})
		}
	}
	sb.FlatPct = min(flat, 75)
	sb.ArmorTotal = float64(armor) * (1 + float64(sk("armor"))*0.01)
	return sb, nil
}
```

Plus the `FitSpec`, `Weapon`, `StatBlock` declarations from the Interfaces block (same file).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run TestResolve -v` — Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/combat-sim/resolver.go cmd/combat-sim/resolver_test.go
git commit -m "feat(combat-sim): fit resolver with measured stat-block rules"
```

---

### Task 3: Mitigation core + golden replay tests

This is the load-bearing task: `ResolveVolley` is a PURE function (no RNG) implementing the verified pipeline; the tests replay real logged volleys from the three committed fixtures and demand equality.

**Files:**
- Create: `cmd/combat-sim/engine.go`
- Test: `cmd/combat-sim/engine_test.go`

**Interfaces:**
- Consumes: `StatBlock`, `Weapon` (Task 2).
- Produces (used by Tasks 4–5):

```go
type SideState struct {
	Stats       *StatBlock
	Hull, Shield int
	Ammo        []int // per weapon; -1 = infinite (Magazine 0)
	Cool        []int // ticks until weapon ready
	Stance      Stance
	HitThisTick bool
	Fled        bool
}

type VolleyOutcome struct{ ShieldDrain, HullDmg int }

// ResolveVolley applies one LANDED volley (hit roll already succeeded) of the
// weapons listed in fired (indexes into att.Weapons) with critFlags[i] per
// fired weapon, against tgt's current pools. stanceInMult is the target's
// incoming-damage stance multiplier. Mutates nothing; returns the damages.
func ResolveVolley(att *StatBlock, tgt *SideState, fired []int, critFlags []bool, stanceInMult float64) VolleyOutcome

func NewSide(sb *StatBlock, stance Stance) *SideState
type Stance string
const (
	StanceFire  Stance = "fire"
	StanceBrace Stance = "brace"
	StanceEvade Stance = "evade"
	StanceFlee  Stance = "flee"
)
```

The pipeline (every constant and floor below is measured; see spec §engine.go):

```
raw     = Σ over fired weapons: floor(dmg × 1.5) if crit else dmg
pre     = floor(raw × (1 + WeaponSkillPct/100))
pre     = floor(pre × stanceInMult)
spills  = 0
if tgt.Shield > 0:
    v1 = pre × (1 − ShieldsSkill/100);  spill if frac(v1) ≥ 0.5;  x1 = floor(v1)
    e  = shieldEff[type]   // energy .75, kinetic 1, void 0, explosive/em/thermal 1
    if e == 0: hullIn = pre  (void skips shields; NO x1)   // then jump to armor
    v2 = x1 × e;            spill if frac(v2) ≥ 0.5;       d = floor(v2)
    v3 = d × (1 − FlatPct/100); spill if frac(v3) ≥ 0.5;   drain = floor(v3)
    if tgt.Shield ≥ drain:
        ShieldDrain = drain
        HullDmg     = floor(spills × (1 − FlatPct/100))    // spill bypasses armor
        return
    // breakthrough: NO shields-skill term, no spills
    ShieldDrain = tgt.Shield
    hullIn      = pre − floor(tgt.Shield / e)
else:
    hullIn = pre
// armor (skip if hullIn ≤ 0)
counted = ArmorTotal × armorMult[type]  // kinetic/void 1.5, explosive/em 1.0, energy .75, thermal .25
if armorLaw == auto: pct law when counted ≥ 12 else flat law   // crossover open; 12 splits the measured ranges
    pct:  HullDmg = floor(hullIn × (1 − counted/(counted+150)))
    flat: HullDmg = hullIn − floor(ArmorTotal × 0.75)
HullDmg = max(HullDmg, 1) if hullIn ≥ 1   // min-1 (dev-stated)
HullDmg = min(HullDmg, tgt.Hull)          // kill cap
```

Hardcode `armorLaw = "auto"` via a package-level `var armorLaw = "auto"` in this task; Task 4 moves it into Calibration.

- [ ] **Step 1: Write the failing golden tests**

`cmd/combat-sim/engine_test.go`. Every expectation below is a real logged volley (`data/battles/{509e1ef4,b7847bbc,7c044558}*.json`); comments give tick + note. NOTE: the engine returns ACTUAL shield drain; logs report NET drain (actual − regen `floor(recharge/3)` on hit ticks) — expectations below are actual, with the net shown in comments.

```go
package main

import "testing"

// Stat blocks are hand-built to match the fixtures' logged participants exactly
// (independent of the resolver, which Task 2 tests separately).
func artisEviction() *StatBlock { // 509e: 4× Pulse Laser III, skills W3+G3, target stats below
	return &StatBlock{Name: "artis509", WeaponSkillPct: 6, CritPct: 3,
		MaxHull: 480, MaxShield: 600, Recharge: 4, ArmorTotal: 25, FlatPct: 70, ShieldsSkill: 1,
		Weapons: []Weapon{{Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}}}
}

func molten509() *StatBlock { // 509e: Portfolio, PL II + PL I, W7+G10
	return &StatBlock{Name: "molten509", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 180, MaxShield: 130, Recharge: 2, ArmorTotal: 8, FlatPct: 0, ShieldsSkill: 3,
		Weapons: []Weapon{{Damage: 18, Type: "energy"}, {Damage: 10, Type: "energy"}}}
}

func side(sb *StatBlock, shield, hull int) *SideState {
	s := NewSide(sb, StanceFire)
	s.Shield, s.Hull = shield, hull
	return s
}

func all(n int) []int {
	idx := make([]int, n)
	for i := range n {
		idx[i] = i
	}
	return idx
}

func noCrit(n int) []bool { return make([]bool, n) }

type goldenCase struct {
	name      string
	att       *StatBlock
	tgt       *SideState
	crits     []bool
	wantSh    int
	wantHl    int
	hlTol     int // 0 = exact
}

func TestGoldenVolleys(t *testing.T) {
	// 7c044558 participants.
	moltenBroadaxe := &StatBlock{Name: "moltenBx", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 282, MaxShield: 28, Recharge: 2, ArmorTotal: 28, ShieldsSkill: 4,
		Weapons: []Weapon{{Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 14, Type: "kinetic"}, {Damage: 28, Type: "kinetic"}}}
	artisSurvey := &StatBlock{Name: "artisSurvey", WeaponSkillPct: 6, CritPct: 3,
		MaxHull: 340, MaxShield: 400, Recharge: 9, ArmorTotal: 14, ShieldsSkill: 1,
		Weapons: []Weapon{{Damage: 28, Type: "energy"}, {Damage: 28, Type: "energy"}}}
	// b7847bbc participants.
	moltenUnderwriter := &StatBlock{Name: "moltenUw", WeaponSkillPct: 17, CritPct: 7,
		MaxHull: 130, MaxShield: 95, Recharge: 2, ArmorTotal: 6, ShieldsSkill: 3,
		Weapons: []Weapon{{Damage: 10, Type: "energy"}, {Damage: 10, Type: "energy"}}}
	vera := &StatBlock{Name: "vera", WeaponSkillPct: 0, CritPct: 0,
		MaxHull: 80, MaxShield: 35, Recharge: 1, ArmorTotal: 3, ShieldsSkill: 0,
		Weapons: []Weapon{{Damage: 8, Type: "kinetic"}}}

	cases := []goldenCase{
		// 509e tick 1744820: 118 → shres → 85 shield + 1 spill hull (net 85, recharge 2 → g 0).
		{"509A full-shield", artisEviction(), side(molten509(), 130, 136), noCrit(4), 85, 1, 0},
		// 509e tick 1744822 breakthrough: pool 45 consumes floor(45/.75)=60; 118−60=58; flat armor law (counted 6<12): 58−6=52.
		{"509B breakthrough", artisEviction(), side(molten509(), 45, 135), noCrit(4), 45, 52, 0},
		// 509e tick 1744823 kill cap: hull 83 remaining caps 112 → 83.
		{"509C kill cap", artisEviction(), side(molten509(), 0, 83), noCrit(4), 0, 83, 0},
		// 509e MoltenOne→Artis ×4: 32 → 31(spill) → 23 → flat70 → 6(spill); spills×0.3→0 hull. Actual drain 6; logged net 5 (g=1).
		{"509M flat70", molten509(), side(artisEviction(), 600, 480), noCrit(2), 6, 0, 0},
		// b7847 ticks 385/386: floor(23×.75)=17, no spill (Vera shres 0).
		{"NK1 full-shield", moltenUnderwriter(), side(vera, 35, 80), noCrit(2), 17, 0, 0},
		// b7847 tick 387 crit breakthrough: raw 10+15=25 → 29; pool 1 consumes 1; 28−floor(3×.75)=26.
		{"NKB crit breakthrough", moltenUnderwriter(), side(vera, 1, 80), []bool{false, true}, 1, 26, 0},
		// b7847 ticks 388/389 shields down: 23 − 2 = 21.
		{"NK4 shields down", moltenUnderwriter(), side(vera, 0, 54), noCrit(2), 0, 21, 0},
		// b7847 tick 390 kill cap.
		{"NK6 kill cap", moltenUnderwriter(), side(vera, 0, 12), noCrit(2), 0, 12, 0},
		// b7847 autocannon: 8 → shres spill (7.76) → drain 7 + 1 spill hull.
		{"NKA kinetic spill", vera, side(moltenUnderwriter(), 95, 130), noCrit(1), 7, 1, 0},
		// 7c04 kinetic ×5: 81 → floor(80.19)=80 drain, no spill. Logged net 77 (g=3).
		{"7cK kinetic drain", moltenBroadaxe, side(artisSurvey, 400, 340), noCrit(4), 80, 0, 0},
		// 7c04 tick 961 crit breakthrough: raw 21+14+14+28=77 → 90; pool 15; 75 × (1−21/171) → 65.
		{"7cKB crit breakthrough", moltenBroadaxe, side(artisSurvey, 15, 340), []bool{true, false, false, false}, 15, 65, 0},
		// 7c04 tick 962 shields down: floor(81 × 150/171) = 71.
		{"7cKD shields down", moltenBroadaxe, side(artisSurvey, 0, 275), noCrit(4), 0, 71, 0},
		// 7c04 broadaxe rows: OPEN ±1 alternation (obs 41; 52/53) — tolerance 2.
		{"7cEB broadaxe breakthrough", artisSurvey, side(moltenBroadaxe, 8, 254), noCrit(2), 8, 41, 2},
		{"7cE broadaxe shields down", artisSurvey, side(moltenBroadaxe, 0, 213), noCrit(2), 0, 52, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ResolveVolley(c.att, c.tgt, all(len(c.att.Weapons)), c.crits, 1.0)
			if got.ShieldDrain != c.wantSh {
				t.Errorf("shield drain = %d, want %d", got.ShieldDrain, c.wantSh)
			}
			if d := got.HullDmg - c.wantHl; d < -c.hlTol || d > c.hlTol {
				t.Errorf("hull dmg = %d, want %d ±%d", got.HullDmg, c.wantHl, c.hlTol)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/combat-sim/ -run TestGoldenVolleys -v` — Expected: FAIL (ResolveVolley undefined).

- [ ] **Step 3: Implement the pipeline**

`cmd/combat-sim/engine.go`:

```go
package main

import "math"

type Stance string

const (
	StanceFire  Stance = "fire"
	StanceBrace Stance = "brace"
	StanceEvade Stance = "evade"
	StanceFlee  Stance = "flee"
)

var allStances = []Stance{StanceFire, StanceBrace, StanceEvade, StanceFlee}

// Per-damage-type constants (measured; see spec).
var shieldEff = map[string]float64{
	"energy": 0.75, "kinetic": 1.0, "void": 0.0,
	"explosive": 1.0, "em": 1.0, "thermal": 1.0,
}

var armorMult = map[string]float64{
	"energy": 0.75, "kinetic": 1.5, "void": 1.5,
	"explosive": 1.0, "em": 1.0, "thermal": 0.25,
}

const armorK = 150.0        // half-saturation constant = max bare hull armor
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
	if armorLaw == "pct150" {
		usePct = true
	} else if armorLaw == "flat75" {
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -run TestGoldenVolleys -v` — Expected: all 14 subtests PASS. If a subtest fails, the implementation deviates from the measured model — fix the code, never the expectation (tolerances are already encoded).

- [ ] **Step 5: Commit**

```bash
git add cmd/combat-sim/engine.go cmd/combat-sim/engine_test.go
git commit -m "feat(combat-sim): verified mitigation core with golden replay tests over three fixtures"
```

---

### Task 4: Calibration + tick engine

**Files:**
- Create: `cmd/combat-sim/battle.go`, `data/combat-sim/calibration.json`
- Modify: `cmd/combat-sim/engine.go` (replace `var armorLaw` / crossover const with Calibration fields)
- Test: `cmd/combat-sim/battle_test.go`

**Interfaces:**
- Consumes: `StatBlock`, `SideState`, `ResolveVolley`, `NewSide`, stances (Task 3).
- Produces (used by Task 5):

```go
type Calibration struct {
	HitChanceA        float64 `json:"hit_chance_a"`         // measured range 0.79–0.95 at engaged; default 0.95
	HitChanceB        float64 `json:"hit_chance_b"`
	BraceInMult       float64 `json:"brace_in_mult"`        // 0.25 measured
	EvadeInMult       float64 `json:"evade_in_mult"`        // ASSUMED 0.5
	FleeEscapePerTick float64 `json:"flee_escape_per_tick"` // ASSUMED 0.25
	RegenHitDivisor   int     `json:"regen_hit_divisor"`    // 3 measured
	RegenFromZero     bool    `json:"regen_from_zero"`      // ASSUMED false
	ArmorLaw          string  `json:"armor_law"`            // "auto"|"pct150"|"flat75"
	ArmorLawCrossover float64 `json:"armor_law_crossover"`  // 12, OPEN
	MaxTicks          int     `json:"max_ticks"`            // 500
	Assumed           []string `json:"assumed"`             // names of ASSUMED entries, for table flagging
}

func LoadCalibration(path string) (*Calibration, error)
func DefaultCalibration() *Calibration

type Outcome string
const (
	OutAKill     Outcome = "A-kill"
	OutBKill     Outcome = "B-kill"
	OutAFled     Outcome = "A-fled"
	OutBFled     Outcome = "B-fled"
	OutMutual    Outcome = "mutual"
	OutStalemate Outcome = "stalemate"
)

// RunBattle simulates one battle to completion. rng: math/rand/v2 *rand.Rand.
func RunBattle(a, b *StatBlock, sa, sb Stance, cal *Calibration, rng *rand.Rand) Outcome
```

Tick loop rules (spec §engine.go):
- Both volleys computed from start-of-tick state, applied together; both hulls ≤0 → `mutual`; a kill by A while A also dies is still `mutual`.
- A side fires unless `Stance == StanceFlee`. Fired weapons: `Cool[i] == 0 && Ammo[i] != 0`; firing sets `Cool[i] = w.Cooldown` and decrements positive ammo. Cooldowns tick down each tick.
- Hit: one roll per volley vs that side's hit chance. Crit: per fired weapon vs `CritPct/100`, rolled regardless of hit (measured server behavior; a missed volley's crits do nothing).
- Stance in-multiplier: fire/flee 1.0, brace `cal.BraceInMult`, evade `cal.EvadeInMult`.
- Regen end of tick: if shield was 0 at start of regen and `!cal.RegenFromZero`, nothing; else `+Recharge` if not hit this tick, `+Recharge/cal.RegenHitDivisor` (integer division) if hit; cap at MaxShield.
- Flee: from the second tick, roll `cal.FleeEscapePerTick`; success → that side fled → outcome.
- `cal.MaxTicks` reached → `stalemate`.

`data/combat-sim/calibration.json` (defaults; `_comment` documents provenance):

```json
{
  "_comment": "measured: brace_in_mult (Haven fixture), regen_hit_divisor (3 fixtures), armor constants (see analysis scripts). ASSUMED (uncalibrated until phase-B duels): evade_in_mult, flee_escape_per_tick, regen_from_zero, hit chances beyond the 0.79-0.95 measured envelope.",
  "hit_chance_a": 0.95,
  "hit_chance_b": 0.95,
  "brace_in_mult": 0.25,
  "evade_in_mult": 0.5,
  "flee_escape_per_tick": 0.25,
  "regen_hit_divisor": 3,
  "regen_from_zero": false,
  "armor_law": "auto",
  "armor_law_crossover": 12,
  "max_ticks": 500,
  "assumed": ["evade_in_mult", "flee_escape_per_tick", "regen_from_zero"]
}
```

- [ ] **Step 1: Write the failing tests**

`cmd/combat-sim/battle_test.go`:

```go
package main

import (
	"math/rand/v2"
	"testing"
)

func calFixed() *Calibration {
	c := DefaultCalibration()
	c.HitChanceA, c.HitChanceB = 1.0, 1.0 // deterministic for these tests
	return c
}

// Replays fixture 7c044558 end-to-end deterministically (hit chance 1, no
// crits at CritPct 0): broadaxe kills the survey fit... actually the fixture
// ended with MoltenOne (broadaxe) destroyed — with guaranteed hits both ways
// the higher-DPS side wins; assert the battle TERMINATES with a kill.
func TestRunBattleTerminates(t *testing.T) {
	a := &StatBlock{Name: "A", MaxHull: 282, MaxShield: 28, Recharge: 2, ArmorTotal: 28, ShieldsSkill: 4,
		WeaponSkillPct: 17, CritPct: 0,
		Weapons: []Weapon{{Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 14, Type: "kinetic", Cooldown: 1}, {Damage: 28, Type: "kinetic", Cooldown: 1}}}
	b := &StatBlock{Name: "B", MaxHull: 340, MaxShield: 400, Recharge: 9, ArmorTotal: 14, ShieldsSkill: 1,
		WeaponSkillPct: 6, CritPct: 0,
		Weapons: []Weapon{{Damage: 28, Type: "energy", Cooldown: 1}, {Damage: 28, Type: "energy", Cooldown: 1}}}
	rng := rand.New(rand.NewPCG(1, 1))
	out := RunBattle(a, b, StanceFire, StanceFire, calFixed(), rng)
	if out != OutAKill && out != OutBKill {
		t.Errorf("outcome = %s, want a kill", out)
	}
}

func TestFleeNeverFires(t *testing.T) {
	glass := &StatBlock{Name: "glass", MaxHull: 10, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	tank := &StatBlock{Name: "tank", MaxHull: 10000, MaxShield: 0}
	cal := calFixed()
	cal.FleeEscapePerTick = 0 // can never escape, and never fires: must hit MaxTicks
	cal.MaxTicks = 50
	rng := rand.New(rand.NewPCG(2, 2))
	if out := RunBattle(glass, tank, StanceFlee, StanceFire, cal, rng); out != OutStalemate {
		t.Errorf("fleeing unarmed-vs-unarmed = %s, want stalemate (fleeing side must not fire)", out)
	}
}

func TestFleeEscapes(t *testing.T) {
	a := &StatBlock{Name: "a", MaxHull: 100000, MaxShield: 0}
	b := &StatBlock{Name: "b", MaxHull: 100000, MaxShield: 0}
	cal := calFixed()
	cal.FleeEscapePerTick = 1.0
	rng := rand.New(rand.NewPCG(3, 3))
	if out := RunBattle(a, b, StanceFlee, StanceFire, cal, rng); out != OutAFled {
		t.Errorf("guaranteed escape = %s, want A-fled", out)
	}
}

func TestBraceReducesDamage(t *testing.T) {
	att := &StatBlock{Name: "att", MaxHull: 100, MaxShield: 0,
		Weapons: []Weapon{{Damage: 100, Type: "energy", Cooldown: 1}}}
	def := &StatBlock{Name: "def", MaxHull: 1000, MaxShield: 0}
	cal := calFixed()
	cal.MaxTicks = 4
	rng := rand.New(rand.NewPCG(4, 4))
	// 3 landed 100-dmg volleys: fire eats 300, brace eats 75 (100×0.25 per volley).
	// Use RunBattleState variant? Keep it simple: brace survives 4 ticks, fire doesn't need testing here.
	if out := RunBattle(att, def, StanceFire, StanceBrace, cal, rng); out != OutStalemate {
		t.Errorf("braced 1000-hull vs 25/tick over 4 ticks = %s, want stalemate", out)
	}
}

func TestDeterministicUnderSeed(t *testing.T) {
	a := &StatBlock{Name: "a", MaxHull: 200, MaxShield: 100, Recharge: 2, CritPct: 7, WeaponSkillPct: 10,
		Weapons: []Weapon{{Damage: 20, Type: "energy", Cooldown: 1}}}
	b := &StatBlock{Name: "b", MaxHull: 200, MaxShield: 100, Recharge: 2, CritPct: 7, WeaponSkillPct: 10,
		Weapons: []Weapon{{Damage: 20, Type: "kinetic", Cooldown: 2}}}
	cal := DefaultCalibration()
	cal.HitChanceA, cal.HitChanceB = 0.8, 0.8
	o1 := RunBattle(a, b, StanceFire, StanceFire, cal, rand.New(rand.NewPCG(9, 9)))
	o2 := RunBattle(a, b, StanceFire, StanceFire, cal, rand.New(rand.NewPCG(9, 9)))
	if o1 != o2 {
		t.Errorf("same seed gave %s then %s", o1, o2)
	}
}

func TestLoadCalibrationFile(t *testing.T) {
	c, err := LoadCalibration("../../data/combat-sim/calibration.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.BraceInMult != 0.25 || c.RegenHitDivisor != 3 || c.MaxTicks != 500 {
		t.Errorf("calibration = %+v, want measured defaults", c)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/combat-sim/ -run 'TestRunBattle|TestFlee|TestBrace|TestDeterministic|TestLoadCalibration' -v` — Expected: FAIL (types undefined).

- [ ] **Step 3: Implement battle.go, calibration.json; thread Calibration through armorReduce**

`cmd/combat-sim/battle.go`:

```go
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
	if att.Stance == StanceFlee {
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
	return ResolveVolley(att.Stats, tgt, fired, crits, stanceInMult(tgt.Stance, cal))
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
```

Modify `engine.go`: delete `var armorLaw` and `const armorLawCrossover`; change signatures to
`func armorReduce(hullIn int, armorTotal float64, dmgType string, cal *Calibration) int` and
`func ResolveVolley(att *StatBlock, tgt *SideState, fired []int, critFlags []bool, stanceInMult float64, cal *Calibration) VolleyOutcome`,
using `cal.ArmorLaw` / `cal.ArmorLawCrossover`. Update `engine_test.go` call sites to pass `DefaultCalibration()`.

- [ ] **Step 4: Run the full package tests**

Run: `go test ./cmd/combat-sim/ -v` — Expected: ALL tests pass (golden replay must still pass after the signature change).

- [ ] **Step 5: Commit**

```bash
git add cmd/combat-sim/ data/combat-sim/calibration.json
git commit -m "feat(combat-sim): tick engine with stances, calibration file, regen and flee"
```

---

### Task 5: Monte Carlo runner + stance table

**Files:**
- Create: `cmd/combat-sim/table.go`
- Test: `cmd/combat-sim/table_test.go`

**Interfaces:**
- Consumes: `RunBattle`, `allStances`, `Outcome`, `Calibration` (Task 4).
- Produces (used by Task 6):

```go
type Cell struct {
	StanceA, StanceB Stance
	Counts           map[Outcome]int
	Runs             int
}

// RunTable runs `runs` battles for each of the 16 stance pairs. Deterministic
// under seed: cell (i,j) uses rand.NewPCG(seed, uint64(i*4+j)).
func RunTable(a, b *StatBlock, cal *Calibration, runs int, seed uint64) []Cell // len 16, row-major A×B

// FormatTable renders the ASCII table. Cells resting on ASSUMED calibration
// (any cell where either stance is evade or flee, when those params are in
// cal.Assumed) get a trailing '*'.
func FormatTable(a, b *StatBlock, cells []Cell, cal *Calibration) string
```

Table cell text: dominant outcome + percent, e.g. `A-kill 74%*`. Below the table print a legend: full outcome distribution is available via `--json`; `*` = depends on ASSUMED calibration entries (list them).

- [ ] **Step 1: Write the failing tests**

`cmd/combat-sim/table_test.go`:

```go
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
	cells := RunTable(a, b, DefaultCalibration(), 10, 1)
	out := FormatTable(a, b, cells, DefaultCalibration())
	if !strings.Contains(out, "*") || !strings.Contains(out, "evade_in_mult") {
		t.Errorf("table must flag ASSUMED-dependent cells and legend them:\n%s", out)
	}
	if !strings.Contains(out, "fire") || !strings.Contains(out, "brace") {
		t.Errorf("table missing stance headers:\n%s", out)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/combat-sim/ -run 'TestRunTable|TestFormatTable' -v` — Expected: FAIL.

- [ ] **Step 3: Implement table.go**

```go
package main

import (
	"fmt"
	"math/rand/v2"
	"strings"
)

type Cell struct {
	StanceA, StanceB Stance
	Counts           map[Outcome]int
	Runs             int
}

// RunTable runs `runs` battles per stance pair, deterministic under seed.
func RunTable(a, b *StatBlock, cal *Calibration, runs int, seed uint64) []Cell {
	cells := make([]Cell, 0, 16)
	for i, sa := range allStances {
		for j, sb := range allStances {
			rng := rand.New(rand.NewPCG(seed, uint64(i*len(allStances)+j)))
			c := Cell{StanceA: sa, StanceB: sb, Counts: map[Outcome]int{}, Runs: runs}
			for range runs {
				c.Counts[RunBattle(a, b, sa, sb, cal, rng)]++
			}
			cells = append(cells, c)
		}
	}
	return cells
}

func cellAssumed(c Cell, cal *Calibration) bool {
	assumed := func(name string) bool {
		for _, x := range cal.Assumed {
			if x == name {
				return true
			}
		}
		return false
	}
	if (c.StanceA == StanceEvade || c.StanceB == StanceEvade) && assumed("evade_in_mult") {
		return true
	}
	if (c.StanceA == StanceFlee || c.StanceB == StanceFlee) && assumed("flee_escape_per_tick") {
		return true
	}
	return false
}

func dominant(c Cell) (Outcome, float64) {
	var best Outcome
	bn := -1
	for _, o := range []Outcome{OutAKill, OutBKill, OutAFled, OutBFled, OutMutual, OutStalemate} {
		if c.Counts[o] > bn {
			best, bn = o, c.Counts[o]
		}
	}
	return best, 100 * float64(bn) / float64(c.Runs)
}

// FormatTable renders the 4×4 stance table (A rows × B cols).
func FormatTable(a, b *StatBlock, cells []Cell, cal *Calibration) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "A = %s   vs   B = %s   (%d runs/cell)\n\n", a.Name, b.Name, cells[0].Runs)
	fmt.Fprintf(&sb, "%-10s", "A \\ B")
	for _, s := range allStances {
		fmt.Fprintf(&sb, "%-18s", s)
	}
	sb.WriteString("\n")
	for i, sa := range allStances {
		fmt.Fprintf(&sb, "%-10s", sa)
		for j := range allStances {
			c := cells[i*len(allStances)+j]
			o, pct := dominant(c)
			mark := ""
			if cellAssumed(c, cal) {
				mark = "*"
			}
			fmt.Fprintf(&sb, "%-18s", fmt.Sprintf("%s %.0f%%%s", o, pct, mark))
		}
		sb.WriteString("\n")
	}
	if len(cal.Assumed) > 0 {
		fmt.Fprintf(&sb, "\n* cell depends on ASSUMED calibration: %s (see data/combat-sim/calibration.json)\n",
			strings.Join(cal.Assumed, ", "))
	}
	return sb.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/combat-sim/ -v` — Expected: ALL pass.

- [ ] **Step 5: Commit**

```bash
git add cmd/combat-sim/table.go cmd/combat-sim/table_test.go
git commit -m "feat(combat-sim): Monte Carlo stance table with ASSUMED-parameter flagging"
```

---

### Task 6: CLI, example fits, README, lint

**Files:**
- Create: `cmd/combat-sim/main.go`, `data/combat-sim/fits/molten_broadaxe.json`, `data/combat-sim/fits/artis_survey.json`, `data/combat-sim/README.md`

**Interfaces:**
- Consumes: everything above. Produces: the `combat-sim` binary.

- [ ] **Step 1: Write main.go**

```go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func run() error {
	fitA := flag.String("a", "", "fitting spec JSON for side A (required)")
	fitB := flag.String("b", "", "fitting spec JSON for side B (required)")
	runs := flag.Int("runs", 10000, "battles per stance-pair cell")
	seed := flag.Uint64("seed", 42, "RNG seed (deterministic output per seed)")
	catalog := flag.String("catalog", "data/snapshots/latest", "catalog snapshot dir")
	calPath := flag.String("calibration", "data/combat-sim/calibration.json", "calibration file (missing = built-in defaults)")
	maxTicks := flag.Int("max-ticks", 0, "override calibration max_ticks (0 = keep)")
	jsonOut := flag.String("json", "", "write full per-cell outcome distributions to this file")
	flag.Parse()
	if *fitA == "" || *fitB == "" {
		flag.Usage()
		return fmt.Errorf("--a and --b are required")
	}
	cat, err := LoadCatalog(*catalog)
	if err != nil {
		return err
	}
	cal, err := LoadCalibration(*calPath)
	if os.IsNotExist(err) {
		cal, err = DefaultCalibration(), nil
	}
	if err != nil {
		return err
	}
	if *maxTicks > 0 {
		cal.MaxTicks = *maxTicks
	}
	fa, err := LoadFit(*fitA)
	if err != nil {
		return err
	}
	fb, err := LoadFit(*fitB)
	if err != nil {
		return err
	}
	a, err := Resolve(fa, cat)
	if err != nil {
		return err
	}
	b, err := Resolve(fb, cat)
	if err != nil {
		return err
	}
	cells := RunTable(a, b, cal, *runs, *seed)
	fmt.Print(FormatTable(a, b, cells, cal))
	if *jsonOut != "" {
		raw, err := json.MarshalIndent(cells, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "combat-sim:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Write the example fits**

`data/combat-sim/fits/molten_broadaxe.json`:

```json
{
  "name": "MoltenOne-Broadaxe",
  "hull": "broadaxe",
  "modules": ["autocannon_ii", "autocannon_ii", "autocannon_ii", "flak_cannon_ii", "armor_plate_ii"],
  "skills": {"weapons": 7, "gunnery": 10, "shields": 4}
}
```

`data/combat-sim/fits/artis_survey.json`:

```json
{
  "name": "Artis-Survey",
  "hull": "survey_vessel",
  "modules": ["pulse_laser_iii", "pulse_laser_iii", "shield_booster_iv", "shield_recharger_ii"],
  "skills": {"weapons": 3, "gunnery": 3, "shields": 1}
}
```

- [ ] **Step 3: Build and smoke-run**

```bash
go build -o bin/combat-sim ./cmd/combat-sim
bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json --b data/combat-sim/fits/artis_survey.json --runs 2000
```

Expected: a 4×4 table prints in under a couple of seconds; evade/flee cells carry `*`; no cell's percentages exceed 100. Sanity: fire-vs-fire should favor the broadaxe (its real fight was lost only because the survey fit landed a lucky opening — with symmetric 0.95 hit chance the kinetic DPS side wins most runs; if it does not, inspect, don't hand-tune).

- [ ] **Step 4: Write `data/combat-sim/README.md`**

```markdown
# combat-sim

Hermetic 1v1 combat Monte Carlo over the log-verified damage model
(spec: docs/superpowers/specs/2026-08-31-combat-sim-design.md).

    go build -o bin/combat-sim ./cmd/combat-sim
    bin/combat-sim --a data/combat-sim/fits/molten_broadaxe.json \
                   --b data/combat-sim/fits/artis_survey.json --runs 10000

Inputs: committed catalog snapshots + two fitting-spec JSONs + calibration.json.
No databases, no network, no credentials.

Measured vs ASSUMED: every uncalibrated constant lives in
data/combat-sim/calibration.json with an `assumed` list; table cells that
depend on one are marked `*`. Phase B (scripted stance-pair duels between
owned agents) exists to measure evade_in_mult, flee_escape_per_tick, and
per-pair hit chances.

Not modeled in v1: drone repair (logs omit it — drone-fit survival is
underestimated), boarding, zones/movement (fixed at engaged), ammo reload,
armor-melt and EM debuffs, mixed-damage-type volleys, capital hulls,
wildlife (phase C).
```

- [ ] **Step 5: Full gate: tests, lint, build**

```bash
go build ./... && go test ./cmd/combat-sim/... -count=1
golangci-lint run ./cmd/combat-sim/...
```

Expected: build OK, all tests pass, zero lint findings. Fix any findings before committing.

- [ ] **Step 6: Commit**

```bash
git add cmd/combat-sim/ data/combat-sim/
git commit -m "feat(combat-sim): CLI, example fixture fits, README"
```

---

## Self-Review Notes (already applied)

- Spec coverage: loader/resolver/engine/calibration/table/CLI/tests/README all have tasks; typed-resist stage is intentionally OMITTED from v1 code (no hardener modules in any fixture or example fit; spec lists it — deviation documented in README's "Not modeled" would be wrong, so note: typed resists default 0 and the pipeline slot for them is the `x1` stage; adding them later is a one-line multiply. This is recorded here so the executor knows it is deliberate).
- Mixed-type volleys: v1 resolves single-type volleys only (all fixture and example fits are single-type); `ResolveVolley` takes the type from the fired weapons. Documented in README.
- Golden expectations derive from `data/battles/analysis/verify4.py`/`verify6_percentage_armor.py`; if a golden test fails, run those scripts to re-check the arithmetic before touching expectations.
