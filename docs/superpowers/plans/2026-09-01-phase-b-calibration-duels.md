# Phase B Calibration Duels Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the duel-runner tool and campaign data, run scripted calibration duels between battle_bot1/battle_bot2 in a lawless system, and land measured values in cmd/combat-sim's calibration with fixtures, analysis scripts, and golden tests.

**Architecture:** A new `cmd/tools/duel-runner` in the spacemolt repo holds two persistent logged-in `game.Client` sessions and executes a data-driven campaign file (per-duel fits, per-tick stance scripts, ring holds), appending results to an append-only JSONL manifest. The KB repo carries the campaign data, exported fixtures, four analysis scripts, and the calibration/test updates.

**Tech Stack:** Go 1.24+ (spacemolt repo: pkg/game client, RawCommand plumbing), Python 3 (analysis), existing bin/battle-export batch mode for fixture export.

**Spec:** `docs/superpowers/specs/2026-09-01-phase-b-calibration-duels-design.md` (KB repo — read it first; it is the binding authority).

## Global Constraints

- Two repos: tool code in `/home/robert/spacemolt/spacemolt` (its pre-commit hook runs build+test+lint); campaign/fixtures/analysis/calibration in `/home/robert/spacemolt/kb`.
- All new Go code passes `golangci-lint run` with zero new findings; benchmarks (none expected) use `b.Loop()`.
- Sleeps in the spacemolt repo use the constants in `pkg/game/constants.go` (`game.SleepQuick` = 2s, etc.); never raw durations.
- Built binaries go in `bin/`, never the repo root.
- **Tasks 6 is live-game operations (logins, credits, ship losses). It is controller-executed with the owner watching — never dispatch it to a subagent.** Logging in an agent kills any live session using it; check `ps aux | grep bin/worker` first; 36 seconds between logins of the same agent.
- Commit messages end with the standard Co-Authored-By + Claude-Session trailers used throughout this session.
- Command names (exact): `attack`, `battle` (payload `action`: advance/retreat/stance/target/engage), `get_ship`, `get_status`, `get_skills`, `jump`, `dock`, `undock`, `buy`, `install_mod`, `uninstall_mod`, `set_home_base`, `get_battle_status`. Issue all of them via `client.RawCommand(ctx, cmd, args)`.

---

## File structure

Spacemolt repo (`cmd/tools/duel-runner/`):
- `campaign.go` — campaign/duel/phase types, loader, validation, `PhaseAt`
- `campaign_test.go`
- `manifest.go` — append-only JSONL records, resume filter
- `manifest_test.go`
- `battle.go` — the per-duel control loop against a narrow `side` interface (unit-testable without a network)
- `battle_test.go`
- `session.go` — `*game.Client` adapter implementing `side`, plus logistics (ensureFit, travel, recovery); the only file that touches the live client
- `main.go` — flags, logins, resume loop, S0 gating

KB repo:
- `data/battles/duels/campaign.json` — the full scenario matrix (Task 5)
- `data/battles/duels/README.md` — arena choice, shopping list, funding + run log
- `data/battles/duels/manifest.jsonl`, exported fixtures (Task 6)
- `data/battles/analysis/phaseb_hit_table.py`, `phaseb_stances.py`, `phaseb_flee.py`, `phaseb_armor.py` (Task 7)
- `data/combat-sim/calibration.json`, `cmd/combat-sim/golden_duel_test.go`, README/spec errata (Task 8)

---

### Task 1: Campaign schema, loader, and phase scheduler

**Files:**
- Create: `cmd/tools/duel-runner/campaign.go`
- Test: `cmd/tools/duel-runner/campaign_test.go`
(both in `/home/robert/spacemolt/spacemolt`)

**Interfaces:**
- Produces: `type FitSpec struct{ Hull string; Modules []string }`, `type Phase struct{ FromTick int; StanceA, StanceB string; HoldRing *int }`, `type Duel struct{ ID, Purpose, Attacker, Guest string; FitA, FitB FitSpec; Script []Phase; MaxTicks, Repeats int }`, `type Campaign struct{ ArenaSystem, StagingSystem, StagingStation string; Duels []Duel }`, `func LoadCampaign(path string) (*Campaign, error)`, `func (d *Duel) PhaseAt(tick int) Phase`.

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCampaign(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "campaign.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const validCampaign = `{
  "arena_system": "gsc_test",
  "staging_system": "sys_x",
  "staging_station": "station_x",
  "duels": [{
    "id": "S1-ring2",
    "purpose": "hit table @ distance 4",
    "attacker": "battle_bot1",
    "fit_a": {"hull": "prospect", "modules": ["missile_launcher_i"]},
    "fit_b": {"hull": "prospect", "modules": ["missile_launcher_i"]},
    "script": [
      {"from_tick": 1, "stance_a": "fire", "stance_b": "fire", "hold_ring": 2},
      {"from_tick": 20, "stance_a": "flee", "stance_b": "flee"}
    ],
    "max_ticks": 25,
    "repeats": 2
  }]
}`

func TestLoadCampaignValid(t *testing.T) {
	c, err := LoadCampaign(writeCampaign(t, validCampaign))
	if err != nil {
		t.Fatalf("LoadCampaign: %v", err)
	}
	if c.ArenaSystem != "gsc_test" || len(c.Duels) != 1 {
		t.Fatalf("parsed = %+v", c)
	}
	d := c.Duels[0]
	if d.Attacker != "battle_bot1" || d.MaxTicks != 25 || d.Repeats != 2 {
		t.Errorf("duel = %+v", d)
	}
	if d.Script[0].HoldRing == nil || *d.Script[0].HoldRing != 2 {
		t.Errorf("hold_ring not parsed: %+v", d.Script[0])
	}
	if d.Script[1].HoldRing != nil {
		t.Errorf("phase 2 must have nil HoldRing")
	}
}

func TestPhaseAtPicksLatestPhase(t *testing.T) {
	c, _ := LoadCampaign(writeCampaign(t, validCampaign))
	d := c.Duels[0]
	if p := d.PhaseAt(1); p.StanceA != "fire" {
		t.Errorf("tick 1 = %+v", p)
	}
	if p := d.PhaseAt(19); p.StanceA != "fire" {
		t.Errorf("tick 19 = %+v", p)
	}
	if p := d.PhaseAt(20); p.StanceA != "flee" {
		t.Errorf("tick 20 = %+v", p)
	}
	if p := d.PhaseAt(999); p.StanceB != "flee" {
		t.Errorf("tick 999 = %+v", p)
	}
}

func TestLoadCampaignRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"empty duels":    `{"arena_system":"a","staging_station":"s","duels":[]}`,
		"no id":          `{"arena_system":"a","staging_station":"s","duels":[{"attacker":"x","max_ticks":5,"repeats":1,"script":[{"from_tick":1,"stance_a":"fire","stance_b":"fire"}]}]}`,
		"bad stance":     `{"arena_system":"a","staging_station":"s","duels":[{"id":"d","attacker":"x","max_ticks":5,"repeats":1,"script":[{"from_tick":1,"stance_a":"charge","stance_b":"fire"}]}]}`,
		"no script":      `{"arena_system":"a","staging_station":"s","duels":[{"id":"d","attacker":"x","max_ticks":5,"repeats":1}]}`,
		"zero max_ticks": `{"arena_system":"a","staging_station":"s","duels":[{"id":"d","attacker":"x","max_ticks":0,"repeats":1,"script":[{"from_tick":1,"stance_a":"fire","stance_b":"fire"}]}]}`,
		"dup id":         `{"arena_system":"a","staging_station":"s","duels":[{"id":"d","attacker":"x","max_ticks":5,"repeats":1,"script":[{"from_tick":1,"stance_a":"fire","stance_b":"fire"}]},{"id":"d","attacker":"x","max_ticks":5,"repeats":1,"script":[{"from_tick":1,"stance_a":"fire","stance_b":"fire"}]}]}`,
	}
	for name, body := range cases {
		if _, err := LoadCampaign(writeCampaign(t, body)); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/robert/spacemolt/spacemolt && go test ./cmd/tools/duel-runner/ -run 'TestLoadCampaign|TestPhaseAt' -v`
Expected: FAIL (undefined: LoadCampaign)

- [ ] **Step 3: Implement campaign.go**

```go
// Command duel-runner executes scripted 1v1 calibration duels between two
// owned agents from a campaign file, per the Phase B design:
// kb/docs/superpowers/specs/2026-09-01-phase-b-calibration-duels-design.md
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// FitSpec is the exact hull + module list a bot must carry for a duel.
type FitSpec struct {
	Hull    string   `json:"hull"`
	Modules []string `json:"modules"`
}

// Phase is one segment of a duel's stance script. HoldRing, when set,
// pins the shared separation ring (0=engaged .. 3=outer) by issuing
// advance/retreat corrections.
type Phase struct {
	FromTick int    `json:"from_tick"`
	StanceA  string `json:"stance_a"`
	StanceB  string `json:"stance_b"`
	HoldRing *int   `json:"hold_ring,omitempty"`
}

// Duel is one scenario entry; it runs Repeats times.
type Duel struct {
	ID       string  `json:"id"`
	Purpose  string  `json:"purpose"`
	Attacker string  `json:"attacker"`
	Guest    string  `json:"guest,omitempty"` // replaces bot B when set (S6c)
	FitA     FitSpec `json:"fit_a"`
	FitB     FitSpec `json:"fit_b"`
	Script   []Phase `json:"script"`
	MaxTicks int     `json:"max_ticks"`
	Repeats  int     `json:"repeats"`
}

// Campaign is the whole scenario matrix plus its geography.
type Campaign struct {
	ArenaSystem    string `json:"arena_system"`
	StagingSystem  string `json:"staging_system"`
	StagingStation string `json:"staging_station"`
	Duels          []Duel `json:"duels"`
}

var validStances = map[string]bool{"fire": true, "brace": true, "evade": true, "flee": true}

// LoadCampaign reads and validates a campaign file. Every defect found is
// an error naming the duel — a campaign typo must never surface mid-run.
func LoadCampaign(path string) (*Campaign, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Campaign
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.ArenaSystem == "" || c.StagingSystem == "" || c.StagingStation == "" {
		return nil, fmt.Errorf("%s: arena_system, staging_system, staging_station are all required", path)
	}
	if len(c.Duels) == 0 {
		return nil, fmt.Errorf("%s: campaign has no duels", path)
	}
	seen := map[string]bool{}
	for i, d := range c.Duels {
		if d.ID == "" {
			return nil, fmt.Errorf("duel %d: missing id", i)
		}
		if seen[d.ID] {
			return nil, fmt.Errorf("duel %q: duplicate id", d.ID)
		}
		seen[d.ID] = true
		if d.MaxTicks <= 0 {
			return nil, fmt.Errorf("duel %q: max_ticks must be > 0", d.ID)
		}
		if d.Repeats <= 0 {
			return nil, fmt.Errorf("duel %q: repeats must be > 0", d.ID)
		}
		if len(d.Script) == 0 {
			return nil, fmt.Errorf("duel %q: empty script", d.ID)
		}
		for _, p := range d.Script {
			if !validStances[p.StanceA] || !validStances[p.StanceB] {
				return nil, fmt.Errorf("duel %q: invalid stance %q/%q", d.ID, p.StanceA, p.StanceB)
			}
			if p.HoldRing != nil && (*p.HoldRing < 0 || *p.HoldRing > 3) {
				return nil, fmt.Errorf("duel %q: hold_ring %d out of range 0..3", d.ID, *p.HoldRing)
			}
		}
	}
	return &c, nil
}

// PhaseAt returns the script phase in force at tick (the last phase whose
// FromTick <= tick; before the first phase, the first phase applies).
func (d *Duel) PhaseAt(tick int) Phase {
	cur := d.Script[0]
	for _, p := range d.Script {
		if p.FromTick <= tick {
			cur = p
		}
	}
	return cur
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/tools/duel-runner/ -v`
Expected: PASS (all three tests)

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./cmd/tools/duel-runner/
git add cmd/tools/duel-runner/campaign.go cmd/tools/duel-runner/campaign_test.go
git commit -m "feat(duel-runner): campaign schema, loader, phase scheduler"
```

---

### Task 2: Append-only manifest with resume

**Files:**
- Create: `cmd/tools/duel-runner/manifest.go`
- Test: `cmd/tools/duel-runner/manifest_test.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `type Record struct{ ScenarioID string; Repeat int; BattleID string; Started, Ended time.Time; Outcome string; Void bool }` (JSON tags snake_case: scenario_id, repeat, battle_id, started, ended, outcome, void), `func AppendRecord(path string, r Record) error`, `func LoadDone(path string) (map[string]bool, error)`, `func DoneKey(id string, repeat int) string` (returns `id + "#" + strconv.Itoa(repeat)`).

- [ ] **Step 1: Write the failing tests**

```go
package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestManifestRoundTripAndResume(t *testing.T) {
	p := filepath.Join(t.TempDir(), "manifest.jsonl")
	now := time.Now().UTC()
	recs := []Record{
		{ScenarioID: "S0", Repeat: 1, BattleID: "b1", Started: now, Ended: now, Outcome: "A-fled"},
		{ScenarioID: "S1-ring2", Repeat: 1, BattleID: "b2", Started: now, Ended: now, Outcome: "stalemate"},
		{ScenarioID: "S1-ring2", Repeat: 2, BattleID: "b3", Started: now, Ended: now, Outcome: "stalemate", Void: true},
	}
	for _, r := range recs {
		if err := AppendRecord(p, r); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	done, err := LoadDone(p)
	if err != nil {
		t.Fatalf("LoadDone: %v", err)
	}
	if !done[DoneKey("S0", 1)] || !done[DoneKey("S1-ring2", 1)] {
		t.Errorf("completed duels missing from done set: %v", done)
	}
	// A void record does NOT count as done — it must be re-run.
	if done[DoneKey("S1-ring2", 2)] {
		t.Error("void record counted as done")
	}
}

func TestLoadDoneMissingFileIsEmpty(t *testing.T) {
	done, err := LoadDone(filepath.Join(t.TempDir(), "nope.jsonl"))
	if err != nil {
		t.Fatalf("missing manifest must not error: %v", err)
	}
	if len(done) != 0 {
		t.Errorf("done = %v, want empty", done)
	}
}
```

- [ ] **Step 2: Run to verify FAIL** — `go test ./cmd/tools/duel-runner/ -run TestManifest -v` → undefined: Record.

- [ ] **Step 3: Implement manifest.go**

```go
package main

import (
	"bufio"
	"encoding/json"
	"os"
	"strconv"
	"time"
)

// Record is one completed (or voided) duel run. The manifest is append-only
// JSONL: a killed session loses at most the in-flight duel.
type Record struct {
	ScenarioID string    `json:"scenario_id"`
	Repeat     int       `json:"repeat"`
	BattleID   string    `json:"battle_id"`
	Started    time.Time `json:"started"`
	Ended      time.Time `json:"ended"`
	Outcome    string    `json:"outcome"`
	Void       bool      `json:"void"`
}

// DoneKey identifies one (scenario, repeat) run.
func DoneKey(id string, repeat int) string { return id + "#" + strconv.Itoa(repeat) }

// AppendRecord appends one JSON line, fsyncing so a crash cannot lose it.
func AppendRecord(path string, r Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// LoadDone returns the set of completed non-void (scenario, repeat) keys.
// A missing manifest is an empty campaign, not an error.
func LoadDone(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	done := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		if !r.Void {
			done[DoneKey(r.ScenarioID, r.Repeat)] = true
		}
	}
	return done, sc.Err()
}
```

- [ ] **Step 4: Run to verify PASS**, **Step 5: lint + commit** (`feat(duel-runner): append-only manifest with resume`).

---

### Task 3: Duel control loop (pure, against a `side` interface)

**Files:**
- Create: `cmd/tools/duel-runner/battle.go`
- Test: `cmd/tools/duel-runner/battle_test.go`

**Interfaces:**
- Consumes: `Duel`, `Phase`, `(*Duel).PhaseAt` from Task 1.
- Produces:

```go
// BattleView is the per-tick snapshot the loop consumes.
type BattleView struct {
	BattleID         string
	Tick             int
	Ended            bool
	Outcome          string // set when Ended
	MyZone           string // outer|mid|inner|engaged
	MyStance         string
	ParticipantCount int
}
type side interface {
	Name() string
	// Battle issues a free tactical action: stance/advance/retreat.
	Battle(action string, kv map[string]any) error
	// View returns the latest battle_update-derived snapshot; ok=false
	// before the battle exists client-side.
	View() (BattleView, bool)
}
type duelResult struct {
	BattleID string
	Outcome  string
	Void     bool
	Ticks    int
}
func runDuel(a, b side, d Duel, wait func(), logger *log.Logger) (duelResult, error)
```

`wait` is the tick pacer (`func() { time.Sleep(game.SleepQuick) }` in production; a no-op in tests). Ring math: `ringOf = map[string]int{"engaged":0,"inner":1,"mid":2,"outer":3}`.

- [ ] **Step 1: Write the failing tests** — a scripted fake `side` that plays back a sequence of `BattleView`s and records the actions it was ordered to take:

```go
package main

import (
	"log"
	"os"
	"testing"
)

type fakeSide struct {
	name    string
	views   []BattleView // consumed one per View() call; last repeats
	i       int
	actions []string // "stance:fire", "advance", "retreat"
}

func (f *fakeSide) Name() string { return f.name }
func (f *fakeSide) Battle(action string, kv map[string]any) error {
	if action == "stance" {
		f.actions = append(f.actions, "stance:"+kv["stance"].(string))
	} else {
		f.actions = append(f.actions, action)
	}
	return nil
}
func (f *fakeSide) View() (BattleView, bool) {
	if len(f.views) == 0 {
		return BattleView{}, false
	}
	v := f.views[min(f.i, len(f.views)-1)]
	f.i++
	return v, true
}

func testLogger() *log.Logger { return log.New(os.Stderr, "", 0) }

func mkViews(n int, zone string, parts int) []BattleView {
	vs := make([]BattleView, 0, n+1)
	for t := 1; t <= n; t++ {
		vs = append(vs, BattleView{BattleID: "bx", Tick: t, MyZone: zone, ParticipantCount: parts})
	}
	vs = append(vs, BattleView{BattleID: "bx", Tick: n + 1, Ended: true, Outcome: "stalemate", ParticipantCount: parts})
	return vs
}

func ringPtr(r int) *int { return &r }

func TestRunDuelAppliesStancesAndHoldsRing(t *testing.T) {
	// Both start at outer (ring 3); script holds ring 2 → each side must
	// be ordered to advance until its zone reads mid, then hold.
	views := append(mkViews(3, "outer", 2)[:3], mkViews(3, "mid", 2)...)
	a := &fakeSide{name: "A", views: views}
	b := &fakeSide{name: "B", views: views}
	d := Duel{ID: "t", Attacker: "A", MaxTicks: 10,
		Script: []Phase{{FromTick: 1, StanceA: "fire", StanceB: "fire", HoldRing: ringPtr(2)}}}
	res, err := runDuel(a, b, d, func() {}, testLogger())
	if err != nil {
		t.Fatalf("runDuel: %v", err)
	}
	if res.Outcome != "stalemate" || res.BattleID != "bx" || res.Void {
		t.Errorf("result = %+v", res)
	}
	if a.actions[0] != "stance:fire" {
		t.Errorf("first action = %v, want stance:fire", a.actions)
	}
	advances := 0
	for _, act := range a.actions {
		if act == "advance" {
			advances++
		}
	}
	if advances == 0 {
		t.Errorf("at outer holding ring 2: expected advance orders, got %v", a.actions)
	}
	for i, act := range a.actions {
		_ = i
		if act == "retreat" {
			t.Errorf("holding ring 2 from outer must never retreat: %v", a.actions)
		}
	}
}

func TestRunDuelVoidsOnThirdParticipant(t *testing.T) {
	views := []BattleView{
		{BattleID: "bx", Tick: 1, MyZone: "outer", ParticipantCount: 2},
		{BattleID: "bx", Tick: 2, MyZone: "outer", ParticipantCount: 3}, // pirate joins
		{BattleID: "bx", Tick: 3, Ended: true, Outcome: "interference", ParticipantCount: 3},
	}
	a := &fakeSide{name: "A", views: views}
	b := &fakeSide{name: "B", views: views}
	d := Duel{ID: "t", Attacker: "A", MaxTicks: 10,
		Script: []Phase{{FromTick: 1, StanceA: "fire", StanceB: "fire"}}}
	res, err := runDuel(a, b, d, func() {}, testLogger())
	if err != nil {
		t.Fatalf("runDuel: %v", err)
	}
	if !res.Void {
		t.Errorf("third participant must void the duel: %+v", res)
	}
	// After the join both sides must have been ordered to flee out.
	last := a.actions[len(a.actions)-1]
	if last != "stance:flee" {
		t.Errorf("void must order flee, actions = %v", a.actions)
	}
}

func TestRunDuelMaxTicksOrdersFleeOut(t *testing.T) {
	// Battle never ends on its own within MaxTicks: the loop must switch
	// both sides to flee and keep going until the battle ends.
	views := append(mkViews(30, "outer", 2)[:5], // 5 live ticks
		BattleView{BattleID: "bx", Tick: 6, MyZone: "outer", ParticipantCount: 2},
		BattleView{BattleID: "bx", Tick: 7, Ended: true, Outcome: "escape", ParticipantCount: 2})
	a := &fakeSide{name: "A", views: views}
	b := &fakeSide{name: "B", views: views}
	d := Duel{ID: "t", Attacker: "A", MaxTicks: 4,
		Script: []Phase{{FromTick: 1, StanceA: "fire", StanceB: "fire"}}}
	res, err := runDuel(a, b, d, func() {}, testLogger())
	if err != nil {
		t.Fatalf("runDuel: %v", err)
	}
	if res.Outcome != "escape" {
		t.Errorf("outcome = %q", res.Outcome)
	}
	sawFlee := false
	for _, act := range a.actions {
		if act == "stance:flee" {
			sawFlee = true
		}
	}
	if !sawFlee {
		t.Errorf("past MaxTicks the loop must flee out: %v", a.actions)
	}
}
```

- [ ] **Step 2: verify FAIL** (`go test ./cmd/tools/duel-runner/ -run TestRunDuel -v` → undefined: runDuel)

- [ ] **Step 3: Implement battle.go**

```go
package main

import (
	"fmt"
	"log"
)

// BattleView is the per-tick snapshot the control loop consumes. session.go
// builds it from the client's battle_update-derived state; tests fake it.
type BattleView struct {
	BattleID         string
	Tick             int
	Ended            bool
	Outcome          string
	MyZone           string
	MyStance         string
	ParticipantCount int
}

type side interface {
	Name() string
	Battle(action string, kv map[string]any) error
	View() (BattleView, bool)
}

type duelResult struct {
	BattleID string
	Outcome  string
	Void     bool
	Ticks    int
}

var ringOf = map[string]int{"engaged": 0, "inner": 1, "mid": 2, "outer": 3}

// applyOrders issues the phase's stance for one side and, when a ring hold
// is set, an advance/retreat correction toward it. Stance orders are
// re-issued only on change (they queue for the next tick and are free).
func applyOrders(s side, stance string, hold *int, lastStance *string, v BattleView, logger *log.Logger) error {
	if *lastStance != stance {
		if err := s.Battle("stance", map[string]any{"stance": stance}); err != nil {
			return err
		}
		*lastStance = stance
	}
	if hold == nil || stance == "flee" { // flee auto-retreats; never fight it
		return nil
	}
	ring, ok := ringOf[v.MyZone]
	if !ok {
		return nil // unknown zone label: skip correction this tick
	}
	switch {
	case ring > *hold:
		return s.Battle("advance", nil)
	case ring < *hold:
		return s.Battle("retreat", nil)
	}
	return nil
}

// runDuel drives one battle from first view to battle end. The attack that
// creates the battle has already been issued by the caller; runDuel only
// applies the script, voids on interference, and flees out past MaxTicks.
func runDuel(a, b side, d Duel, wait func(), logger *log.Logger) (duelResult, error) {
	var res duelResult
	lastA, lastB := "", ""
	voided := false
	for i := 0; ; i++ {
		if i > d.MaxTicks*20+200 {
			return res, fmt.Errorf("duel %s: no battle end after %d polls", d.ID, i)
		}
		va, okA := a.View()
		if !okA {
			wait()
			continue
		}
		res.BattleID = va.BattleID
		res.Ticks = va.Tick
		if va.Ended {
			res.Outcome = va.Outcome
			res.Void = voided
			return res, nil
		}
		phase := d.PhaseAt(va.Tick)
		stanceA, stanceB, hold := phase.StanceA, phase.StanceB, phase.HoldRing
		if va.ParticipantCount > 2 && !voided {
			voided = true
			logger.Printf("duel %s: %d participants — voiding, fleeing out", d.ID, va.ParticipantCount)
		}
		if voided || va.Tick > d.MaxTicks {
			stanceA, stanceB, hold = "flee", "flee", nil
		}
		if err := applyOrders(a, stanceA, hold, &lastA, va, logger); err != nil {
			return res, fmt.Errorf("side A orders: %w", err)
		}
		vb, okB := b.View()
		if okB {
			if err := applyOrders(b, stanceB, hold, &lastB, vb, logger); err != nil {
				return res, fmt.Errorf("side B orders: %w", err)
			}
		}
		wait()
	}
}
```

- [ ] **Step 4: verify PASS**, **Step 5: lint + commit** (`feat(duel-runner): scripted duel control loop with void and flee-out rules`).

---

### Task 4: Live session adapter, logistics, and main

**Files:**
- Create: `cmd/tools/duel-runner/session.go`
- Create: `cmd/tools/duel-runner/main.go`
- Test: `cmd/tools/duel-runner/session_test.go` (fit-diff only; the rest is exercised live by S0)

**Interfaces:**
- Consumes: everything from Tasks 1–3.
- Produces: `type Bot struct` implementing `side`; `func computeFitActions(current, want []string) (toRemove, toInstall []string)`; main flags `--campaign`, `--manifest`, `--a`, `--b`, `--guest`, `--only` (comma-separated scenario ids), `--dry-run`.

- [ ] **Step 1: Write the failing fit-diff test**

```go
package main

import (
	"reflect"
	"sort"
	"testing"
)

func TestComputeFitActions(t *testing.T) {
	cur := []string{"mining_laser_i", "pulse_laser_i", "pulse_laser_i"}
	want := []string{"pulse_laser_i", "missile_launcher_i"}
	rem, inst := computeFitActions(cur, want)
	sort.Strings(rem)
	sort.Strings(inst)
	if !reflect.DeepEqual(rem, []string{"mining_laser_i", "pulse_laser_i"}) {
		t.Errorf("remove = %v", rem)
	}
	if !reflect.DeepEqual(inst, []string{"missile_launcher_i"}) {
		t.Errorf("install = %v", inst)
	}
	// Duplicates count: two pulse lasers wanted, one present → install one.
	rem2, inst2 := computeFitActions([]string{"pulse_laser_i"}, []string{"pulse_laser_i", "pulse_laser_i"})
	if len(rem2) != 0 || !reflect.DeepEqual(inst2, []string{"pulse_laser_i"}) {
		t.Errorf("dup case: rem=%v inst=%v", rem2, inst2)
	}
}
```

- [ ] **Step 2: verify FAIL**, then **Step 3: implement session.go**

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rsned/spacemolt/pkg/game"
)

// Bot wraps a logged-in client as a duel `side` plus the out-of-battle
// logistics the campaign needs (refit, travel, recovery).
type Bot struct {
	agentID string
	client  *game.Client
	ctx     context.Context
	logger  *log.Logger
}

// settle mirrors battle-export: a reply lands in the raw cache ~2s after
// Submit returns.
func (b *Bot) settle() { time.Sleep(game.SleepQuick) }

func Login(ctx context.Context, agentID string, logger *log.Logger) (*Bot, error) {
	client, _, err := game.InitializeAgent(agentID, logger, ctx, false)
	if err != nil {
		return nil, fmt.Errorf("login %s: %w", agentID, err)
	}
	b := &Bot{agentID: agentID, client: client, ctx: ctx, logger: logger}
	b.settle()
	return b, nil
}

func (b *Bot) Close() { _ = b.client.Close() }

func (b *Bot) Name() string { return b.agentID }

func (b *Bot) Raw(cmd string, args map[string]any) error {
	if err := b.client.RawCommand(b.ctx, cmd, args); err != nil {
		return fmt.Errorf("%s %s: %w", b.agentID, cmd, err)
	}
	b.settle()
	return nil
}

func (b *Bot) Battle(action string, kv map[string]any) error {
	args := map[string]any{"action": action}
	for k, v := range kv {
		args[k] = v
	}
	// Battle actions are free and queue for the next tick; no settle needed
	// beyond the client's own send path.
	return b.client.RawCommand(b.ctx, "battle", args)
}

// View reads the latest battle state parsed from battle_update pushes.
func (b *Bot) View() (BattleView, bool) {
	st := b.client.State() // thread-safe snapshot per pkg/game (Clone)
	bs := st.Battle
	if bs == nil || bs.BattleID == "" {
		return BattleView{}, false
	}
	v := BattleView{
		BattleID:         bs.BattleID,
		Tick:             bs.Tick,
		Ended:            bs.Ended,
		Outcome:          bs.Outcome,
		ParticipantCount: len(bs.Participants),
	}
	for _, p := range bs.Participants {
		if p.Username == st.Player.Username {
			v.MyZone, v.MyStance = p.Zone, p.Stance
		}
	}
	return v, true
}

// --- logistics -----------------------------------------------------------

// computeFitActions diffs the current module list against the wanted one,
// respecting duplicates (multiset difference in both directions).
func computeFitActions(current, want []string) (toRemove, toInstall []string) {
	need := map[string]int{}
	for _, m := range want {
		need[m]++
	}
	for _, m := range current {
		if need[m] > 0 {
			need[m]--
		} else {
			toRemove = append(toRemove, m)
		}
	}
	for m, n := range need {
		for range n {
			toInstall = append(toInstall, m)
		}
	}
	return toRemove, toInstall
}

// shipInfo is the slice of get_ship the logistics code needs.
type shipInfo struct {
	Ship struct {
		ClassID string `json:"class_id"`
		Modules []struct {
			ID     string `json:"id"`
			ItemID string `json:"item_id"`
		} `json:"modules"`
	} `json:"ship"`
}

// EnsureFit docks (caller guarantees at staging), buys and installs until
// get_ship matches the FitSpec exactly, and errors on any mismatch it
// cannot resolve (wrong hull requires manual intervention — hull swaps are
// campaign setup, not per-duel work).
func (b *Bot) EnsureFit(fit FitSpec) error {
	if err := b.Raw("get_ship", nil); err != nil {
		return err
	}
	var info shipInfo
	if err := json.Unmarshal(b.client.GetRawJSON("ship"), &info); err != nil {
		return fmt.Errorf("%s: parse get_ship: %w", b.agentID, err)
	}
	if fit.Hull != "" && info.Ship.ClassID != fit.Hull {
		return fmt.Errorf("%s: hull is %q, scenario needs %q (swap hulls manually or via campaign setup)",
			b.agentID, info.Ship.ClassID, fit.Hull)
	}
	current := make([]string, 0, len(info.Ship.Modules))
	byItem := map[string][]string{} // item id -> instance ids
	for _, m := range info.Ship.Modules {
		current = append(current, m.ItemID)
		byItem[m.ItemID] = append(byItem[m.ItemID], m.ID)
	}
	toRemove, toInstall := computeFitActions(current, fit.Modules)
	for _, item := range toRemove {
		inst := byItem[item][0]
		byItem[item] = byItem[item][1:]
		if err := b.Raw("uninstall_mod", map[string]any{"module_id": inst}); err != nil {
			return err
		}
	}
	for _, item := range toInstall {
		// buy is a no-op cost-wise if cargo already holds one from a spare.
		if err := b.Raw("buy", map[string]any{"item_id": item, "quantity": 1}); err != nil {
			b.logger.Printf("%s: buy %s failed (may already own one): %v", b.agentID, item, err)
		}
		if err := b.Raw("install_mod", map[string]any{"module_id": item}); err != nil {
			return err
		}
	}
	// Verify: re-read and diff again; any residue is a hard error.
	if err := b.Raw("get_ship", nil); err != nil {
		return err
	}
	var after shipInfo
	if err := json.Unmarshal(b.client.GetRawJSON("ship"), &after); err != nil {
		return err
	}
	now := make([]string, 0, len(after.Ship.Modules))
	for _, m := range after.Ship.Modules {
		now = append(now, m.ItemID)
	}
	if rem, inst := computeFitActions(now, fit.Modules); len(rem)+len(inst) > 0 {
		return fmt.Errorf("%s: fit verify failed: extra=%v missing=%v", b.agentID, rem, inst)
	}
	return nil
}

func (b *Bot) Attack(target string) error {
	return b.Raw("attack", map[string]any{"target": target})
}

func (b *Bot) Jump(system string) error {
	if err := b.Raw("jump", map[string]any{"system": system}); err != nil {
		return err
	}
	time.Sleep(game.SleepJump)
	return nil
}

func (b *Bot) Dock(poi string) error {
	if err := b.Raw("dock", map[string]any{"poi_id": poi}); err != nil {
		return err
	}
	time.Sleep(game.SleepDock)
	return nil
}

func (b *Bot) Undock() error { return b.Raw("undock", nil) }
```

Note for the implementer: the exact field names in `shipInfo` and the
`jump`/`dock`/`buy` payload keys MUST be verified against
`pkg/game/serverapi/responses.go` and `server_docs/api.md` before S0 —
the project rule is "check actual field names, do NOT assume". If they
differ, fix them here and in this plan's record; the structure stands.
Likewise `b.client.State()` — if the accessor is named differently
(e.g. `GetState()` or `State` field with `Clone()`), use the real one;
the BattleView mapping stands.

- [ ] **Step 4: Implement main.go**

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/rsned/spacemolt/pkg/game"
)

func main() {
	campaignPath := flag.String("campaign", "", "campaign JSON (required)")
	manifestPath := flag.String("manifest", "", "manifest JSONL to append to (required)")
	agentA := flag.String("a", "battle_bot1", "side A agent id")
	agentB := flag.String("b", "battle_bot2", "side B agent id (per-duel guest overrides)")
	only := flag.String("only", "", "comma-separated scenario ids to run (default: all)")
	dryRun := flag.Bool("dry-run", false, "print the run list and exit without logging in")
	flag.Parse()
	logger := log.New(os.Stderr, "[duel-runner] ", log.LstdFlags)
	if *campaignPath == "" || *manifestPath == "" {
		fmt.Fprintln(os.Stderr, "usage: duel-runner --campaign c.json --manifest m.jsonl [--only S0]")
		os.Exit(2)
	}
	camp, err := LoadCampaign(*campaignPath)
	if err != nil {
		logger.Fatal(err)
	}
	done, err := LoadDone(*manifestPath)
	if err != nil {
		logger.Fatal(err)
	}
	filter := map[string]bool{}
	for _, id := range strings.Split(*only, ",") {
		if id != "" {
			filter[id] = true
		}
	}
	type run struct {
		duel   Duel
		repeat int
	}
	var runs []run
	for _, d := range camp.Duels {
		if len(filter) > 0 && !filter[d.ID] {
			continue
		}
		for r := 1; r <= d.Repeats; r++ {
			if !done[DoneKey(d.ID, r)] {
				runs = append(runs, run{d, r})
			}
		}
	}
	logger.Printf("%d runs pending", len(runs))
	if *dryRun {
		for _, r := range runs {
			logger.Printf("  %s repeat %d (%s)", r.duel.ID, r.repeat, r.duel.Purpose)
		}
		return
	}

	ctx := context.Background()
	botA, err := Login(ctx, *agentA, logger)
	if err != nil {
		logger.Fatal(err)
	}
	defer botA.Close()
	time.Sleep(game.SleepMedium + game.SleepShort) // 36s gap before second login
	botB, err := Login(ctx, *agentB, logger)
	if err != nil {
		logger.Fatal(err)
	}
	defer botB.Close()

	guests := map[string]*Bot{}
	for _, r := range runs {
		bSide := botB
		if r.duel.Guest != "" {
			g, ok := guests[r.duel.Guest]
			if !ok {
				time.Sleep(game.SleepMedium + game.SleepShort)
				g, err = Login(ctx, r.duel.Guest, logger)
				if err != nil {
					logger.Fatal(err)
				}
				defer g.Close()
				guests[r.duel.Guest] = g
			}
			bSide = g
		}
		logger.Printf("=== %s repeat %d: %s", r.duel.ID, r.repeat, r.duel.Purpose)
		rec, err := executeDuel(camp, botA, bSide, r.duel, r.repeat, logger)
		if err != nil {
			logger.Fatalf("%s repeat %d: %v (manifest is consistent; re-run to resume)", r.duel.ID, r.repeat, err)
		}
		if err := AppendRecord(*manifestPath, rec); err != nil {
			logger.Fatal(err)
		}
		logger.Printf("=== %s repeat %d: %s (battle %s)%s", r.duel.ID, r.repeat, rec.Outcome, rec.BattleID,
			map[bool]string{true: " VOID", false: ""}[rec.Void])
	}
	logger.Printf("campaign section complete")
}

// executeDuel runs preflight, the battle, and recovery for one repeat.
func executeDuel(camp *Campaign, a, b *Bot, d Duel, repeat int, logger *log.Logger) (Record, error) {
	rec := Record{ScenarioID: d.ID, Repeat: repeat, Started: time.Now().UTC()}
	// Preflight: both at staging, correct fits, then into the arena.
	for _, bot := range []*Bot{a, b} {
		fit := d.FitA
		if bot != a {
			fit = d.FitB
		}
		if err := bot.EnsureFit(fit); err != nil {
			return rec, err
		}
	}
	for _, bot := range []*Bot{a, b} {
		if err := bot.Undock(); err != nil {
			logger.Printf("%s undock: %v (may already be in space)", bot.Name(), err)
		}
		if err := bot.Jump(camp.ArenaSystem); err != nil {
			return rec, err
		}
	}
	attacker, defender := a, b
	if d.Attacker == b.Name() {
		attacker, defender = b, a
	}
	if err := attacker.Attack(defender.Name()); err != nil {
		return rec, err
	}
	res, err := runDuel(a, b, d, func() { time.Sleep(game.SleepQuick) }, logger)
	if err != nil {
		return rec, err
	}
	rec.BattleID, rec.Outcome, rec.Void, rec.Ended = res.BattleID, res.Outcome, res.Void, time.Now().UTC()
	// Recovery: both bots return to staging and dock (a destroyed bot has
	// already respawned there with a free starter hull).
	for _, bot := range []*Bot{a, b} {
		if err := bot.Jump(camp.StagingSystem); err != nil {
			logger.Printf("%s return jump: %v (bot may have respawned at staging already)", bot.Name(), err)
		}
		if err := bot.Dock(camp.StagingStation); err != nil {
			logger.Printf("%s dock: %v", bot.Name(), err)
		}
	}
	return rec, nil
}
```

- [ ] **Step 5: Build, lint, run help + dry-run against a scratch campaign**

```bash
go build -o bin/duel-runner ./cmd/tools/duel-runner
golangci-lint run ./cmd/tools/duel-runner/
bin/duel-runner --campaign /tmp/c.json --manifest /tmp/m.jsonl --dry-run
```
Expected: dry-run lists pending duels, no login occurs.

- [ ] **Step 6: Commit** (`feat(duel-runner): live session adapter, logistics, campaign main`).

---

### Task 5: Campaign data + duels README (KB repo)

**Files:**
- Create: `data/battles/duels/campaign.json`
- Create: `data/battles/duels/README.md`

**Interfaces:** consumes the schema from Task 1 exactly.

- [ ] **Step 1: Record the arena.** OWNER-DECIDED: arena = `ashford`
(lawless, connected to `treasure_cache`); staging system =
`treasure_cache`, staging station = its station poi (read the exact poi
id from the KB systems page or `get_system`). As a sanity check, count
ashford's battle volume in the cached bulk-feed shards (one grep) and
note it in `data/battles/duels/README.md` — if it turns out to be a
busy pirate pocket, flag it to the owner before S0 rather than switching
unilaterally.

- [ ] **Step 2: Write campaign.json** — the full matrix, verbatim from the spec's scenario section. Fits (hulls: `prospect` free; `lawn_dart` fast(6); `bad_idea` 3-weapon attacker for S7; `dualism` 3-defense-slot ladder target), weapons `pulse_laser_i` / `autocannon_i` / `missile_launcher_i`, plates `armor_plate_i`. Campaign geography: `"arena_system": "ashford"`, `"staging_system": "treasure_cache"`, `"staging_station"` = the station poi id recorded in Step 1. The duels, in order: `S0-probe` (1 repeat, 5 ticks then mutual flee), `S1-ring0/1/2/3` (+`S1-odd` one-side-hold), `S2-fast-attacker`/`S2-fast-target` at rings 0 and 2, `S3-evade` (2), `S4-brace` (2), `S5-regen-zero` (2), `S6a-flee-base`, `S6b-flee-reset` (script: flee from 1, fire at 3, flee at 4), `S6c-flee-guest` (guest craftsman-1), `S6d-flee-fast`/`S6d-flee-slow`, `S6e-flee-from-engaged`, `S7-armor-N` for each plate count 0..3 on `dualism` (armor 7/12/17/22) plus `S7-prospect` (armor 4/9) and one `S7-kinetic-check`. Every duel: explicit `fit_a`/`fit_b`, script phases with `hold_ring` where the spec pins one, `max_ticks`, `repeats` per the spec's duel counts.
- [ ] **Step 3: Validate** — `cd /home/robert/spacemolt/spacemolt && go run ./cmd/tools/duel-runner --campaign <kb>/data/battles/duels/campaign.json --manifest /tmp/x.jsonl --dry-run` lists every (duel, repeat) with no validation errors.
- [ ] **Step 4: Write README.md** — arena decision, shopping list with prices and per-scenario allocation, funding checklist (which donor agent, transfer method, `set_home_base` step for both bots at staging), the S0 gate, and a run-log table to fill during Task 6.
- [ ] **Step 5: Commit** (`feat(duels): Phase B campaign data and runbook`).

---

### Task 6: Campaign execution (CONTROLLER-RUN, live game — never a subagent)

**Files:** `data/battles/duels/manifest.jsonl` + exported fixtures (created by running).

Sequence, each step gated on the previous:

- [ ] **Step 1: Setup.** Owner confirms bots are pre-positioned at staging and funded; both bots `set_home_base` to the staging station; `get_skills` on both bots recorded into the README (expected all-zero — any nonzero value is recorded and carried into analysis, not a blocker).
- [ ] **Step 2: S0 probe.** `bin/duel-runner --campaign … --manifest … --only S0-probe`. Then STOP: export the S0 battle, read it end-to-end (attack accepted between own accounts? rep/police events? free actions applied on the expected tick? manifest row correct?). Report findings to the owner before continuing. If S0 shows rep/police damage, halt Phase B and reassess.
- [ ] **Step 3: Free-hull scenarios.** `--only` batches in spec order: S1, S3, S4, S5, S6a/b/e. Watch the first duel of each batch live; the rest run unattended (resumable).
- [ ] **Step 4: Bought-hull scenarios.** Buy `lawn_dart`, `bad_idea`, `dualism` + plates per shopping list (owner watches the spend); run S2, S6d, S7. Hull swaps between scenarios via `buy_ship`/`switch_ship` at staging, recorded in the README run log.
- [ ] **Step 5: Guest scenario.** Check `ps aux | grep bin/worker` for craftsman-1; run S6c; craftsman-1 logs out (session closes with the runner).
- [ ] **Step 6: Export + commit.** Collect battle ids: `jq -r 'select(.void|not) | .battle_id' manifest.jsonl | paste -sd,`. One craftsman-boss batch: `bin/battle-export --agent craftsman-boss --battle "<ids>" --out-dir <kb>/data/battles/duels`. Commit manifest + fixtures + README run log in the KB repo.

---

### Task 7: Analysis scripts (KB repo)

**Files:**
- Create: `data/battles/analysis/phaseb_hit_table.py`
- Create: `data/battles/analysis/phaseb_stances.py`
- Create: `data/battles/analysis/phaseb_flee.py`
- Create: `data/battles/analysis/phaseb_armor.py`

Each script: stdlib-only, reads `data/battles/duels/*.raw.json`, prints every raw observation next to the fitted value. Skeleton shared by all four (repeat it in each file; no shared module for four small scripts):

```python
#!/usr/bin/env python3
"""<per-script docstring: what it measures, which scenarios feed it>"""
import glob, json, collections, sys

def entries(dirpath="data/battles/duels"):
    for f in sorted(glob.glob(f"{dirpath}/*.raw.json")):
        for page in json.load(open(f)):
            for e in page.get("entries") or []:
                yield f.rsplit("/", 1)[1][:8], e
```

- [ ] **Step 1: phaseb_hit_table.py** — group every attack's `hit_chance` by `zone_distance` and by (attacker ship speed pair, from snapshots); print, per distance: n, distinct values, min/max. Second table: S2 duels only, hit_chance delta vs the same-distance S1 value per speed difference. Output ends with a proposed `hit_chance_by_distance` array and the residuals of a `base[d] + k*speed_diff` fit (least squares over the distinct values; `k` printed with its sign convention).
- [ ] **Step 2: phaseb_stances.py** — three sections. Evade (S3 fixtures): attacker hit_chance vs the S1 ring-0 value (delta column), landed pre→net damage ratio on the evader, evader fuel per tick from snapshots, count of evader attacks (must be 0). Brace (S4): per-tick shield delta and `regen[]` rows for the braced side; print observed regen vs `recharge`, `2*recharge`, `floor(2*recharge/3)`, `2*floor(recharge/3)` so the multiplier and truncation order are read off the table. Regen-zero (S5): the braced-attacker timeline of the target's shield from the tick it hits 0.
- [ ] **Step 3: phaseb_flee.py** — every `flee[]` event (counter, required, escaped) with the fleeing pilot, their hull's base_speed (from the sim's vendored catalog), Tactics level (S6c: craftsman-1's known sheet; bots: 0), and the same-tick zone. Prints one row per event plus a summary: required-vs-(speed diff, tactics) table — this is the table that resolves the doc-vs-bot contradiction.
- [ ] **Step 4: phaseb_armor.py** — S7 fixtures: for every hull-landing volley, print armor_total (from the fit's plates + hull base, hardcoded per scenario id in a dict at the top of the script), pre_hit, observed hull damage, and the predictions of `pre - floor(0.75*armor*0.75)`-style flat law and `pre*(1-a_c/(a_c+150))` with `a_c = 0.75*armor` (energy) or `1.5*armor` (kinetic check), all floored, min-1 applied. Ends with per-law exact-match counts per armor step.
- [ ] **Step 5: Run all four** against the Task 6 fixtures; iterate until each prints coherent tables. Commit (`feat(analysis): Phase B measurement scripts`).

---

### Task 8: Calibration, golden duel tests, docs (KB repo)

**Files:**
- Modify: `data/combat-sim/calibration.json`
- Create: `cmd/combat-sim/golden_duel_test.go`
- Modify: `cmd/combat-sim/README.md` (Measured vs ASSUMED), spec errata section in `docs/superpowers/specs/2026-09-01-phase-b-calibration-duels-design.md`, memory file.

- [ ] **Step 1: Update calibration.json** from the analysis outputs: set `hit_chance_a/b` to the measured ring-0 value; add `"hit_chance_by_distance": [d0..d6]` (measured; documented in `_comment` as v2-zones input, unused by the v1 engine); replace every doc-backed value that measurement refined (evade debuff, brace regen mult, flee_ticks_required) with the measured number; shrink or empty `assumed` accordingly, with the deferral reason for anything left.
- [ ] **Step 2: Golden duel tests.** Copy the two chosen fixtures (one flee duel, one brace duel) into `data/combat-sim/golden-duels/`. Write `golden_duel_test.go` as a data-driven harness:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Replays every fixture in data/combat-sim/golden-duels/: for each raw
// battle log, walk the entries and assert the engine's stance rules hold —
// non-fire stances never attack, flee events escape exactly when counter
// reaches required, and the braced side's logged shield regen matches the
// calibrated brace multiplier arithmetic.
func TestGoldenDuels(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("..", "..", "data", "combat-sim", "golden-duels", "*.raw.json"))
	if len(files) == 0 {
		t.Fatal("no golden duel fixtures committed")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var pages []struct {
				Entries []struct {
					Tick      int `json:"tick"`
					Snapshots []struct {
						PlayerID string `json:"player_id"`
						Stance   string `json:"stance"`
						Shield   int    `json:"shield"`
					} `json:"snapshots"`
					Attacks []struct {
						AttackerID string `json:"attacker_id"`
					} `json:"attacks"`
					Flee []struct {
						PlayerID    string `json:"player_id"`
						FleeCounter int    `json:"flee_counter"`
						FleeRequired int   `json:"flee_required"`
						Escaped     bool  `json:"escaped"`
					} `json:"flee"`
				} `json:"entries"`
			}
			if err := json.Unmarshal(raw, &pages); err != nil {
				t.Fatal(err)
			}
			for _, page := range pages {
				for _, e := range page.Entries {
					stance := map[string]string{}
					for _, s := range e.Snapshots {
						stance[s.PlayerID] = s.Stance
					}
					for _, a := range e.Attacks {
						if st := stance[a.AttackerID]; st != "" && st != "fire" {
							t.Errorf("tick %d: %s attacked while stance %q", e.Tick, a.AttackerID, st)
						}
					}
					for _, fl := range e.Flee {
						if fl.Escaped && fl.FleeCounter < fl.FleeRequired {
							t.Errorf("tick %d: escaped at counter %d < required %d", e.Tick, fl.FleeCounter, fl.FleeRequired)
						}
					}
				}
			}
		})
	}
}
```

Extend it (same pattern, exact assertions from the analysis numbers) with the brace-regen arithmetic check once phaseb_stances.py has pinned the truncation order.
- [ ] **Step 3: Run the full combat-sim suite** (`go test ./cmd/combat-sim/`), regenerate the README example table if any calibrated number moved it, update the Measured-vs-ASSUMED section, append a dated errata block to the Phase B spec with every measured value, and update the combat-mechanics memory file.
- [ ] **Step 4: Commit + push** everything from Tasks 6–8 (`feat(combat-sim): Phase B measured calibration, duel fixtures, golden duel tests`).
