# Tectonics Rifting (Plate Birth) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Large, long-intact plates split along their thinnest interior crust into two plates that drift apart, so plate counts oscillate instead of decaying.

**Architecture:** A new `rift.go` in `pkg/tectonics` adds `(*State).Rift`: eligibility by seeded share and rest period, a Dijkstra thinnest path across the plate between two boundary pixels, a connected-component split, ridge crust on the line, and a new plate + motion appended to the tables. `Run` calls it once per step after `Step`. `Plate` gains `Born`/`LastRift`, `PlateRow` gains `born`; nothing else in the bundle contract changes.

**Tech Stack:** Go 1.24+ stdlib (`container/heap`, `math/rand/v2`), existing `pkg/tectonics` helpers; Python stats script gains one line.

**Spec:** `docs/superpowers/specs/2026-10-03-tectonics-rifting-design.md`

## Global Constraints

- Worktree `/home/robert/spacemolt/kb-tectonics`, branch `tectonics/checkpoint-1`. Only `pkg/tectonics/`, `cmd/tectonics-lab/golden_test.go` + its golden, and `scripts/tectonics_stats.py` change.
- Knob defaults exactly: `RiftMinShareMin 0.15`, `RiftMinShareMax 0.25`, `RiftRestMyrMin 100`, `RiftRestMyrMax 200`, `RiftChancePerMyr 0.02`, `RiftMinChildShare 0.03`, `RiftThinPower 3`, `RiftMaxPlates 200`.
- Seeds: per-planet share/rest via `newRNG(master, "rift.params")`; per-step draws, start pixel and child speed via one `newRNG(master, "sim.rift")` stream created in `Run`. Same seed ⇒ byte-identical bundle.
- At most one plate rifts per step: the lowest eligible id whose draw passes.
- The rift line gets `RidgeThickness ± RidgeJitter`, age 0, `FeatDivergent`, feature age 0. The largest component keeps the parent id; all other components form the child; path pixels join the nearest labelled side by BFS. A child under `RiftMinChildShare` cancels the rift with no state change and no `LastRift` update.
- Child: new id = `len(Plates)`, `Major=false`, `Born=StepNo`, `LastRift=StepNo`, pole from `pushDirection` + jitter, speed from `drawSpeed`. Parent: `LastRift=StepNo`, re-aimed with the `ReaimFresh` blend.
- Bundle contract: `PlateRow` gains `born` (JSON `born`); everything else unchanged.
- Golden recipe becomes `terran/2026/64/20/v6`.
- gofmt + `golangci-lint run ./pkg/tectonics/... ./cmd/tectonics-lab/...` clean after every task; `go test ./pkg/tectonics/ ./cmd/tectonics-lab/` green before every commit. Commit trailer `Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>`; `git add` named paths only.

## File Structure

| File | Change |
| --- | --- |
| `pkg/tectonics/params.go` | eight `Rift*` knobs + defaults |
| `pkg/tectonics/watershed.go` | `Plate.Born`, `Plate.LastRift` |
| `pkg/tectonics/sim.go` | `PlateRow.Born`, `Snapshot` copies it, `Validate` rift rules, `Run` wiring, `riftParams` draw |
| `pkg/tectonics/step.go` | `retire` carries `Born`/`LastRift`; `carryPlateFlags` helper shared with rift |
| `pkg/tectonics/motion.go` | `reaimPlate` extracted from `Reaim` for reuse |
| `pkg/tectonics/rift.go` (new) | eligibility, boundary pixels, Dijkstra path, split, `Rift` |
| `pkg/tectonics/rift_test.go` (new) | spec tests |
| `cmd/tectonics-lab/golden_test.go` + `testdata/golden_face64.json` | v6 rebake |
| `scripts/tectonics_stats.py` | `plate births` line |

---

### Task 1: Knobs, plate fields and flag carrying

**Files:**
- Modify: `pkg/tectonics/params.go`, `pkg/tectonics/watershed.go` (Plate struct), `pkg/tectonics/sim.go` (PlateRow, Snapshot, Validate), `pkg/tectonics/step.go` (retire)
- Test: `pkg/tectonics/params_test.go`, `pkg/tectonics/sim_test.go`, `pkg/tectonics/step_test.go`

**Interfaces:**
- Produces: `Params.{RiftMinShareMin, RiftMinShareMax, RiftRestMyrMin, RiftRestMyrMax, RiftChancePerMyr, RiftMinChildShare, RiftThinPower float64; RiftMaxPlates int}`; `Plate.Born int`, `Plate.LastRift int`; `PlateRow.Born int` (`json:"born"`); `(s *State) carryPlateFlags(stats []Plate)` copies `Retired` (OR), `Major`, `Born`, `LastRift` from `s.Plates[i]` into `stats[i]` for `i < len(s.Plates)`.

- [ ] **Step 1: Write the failing tests**

Append to `pkg/tectonics/params_test.go` inside `TestDefaultParamsPerArchetype`, after the tuning-defaults check:

```go
	if p.RiftMinShareMin != 0.15 || p.RiftMinShareMax != 0.25 || p.RiftRestMyrMin != 100 || p.RiftRestMyrMax != 200 ||
		p.RiftChancePerMyr != 0.02 || p.RiftMinChildShare != 0.03 || p.RiftThinPower != 3 || p.RiftMaxPlates != 200 {
		t.Errorf("rift defaults %+v", p)
	}
```

Append to `TestValidate` in `pkg/tectonics/sim_test.go`:

```go
	for _, bad := range []func(*Params){
		func(p *Params) { p.RiftMinShareMax = p.RiftMinShareMin - 0.01 },
		func(p *Params) { p.RiftRestMyrMin = -1 },
		func(p *Params) { p.RiftChancePerMyr = 1.5 },
		func(p *Params) { p.RiftMinChildShare = 0.5 },
		func(p *Params) { p.RiftThinPower = 0 },
		func(p *Params) { p.RiftMaxPlates = 256 },
	} {
		q := testParams(t, 16)
		bad(&q)
		if err := Validate(q); err == nil {
			t.Errorf("Validate accepted bad rift knobs %+v", q)
		}
	}
```

Add to `pkg/tectonics/step_test.go`:

```go
func TestRetireCarriesBornAndLastRift(t *testing.T) {
	s := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	s.Plates[1].Born, s.Plates[1].LastRift = 4, 7
	s.Step(newRNG(1, "t"))
	if s.Plates[1].Born != 4 || s.Plates[1].LastRift != 7 {
		t.Errorf("plate 1 after step: born %d lastRift %d, want 4 7", s.Plates[1].Born, s.Plates[1].LastRift)
	}
	rows := s.Snapshot(1).Plates
	if rows[1].Born != 4 {
		t.Errorf("PlateRow.Born = %d, want 4", rows[1].Born)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd /home/robert/spacemolt/kb-tectonics && go test ./pkg/tectonics/ -run 'TestDefaultParams|TestValidate|TestRetireCarries' -v`
Expected: compile errors, `p.RiftMinShareMin undefined` and `s.Plates[1].Born undefined`.

- [ ] **Step 3: Add the knobs**

In `pkg/tectonics/params.go`, after the `FaultScar float64` field inside `Params`:

```go

	// Rifting (plate birth), see docs/superpowers/specs/2026-10-03-tectonics-rifting-design.md
	RiftMinShareMin, RiftMinShareMax float64 // seeded share of the sphere a plate must exceed
	RiftRestMyrMin, RiftRestMyrMax   float64 // seeded Myr without a split before eligibility
	RiftChancePerMyr                 float64 // per-Myr probability an eligible plate rifts
	RiftMinChildShare                float64 // smaller half below this cancels the split
	RiftThinPower                    float64 // exponent on thickness in the path cost
	RiftMaxPlates                    int     // no rifts once this many plate ids exist (byte-sized ids)
```

In `DefaultParams`, after the `CollisionUplift: ...` line of the literal:

```go
		RiftMinShareMin: 0.15, RiftMinShareMax: 0.25, RiftRestMyrMin: 100, RiftRestMyrMax: 200,
		RiftChancePerMyr: 0.02, RiftMinChildShare: 0.03, RiftThinPower: 3, RiftMaxPlates: 200,
```

- [ ] **Step 4: Plate and PlateRow fields**

`pkg/tectonics/watershed.go`, `Plate` struct, after `Retired bool`:

```go
	Born     int // step the plate was created (0 for the initial set)
	LastRift int // step of its last split (0 if never)
```

`pkg/tectonics/sim.go`, `PlateRow`, after `Retired bool \`json:"retired"\``:

```go
	Born      int        `json:"born"`
```

and in `Snapshot`, add `Born: pl.Born` to the `PlateRow{...}` literal.

In `Validate`, before the final `}` of the switch:

```go
	case p.RiftMinShareMin <= 0 || p.RiftMinShareMax < p.RiftMinShareMin || p.RiftMinShareMax > 0.9:
		return fmt.Errorf("rift share range %g-%g invalid", p.RiftMinShareMin, p.RiftMinShareMax)
	case p.RiftRestMyrMin < 0 || p.RiftRestMyrMax < p.RiftRestMyrMin:
		return fmt.Errorf("rift rest range %g-%g invalid", p.RiftRestMyrMin, p.RiftRestMyrMax)
	case p.RiftChancePerMyr < 0 || p.RiftChancePerMyr > 1:
		return fmt.Errorf("RiftChancePerMyr %g outside 0..1", p.RiftChancePerMyr)
	case p.RiftMinChildShare <= 0 || p.RiftMinChildShare >= 0.5:
		return fmt.Errorf("RiftMinChildShare %g outside (0, 0.5)", p.RiftMinChildShare)
	case p.RiftThinPower <= 0:
		return fmt.Errorf("RiftThinPower %g must be positive", p.RiftThinPower)
	case p.RiftMaxPlates < 2 || p.RiftMaxPlates > 255:
		return fmt.Errorf("RiftMaxPlates %d outside 2..255", p.RiftMaxPlates)
```

- [ ] **Step 5: carryPlateFlags in step.go**

Replace both flag-copy loops in `retire` (the one at the top and the one after `PlateStats` inside the loop) with calls to a new helper, so `retire` reads:

```go
// carryPlateFlags copies the per-plate bookkeeping that PlateStats cannot
// know (Retired, Major, Born, LastRift) from s.Plates onto freshly computed
// stats. Plates appended after s.Plates (a rift child) are left as given.
func (s *State) carryPlateFlags(stats []Plate) {
	for i := range stats {
		if i >= len(s.Plates) {
			continue
		}
		stats[i].Retired = stats[i].Retired || s.Plates[i].Retired
		stats[i].Major = s.Plates[i].Major
		stats[i].Born = s.Plates[i].Born
		stats[i].LastRift = s.Plates[i].LastRift
	}
}

// retire folds plates under MinPlateArea into their most-shared neighbour.
func (s *State) retire(stats []Plate) []Plate {
	s.carryPlateFlags(stats)
	nb := Neighbors4(s.Th.S)
	for i := range stats {
		if stats[i].Retired || stats[i].Area >= s.P.MinPlateArea {
			continue
		}
		shared := map[int32]int{}
		for j, l := range s.Labels.Cells {
			if l != int32(i) {
				continue
			}
			for _, k := range nb[j] {
				if m := s.Labels.Cells[k]; m != int32(i) && !stats[m].Retired {
					shared[m]++
				}
			}
		}
		best, bestN := int32(-1), -1
		for m, c := range shared {
			if c > bestN || (c == bestN && m < best) {
				best, bestN = m, c
			}
		}
		if best < 0 {
			continue
		}
		for j, l := range s.Labels.Cells {
			if l == int32(i) {
				s.Labels.Cells[j] = best
			}
		}
		stats = PlateStats(s.Th, s.Labels, len(stats))
		s.carryPlateFlags(stats)
		stats[i].Retired = true
	}
	return stats
}
```

- [ ] **Step 6: Run the package tests and lint, expect PASS**

Run: `gofmt -l pkg/tectonics && go build ./... && go test ./pkg/tectonics/ -count=1 && golangci-lint run ./pkg/tectonics/...`

The cmd golden test will now FAIL (plate rows carry `born`, which is hashed only from Task 3 on; the frames themselves are unchanged, so it should still pass — if it fails, note it and continue; Task 3 rebakes).

- [ ] **Step 7: Commit**

```bash
git add pkg/tectonics/params.go pkg/tectonics/params_test.go pkg/tectonics/watershed.go pkg/tectonics/sim.go pkg/tectonics/sim_test.go pkg/tectonics/step.go pkg/tectonics/step_test.go
git commit -m "feat(tectonics): rift knobs, plate Born/LastRift, flag carrying" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---
### Task 2: The rift itself

**Files:**
- Create: `pkg/tectonics/rift.go`
- Modify: `pkg/tectonics/motion.go` (extract `reaimPlate` from `Reaim`)
- Test: `pkg/tectonics/rift_test.go`

**Interfaces:**
- Consumes: `State`, `Grid`, `Neighbors4`, `Dirs`, `PlateStats`, `carryPlateFlags`, `pushDirection`, `poleFor`, `rotate`, `drawSpeed`, `DegPerStep`, `velocityAt`, `FeatDivergent`, `Params` rift knobs, `twoHemispheres` (motion_test.go) and `testParams` (thickness_test.go).
- Produces: `type riftParams struct{ share, rest float64 }`; `func reaimPlate(th *Grid[float64], labels *Grid[int32], pl Plate, m *Motion, p Params, rng *rand.Rand)` (the body of today's `Reaim` loop; `Reaim` becomes a loop over it); `(s *State) boundaryPixels(id int32) []int32`; `(s *State) riftPath(id int32, start, end int32) []int32` (Dijkstra; nil if unreachable); `(s *State) riftPlate(id int32, start int32, rng *rand.Rand) bool` (split at a given start pixel; false = cancelled); `(s *State) Rift(rng *rand.Rand, rp riftParams) bool` (eligibility + draw + `riftPlate`).

- [ ] **Step 1: Write the failing tests**

```go
// pkg/tectonics/rift_test.go
package tectonics

import (
	"math"
	"testing"
)

// riftWorld is two hemispheres (plate 0 = z>=0, plate 1 = z<0) with a thin
// trough along the x=0 great circle so plate 0's cheapest interior path runs
// from the equator over the pole to the far equator.
func riftWorld(t *testing.T, S int) *State {
	t.Helper()
	th, labels, plates := twoHemispheres(S, 0.8, 0.8)
	for i, d := range Dirs(S) {
		if math.Abs(d[0]) < 0.08 {
			th.Cells[i] = 0.1
		}
	}
	p := testParams(t, S)
	p.RelaxIters = 0
	ms := InitMotion(th, labels, plates, p, 3)
	return NewState(p, th, labels, plates, ms)
}

// boundary pixel of plate 0 nearest the +y direction (on the equator, x≈0)
func startNearPlusY(s *State) int32 {
	dirs := Dirs(s.Th.S)
	best, bestDot := int32(-1), -2.0
	for _, i := range s.boundaryPixels(0) {
		if d := dirs[i][1]; d > bestDot {
			best, bestDot = i, d
		}
	}
	return best
}

func TestRiftSplitsAlongThinTrough(t *testing.T) {
	s := riftWorld(t, 32)
	rng := newRNG(9, "rift")
	if !s.riftPlate(0, startNearPlusY(s), rng) {
		t.Fatal("rift cancelled")
	}
	if len(s.Plates) != 3 || len(s.Motions) != 3 {
		t.Fatalf("plates %d motions %d, want 3 and 3", len(s.Plates), len(s.Motions))
	}
	dirs := Dirs(32)
	var pos, neg, parent, child int
	ridge := 0
	for i, l := range s.Labels.Cells {
		switch l {
		case 0:
			parent++
			if dirs[i][0] > 0 {
				pos++
			}
		case 2:
			child++
			if dirs[i][0] < 0 {
				neg++
			}
		}
		if s.FeatType.Cells[i] == FeatDivergent {
			ridge++
			if s.Th.Cells[i] > s.P.RidgeThickness+s.P.RidgeJitter+1e-9 || s.Age.Cells[i] != 0 || s.FeatAge.Cells[i] != 0 {
				t.Fatalf("ridge pixel %d: th %g age %d featAge %d", i, s.Th.Cells[i], s.Age.Cells[i], s.FeatAge.Cells[i])
			}
		}
	}
	if parent == 0 || child == 0 || ridge == 0 {
		t.Fatalf("parent %d child %d ridge %d", parent, child, ridge)
	}
	// the halves sit on opposite signs of x; either side may be the parent
	sameSide := float64(pos)/float64(parent) >= 0.95 && float64(neg)/float64(child) >= 0.95
	otherSide := float64(parent-pos)/float64(parent) >= 0.95 && float64(child-neg)/float64(child) >= 0.95
	if !sameSide && !otherSide {
		t.Errorf("halves not split by x=0: parent +x %d/%d, child -x %d/%d", pos, parent, neg, child)
	}
	c := s.Plates[2]
	if c.Born != s.StepNo || c.LastRift != s.StepNo || c.Major || c.Retired || c.ID != 2 {
		t.Errorf("child plate %+v", c)
	}
	if s.Plates[0].LastRift != s.StepNo {
		t.Errorf("parent LastRift %d, want %d", s.Plates[0].LastRift, s.StepNo)
	}
	away := add(c.Centroid, scale(s.Plates[0].Centroid, -1))
	if dot(velocityAt(s.Motions[2], c.Centroid), away) <= 0 {
		t.Error("child does not move away from the parent")
	}
	if dot(velocityAt(s.Motions[0], s.Plates[0].Centroid), away) >= 0 {
		t.Error("parent does not move away from the child")
	}
	total := 0.0
	for _, pl := range s.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
}

func TestRiftEligibilityAndCancel(t *testing.T) {
	s := riftWorld(t, 24)
	s.P.RiftChancePerMyr = 1
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.6, rest: 0}) {
		t.Error("plate below the share threshold rifted")
	}
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 1000}) {
		t.Error("plate inside its rest period rifted")
	}
	s.P.RiftMinChildShare = 0.45 // impossible for a 0.5-share parent
	before := s.Labels.Clone()
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 0}) {
		t.Error("rift with an undersized child was not cancelled")
	}
	for i := range before.Cells {
		if before.Cells[i] != s.Labels.Cells[i] {
			t.Fatal("cancelled rift changed labels")
		}
	}
	if len(s.Plates) != 2 || s.Plates[0].LastRift != 0 {
		t.Errorf("cancelled rift changed tables: %d plates, LastRift %d", len(s.Plates), s.Plates[0].LastRift)
	}
	s.P.RiftMinChildShare = 0.03
	if !s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 0}) {
		t.Error("eligible plate did not rift")
	}
	if len(s.Plates) != 3 {
		t.Errorf("%d plates after rift", len(s.Plates))
	}
	s.P.RiftMaxPlates = 3
	if s.Rift(newRNG(2, "r"), riftParams{share: 0.1, rest: 0}) {
		t.Error("rifted past RiftMaxPlates")
	}
}

func TestRiftKeepsTablesParallelAndDeterministic(t *testing.T) {
	run := func() *State {
		s := riftWorld(t, 16)
		s.P.RiftChancePerMyr = 1
		s.P.RiftMinChildShare = 0.02
		step, rift := newRNG(5, "sim.step"), newRNG(5, "sim.rift")
		for range 12 {
			s.Step(step)
			s.Rift(rift, riftParams{share: 0.08, rest: 0})
		}
		return s
	}
	a, b := run(), run()
	if len(a.Plates) < 3 {
		t.Fatalf("expected several rifts, got %d plates", len(a.Plates))
	}
	if len(a.Plates) != len(a.Motions) || len(a.Plates) != len(b.Plates) {
		t.Fatalf("tables: %d plates %d motions, other run %d", len(a.Plates), len(a.Motions), len(b.Plates))
	}
	total := 0.0
	for _, pl := range a.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
	for i := range a.Labels.Cells {
		if a.Labels.Cells[i] < 0 || int(a.Labels.Cells[i]) >= len(a.Plates) {
			t.Fatalf("label %d out of range at %d", a.Labels.Cells[i], i)
		}
		if a.Labels.Cells[i] != b.Labels.Cells[i] || a.Th.Cells[i] != b.Th.Cells[i] {
			t.Fatalf("runs differ at pixel %d", i)
		}
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/tectonics/ -run TestRift -v`
Expected: compile error, `s.boundaryPixels undefined` / `undefined: riftParams`.

- [ ] **Step 3: Extract reaimPlate in motion.go**

Replace `Reaim` with:

```go
// reaimPlate blends one plate's current push direction with a fresh one from
// its present thinnest boundary (weight p.ReaimFresh), so locked plates turn
// away and a freshly rifted plate pushes off its new ridge.
func reaimPlate(th *Grid[float64], labels *Grid[int32], pl Plate, m *Motion, p Params, rng *rand.Rand) {
	fresh := pushDirection(th, labels, pl.ID, pl.Centroid)
	if dot(fresh, fresh) == 0 {
		return
	}
	old := unit(tangent(velocityAt(*m, pl.Centroid), pl.Centroid))
	sum := add(scale(old, 1-p.ReaimFresh), scale(fresh, p.ReaimFresh))
	blend := fresh // old and fresh cancel: unit() would not return zero, so test the raw sum
	if dot(sum, sum) >= 1e-12 {
		blend = unit(sum)
	}
	blend = rotate(blend, pl.Centroid, (rng.Float64()*2-1)*p.PushJitterDeg*0.5)
	m.Pole = poleFor(pl.Centroid, unit(tangent(blend, pl.Centroid)))
}

// Reaim re-aims every live plate; see reaimPlate.
func Reaim(th *Grid[float64], labels *Grid[int32], plates []Plate, ms []Motion, p Params, rng *rand.Rand) {
	for i, pl := range plates {
		if pl.Retired {
			continue
		}
		reaimPlate(th, labels, pl, &ms[i], p, rng)
	}
}
```

`TestReaimBlendsTowardNewPush` must still pass unchanged.

- [ ] **Step 4: Write rift.go**

```go
package tectonics

import (
	"container/heap"
	"math"
	"math/rand/v2"
)

// riftParams are the per-planet seeded eligibility thresholds: the share of
// the sphere a plate must exceed and the Myr it must go without a split.
type riftParams struct {
	share, rest float64
}

// drawRiftParams seeds the eligibility thresholds for one planet.
func drawRiftParams(p Params, master int64) riftParams {
	rng := newRNG(master, "rift.params")
	return riftParams{
		share: p.RiftMinShareMin + rng.Float64()*(p.RiftMinShareMax-p.RiftMinShareMin),
		rest:  p.RiftRestMyrMin + rng.Float64()*(p.RiftRestMyrMax-p.RiftRestMyrMin),
	}
}

// boundaryPixels lists plate id's pixels that touch another plate, in index order.
func (s *State) boundaryPixels(id int32) []int32 {
	nb := Neighbors4(s.Th.S)
	var out []int32
	for i, l := range s.Labels.Cells {
		if l != id {
			continue
		}
		for _, j := range nb[i] {
			if s.Labels.Cells[j] != id {
				out = append(out, int32(i))
				break
			}
		}
	}
	return out
}

type pathItem struct {
	cost float64
	idx  int32
}

type pathHeap []pathItem

func (h pathHeap) Len() int { return len(h) }
func (h pathHeap) Less(i, j int) bool {
	if h[i].cost != h[j].cost {
		return h[i].cost < h[j].cost
	}
	return h[i].idx < h[j].idx
}
func (h pathHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *pathHeap) Push(x any)  { *h = append(*h, x.(pathItem)) }
func (h *pathHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// riftPath is the cheapest 4-connected path from start to end through plate
// id's pixels, where entering a pixel costs thickness^RiftThinPower (plus a
// tiny constant so the path still prefers short routes through equal crust).
// It returns nil when end is unreachable.
func (s *State) riftPath(id int32, start, end int32) []int32 {
	nb := Neighbors4(s.Th.S)
	N := s.Th.Len()
	dist := make([]float64, N)
	prev := make([]int32, N)
	for i := range dist {
		dist[i] = math.Inf(1)
		prev[i] = -1
	}
	dist[start] = 0
	h := &pathHeap{{0, start}}
	for h.Len() > 0 {
		it := heap.Pop(h).(pathItem)
		if it.cost > dist[it.idx] {
			continue
		}
		if it.idx == end {
			break
		}
		for _, j := range nb[it.idx] {
			if s.Labels.Cells[j] != id {
				continue
			}
			c := it.cost + math.Pow(s.Th.Cells[j], s.P.RiftThinPower) + 1e-6
			if c < dist[j] {
				dist[j], prev[j] = c, it.idx
				heap.Push(h, pathItem{c, j})
			}
		}
	}
	if math.IsInf(dist[end], 1) {
		return nil
	}
	var path []int32
	for i := end; i >= 0; i = prev[i] {
		path = append(path, i)
	}
	return path
}

// riftPlate splits plate id along the thinnest path from the boundary pixel
// start to the boundary pixel farthest from it. The largest remaining
// component keeps id; every other component becomes a new plate. It returns
// false, changing nothing, when the path fails or the child would be smaller
// than RiftMinChildShare.
func (s *State) riftPlate(id int32, start int32, rng *rand.Rand) bool {
	S := s.Th.S
	N := s.Th.Len()
	dirs := Dirs(S)
	nb := Neighbors4(S)
	bounds := s.boundaryPixels(id)
	if len(bounds) < 2 {
		return false
	}
	end, endDot := int32(-1), 2.0
	for _, b := range bounds {
		if d := dot(dirs[start], dirs[b]); d < endDot {
			end, endDot = b, d
		}
	}
	path := s.riftPath(id, start, end)
	if path == nil {
		return false
	}
	onPath := make([]bool, N)
	for _, i := range path {
		onPath[i] = true
	}
	// connected components of the plate minus the path
	comp := make([]int32, N)
	for i := range comp {
		comp[i] = -1
	}
	var sizes []int
	for i := range N {
		if s.Labels.Cells[i] != id || onPath[i] || comp[i] >= 0 {
			continue
		}
		c := int32(len(sizes))
		sizes = append(sizes, 0)
		queue := []int32{int32(i)}
		comp[i] = c
		for len(queue) > 0 {
			u := queue[0]
			queue = queue[1:]
			sizes[c]++
			for _, j := range nb[u] {
				if s.Labels.Cells[j] == id && !onPath[j] && comp[j] < 0 {
					comp[j] = c
					queue = append(queue, j)
				}
			}
		}
	}
	if len(sizes) < 2 {
		return false
	}
	largest := 0
	for c, n := range sizes {
		if n > sizes[largest] {
			largest = c
		}
	}
	childPixels := 0
	for c, n := range sizes {
		if c != largest {
			childPixels += n
		}
	}
	if float64(childPixels)/float64(N) < s.P.RiftMinChildShare {
		return false
	}
	// commit: relabel, then give path pixels to the nearest labelled side
	child := int32(len(s.Plates))
	for i := range N {
		if comp[i] >= 0 && comp[i] != int32(largest) {
			s.Labels.Cells[i] = child
		}
	}
	queue := make([]int32, 0, N)
	for i := range N {
		if s.Labels.Cells[i] == id || s.Labels.Cells[i] == child {
			if !onPath[i] {
				queue = append(queue, int32(i))
			}
		}
	}
	assigned := make([]bool, N)
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		for _, j := range nb[u] {
			if onPath[j] && !assigned[j] {
				assigned[j] = true
				s.Labels.Cells[j] = s.Labels.Cells[u]
				queue = append(queue, j)
			}
		}
	}
	for _, i := range path {
		s.Th.Cells[i] = s.P.RidgeThickness + (rng.Float64()*2-1)*s.P.RidgeJitter
		s.Age.Cells[i] = 0
		s.FeatType.Cells[i] = FeatDivergent
		s.FeatAge.Cells[i] = 0
	}
	// tables
	stats := PlateStats(s.Th, s.Labels, len(s.Plates)+1)
	s.carryPlateFlags(stats)
	stats[child].Born, stats[child].LastRift = s.StepNo, s.StepNo
	stats[id].LastRift = s.StepNo
	s.Plates = stats
	c := s.Plates[child]
	t := pushDirection(s.Th, s.Labels, child, c.Centroid)
	if dot(t, t) == 0 {
		t = unit(tangent([3]float64{0.3, 0.5, 0.8}, c.Centroid))
	}
	t = rotate(t, c.Centroid, (rng.Float64()*2-1)*s.P.PushJitterDeg)
	speed := drawSpeed(rng, s.P)
	s.Motions = append(s.Motions, Motion{Pole: poleFor(c.Centroid, t), SpeedCmYr: speed,
		DegPerStep: DegPerStep(speed, s.P.RadiusKm, s.P.MyrPerStep())})
	reaimPlate(s.Th, s.Labels, s.Plates[id], &s.Motions[id], s.P, rng)
	return true
}

// Rift lets at most one eligible plate split this step: eligible means live,
// above rp.share of the sphere, rp.rest Myr since its last split, and fewer
// than RiftMaxPlates ids in use. Each eligible plate, in id order, rolls
// against RiftChancePerMyr scaled to the step length; the first to pass rifts
// at a seeded boundary pixel.
func (s *State) Rift(rng *rand.Rand, rp riftParams) bool {
	if len(s.Plates) >= s.P.RiftMaxPlates {
		return false
	}
	perStep := 1 - math.Pow(1-s.P.RiftChancePerMyr, s.P.MyrPerStep())
	for k, pl := range s.Plates {
		if pl.Retired || pl.Area < rp.share || float64(s.StepNo-pl.LastRift)*s.P.MyrPerStep() < rp.rest {
			continue
		}
		if rng.Float64() >= perStep {
			continue
		}
		bounds := s.boundaryPixels(int32(k))
		if len(bounds) < 2 {
			continue
		}
		return s.riftPlate(int32(k), bounds[rng.IntN(len(bounds))], rng)
	}
	return false
}
```

- [ ] **Step 5: Run the rift tests, then the package, lint**

Run: `go test ./pkg/tectonics/ -run TestRift -v && go test ./pkg/tectonics/ -count=1 && golangci-lint run ./pkg/tectonics/...`

If `golangci-lint` flags the `x.(pathItem)` type assertion, the existing `floodHeap` in watershed.go uses the same form and passes; keep it. If `TestRiftSplitsAlongThinTrough` fails on the 95% split test, print the path length and the parent/child pixel counts: a path that hugs the boundary instead of crossing means `endDot` picked a neighbour of `start`; the farthest boundary pixel must be the one with the smallest dot product, as written.

- [ ] **Step 6: Commit**

```bash
git add pkg/tectonics/rift.go pkg/tectonics/rift_test.go pkg/tectonics/motion.go
git commit -m "feat(tectonics): rifting — large, long-intact plates split along their thinnest crust" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 3: Wire into Run, golden v6, births in the stats script

**Files:**
- Modify: `pkg/tectonics/sim.go` (Run), `cmd/tectonics-lab/golden_test.go` + `cmd/tectonics-lab/testdata/golden_face64.json`, `scripts/tectonics_stats.py`
- Test: `pkg/tectonics/sim_test.go`

**Interfaces:**
- Consumes: `drawRiftParams`, `(*State).Rift`, `Frame`, `PlateRow.Born`.
- Produces: `Run` calls `s.Rift(riftRNG, rp)` after every `Step`; stats script prints `plate births (ids first seen after frame 0)`.

- [ ] **Step 1: Write the failing test**

Append to `pkg/tectonics/sim_test.go`:

```go
func TestRunRiftsWhenForced(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 20, 20
	p.RiftChancePerMyr, p.RiftMinShareMin, p.RiftMinShareMax = 1, 0.05, 0.05
	p.RiftRestMyrMin, p.RiftRestMyrMax, p.RiftMinChildShare = 0, 0, 0.02
	var last Frame
	if err := Run(p, 77, func(f Frame) error { last = f; return nil }); err != nil {
		t.Fatal(err)
	}
	born := 0
	for _, r := range last.Plates {
		if r.Born > 0 {
			born++
		}
	}
	if born == 0 {
		t.Error("no plates born in a run with forced rifting")
	}
	p.RiftChancePerMyr = 0
	if err := Run(p, 77, func(f Frame) error { last = f; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, r := range last.Plates {
		if r.Born > 0 {
			t.Fatal("a plate was born with RiftChancePerMyr 0")
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./pkg/tectonics/ -run TestRunRifts -v`
Expected: FAIL `no plates born in a run with forced rifting`.

- [ ] **Step 3: Wire Run**

In `Run` (sim.go), after `reaimRNG := newRNG(master, "sim.reaim")`:

```go
	riftRNG := newRNG(master, "sim.rift")
	rp := drawRiftParams(p, master)
```

and inside the step loop, immediately after `s.Step(stepRNG)`:

```go
		s.Rift(riftRNG, rp)
```

- [ ] **Step 4: Rebake the golden as v6**

In `cmd/tectonics-lab/golden_test.go` change the recipe string `terran/2026/64/20/v5` to `terran/2026/64/20/v6`, and add the `Born` field to `frameHash` after the `Retired` byte: `_, _ = h.Write([]byte{byte(pl.Born), byte(pl.Born >> 8)})`. Then:

Run: `go test ./cmd/tectonics-lab/ -run TestGoldenFace64 -update -count=1 -v && go test ./cmd/tectonics-lab/ -count=1`
Expected: "golden rewritten: 21 frames", then PASS on the clean run.

- [ ] **Step 5: Births in the stats script**

In `scripts/tectonics_stats.py`, inside `summary()` after `lifetimes = [...]`, add `births = sum(1 for p in first if first[p] > 0)`, put `"births": births` into `agg`, and add this row to the `lines` list after the lifetime row:

```python
        ("plate births (ids first seen after frame 0)", lambda a: f"{a['births']}"),
```

Update the docstring's metric list with a `births` line. Run: `~/gplates-venv/bin/python scripts/tectonics_stats.py data/tectonics/earth-nnr` and confirm the new row prints.

- [ ] **Step 6: Full gate and commit**

Run: `gofmt -l pkg/tectonics cmd/tectonics-lab && go build ./... && go test ./pkg/tectonics/ ./cmd/tectonics-lab/ -count=1 && golangci-lint run ./pkg/tectonics/... ./cmd/tectonics-lab/...`

```bash
git add pkg/tectonics/sim.go pkg/tectonics/sim_test.go cmd/tectonics-lab/golden_test.go cmd/tectonics-lab/testdata/golden_face64.json scripts/tectonics_stats.py
git commit -m "feat(tectonics): rifting wired into Run; golden v6; births in the stats script" -m "Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

- [ ] **Step 7: Bake and compare (report, do not tune)**

Run: `go build -o bin/tectonics-lab ./cmd/tectonics-lab && ./bin/tectonics-lab run -seed 42 -archetype terran && ~/gplates-venv/bin/python scripts/tectonics_stats.py data/tectonics/earth-nnr data/tectonics/seed-42-terran`

Put the table in the report. The spec's target is counts oscillating in the 12–25 band with the largest plate near 25–35% most of the run; if the first bake is outside that, report it and leave the knobs for the human's tuning pass.

---

## Self-review notes

- Spec coverage: knobs + Validate → Task 1; eligibility, endpoints, path, split, crust, tables, child motion, parent re-aim → Task 2; Run wiring, seeds `rift.params` + `sim.rift`, golden v6, stats births → Task 3; viewer unchanged by decision.
- Type consistency: `riftParams{share, rest}`, `carryPlateFlags`, `reaimPlate`, `boundaryPixels`, `riftPath(id, start, end)`, `riftPlate(id, start, rng)`, `Rift(rng, rp)` are used with the same names and signatures in every task.
- Known judgment calls: path pixels join the nearest labelled side (consistent with the gap rule); a cancelled rift consumes rng draws (the per-plate roll and the start pixel), which is fine for determinism since the sequence is still a pure function of the seed.
