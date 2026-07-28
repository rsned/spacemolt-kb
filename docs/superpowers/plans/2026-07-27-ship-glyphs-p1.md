# Ship Glyphs P1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate a deterministic top-down blueprint-line-art SVG glyph for every ship in the catalog, plus a single contact sheet page showing all of them grouped by faction, so the design language can be tuned by eye before any record-sheet work begins.

**Architecture:** A new `pkg/shipglyph` package turns a *descriptor* (a declarative description of a ship's hull parts, proportions and mount zones) into an SVG. Descriptors come from a stat-inferred layer keyed on `class`/`faction`/`scale`/slot counts, optionally overridden field-by-field by hand-authored JSON in `overlays/shipshapes/`. A new `cmd/generate-ship-glyphs` reads the ship catalog, renders one SVG per ship into `kb/ships/glyphs/`, and writes the contact sheet.

**Tech Stack:** Go 1.25, stdlib only (`hash/fnv`, `math`, `encoding/json`, `html/template`). No new module dependencies. Output is plain SVG text.

## Global Constraints

- Module is `github.com/rsned/spacemolt-kb`, Go 1.25.0. Use modern Go: `for i := range n` integer ranges, `b.Loop()` in any benchmark, `slices`/`cmp` from stdlib.
- `golangci-lint` must introduce **no new findings**. Config at `.golangci.yml`.
- `go build ./...` and `go test ./...` must pass after every task.
- **Rendering must be deterministic.** All pseudo-randomness derives from `hash/fnv` FNV-1a of the ship ID, matching the existing convention in `cmd/generate-factions-kb/silhouette.go`. Running the generator twice must produce a zero-byte diff.
- Ship data source is the catalog JSON, **not** the SQLite DB: `../spacemolt/data/game-api/latest/catalog_ships.json`, unmarshalled as `{"items": [...]}`. It currently holds 335 ships. This matches how `cmd/generate-items-kb` loads ships (`cmd/generate-items-kb/ships.go:86`).
- Every package and exported identifier gets a doc comment. Package doc goes on the file named after the package (`pkg/shipglyph/shipglyph.go`).
- Glyph coordinate space: **`t` runs 0 at the nose to 1 at the tail; `y` is signed offset from the centerline.** All descriptor geometry is in this normalized space; only the renderer converts to SVG user units.
- Colors must work in both KB themes. Use `currentColor` for strokes in generated SVG and let CSS set it — never hardcode a hex or hsl value in a glyph.
- Commit after every task.

---

### Task 1: Descriptor types, overlay loading, and merge

**Files:**
- Create: `pkg/shipglyph/shipglyph.go`
- Create: `pkg/shipglyph/descriptor.go`
- Test: `pkg/shipglyph/descriptor_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Descriptor`, `HullPart`, `Appendage`, `MountZones` structs; `LoadOverlay(dir, id string) (Descriptor, bool, error)`; `Merge(base, over Descriptor) Descriptor`.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/descriptor_test.go`:

```go
package shipglyph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOverlayWinsFieldByField(t *testing.T) {
	base := Descriptor{
		ID:      "prayer",
		Aspect:  2.0,
		Greeble: "light",
		Hull:    []HullPart{{Kind: "beam", Span: [2]float64{0, 1}}},
		MountZones: MountZones{
			Weapon: [][2]float64{{0.1, 0.4}},
		},
	}
	over := Descriptor{
		Aspect: 3.2,
		Hull: []HullPart{
			{Kind: "container_stack", Span: [2]float64{0.15, 0.75}, Grid: [2]int{2, 2}},
		},
	}

	got := Merge(base, over)

	if got.Aspect != 3.2 {
		t.Errorf("Aspect = %v, want 3.2 (overlay wins)", got.Aspect)
	}
	if got.Greeble != "light" {
		t.Errorf("Greeble = %q, want %q (base survives)", got.Greeble, "light")
	}
	if len(got.Hull) != 1 || got.Hull[0].Kind != "container_stack" {
		t.Errorf("Hull = %+v, want the overlay's container_stack", got.Hull)
	}
	if len(got.MountZones.Weapon) != 1 {
		t.Errorf("MountZones.Weapon = %+v, want base's zone to survive", got.MountZones.Weapon)
	}
	if got.ID != "prayer" {
		t.Errorf("ID = %q, want %q", got.ID, "prayer")
	}
}

func TestLoadOverlayMissingFileIsNotAnError(t *testing.T) {
	_, ok, err := LoadOverlay(t.TempDir(), "nope")
	if err != nil {
		t.Fatalf("LoadOverlay returned error for missing file: %v", err)
	}
	if ok {
		t.Errorf("ok = true, want false for a missing overlay")
	}
}

func TestLoadOverlayParsesJSON(t *testing.T) {
	dir := t.TempDir()
	body := `{
	  "id": "prayer",
	  "aspect": 3.2,
	  "symmetry": "bilateral",
	  "hull": [
	    {"kind": "container_stack", "span": [0.15, 0.75], "grid": [2, 2]},
	    {"kind": "engine_cone", "span": [0.75, 1.0], "bells": 4}
	  ],
	  "appendages": [{"kind": "wing", "at": 0.62, "sweep": 38, "span": 0.55, "side": "both"}],
	  "mountZones": {"weapon": [[0.1, 0.45]]},
	  "greeble": "heavy"
	}`
	if err := os.WriteFile(filepath.Join(dir, "prayer.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	d, ok, err := LoadOverlay(dir, "prayer")
	if err != nil || !ok {
		t.Fatalf("LoadOverlay: ok=%v err=%v", ok, err)
	}
	if len(d.Hull) != 2 {
		t.Fatalf("Hull len = %d, want 2", len(d.Hull))
	}
	if d.Hull[0].Grid != [2]int{2, 2} {
		t.Errorf("Grid = %v, want [2 2]", d.Hull[0].Grid)
	}
	if d.Hull[1].Bells != 4 {
		t.Errorf("Bells = %d, want 4", d.Hull[1].Bells)
	}
	if len(d.Appendages) != 1 || d.Appendages[0].Sweep != 38 {
		t.Errorf("Appendages = %+v", d.Appendages)
	}
	if d.Greeble != "heavy" {
		t.Errorf("Greeble = %q, want heavy", d.Greeble)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestMerge -v`
Expected: FAIL — the package does not exist (`no Go files in .../pkg/shipglyph`).

- [ ] **Step 3: Write the package doc file**

Create `pkg/shipglyph/shipglyph.go`:

```go
// Package shipglyph renders top-down blueprint-style ship outlines as SVG.
//
// A ship's shape is described by a Descriptor: a list of hull parts along the
// spine plus appendages and weapon mount zones, all in a normalized coordinate
// space where t runs 0 at the nose to 1 at the tail and y is a signed offset
// from the centerline. Descriptors are produced by Infer from catalog stats and
// may be overridden field-by-field by hand-authored JSON overlays.
//
// Rendering is deterministic: all pseudo-random variation derives from an
// FNV-1a hash of the ship ID, so regenerating produces byte-identical output.
package shipglyph
```

- [ ] **Step 4: Write the descriptor types and merge**

Create `pkg/shipglyph/descriptor.go`:

```go
package shipglyph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// HullPart is one section of the hull along the spine. Kind selects which
// geometry routine interprets the remaining fields; unused fields are zero.
type HullPart struct {
	// Kind is one of: beam, box, container_stack, bay_rack, engine_cone,
	// open_frame, drum, disc, lobe_cluster.
	Kind string `json:"kind"`
	// Span is the [start, end] spine range this part occupies, in t.
	Span [2]float64 `json:"span"`
	// Points are [t, halfwidth] control points, used by kind "beam".
	Points [][2]float64 `json:"points,omitempty"`
	// Grid is [across, deep] container counts, used by kind "container_stack".
	Grid [2]int `json:"grid,omitempty"`
	// Cells is the number of bays, used by kind "bay_rack".
	Cells int `json:"cells,omitempty"`
	// Bells is the number of engine nozzles, used by kind "engine_cone".
	Bells int `json:"bells,omitempty"`
	// Lobes is the number of organic lobes, used by kind "lobe_cluster".
	Lobes int `json:"lobes,omitempty"`
	// Radius is the half-width, used by kinds "drum" and "disc".
	Radius float64 `json:"radius,omitempty"`
	// Half is the constant half-width, used by kind "box".
	Half float64 `json:"half,omitempty"`
	// Seat marks an exposed pilot seat, used by kind "open_frame".
	Seat bool `json:"seat,omitempty"`
}

// Appendage is a feature attached to the hull rather than part of the spine.
type Appendage struct {
	// Kind is one of: wing, sponson, nacelle, outrigger, boom, drone_rack,
	// tow_arm, antenna_mast.
	Kind string `json:"kind"`
	// At is the spine position, in t, where the appendage attaches.
	At float64 `json:"at"`
	// Sweep is the trailing sweep angle in degrees, for wings.
	Sweep float64 `json:"sweep,omitempty"`
	// Span is how far the appendage extends from the hull, in y units.
	Span float64 `json:"span,omitempty"`
	// Side is "both", "port" or "starboard". Empty means "both".
	Side string `json:"side,omitempty"`
}

// MountZones are the spine ranges along which hardpoint markers of each slot
// type may be placed. Each entry is a [start, end] range in t.
type MountZones struct {
	Weapon  [][2]float64 `json:"weapon,omitempty"`
	Defense [][2]float64 `json:"defense,omitempty"`
	Utility [][2]float64 `json:"utility,omitempty"`
}

// Descriptor is a complete declarative description of a ship's top-down shape.
// Every field is optional; absent fields are supplied by the inferred layer.
type Descriptor struct {
	ID string `json:"id,omitempty"`
	// Aspect is length divided by maximum beam.
	Aspect float64 `json:"aspect,omitempty"`
	// Symmetry is "bilateral", "asymmetric" or "radial".
	Symmetry   string      `json:"symmetry,omitempty"`
	Hull       []HullPart  `json:"hull,omitempty"`
	Appendages []Appendage `json:"appendages,omitempty"`
	MountZones MountZones  `json:"mountZones,omitempty"`
	// Greeble is "none", "light" or "heavy" surface detail density.
	Greeble string `json:"greeble,omitempty"`
}

// Merge overlays over onto base field by field. A field set in over replaces
// the corresponding field in base; a zero-valued field in over leaves base
// untouched. Slice fields are replaced wholesale, never appended, so an
// overlay that specifies Hull fully controls the hull.
func Merge(base, over Descriptor) Descriptor {
	out := base
	if over.ID != "" {
		out.ID = over.ID
	}
	if over.Aspect != 0 {
		out.Aspect = over.Aspect
	}
	if over.Symmetry != "" {
		out.Symmetry = over.Symmetry
	}
	if len(over.Hull) > 0 {
		out.Hull = over.Hull
	}
	if len(over.Appendages) > 0 {
		out.Appendages = over.Appendages
	}
	if len(over.MountZones.Weapon) > 0 {
		out.MountZones.Weapon = over.MountZones.Weapon
	}
	if len(over.MountZones.Defense) > 0 {
		out.MountZones.Defense = over.MountZones.Defense
	}
	if len(over.MountZones.Utility) > 0 {
		out.MountZones.Utility = over.MountZones.Utility
	}
	if over.Greeble != "" {
		out.Greeble = over.Greeble
	}
	return out
}

// LoadOverlay reads dir/<id>.json. It reports ok=false with a nil error when
// no overlay exists for the ship, which is the common case.
func LoadOverlay(dir, id string) (Descriptor, bool, error) {
	var d Descriptor
	if dir == "" {
		return d, false, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return d, false, nil
	}
	if err != nil {
		return d, false, fmt.Errorf("read overlay for %q: %w", id, err)
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, false, fmt.Errorf("parse overlay for %q: %w", id, err)
	}
	return d, true, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS — all three tests.

- [ ] **Step 6: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): descriptor types, overlay loading and field-wise merge"
```

---

### Task 2: Stat-inferred descriptors

**Files:**
- Create: `pkg/shipglyph/infer.go`
- Test: `pkg/shipglyph/infer_test.go`

**Interfaces:**
- Consumes: `Descriptor`, `HullPart`, `MountZones` from Task 1.
- Produces: `Stats` struct; `Infer(s Stats) Descriptor`; `archetypeOf(class string) string`.

Archetype families group the ~30 catalog classes into eight shapes. Any class not listed falls back to `spine`.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/infer_test.go`:

```go
package shipglyph

import "testing"

func TestArchetypeOfKnownAndUnknownClasses(t *testing.T) {
	cases := map[string]string{
		"Liner":        "needle",
		"Courier":      "needle",
		"Dreadnought":  "spine",
		"Freighter":    "slab",
		"Bulk Hauler":  "slab",
		"Tanker":       "drum",
		"Gas Harvester": "drum",
		"Miner":        "rig",
		"Salvager":     "rig",
		"Fleet Carrier": "rack",
		"Drone Carrier": "rack",
		"Fighter":      "dart",
		"Research":     "pod",
		"Nonsense":     "spine",
	}
	for class, want := range cases {
		if got := archetypeOf(class); got != want {
			t.Errorf("archetypeOf(%q) = %q, want %q", class, got, want)
		}
	}
}

func TestInferAlwaysProducesUsableGeometry(t *testing.T) {
	s := Stats{ID: "prayer", Class: "Freighter", Faction: "outerrim", Scale: 1, Utility: 0}
	d := Infer(s)

	if d.ID != "prayer" {
		t.Errorf("ID = %q, want prayer", d.ID)
	}
	if d.Aspect <= 0 {
		t.Errorf("Aspect = %v, want positive", d.Aspect)
	}
	if len(d.Hull) == 0 {
		t.Fatalf("Hull is empty; every ship must get geometry")
	}
	for i, p := range d.Hull {
		if p.Span[1] <= p.Span[0] {
			t.Errorf("Hull[%d].Span = %v, want end > start", i, p.Span)
		}
	}
}

func TestInferNeedleIsNarrowerThanSlab(t *testing.T) {
	needle := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	slab := Infer(Stats{ID: "ledger", Class: "Freighter", Faction: "nebula", Scale: 4})

	if needle.Aspect <= slab.Aspect {
		t.Errorf("needle Aspect %v should exceed slab Aspect %v", needle.Aspect, slab.Aspect)
	}
}

func TestInferMountZonesAlwaysPresent(t *testing.T) {
	d := Infer(Stats{ID: "magnate", Class: "Command", Faction: "solarian", Scale: 4,
		Weapon: 3, Defense: 6, Utility: 5})

	if len(d.MountZones.Weapon) == 0 {
		t.Errorf("Weapon zones empty")
	}
	if len(d.MountZones.Defense) == 0 {
		t.Errorf("Defense zones empty")
	}
	if len(d.MountZones.Utility) == 0 {
		t.Errorf("Utility zones empty")
	}
}

func TestInferIsDeterministic(t *testing.T) {
	s := Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1, Weapon: 2}
	a, b := Infer(s), Infer(s)
	if a.Aspect != b.Aspect || len(a.Hull) != len(b.Hull) {
		t.Errorf("Infer is not deterministic: %+v vs %+v", a, b)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestInfer -v`
Expected: FAIL — `undefined: Stats`, `undefined: Infer`, `undefined: archetypeOf`.

- [ ] **Step 3: Write the inference layer**

Create `pkg/shipglyph/infer.go`:

```go
package shipglyph

// Stats is the subset of catalog ship data that drives shape inference. It is
// deliberately decoupled from the catalog JSON structs so the package has no
// dependency on the generator.
type Stats struct {
	ID       string
	Name     string
	Class    string
	Category string
	Faction  string
	Tier     int
	Scale    int
	Weapon   int
	Defense  int
	Utility  int
	Cargo    int
}

// classArchetype maps catalog ship classes to shape families. Classes absent
// from this table fall back to "spine".
var classArchetype = map[string]string{
	"Liner": "needle", "Courier": "needle", "Shuttle": "needle",
	"Yacht": "needle", "Scout": "needle", "Explorer": "needle",

	"Cruiser": "spine", "Battlecruiser": "spine", "Dreadnought": "spine",
	"Command": "spine", "Patrol": "spine", "Assault": "spine",
	"Raider": "spine", "Interdictor": "spine",

	"Freighter": "slab", "Bulk Hauler": "slab", "Hazmat Freighter": "slab",

	"Tanker": "drum", "Refinery": "drum",
	"Gas Harvester": "drum", "Ice Harvester": "drum",

	"Miner": "rig", "Salvager": "rig",

	"Carrier": "rack", "Fleet Carrier": "rack",
	"Drone Carrier": "rack", "Logistics": "rack",

	"Fighter": "dart", "Heavy Fighter": "dart",

	"Research": "pod", "Intelligence": "pod", "Electronic Warfare": "pod",
}

// archetypeOf returns the shape family for a catalog ship class.
func archetypeOf(class string) string {
	if a, ok := classArchetype[class]; ok {
		return a
	}
	return "spine"
}

// archetypeAspect is the base length-to-beam ratio per family.
var archetypeAspect = map[string]float64{
	"needle": 6.5, "dart": 2.6, "spine": 3.6, "slab": 2.4,
	"drum": 2.8, "rig": 2.2, "rack": 2.9, "pod": 2.7,
}

// Infer builds a complete Descriptor from catalog stats alone. It always
// returns usable geometry, so a ship with no overlay and no lore still renders.
func Infer(s Stats) Descriptor {
	fam := archetypeOf(s.Class)

	// Larger hulls read as slightly stubbier; scale 3 is neutral.
	aspect := archetypeAspect[fam] * (1 - 0.06*float64(s.Scale-3))
	if aspect < 1.2 {
		aspect = 1.2
	}

	d := Descriptor{
		ID:         s.ID,
		Aspect:     aspect,
		Symmetry:   "bilateral",
		Hull:       archetypeHull(fam, s),
		Appendages: archetypeAppendages(fam, s),
		MountZones: archetypeZones(fam),
		Greeble:    greebleFor(s),
	}
	if s.Faction == "outerrim" || s.Faction == "pirate" {
		d.Symmetry = "asymmetric"
	}
	return d
}

// archetypeHull returns the hull part list for a shape family.
func archetypeHull(fam string, s Stats) []HullPart {
	switch fam {
	case "needle":
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.01}, {0.25, 0.06}, {0.5, 0.10}, {0.8, 0.07}, {1, 0.03}},
		}}
	case "dart":
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.03}, {0.35, 0.16}, {0.7, 0.20}, {1, 0.10}},
		}}
	case "spine":
		return []HullPart{
			{Kind: "beam", Span: [2]float64{0, 0.82},
				Points: [][2]float64{{0, 0.09}, {0.15, 0.20}, {0.55, 0.22}, {0.82, 0.18}}},
			{Kind: "engine_cone", Span: [2]float64{0.82, 1}, Bells: enginesFor(s)},
		}
	case "slab":
		return []HullPart{
			{Kind: "box", Span: [2]float64{0.08, 0.80}, Half: 0.26},
			{Kind: "engine_cone", Span: [2]float64{0.80, 1}, Bells: enginesFor(s)},
			{Kind: "open_frame", Span: [2]float64{0, 0.08}, Seat: s.Utility == 0},
		}
	case "drum":
		return []HullPart{
			{Kind: "drum", Span: [2]float64{0.14, 0.78}, Radius: 0.28},
			{Kind: "engine_cone", Span: [2]float64{0.78, 1}, Bells: enginesFor(s)},
			{Kind: "beam", Span: [2]float64{0, 0.14},
				Points: [][2]float64{{0, 0.05}, {0.14, 0.16}}},
		}
	case "rig":
		return []HullPart{
			{Kind: "box", Span: [2]float64{0.20, 0.78}, Half: 0.24},
			{Kind: "engine_cone", Span: [2]float64{0.78, 1}, Bells: enginesFor(s)},
			{Kind: "open_frame", Span: [2]float64{0, 0.20}},
		}
	case "rack":
		return []HullPart{
			{Kind: "bay_rack", Span: [2]float64{0.10, 0.84}, Cells: bayCells(s)},
			{Kind: "engine_cone", Span: [2]float64{0.84, 1}, Bells: enginesFor(s)},
		}
	case "pod":
		return []HullPart{
			{Kind: "disc", Span: [2]float64{0.05, 0.62}, Radius: 0.30},
			{Kind: "beam", Span: [2]float64{0.62, 1},
				Points: [][2]float64{{0.62, 0.14}, {1, 0.08}}},
		}
	default:
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.10}, {0.5, 0.20}, {1, 0.12}},
		}}
	}
}

// archetypeAppendages returns the non-spine features for a shape family.
func archetypeAppendages(fam string, s Stats) []Appendage {
	switch fam {
	case "needle":
		return []Appendage{{Kind: "wing", At: 0.66, Sweep: 42, Span: 0.34, Side: "both"}}
	case "dart":
		return []Appendage{{Kind: "wing", At: 0.58, Sweep: 28, Span: 0.42, Side: "both"}}
	case "rig":
		return []Appendage{{Kind: "tow_arm", At: 0.16, Span: 0.20, Side: "both"}}
	case "rack":
		return []Appendage{{Kind: "drone_rack", At: 0.46, Span: 0.16, Side: "both"}}
	case "drum":
		return []Appendage{{Kind: "nacelle", At: 0.70, Span: 0.16, Side: "both"}}
	case "pod":
		return []Appendage{{Kind: "antenna_mast", At: 0.30, Span: 0.22, Side: "both"}}
	default:
		if s.Weapon >= 4 {
			return []Appendage{{Kind: "sponson", At: 0.50, Span: 0.12, Side: "both"}}
		}
		return nil
	}
}

// archetypeZones returns default hardpoint mount ranges for a shape family.
func archetypeZones(fam string) MountZones {
	switch fam {
	case "needle", "dart":
		return MountZones{
			Weapon:  [][2]float64{{0.10, 0.45}},
			Defense: [][2]float64{{0.35, 0.70}},
			Utility: [][2]float64{{0.55, 0.92}},
		}
	case "rack":
		return MountZones{
			Weapon:  [][2]float64{{0.08, 0.30}},
			Defense: [][2]float64{{0.20, 0.80}},
			Utility: [][2]float64{{0.30, 0.90}},
		}
	default:
		return MountZones{
			Weapon:  [][2]float64{{0.06, 0.40}},
			Defense: [][2]float64{{0.30, 0.72}},
			Utility: [][2]float64{{0.50, 0.94}},
		}
	}
}

// enginesFor returns a plausible engine nozzle count from hull scale.
func enginesFor(s Stats) int {
	n := 1 + s.Scale/2
	if n < 1 {
		n = 1
	}
	if n > 4 {
		n = 4
	}
	return n
}

// bayCells returns the number of carrier bays from hull scale.
func bayCells(s Stats) int {
	n := 2 + s.Scale
	if n < 2 {
		n = 2
	}
	if n > 7 {
		n = 7
	}
	return n
}

// greebleFor picks surface detail density from faction and scale.
func greebleFor(s Stats) string {
	switch s.Faction {
	case "outerrim", "pirate":
		return "heavy"
	case "nebula":
		return "none"
	}
	if s.Scale >= 4 {
		return "heavy"
	}
	return "light"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS — all tests including Task 1's.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): stat-inferred descriptors with eight class archetypes"
```

---

### Task 3: Deterministic seeding and faction styles

**Files:**
- Create: `pkg/shipglyph/style.go`
- Test: `pkg/shipglyph/style_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `Point` struct; `SeedOf(id string) uint64`; `Style` struct with fields `Name string`, `Chamfer float64`, `Smooth bool`, `Flute bool`, `Jitter float64`, `Lobed bool`; `StyleFor(faction string) Style`; `rng` type with method `next() float64`.

`rng` is a tiny deterministic generator (splitmix64) so jitter is reproducible without `math/rand` global state.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/style_test.go`:

```go
package shipglyph

import "testing"

func TestSeedOfIsStableAndDistinct(t *testing.T) {
	if SeedOf("prayer") != SeedOf("prayer") {
		t.Errorf("SeedOf is not stable")
	}
	if SeedOf("prayer") == SeedOf("comet") {
		t.Errorf("SeedOf collided for two different ids")
	}
}

func TestStyleForKnownFactions(t *testing.T) {
	if StyleFor("crimson").Chamfer <= 0 {
		t.Errorf("crimson should chamfer")
	}
	if !StyleFor("nebula").Smooth {
		t.Errorf("nebula should be smooth")
	}
	if !StyleFor("solarian").Flute {
		t.Errorf("solarian should flute")
	}
	if StyleFor("outerrim").Jitter <= 0 {
		t.Errorf("outerrim should jitter")
	}
	if !StyleFor("voidborn").Lobed {
		t.Errorf("voidborn should be lobed")
	}
	p := StyleFor("pirate")
	if p.Jitter <= 0 || p.Chamfer <= 0 {
		t.Errorf("pirate should both jitter and chamfer, got %+v", p)
	}
}

func TestStyleForUnknownFactionFallsBack(t *testing.T) {
	s := StyleFor("")
	if s.Name == "" {
		t.Errorf("unknown faction produced an unnamed style")
	}
}

func TestRNGIsDeterministicAndBounded(t *testing.T) {
	a, b := newRNG(42), newRNG(42)
	for range 100 {
		x, y := a.next(), b.next()
		if x != y {
			t.Fatalf("rng diverged: %v vs %v", x, y)
		}
		if x < 0 || x >= 1 {
			t.Fatalf("rng out of [0,1): %v", x)
		}
	}
	c := newRNG(43)
	if c.next() == newRNG(42).next() {
		t.Errorf("different seeds produced the same first value")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run 'TestSeedOf|TestStyleFor|TestRNG' -v`
Expected: FAIL — `undefined: SeedOf`, `undefined: StyleFor`, `undefined: newRNG`.

- [ ] **Step 3: Write the style layer**

Create `pkg/shipglyph/style.go`:

```go
package shipglyph

import "hash/fnv"

// Point is a position in glyph space: X runs 0 at the nose to 1 at the tail,
// Y is a signed offset from the centerline.
type Point struct{ X, Y float64 }

// SeedOf returns a stable 64-bit FNV-1a hash of a ship ID, used to derive all
// deterministic visual variation. Matches the convention in
// cmd/generate-factions-kb/silhouette.go.
func SeedOf(id string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	return h.Sum64()
}

// rng is a deterministic splitmix64 generator. It avoids math/rand so output
// cannot drift with Go version or global seeding.
type rng struct{ state uint64 }

// newRNG returns a generator seeded from s.
func newRNG(s uint64) *rng { return &rng{state: s} }

// next returns the next value in [0, 1).
func (r *rng) next() float64 {
	r.state += 0x9e3779b97f4a7c15
	z := r.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return float64(z>>11) / float64(uint64(1)<<53)
}

// Style is a faction's rendering treatment. It controls only how hull control
// points are joined and decorated, never the geometry itself, so the same
// Descriptor renders in any faction's language.
type Style struct {
	// Name is the faction key this style was built for.
	Name string
	// Chamfer is the fraction of a segment cut away at each vertex, giving
	// the angular Crimson look. Zero disables chamfering.
	Chamfer float64
	// Smooth joins control points with a Catmull-Rom spline.
	Smooth bool
	// Flute adds regular perpendicular notches along the hull sides.
	Flute bool
	// Jitter is the fractional per-vertex displacement applied independently
	// to each side, producing the Outer Rim's welded-scrap asymmetry.
	Jitter float64
	// Lobed expands each control point into an organic bulge before
	// smoothing, producing the Voidborn's flowing forms.
	Lobed bool
}

// StyleFor returns the rendering treatment for a faction. Unknown or empty
// factions get a neutral style so every ship renders.
func StyleFor(faction string) Style {
	switch faction {
	case "crimson":
		return Style{Name: "crimson", Chamfer: 0.22}
	case "nebula":
		return Style{Name: "nebula", Smooth: true}
	case "solarian":
		return Style{Name: "solarian", Flute: true, Chamfer: 0.10}
	case "outerrim":
		return Style{Name: "outerrim", Jitter: 0.08}
	case "voidborn":
		return Style{Name: "voidborn", Smooth: true, Lobed: true}
	case "pirate":
		return Style{Name: "pirate", Jitter: 0.10, Chamfer: 0.18}
	default:
		return Style{Name: "neutral", Chamfer: 0.08}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): deterministic seeding and six faction styles"
```

---

### Task 4: Hull part half-widths and profile sampling

**Files:**
- Create: `pkg/shipglyph/parts.go`
- Test: `pkg/shipglyph/parts_test.go`

**Interfaces:**
- Consumes: `HullPart`, `Descriptor` (Task 1); `Point`, `Style`, `rng`, `newRNG` (Task 3).
- Produces: `partHalfWidth(p HullPart, t float64) float64`; `profileSamples` constant; `sampleProfile(d Descriptor, st Style, seed uint64, side int) []Point`.

`sampleProfile` returns `profileSamples` points along one side of the hull, with `side` being `+1` (starboard) or `-1` (port). Jitter is applied per side so asymmetric factions differ left to right.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/parts_test.go`:

```go
package shipglyph

import (
	"math"
	"testing"
)

func TestPartHalfWidthOutsideSpanIsZero(t *testing.T) {
	p := HullPart{Kind: "box", Span: [2]float64{0.2, 0.8}, Half: 0.3}
	if got := partHalfWidth(p, 0.1); got != 0 {
		t.Errorf("before span = %v, want 0", got)
	}
	if got := partHalfWidth(p, 0.9); got != 0 {
		t.Errorf("after span = %v, want 0", got)
	}
	if got := partHalfWidth(p, 0.5); math.Abs(got-0.3) > 1e-9 {
		t.Errorf("inside span = %v, want 0.3", got)
	}
}

func TestPartHalfWidthBeamInterpolates(t *testing.T) {
	p := HullPart{
		Kind: "beam", Span: [2]float64{0, 1},
		Points: [][2]float64{{0, 0.0}, {0.5, 0.2}, {1, 0.0}},
	}
	mid := partHalfWidth(p, 0.5)
	if math.Abs(mid-0.2) > 1e-9 {
		t.Errorf("at control point = %v, want 0.2", mid)
	}
	quarter := partHalfWidth(p, 0.25)
	if quarter <= 0 || quarter >= 0.2 {
		t.Errorf("interpolated = %v, want strictly between 0 and 0.2", quarter)
	}
}

func TestPartHalfWidthEngineConeTapers(t *testing.T) {
	p := HullPart{Kind: "engine_cone", Span: [2]float64{0.8, 1.0}, Bells: 3}
	near, far := partHalfWidth(p, 0.82), partHalfWidth(p, 0.99)
	if near <= far {
		t.Errorf("cone should narrow toward the tail: %v then %v", near, far)
	}
}

func TestSampleProfileLengthAndNonNegativeWidth(t *testing.T) {
	d := Infer(Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4})
	got := sampleProfile(d, StyleFor("crimson"), SeedOf("war_wagon"), 1)

	if len(got) != profileSamples {
		t.Fatalf("len = %d, want %d", len(got), profileSamples)
	}
	for i, p := range got {
		if p.Y < 0 {
			t.Fatalf("sample %d has negative Y %v; side +1 must stay non-negative", i, p.Y)
		}
		if p.X < 0 || p.X > 1 {
			t.Fatalf("sample %d has X %v outside [0,1]", i, p.X)
		}
	}
}

func TestSampleProfileIsDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	a := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	b := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d diverged: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestSampleProfileJitterDiffersBySide(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	star := sampleProfile(d, st, SeedOf("yard_sale"), 1)
	port := sampleProfile(d, st, SeedOf("yard_sale"), -1)

	same := true
	for i := range star {
		if math.Abs(star[i].Y) != math.Abs(port[i].Y) {
			same = false
			break
		}
	}
	if same {
		t.Errorf("outerrim sides are mirror-identical; jitter should break symmetry")
	}
}

func TestSampleProfileSymmetricFactionIsMirrored(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	st := StyleFor("nebula")
	star := sampleProfile(d, st, SeedOf("comet"), 1)
	port := sampleProfile(d, st, SeedOf("comet"), -1)

	for i := range star {
		if math.Abs(star[i].Y-(-port[i].Y)) > 1e-9 {
			t.Fatalf("sample %d not mirrored: %v vs %v", i, star[i].Y, port[i].Y)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run 'TestPart|TestSampleProfile' -v`
Expected: FAIL — `undefined: partHalfWidth`, `undefined: sampleProfile`, `undefined: profileSamples`.

- [ ] **Step 3: Write the parts layer**

Create `pkg/shipglyph/parts.go`:

```go
package shipglyph

import "math"

// profileSamples is how many points are taken along each side of the hull.
// High enough for smooth curves at glyph size, low enough to keep SVGs small.
const profileSamples = 96

// partHalfWidth returns the hull half-width contributed by part p at spine
// position t. It returns 0 when t falls outside the part's span.
func partHalfWidth(p HullPart, t float64) float64 {
	lo, hi := p.Span[0], p.Span[1]
	if hi <= lo || t < lo || t > hi {
		return 0
	}
	// u is the position within the part, 0 at its start and 1 at its end.
	u := (t - lo) / (hi - lo)

	switch p.Kind {
	case "beam":
		return beamHalfWidth(p.Points, t)
	case "box":
		return p.Half
	case "container_stack":
		across := p.Grid[0]
		if across < 1 {
			across = 1
		}
		return 0.11 * float64(across)
	case "bay_rack":
		return 0.30
	case "engine_cone":
		// Widest where it meets the hull, tapering to the nozzle throat.
		return 0.20 - 0.09*u
	case "open_frame":
		if p.Seat {
			return 0.13
		}
		return 0.09
	case "drum":
		// Cylindrical body with rounded ends.
		return p.Radius * math.Sqrt(math.Max(0, 1-math.Pow(2*u-1, 6)))
	case "disc":
		return p.Radius * math.Sqrt(math.Max(0, 1-math.Pow(2*u-1, 2)))
	case "lobe_cluster":
		n := p.Lobes
		if n < 1 {
			n = 1
		}
		// Sum of evenly spaced bulges along the span.
		var w float64
		for i := range n {
			c := (float64(i) + 0.5) / float64(n)
			d := (u - c) * float64(n) * 1.6
			w += 0.22 * math.Exp(-d*d)
		}
		return w
	default:
		return 0.15
	}
}

// beamHalfWidth linearly interpolates [t, halfwidth] control points. Points
// must be sorted by t; values outside the range clamp to the end points.
func beamHalfWidth(pts [][2]float64, t float64) float64 {
	if len(pts) == 0 {
		return 0
	}
	if t <= pts[0][0] {
		return pts[0][1]
	}
	last := pts[len(pts)-1]
	if t >= last[0] {
		return last[1]
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if t <= b[0] {
			span := b[0] - a[0]
			if span <= 0 {
				return b[1]
			}
			f := (t - a[0]) / span
			return a[1] + f*(b[1]-a[1])
		}
	}
	return last[1]
}

// hullHalfWidth returns the widest contribution of any part at t.
func hullHalfWidth(d Descriptor, t float64) float64 {
	var w float64
	for _, p := range d.Hull {
		if h := partHalfWidth(p, t); h > w {
			w = h
		}
	}
	return w
}

// sampleProfile walks the spine from nose to tail and returns one side of the
// hull outline. side is +1 for starboard (positive Y) or -1 for port.
// Style-driven jitter is seeded per side, so asymmetric factions differ left to
// right while symmetric ones mirror exactly.
func sampleProfile(d Descriptor, st Style, seed uint64, side int) []Point {
	// Port gets a distinct sub-seed so its jitter is independent. Symmetric
	// styles have Jitter == 0 and are therefore unaffected.
	sub := seed
	if side < 0 {
		sub = seed ^ 0x5bf03635
	}
	r := newRNG(sub)

	out := make([]Point, profileSamples)
	for i := range profileSamples {
		t := float64(i) / float64(profileSamples-1)
		w := hullHalfWidth(d, t)

		if st.Lobed {
			// Organic swelling: a slow wave that never narrows the hull.
			w *= 1 + 0.18*math.Sin(t*math.Pi*3.0)
		}
		if st.Flute {
			// Regular perpendicular notches.
			w *= 1 - 0.06*math.Abs(math.Sin(t*math.Pi*9))
		}
		if st.Jitter > 0 {
			w *= 1 + st.Jitter*(r.next()*2-1)
		}
		if w < 0 {
			w = 0
		}
		out[i] = Point{X: t, Y: w * float64(side)}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): hull part half-widths and per-side profile sampling"
```

---

### Task 5: Outline assembly with chamfering and smoothing

**Files:**
- Create: `pkg/shipglyph/outline.go`
- Test: `pkg/shipglyph/outline_test.go`

**Interfaces:**
- Consumes: `sampleProfile`, `profileSamples` (Task 4); `Point`, `Style` (Task 3); `Descriptor` (Task 1).
- Produces: `Outline(d Descriptor, st Style, seed uint64) []Point` returning a closed loop in glyph space, starboard side nose-to-tail then port side tail-to-nose.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/outline_test.go`:

```go
package shipglyph

import (
	"math"
	"testing"
)

func TestOutlineIsAClosedLoopOfBothSides(t *testing.T) {
	d := Infer(Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1})
	loop := Outline(d, StyleFor("crimson"), SeedOf("crowbar"))

	if len(loop) < profileSamples {
		t.Fatalf("len = %d, want at least %d", len(loop), profileSamples)
	}
	var sawPositive, sawNegative bool
	for _, p := range loop {
		if p.Y > 1e-9 {
			sawPositive = true
		}
		if p.Y < -1e-9 {
			sawNegative = true
		}
	}
	if !sawPositive || !sawNegative {
		t.Errorf("loop does not span both sides: +%v -%v", sawPositive, sawNegative)
	}
}

func TestOutlineStartsAtNoseAndReachesTail(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	loop := Outline(d, StyleFor("nebula"), SeedOf("comet"))

	var minX, maxX = 1.0, 0.0
	for _, p := range loop {
		minX = math.Min(minX, p.X)
		maxX = math.Max(maxX, p.X)
	}
	if minX > 1e-6 {
		t.Errorf("minX = %v, want ~0 (nose)", minX)
	}
	if maxX < 1-1e-6 {
		t.Errorf("maxX = %v, want ~1 (tail)", maxX)
	}
}

func TestOutlineChamferAddsVertices(t *testing.T) {
	d := Descriptor{
		Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.3}},
	}
	plain := Outline(d, Style{Name: "flat"}, 7)
	cham := Outline(d, Style{Name: "cham", Chamfer: 0.3}, 7)

	if len(cham) <= len(plain) {
		t.Errorf("chamfered loop has %d points, plain has %d; chamfering should add vertices",
			len(cham), len(plain))
	}
}

func TestOutlineIsDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "excessive_force", Class: "Drone Carrier", Faction: "outerrim", Scale: 4})
	st := StyleFor("outerrim")
	a := Outline(d, st, SeedOf("excessive_force"))
	b := Outline(d, st, SeedOf("excessive_force"))

	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("point %d diverged: %v vs %v", i, a[i], b[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestOutline -v`
Expected: FAIL — `undefined: Outline`.

- [ ] **Step 3: Write the outline assembler**

Create `pkg/shipglyph/outline.go`:

```go
package shipglyph

// Outline assembles the closed hull loop in glyph space. The loop runs along
// the starboard side from nose to tail, then back along the port side from
// tail to nose. The caller is responsible for closing the path.
func Outline(d Descriptor, st Style, seed uint64) []Point {
	star := sampleProfile(d, st, seed, 1)
	port := sampleProfile(d, st, seed, -1)

	loop := make([]Point, 0, len(star)+len(port))
	loop = append(loop, star...)
	for i := len(port) - 1; i >= 0; i-- {
		loop = append(loop, port[i])
	}

	if st.Chamfer > 0 {
		loop = chamfer(loop, st.Chamfer)
	}
	if st.Smooth {
		loop = smooth(loop)
	}
	return loop
}

// chamfer replaces each vertex with two points cut back along its adjacent
// edges by fraction f, producing the angular faceted look. f is clamped to
// (0, 0.5] so adjacent chamfers cannot overlap.
func chamfer(loop []Point, f float64) []Point {
	n := len(loop)
	if n < 3 {
		return loop
	}
	if f > 0.5 {
		f = 0.5
	}
	out := make([]Point, 0, n*2)
	for i := range n {
		prev := loop[(i-1+n)%n]
		cur := loop[i]
		next := loop[(i+1)%n]
		out = append(out,
			Point{X: cur.X + (prev.X-cur.X)*f, Y: cur.Y + (prev.Y-cur.Y)*f},
			Point{X: cur.X + (next.X-cur.X)*f, Y: cur.Y + (next.Y-cur.Y)*f},
		)
	}
	return out
}

// smooth applies one pass of Chaikin corner cutting, which converges toward a
// quadratic B-spline. Used for the flowing Nebula and Voidborn hulls.
func smooth(loop []Point) []Point {
	n := len(loop)
	if n < 3 {
		return loop
	}
	out := make([]Point, 0, n*2)
	for i := range n {
		a := loop[i]
		b := loop[(i+1)%n]
		out = append(out,
			Point{X: 0.75*a.X + 0.25*b.X, Y: 0.75*a.Y + 0.25*b.Y},
			Point{X: 0.25*a.X + 0.75*b.X, Y: 0.25*a.Y + 0.75*b.Y},
		)
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

Note: `TestOutlineStartsAtNoseAndReachesTail` passes because Nebula's style has `Smooth: true` but no chamfer, and Chaikin smoothing preserves the convex hull's extent to within the tolerance used. If it fails by a hair, that is a real signal — loosen the tolerance in the test to `1e-3`, do not change the smoothing.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): closed outline assembly with chamfer and Chaikin smoothing"
```

---

### Task 6: Appendages and hardpoint placement

**Files:**
- Create: `pkg/shipglyph/hardpoints.go`
- Create: `pkg/shipglyph/appendages.go`
- Test: `pkg/shipglyph/hardpoints_test.go`
- Test: `pkg/shipglyph/appendages_test.go`

**Interfaces:**
- Consumes: `Descriptor`, `MountZones`, `Appendage` (Task 1); `Stats` (Task 2); `Point` (Task 3); `hullHalfWidth` (Task 4).
- Produces: `Hardpoint` struct with fields `ID string`, `Kind string`, `Pos Point`; `Hardpoints(d Descriptor, s Stats) []Hardpoint`; `AppendageShape` struct with fields `ID string`, `Kind string`, `Poly []Point`; `AppendageShapes(d Descriptor) []AppendageShape`.

IDs follow the stable scheme `hp-w1`, `hp-d1`, `hp-u1`, numbered from 1 in placement order. Slot **counts** come from `Stats`, never from the descriptor.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/hardpoints_test.go`:

```go
package shipglyph

import (
	"math"
	"strings"
	"testing"
)

func TestHardpointsCountMatchesSlots(t *testing.T) {
	s := Stats{ID: "magnate", Class: "Command", Faction: "solarian", Scale: 4,
		Weapon: 3, Defense: 6, Utility: 5}
	hps := Hardpoints(Infer(s), s)

	if len(hps) != 14 {
		t.Fatalf("len = %d, want 14 (3+6+5)", len(hps))
	}
	var w, d, u int
	for _, h := range hps {
		switch h.Kind {
		case "weapon":
			w++
		case "defense":
			d++
		case "utility":
			u++
		default:
			t.Errorf("unexpected kind %q", h.Kind)
		}
	}
	if w != 3 || d != 6 || u != 5 {
		t.Errorf("counts = %d/%d/%d, want 3/6/5", w, d, u)
	}
}

func TestHardpointIDsAreStableAndUnique(t *testing.T) {
	s := Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1,
		Weapon: 2, Defense: 1, Utility: 1}
	hps := Hardpoints(Infer(s), s)

	seen := map[string]bool{}
	for _, h := range hps {
		if seen[h.ID] {
			t.Errorf("duplicate hardpoint ID %q", h.ID)
		}
		seen[h.ID] = true
		if !strings.HasPrefix(h.ID, "hp-") {
			t.Errorf("ID %q does not use the hp- prefix", h.ID)
		}
	}
	if !seen["hp-w1"] || !seen["hp-w2"] {
		t.Errorf("weapon IDs not numbered from 1: %v", seen)
	}
}

func TestHardpointsSitInsideTheHull(t *testing.T) {
	s := Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4,
		Weapon: 2, Defense: 2, Utility: 8}
	d := Infer(s)
	for _, h := range Hardpoints(d, s) {
		w := hullHalfWidth(d, h.Pos.X)
		if math.Abs(h.Pos.Y) > w+1e-9 {
			t.Errorf("%s at Y=%v exceeds half-width %v at X=%v", h.ID, h.Pos.Y, w, h.Pos.X)
		}
	}
}

func TestHardpointsZeroSlotsProducesNone(t *testing.T) {
	s := Stats{ID: "prayer", Class: "Freighter", Faction: "outerrim", Scale: 1}
	if hps := Hardpoints(Infer(s), s); len(hps) != 0 {
		t.Errorf("len = %d, want 0 for a ship with no slots", len(hps))
	}
}

func TestHardpointsAreDeterministic(t *testing.T) {
	s := Stats{ID: "superposition", Class: "Drone Carrier", Faction: "voidborn", Scale: 4,
		Weapon: 2, Defense: 5, Utility: 6}
	d := Infer(s)
	a, b := Hardpoints(d, s), Hardpoints(d, s)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("hardpoint %d diverged: %+v vs %+v", i, a[i], b[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestHardpoint -v`
Expected: FAIL — `undefined: Hardpoints`.

- [ ] **Step 3: Write the placer**

Create `pkg/shipglyph/hardpoints.go`:

```go
package shipglyph

import "fmt"

// Hardpoint is a module mounting marker on the hull. Slot counts are
// authoritative from the catalog; the descriptor only supplies the zones
// along which markers may be placed.
type Hardpoint struct {
	// ID is the stable SVG element ID, e.g. "hp-w1".
	ID string
	// Kind is "weapon", "defense" or "utility".
	Kind string
	// Pos is the marker position in glyph space.
	Pos Point
}

// Hardpoints distributes one marker per slot along the descriptor's mount
// zones. Markers alternate starboard and port so pairs read as symmetric
// mounts, and are inset from the hull edge so they always sit inside the
// outline.
func Hardpoints(d Descriptor, s Stats) []Hardpoint {
	var out []Hardpoint
	out = append(out, placeKind(d, "weapon", "w", s.Weapon, d.MountZones.Weapon)...)
	out = append(out, placeKind(d, "defense", "d", s.Defense, d.MountZones.Defense)...)
	out = append(out, placeKind(d, "utility", "u", s.Utility, d.MountZones.Utility)...)
	return out
}

// hardpointInset keeps markers clear of the hull edge, as a fraction of the
// local half-width.
const hardpointInset = 0.55

// placeKind spreads n markers evenly across the given zones.
func placeKind(d Descriptor, kind, prefix string, n int, zones [][2]float64) []Hardpoint {
	if n <= 0 || len(zones) == 0 {
		return nil
	}
	out := make([]Hardpoint, 0, n)
	for i := range n {
		// Spread across the concatenated zones by index.
		z := zones[i%len(zones)]
		var f float64
		if n == 1 {
			f = 0.5
		} else {
			f = float64(i) / float64(n-1)
		}
		t := z[0] + f*(z[1]-z[0])

		side := 1.0
		if i%2 == 1 {
			side = -1.0
		}
		// A lone marker sits on the centerline rather than off to one side.
		y := 0.0
		if n > 1 {
			y = side * hullHalfWidth(d, t) * hardpointInset
		}

		out = append(out, Hardpoint{
			ID:   fmt.Sprintf("hp-%s%d", prefix, i+1),
			Kind: kind,
			Pos:  Point{X: t, Y: y},
		})
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -run TestHardpoint -v`
Expected: PASS.

- [ ] **Step 5: Write the failing appendage test**

`Infer` already produces wings, nacelles, tow arms and drone racks, but nothing
draws them yet. Create `pkg/shipglyph/appendages_test.go`:

```go
package shipglyph

import "testing"

func TestAppendageShapesBothSidesProduceTwoShapes(t *testing.T) {
	d := Descriptor{
		Aspect:     4,
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "wing", At: 0.6, Sweep: 40, Span: 0.4, Side: "both"}},
	}
	got := AppendageShapes(d)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (one per side)", len(got))
	}
	if got[0].ID == got[1].ID {
		t.Errorf("both shapes share ID %q", got[0].ID)
	}
	var sawPos, sawNeg bool
	for _, sh := range got {
		for _, p := range sh.Poly {
			if p.Y > 1e-9 {
				sawPos = true
			}
			if p.Y < -1e-9 {
				sawNeg = true
			}
		}
	}
	if !sawPos || !sawNeg {
		t.Errorf("appendages do not appear on both sides")
	}
}

func TestAppendageShapesSingleSide(t *testing.T) {
	d := Descriptor{
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "drone_rack", At: 0.5, Span: 0.2, Side: "port"}},
	}
	got := AppendageShapes(d)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	for _, p := range got[0].Poly {
		if p.Y > 1e-9 {
			t.Errorf("port appendage has positive Y %v", p.Y)
		}
	}
}

func TestAppendageShapesExtendBeyondTheHull(t *testing.T) {
	d := Descriptor{
		Hull:       []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
		Appendages: []Appendage{{Kind: "wing", At: 0.5, Sweep: 30, Span: 0.5, Side: "both"}},
	}
	var maxY float64
	for _, sh := range AppendageShapes(d) {
		for _, p := range sh.Poly {
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}
	if maxY <= 0.2 {
		t.Errorf("max Y = %v, want beyond the 0.2 hull half-width", maxY)
	}
}

func TestAppendageShapesIDsAreStableAndUnique(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	seen := map[string]bool{}
	for _, sh := range AppendageShapes(d) {
		if seen[sh.ID] {
			t.Errorf("duplicate appendage ID %q", sh.ID)
		}
		seen[sh.ID] = true
	}
	if len(seen) == 0 {
		t.Errorf("a Nebula liner should have wings")
	}
}

func TestAppendageShapesNoneIsEmpty(t *testing.T) {
	d := Descriptor{Hull: []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}}}
	if got := AppendageShapes(d); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestAppendage -v`
Expected: FAIL — `undefined: AppendageShapes`.

- [ ] **Step 7: Write the appendage geometry**

Create `pkg/shipglyph/appendages.go`:

```go
package shipglyph

import (
	"fmt"
	"math"
)

// AppendageShape is a closed polygon for one hull appendage, on one side.
type AppendageShape struct {
	// ID is the stable SVG element ID, e.g. "ap-wing-1p".
	ID string
	// Kind mirrors the descriptor's Appendage.Kind.
	Kind string
	// Poly is the closed outline in glyph space.
	Poly []Point
}

// AppendageShapes builds polygons for every appendage in the descriptor. A
// "both" appendage yields two shapes, one per side.
func AppendageShapes(d Descriptor) []AppendageShape {
	var out []AppendageShape
	for i, a := range d.Appendages {
		for _, side := range sidesOf(a.Side) {
			suffix := "s"
			if side < 0 {
				suffix = "p"
			}
			out = append(out, AppendageShape{
				ID:   fmt.Sprintf("ap-%s-%d%s", a.Kind, i+1, suffix),
				Kind: a.Kind,
				Poly: appendagePoly(d, a, side),
			})
		}
	}
	return out
}

// sidesOf expands an Appendage.Side value into the sides it occupies.
func sidesOf(side string) []float64 {
	switch side {
	case "port":
		return []float64{-1}
	case "starboard":
		return []float64{1}
	default:
		return []float64{1, -1}
	}
}

// appendagePoly returns the closed outline of one appendage on one side. Every
// kind is a quadrilateral rooted on the hull edge; only the outboard corners
// differ, which keeps the shapes readable at glyph size.
func appendagePoly(d Descriptor, a Appendage, side float64) []Point {
	root := hullHalfWidth(d, a.At)
	span := a.Span
	if span <= 0 {
		span = 0.2
	}

	// Sweep converts to how far aft the outboard edge trails the root.
	trail := span * math.Tan(a.Sweep*math.Pi/180)

	var chordFwd, chordAft, tipFwd, tipAft float64
	switch a.Kind {
	case "wing":
		chordFwd, chordAft = 0.06, 0.14
		tipFwd, tipAft = 0.02, 0.05
	case "nacelle":
		chordFwd, chordAft = 0.10, 0.10
		tipFwd, tipAft = 0.08, 0.08
	case "sponson":
		chordFwd, chordAft = 0.05, 0.05
		tipFwd, tipAft = 0.04, 0.04
	case "drone_rack":
		chordFwd, chordAft = 0.16, 0.16
		tipFwd, tipAft = 0.14, 0.14
	case "tow_arm", "boom", "outrigger":
		chordFwd, chordAft = 0.03, 0.03
		tipFwd, tipAft = 0.02, 0.02
	case "antenna_mast":
		chordFwd, chordAft = 0.02, 0.02
		tipFwd, tipAft = 0.005, 0.005
	default:
		chordFwd, chordAft = 0.06, 0.06
		tipFwd, tipAft = 0.04, 0.04
	}

	inner := root * 0.9 * side
	outer := (root + span) * side

	return []Point{
		{X: a.At - chordFwd, Y: inner},
		{X: a.At + chordAft, Y: inner},
		{X: a.At + trail + tipAft, Y: outer},
		{X: a.At + trail - tipFwd, Y: outer},
	}
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

- [ ] **Step 9: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): appendage polygons and deterministic hardpoint placement"
```

---

### Task 7: Region partition

**Files:**
- Create: `pkg/shipglyph/regions.go`
- Test: `pkg/shipglyph/regions_test.go`

**Interfaces:**
- Consumes: `Descriptor`, `Style`, `Point` (Tasks 1, 3); `sampleProfile`, `profileSamples`, `hullHalfWidth` (Task 4).
- Produces: `RegionNames` slice; `Regions(d Descriptor, st Style, seed uint64) map[string][]Point`.

Regions are built from the sampled profiles rather than by clipping the finished outline: bow is `t < 0.25` across both sides, stern is `t >= 0.75` across both sides, port and starboard cover `0.25 <= t < 0.75` on their own side down to the centerline, and core is a narrow centerline inset spanning the middle. Element IDs are `region-bow`, `region-port`, `region-star`, `region-stern`, `region-core`.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/regions_test.go`:

```go
package shipglyph

import "testing"

func TestRegionsCoverAllFiveNames(t *testing.T) {
	d := Infer(Stats{ID: "war_wagon", Class: "Bulk Hauler", Faction: "crimson", Scale: 4})
	got := Regions(d, StyleFor("crimson"), SeedOf("war_wagon"))

	if len(RegionNames) != 5 {
		t.Fatalf("RegionNames has %d entries, want 5", len(RegionNames))
	}
	for _, name := range RegionNames {
		poly, ok := got[name]
		if !ok {
			t.Errorf("missing region %q", name)
			continue
		}
		if len(poly) < 3 {
			t.Errorf("region %q has %d points, want at least 3", name, len(poly))
		}
	}
}

func TestRegionsBowIsForwardAndSternIsAft(t *testing.T) {
	d := Infer(Stats{ID: "comet", Class: "Liner", Faction: "nebula", Scale: 4})
	got := Regions(d, StyleFor("nebula"), SeedOf("comet"))

	for _, p := range got["bow"] {
		if p.X > 0.26 {
			t.Errorf("bow point at X=%v, want <= 0.25", p.X)
		}
	}
	for _, p := range got["stern"] {
		if p.X < 0.74 {
			t.Errorf("stern point at X=%v, want >= 0.75", p.X)
		}
	}
}

func TestRegionsPortAndStarboardAreOnOpposingSides(t *testing.T) {
	d := Infer(Stats{ID: "crowbar", Class: "Salvager", Faction: "crimson", Scale: 1})
	got := Regions(d, StyleFor("crimson"), SeedOf("crowbar"))

	for _, p := range got["star"] {
		if p.Y < -1e-9 {
			t.Errorf("starboard point has negative Y %v", p.Y)
		}
	}
	for _, p := range got["port"] {
		if p.Y > 1e-9 {
			t.Errorf("port point has positive Y %v", p.Y)
		}
	}
}

func TestRegionsAreDeterministic(t *testing.T) {
	d := Infer(Stats{ID: "yard_sale", Class: "Salvager", Faction: "outerrim", Scale: 3})
	st := StyleFor("outerrim")
	a := Regions(d, st, SeedOf("yard_sale"))
	b := Regions(d, st, SeedOf("yard_sale"))
	for _, name := range RegionNames {
		if len(a[name]) != len(b[name]) {
			t.Fatalf("region %q length diverged", name)
		}
		for i := range a[name] {
			if a[name][i] != b[name][i] {
				t.Fatalf("region %q point %d diverged", name, i)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestRegions -v`
Expected: FAIL — `undefined: Regions`, `undefined: RegionNames`.

- [ ] **Step 3: Write the partitioner**

Create `pkg/shipglyph/regions.go`:

```go
package shipglyph

// RegionNames are the five damage-diagram regions, in the order the hull pip
// track threads through them.
var RegionNames = []string{"bow", "port", "star", "stern", "core"}

// Region boundaries along the spine.
const (
	bowEnd     = 0.25
	sternStart = 0.75
	coreInset  = 0.45 // core half-width as a fraction of the local hull
)

// Regions partitions the hull into the five diagram regions. Each region is a
// closed polygon in glyph space, suitable for emitting as its own <path> with
// a stable element ID.
func Regions(d Descriptor, st Style, seed uint64) map[string][]Point {
	star := sampleProfile(d, st, seed, 1)
	port := sampleProfile(d, st, seed, -1)

	out := map[string][]Point{
		"bow":   spanPolygon(star, port, 0, bowEnd),
		"stern": spanPolygon(star, port, sternStart, 1),
		"star":  sidePolygon(star, bowEnd, sternStart),
		"port":  sidePolygon(port, bowEnd, sternStart),
		"core":  corePolygon(d, star, bowEnd, sternStart),
	}
	return out
}

// spanPolygon returns the full-width slice of the hull between lo and hi:
// starboard edge forward to aft, then port edge back.
func spanPolygon(star, port []Point, lo, hi float64) []Point {
	var out []Point
	for _, p := range star {
		if p.X >= lo && p.X <= hi {
			out = append(out, p)
		}
	}
	for i := len(port) - 1; i >= 0; i-- {
		if port[i].X >= lo && port[i].X <= hi {
			out = append(out, port[i])
		}
	}
	return out
}

// sidePolygon returns one side's slice between lo and hi, closed along the
// centerline.
func sidePolygon(side []Point, lo, hi float64) []Point {
	var out []Point
	for _, p := range side {
		if p.X >= lo && p.X <= hi {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return out
	}
	// Close back along the centerline.
	out = append(out, Point{X: out[len(out)-1].X, Y: 0}, Point{X: out[0].X, Y: 0})
	return out
}

// corePolygon returns the narrow centerline spine between lo and hi.
func corePolygon(d Descriptor, star []Point, lo, hi float64) []Point {
	var upper, lower []Point
	for _, p := range star {
		if p.X < lo || p.X > hi {
			continue
		}
		w := hullHalfWidth(d, p.X) * coreInset
		upper = append(upper, Point{X: p.X, Y: w})
		lower = append(lower, Point{X: p.X, Y: -w})
	}
	out := upper
	for i := len(lower) - 1; i >= 0; i-- {
		out = append(out, lower[i])
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): five-region hull partition for the damage diagram"
```

---

### Task 8: SVG rendering with stable element IDs

**Files:**
- Create: `pkg/shipglyph/render.go`
- Test: `pkg/shipglyph/render_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–7.
- Produces: `Options` struct with fields `Size float64`, `ShowHardpoints bool`, `Title string`; `Render(d Descriptor, s Stats, opts Options) string`.

The glyph is drawn nose-up: glyph `t` maps to the SVG Y axis, glyph `y` maps to the SVG X axis, matching the AeroTech reference where the ship points up the page. The viewBox is always `0 0 Size Size` so every glyph drops into a uniform grid cell.

- [ ] **Step 1: Write the failing test**

Create `pkg/shipglyph/render_test.go`:

```go
package shipglyph

import (
	"strings"
	"testing"
)

func renderFixture(t *testing.T, s Stats) string {
	t.Helper()
	d := Infer(s)
	return Render(d, s, Options{Size: 200, ShowHardpoints: true, Title: s.Name})
}

func TestRenderProducesWellFormedSVG(t *testing.T) {
	out := renderFixture(t, Stats{ID: "war_wagon", Name: "War Wagon",
		Class: "Bulk Hauler", Faction: "crimson", Scale: 4, Weapon: 2, Defense: 2, Utility: 8})

	if !strings.HasPrefix(out, "<svg") {
		t.Errorf("output does not start with <svg>:\n%s", out[:min(120, len(out))])
	}
	if !strings.HasSuffix(strings.TrimSpace(out), "</svg>") {
		t.Errorf("output does not end with </svg>")
	}
	if !strings.Contains(out, `viewBox="0 0 200 200"`) {
		t.Errorf("missing or wrong viewBox")
	}
	if !strings.Contains(out, "<title>War Wagon</title>") {
		t.Errorf("missing accessible title")
	}
}

func TestRenderEmitsAllStableRegionIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	for _, name := range RegionNames {
		want := `id="region-` + name + `"`
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if !strings.Contains(out, `id="hull"`) {
		t.Errorf(`missing id="hull" group`)
	}
	if !strings.Contains(out, `id="hardpoints"`) {
		t.Errorf(`missing id="hardpoints" group`)
	}
}

func TestRenderDrawsAppendages(t *testing.T) {
	// A Nebula liner is inferred with swept wings; they must reach the SVG.
	out := renderFixture(t, Stats{ID: "comet", Name: "Comet",
		Class: "Liner", Faction: "nebula", Scale: 4, Defense: 4, Utility: 5})

	if !strings.Contains(out, `id="appendages"`) {
		t.Errorf(`missing id="appendages" group`)
	}
	if !strings.Contains(out, `id="ap-wing-1s"`) || !strings.Contains(out, `id="ap-wing-1p"`) {
		t.Errorf("missing per-side wing IDs")
	}
}

func TestRenderOmitsAppendageGroupWhenThereAreNone(t *testing.T) {
	d := Descriptor{
		Aspect: 3,
		Hull:   []HullPart{{Kind: "box", Span: [2]float64{0, 1}, Half: 0.2}},
	}
	out := Render(d, Stats{ID: "bare", Name: "Bare", Faction: "crimson"}, Options{Size: 200})

	if strings.Contains(out, `id="appendages"`) {
		t.Errorf("emitted an empty appendages group")
	}
}

func TestRenderEmitsHardpointIDs(t *testing.T) {
	out := renderFixture(t, Stats{ID: "magnate", Name: "Magnate",
		Class: "Command", Faction: "solarian", Scale: 4, Weapon: 3, Defense: 6, Utility: 5})

	for _, id := range []string{"hp-w1", "hp-w3", "hp-d6", "hp-u5"} {
		if !strings.Contains(out, `id="`+id+`"`) {
			t.Errorf("missing hardpoint %s", id)
		}
	}
}

func TestRenderUsesCurrentColorNotHardcodedColors(t *testing.T) {
	out := renderFixture(t, Stats{ID: "paradox", Name: "Paradox",
		Class: "Fighter", Faction: "voidborn", Scale: 1, Weapon: 2, Defense: 2, Utility: 1})

	if !strings.Contains(out, "currentColor") {
		t.Errorf("expected currentColor strokes for theme compatibility")
	}
	if strings.Contains(out, "#") || strings.Contains(out, "hsl(") {
		t.Errorf("glyph hardcodes a color; it must inherit from CSS:\n%s", out)
	}
}

func TestRenderIsByteIdenticalAcrossRuns(t *testing.T) {
	s := Stats{ID: "yard_sale", Name: "Yard Sale",
		Class: "Salvager", Faction: "outerrim", Scale: 3, Defense: 1, Utility: 4}
	if renderFixture(t, s) != renderFixture(t, s) {
		t.Errorf("Render is not deterministic")
	}
}

func TestRenderHandlesZeroSlotShip(t *testing.T) {
	out := renderFixture(t, Stats{ID: "prayer", Name: "Prayer",
		Class: "Freighter", Faction: "outerrim", Scale: 1})

	if !strings.Contains(out, `id="hull"`) {
		t.Errorf("hull missing for a zero-slot ship")
	}
	if strings.Contains(out, `id="hp-`) {
		t.Errorf("zero-slot ship should have no hardpoint markers")
	}
}

func TestRenderEveryFactionProducesOutput(t *testing.T) {
	for _, f := range []string{"crimson", "nebula", "solarian", "outerrim", "voidborn", "pirate", ""} {
		s := Stats{ID: "x_" + f, Name: "X", Class: "Cruiser", Faction: f, Scale: 3, Weapon: 2}
		out := renderFixture(t, s)
		if !strings.Contains(out, "<path") {
			t.Errorf("faction %q produced no paths", f)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/shipglyph/ -run TestRender -v`
Expected: FAIL — `undefined: Render`, `undefined: Options`.

- [ ] **Step 3: Write the renderer**

Create `pkg/shipglyph/render.go`:

```go
package shipglyph

import (
	"fmt"
	"html"
	"strings"
)

// Options controls glyph rendering.
type Options struct {
	// Size is the square viewBox edge length in SVG user units.
	Size float64
	// ShowHardpoints emits the module mounting markers.
	ShowHardpoints bool
	// Title is the accessible title, normally the ship's display name.
	Title string
}

// glyphMargin is the fraction of Size left empty around the hull.
const glyphMargin = 0.08

// Render returns a self-contained inline SVG for the ship, drawn nose-up.
// Strokes use currentColor so the KB theme controls the ink. Element IDs are
// stable across regenerations so consumers can paint state onto them.
func Render(d Descriptor, s Stats, opts Options) string {
	if opts.Size <= 0 {
		opts.Size = 200
	}
	st := StyleFor(s.Faction)
	seed := SeedOf(s.ID)

	// The hull occupies the full length; beam is length divided by aspect.
	aspect := d.Aspect
	if aspect <= 0 {
		aspect = 3
	}
	usable := opts.Size * (1 - 2*glyphMargin)
	length := usable
	beam := usable / aspect
	cx := opts.Size / 2
	top := opts.Size * glyphMargin

	// project maps glyph space to SVG user space, nose at the top.
	project := func(p Point) (float64, float64) {
		return cx + p.Y*beam, top + p.X*length
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="ship-glyph" viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg" role="img">`,
		opts.Size, opts.Size)
	if opts.Title != "" {
		fmt.Fprintf(&b, `<title>%s</title>`, html.EscapeString(opts.Title))
	}
	b.WriteString(`<g id="hull" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">`)

	regions := Regions(d, st, seed)
	for _, name := range RegionNames {
		poly := regions[name]
		if len(poly) < 3 {
			continue
		}
		fmt.Fprintf(&b, `<path id="region-%s" class="glyph-region" d="%s"/>`,
			name, pathData(poly, project))
	}

	// The full outline is drawn last so it reads as the dominant contour.
	outline := Outline(d, st, seed)
	if len(outline) >= 3 {
		fmt.Fprintf(&b, `<path id="region-outline" class="glyph-outline" stroke-width="1.8" d="%s"/>`,
			pathData(outline, project))
	}
	b.WriteString(`</g>`)

	// Appendages sit outside the hull group so they can be styled separately
	// and are never mistaken for damage regions.
	if shapes := AppendageShapes(d); len(shapes) > 0 {
		b.WriteString(`<g id="appendages" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linejoin="round">`)
		for _, sh := range shapes {
			fmt.Fprintf(&b, `<path id="%s" class="glyph-appendage glyph-ap-%s" d="%s"/>`,
				sh.ID, sh.Kind, pathData(sh.Poly, project))
		}
		b.WriteString(`</g>`)
	}

	if opts.ShowHardpoints {
		hps := Hardpoints(d, s)
		if len(hps) > 0 {
			b.WriteString(`<g id="hardpoints" fill="none" stroke="currentColor" stroke-width="1">`)
			for _, h := range hps {
				x, y := project(h.Pos)
				fmt.Fprintf(&b, `<circle id="%s" class="glyph-hp glyph-hp-%s" cx="%.2f" cy="%.2f" r="%.2f"/>`,
					h.ID, h.Kind, x, y, opts.Size*0.018)
			}
			b.WriteString(`</g>`)
		}
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// pathData converts a closed polygon in glyph space to an SVG path.
func pathData(poly []Point, project func(Point) (float64, float64)) string {
	var b strings.Builder
	for i, p := range poly {
		x, y := project(p)
		verb := "L"
		if i == 0 {
			verb = "M"
		}
		fmt.Fprintf(&b, "%s%.2f %.2f", verb, x, y)
		if i < len(poly)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("Z")
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./pkg/shipglyph/ -v`
Expected: PASS.

- [ ] **Step 5: Lint and commit**

```bash
golangci-lint run ./pkg/shipglyph/
git add pkg/shipglyph/
git commit -m "feat(shipglyph): SVG renderer with stable region and hardpoint IDs"
```

---

### Task 9: Generator command writing one SVG per ship

**Files:**
- Create: `cmd/generate-ship-glyphs/main.go`
- Create: `cmd/generate-ship-glyphs/catalog.go`
- Test: `cmd/generate-ship-glyphs/catalog_test.go`

**Interfaces:**
- Consumes: `shipglyph.Stats`, `Infer`, `LoadOverlay`, `Merge`, `Render`, `Options`.
- Produces: `catalogShip` struct; `loadShipCatalog(path string) ([]catalogShip, error)`; `toStats(c catalogShip) shipglyph.Stats`.

Follows the `cmd/generate-items-kb` convention: paths relative to the repo root, `../spacemolt/data/game-api/latest/catalog_ships.json`.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-ship-glyphs/catalog_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadShipCatalogParsesItemsWrapper(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog_ships.json")
	body := `{"items":[
	  {"id":"prayer","name":"Prayer","class":"Freighter","category":"Commercial",
	   "faction":"outerrim","tier":1,"scale":1,"weapon_slots":0,"defense_slots":0,
	   "utility_slots":0,"cargo_capacity":540,"lore":"cargo containers welded to an engine"},
	  {"id":"comet","name":"Comet","class":"Liner","category":"Civilian",
	   "faction":"nebula","tier":4,"scale":4,"weapon_slots":0,"defense_slots":4,
	   "utility_slots":5,"cargo_capacity":40,"lore":"one class of service"}
	]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	ships, err := loadShipCatalog(path)
	if err != nil {
		t.Fatalf("loadShipCatalog: %v", err)
	}
	if len(ships) != 2 {
		t.Fatalf("len = %d, want 2", len(ships))
	}
	if ships[0].ID != "prayer" || ships[0].CargoCapacity != 540 {
		t.Errorf("first ship = %+v", ships[0])
	}
}

func TestToStatsMapsSlotFields(t *testing.T) {
	c := catalogShip{
		ID: "magnate", Name: "Magnate", Class: "Command", Category: "Combat Support",
		Faction: "solarian", Tier: 4, Scale: 4,
		WeaponSlots: 3, DefenseSlots: 6, UtilitySlots: 5, CargoCapacity: 300,
	}
	s := toStats(c)

	if s.Weapon != 3 || s.Defense != 6 || s.Utility != 5 {
		t.Errorf("slots = %d/%d/%d, want 3/6/5", s.Weapon, s.Defense, s.Utility)
	}
	if s.ID != "magnate" || s.Name != "Magnate" || s.Faction != "solarian" {
		t.Errorf("identity fields wrong: %+v", s)
	}
	if s.Cargo != 300 {
		t.Errorf("Cargo = %d, want 300", s.Cargo)
	}
}

func TestLoadShipCatalogMissingFileErrors(t *testing.T) {
	if _, err := loadShipCatalog(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Errorf("expected an error for a missing catalog")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-ship-glyphs/ -v`
Expected: FAIL — no Go files in the directory.

- [ ] **Step 3: Write the catalog loader**

Create `cmd/generate-ship-glyphs/catalog.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

// catalogShip is the subset of catalog_ships.json needed to build a glyph.
type catalogShip struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Category      string `json:"category"`
	Faction       string `json:"faction"`
	Tier          int    `json:"tier"`
	Scale         int    `json:"scale"`
	WeaponSlots   int    `json:"weapon_slots"`
	DefenseSlots  int    `json:"defense_slots"`
	UtilitySlots  int    `json:"utility_slots"`
	CargoCapacity int    `json:"cargo_capacity"`
	Lore          string `json:"lore"`
}

// loadShipCatalog reads the {"items": [...]} catalog produced by the game API.
func loadShipCatalog(path string) ([]catalogShip, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read ship catalog: %w", err)
	}
	var catalog struct {
		Items []catalogShip `json:"items"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse ship catalog: %w", err)
	}
	return catalog.Items, nil
}

// toStats projects a catalog ship onto the shape-inference input.
func toStats(c catalogShip) shipglyph.Stats {
	return shipglyph.Stats{
		ID:       c.ID,
		Name:     c.Name,
		Class:    c.Class,
		Category: c.Category,
		Faction:  c.Faction,
		Tier:     c.Tier,
		Scale:    c.Scale,
		Weapon:   c.WeaponSlots,
		Defense:  c.DefenseSlots,
		Utility:  c.UtilitySlots,
		Cargo:    c.CargoCapacity,
	}
}
```

- [ ] **Step 4: Write the command entry point**

Create `cmd/generate-ship-glyphs/main.go`:

```go
// Command generate-ship-glyphs renders a top-down blueprint SVG for every ship
// in the catalog, plus a contact sheet page for reviewing them all at once.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

func main() {
	catalogPath := flag.String("catalog", "../spacemolt/data/game-api/latest/catalog_ships.json",
		"path to catalog_ships.json")
	overlayDir := flag.String("overlays", "overlays/shipshapes",
		"directory of hand-authored shape overlays (empty to disable)")
	outDir := flag.String("out", "kb/ships/glyphs", "output directory for glyph SVGs")
	size := flag.Float64("size", 200, "glyph viewBox edge length")
	flag.Parse()

	ships, err := loadShipCatalog(*catalogPath)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	rendered := make([]renderedGlyph, 0, len(ships))
	var overlaid int

	for _, c := range ships {
		s := toStats(c)
		d := shipglyph.Infer(s)

		over, ok, err := shipglyph.LoadOverlay(*overlayDir, c.ID)
		if err != nil {
			log.Fatalf("overlay for %s: %v", c.ID, err)
		}
		if ok {
			d = shipglyph.Merge(d, over)
			overlaid++
		}

		svg := shipglyph.Render(d, s, shipglyph.Options{
			Size:           *size,
			ShowHardpoints: true,
			Title:          c.Name,
		})

		path := filepath.Join(*outDir, c.ID+".svg")
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		rendered = append(rendered, renderedGlyph{Stats: s, SVG: svg})
	}

	if err := writeContactSheet(*outDir, rendered); err != nil {
		log.Fatalf("write contact sheet: %v", err)
	}

	fmt.Printf("Generated %d ship glyphs (%d with overlays) in %s/\n",
		len(rendered), overlaid, *outDir)
}
```

This references `renderedGlyph` and `writeContactSheet`, which Task 10 creates. Until then the package will not build — that is expected and is why Steps 5 and 6 below defer the full build check to Task 10.

- [ ] **Step 5: Run the catalog tests**

Run: `go vet ./cmd/generate-ship-glyphs/ 2>&1 | head`
Expected: errors naming `renderedGlyph` and `writeContactSheet` as undefined, and nothing else. Any *other* undefined symbol is a real mistake to fix now.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-ship-glyphs/
git commit -m "feat(generate-ship-glyphs): catalog loader and generator entry point"
```

---

### Task 10: Contact sheet page and styling

**Files:**
- Create: `cmd/generate-ship-glyphs/contactsheet.go`
- Create: `kb/ships/glyphs/glyphs.css`
- Test: `cmd/generate-ship-glyphs/contactsheet_test.go`

**Interfaces:**
- Consumes: `shipglyph.Stats` (Task 2); `toStats` (Task 9).
- Produces: `renderedGlyph` struct with fields `Stats shipglyph.Stats` and `SVG string`; `writeContactSheet(outDir string, glyphs []renderedGlyph) error`; `groupByFaction(glyphs []renderedGlyph) []factionGroup`.

The page follows the existing KB shell: `<link rel="stylesheet" href="../../smui.css">` plus a page-specific stylesheet, a `site-header` with a `theme-toggle` button. Glyph strokes inherit `currentColor`, so the theme handles light and dark with no per-glyph work.

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-ship-glyphs/contactsheet_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

func fixtureGlyphs() []renderedGlyph {
	return []renderedGlyph{
		{Stats: shipglyph.Stats{ID: "crowbar", Name: "Crowbar", Class: "Salvager", Faction: "crimson"},
			SVG: `<svg class="ship-glyph"><title>Crowbar</title></svg>`},
		{Stats: shipglyph.Stats{ID: "comet", Name: "Comet", Class: "Liner", Faction: "nebula"},
			SVG: `<svg class="ship-glyph"><title>Comet</title></svg>`},
		{Stats: shipglyph.Stats{ID: "war_wagon", Name: "War Wagon", Class: "Bulk Hauler", Faction: "crimson"},
			SVG: `<svg class="ship-glyph"><title>War Wagon</title></svg>`},
		{Stats: shipglyph.Stats{ID: "nofaction", Name: "No Faction", Class: "Cruiser", Faction: ""},
			SVG: `<svg class="ship-glyph"><title>No Faction</title></svg>`},
	}
}

func TestGroupByFactionIsSortedAndComplete(t *testing.T) {
	groups := groupByFaction(fixtureGlyphs())

	var total int
	for _, g := range groups {
		total += len(g.Glyphs)
		if g.Name == "" {
			t.Errorf("group has an empty display name")
		}
	}
	if total != 4 {
		t.Errorf("grouped %d glyphs, want 4", total)
	}

	// Deterministic ordering: same input must yield the same group order.
	again := groupByFaction(fixtureGlyphs())
	for i := range groups {
		if groups[i].Name != again[i].Name {
			t.Fatalf("group order is not deterministic at %d: %q vs %q",
				i, groups[i].Name, again[i].Name)
		}
	}
}

func TestWriteContactSheetProducesPage(t *testing.T) {
	dir := t.TempDir()
	if err := writeContactSheet(dir, fixtureGlyphs()); err != nil {
		t.Fatalf("writeContactSheet: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("read page: %v", err)
	}
	page := string(data)

	for _, want := range []string{
		"<!DOCTYPE html>",
		`href="../../smui.css"`,
		`href="glyphs.css"`,
		`class="theme-toggle"`,
		"Crowbar",
		"War Wagon",
		`class="ship-glyph"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %q", want)
		}
	}
}

func TestWriteContactSheetEscapesNames(t *testing.T) {
	dir := t.TempDir()
	glyphs := []renderedGlyph{{
		Stats: shipglyph.Stats{ID: "x", Name: `Ship <script>alert(1)</script>`, Class: "Cruiser", Faction: "crimson"},
		SVG:   `<svg class="ship-glyph"></svg>`,
	}}
	if err := writeContactSheet(dir, glyphs); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if strings.Contains(string(data), "<script>alert(1)</script>") {
		t.Errorf("ship name was not HTML-escaped")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-ship-glyphs/ -run TestGroupByFaction -v`
Expected: FAIL — `undefined: renderedGlyph`, `undefined: groupByFaction`, `undefined: writeContactSheet`.

- [ ] **Step 3: Write the contact sheet generator**

Create `cmd/generate-ship-glyphs/contactsheet.go`:

```go
package main

import (
	htmltpl "html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

// renderedGlyph pairs a ship's stats with its finished SVG markup.
type renderedGlyph struct {
	Stats shipglyph.Stats
	SVG   string
}

// factionGroup is one section of the contact sheet.
type factionGroup struct {
	Key    string
	Name   string
	Glyphs []renderedGlyph
}

// factionDisplay maps catalog faction keys to display names. Ships with no
// faction are grouped under "Unaligned".
var factionDisplay = map[string]string{
	"crimson":  "Crimson Fleet",
	"nebula":   "Nebula Federation",
	"solarian": "Solarian Confederacy",
	"outerrim": "Outer Rim",
	"voidborn": "Voidborn Collective",
	"pirate":   "Pirate",
	"":         "Unaligned",
}

// factionOrder fixes section ordering so regenerating produces clean diffs.
var factionOrder = []string{"crimson", "nebula", "solarian", "outerrim", "voidborn", "pirate", ""}

// groupByFaction buckets glyphs into ordered sections, sorting ships within a
// section by class then name so the layout is stable across runs.
func groupByFaction(glyphs []renderedGlyph) []factionGroup {
	buckets := map[string][]renderedGlyph{}
	for _, g := range glyphs {
		buckets[g.Stats.Faction] = append(buckets[g.Stats.Faction], g)
	}

	// Any faction key not in factionOrder is appended alphabetically, so a
	// new faction in the catalog still renders.
	order := slices.Clone(factionOrder)
	var extra []string
	for k := range buckets {
		if !slices.Contains(order, k) {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	order = append(order, extra...)

	var out []factionGroup
	for _, key := range order {
		items := buckets[key]
		if len(items) == 0 {
			continue
		}
		slices.SortFunc(items, func(a, b renderedGlyph) int {
			if c := strings.Compare(a.Stats.Class, b.Stats.Class); c != 0 {
				return c
			}
			return strings.Compare(a.Stats.Name, b.Stats.Name)
		})
		name, ok := factionDisplay[key]
		if !ok || name == "" {
			name = key
		}
		out = append(out, factionGroup{Key: key, Name: name, Glyphs: items})
	}
	return out
}

// contactSheetTmpl renders the whole page. Ship names go through the template's
// automatic escaping; SVG markup is marked trusted because it is built entirely
// from literals and formatted numbers by pkg/shipglyph.
var contactSheetTmpl = htmltpl.Must(htmltpl.New("contactsheet").Funcs(htmltpl.FuncMap{
	"svg": func(s string) htmltpl.HTML { return htmltpl.HTML(s) }, //nolint:gosec // generated by pkg/shipglyph from literals
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Ship Glyph Contact Sheet</title>
<link rel="stylesheet" href="../../smui.css">
<link rel="stylesheet" href="glyphs.css">
</head>
<body>
<header class="site-header">
  <nav class="breadcrumb"><a href="../../index.html">KB</a> / <a href="../index.html">Ships</a> / Glyphs</nav>
  <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">◐</button>
</header>
<main class="glyph-sheet">
<h1>Ship Glyph Contact Sheet</h1>
<p class="glyph-intro">{{.Total}} ships, grouped by faction. Top-down schematic outlines generated from class archetype, faction design language and slot counts.</p>
{{range .Groups}}
<section class="glyph-group" id="faction-{{.Key}}">
  <h2>{{.Name}} <span class="glyph-count">{{len .Glyphs}}</span></h2>
  <div class="glyph-grid">
  {{range .Glyphs}}
    <figure class="glyph-cell">
      {{svg .SVG}}
      <figcaption><span class="glyph-name">{{.Stats.Name}}</span><span class="glyph-class">{{.Stats.Class}}</span></figcaption>
    </figure>
  {{end}}
  </div>
</section>
{{end}}
</main>
<script>
(function () {
  var toggle = document.getElementById('theme-toggle');
  if (!toggle) return;
  toggle.addEventListener('click', function () {
    var root = document.documentElement;
    root.dataset.theme = root.dataset.theme === 'dark' ? 'light' : 'dark';
  });
})();
</script>
</body>
</html>
`))

// writeContactSheet renders index.html into outDir.
func writeContactSheet(outDir string, glyphs []renderedGlyph) error {
	data := struct {
		Total  int
		Groups []factionGroup
	}{
		Total:  len(glyphs),
		Groups: groupByFaction(glyphs),
	}

	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return contactSheetTmpl.Execute(f, data)
}
```

- [ ] **Step 4: Write the stylesheet**

Create `kb/ships/glyphs/glyphs.css`:

```css
/* Ship glyph contact sheet */

.glyph-sheet {
  padding: 1.5rem;
  max-width: 1600px;
  margin: 0 auto;
}

.glyph-intro {
  color: hsl(var(--muted-foreground));
  font-size: var(--text-ui);
  margin-bottom: 2rem;
}

.glyph-group {
  margin-bottom: 3rem;
}

.glyph-group h2 {
  font-size: var(--text-label);
  text-transform: uppercase;
  letter-spacing: 1.5px;
  color: hsl(var(--muted-foreground));
  padding: 0.5rem 0.75rem;
  background: hsl(var(--smui-surface-2));
  border-bottom: 1px solid hsl(var(--border));
  margin-bottom: 1rem;
}

.glyph-count {
  opacity: 0.6;
  margin-left: 0.5rem;
}

.glyph-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 1rem;
}

.glyph-cell {
  margin: 0;
  padding: 0.5rem;
  border: 1px solid hsl(var(--border));
  border-radius: 4px;
  background: hsl(var(--smui-surface-2));
  text-align: center;
}

/* Glyph strokes inherit currentColor, so both themes work with no overrides. */
.ship-glyph {
  width: 100%;
  height: auto;
  color: hsl(var(--foreground));
}

.glyph-region {
  opacity: 0.35;
}

.glyph-outline {
  opacity: 1;
}

.glyph-appendage {
  opacity: 0.85;
}

.glyph-hp {
  opacity: 0.7;
}

.glyph-cell figcaption {
  display: flex;
  flex-direction: column;
  gap: 0.1rem;
  margin-top: 0.4rem;
}

.glyph-name {
  font-size: var(--text-ui);
}

.glyph-class {
  font-size: var(--text-label);
  color: hsl(var(--muted-foreground));
}
```

- [ ] **Step 5: Run all tests and build**

Run: `go build ./... && go test ./... 2>&1 | tail -20`
Expected: PASS everywhere. The generator package now compiles, since `renderedGlyph` and `writeContactSheet` exist.

- [ ] **Step 6: Run the generator and verify determinism**

```bash
go build -o bin/generate-ship-glyphs ./cmd/generate-ship-glyphs
./bin/generate-ship-glyphs
ls kb/ships/glyphs/*.svg | wc -l          # expect 335
sha256sum kb/ships/glyphs/prayer.svg > /tmp/a
./bin/generate-ship-glyphs
sha256sum kb/ships/glyphs/prayer.svg > /tmp/b
diff /tmp/a /tmp/b && echo "DETERMINISTIC"
```

Expected: 335 SVGs, `index.html` present, and `DETERMINISTIC` printed. The binary goes in `bin/`, never the repo root.

- [ ] **Step 7: Lint and commit**

```bash
golangci-lint run ./cmd/generate-ship-glyphs/ ./pkg/shipglyph/
git add cmd/generate-ship-glyphs/ kb/ships/glyphs/ pkg/shipglyph/
git commit -m "feat(kb): ship glyph contact sheet for all catalog ships"
```

---

## Final verification

- [ ] `go build ./...` passes.
- [ ] `go test ./...` passes.
- [ ] `golangci-lint run` reports no new findings versus `main`.
- [ ] `kb/ships/glyphs/` contains one SVG per catalog ship plus `index.html` and `glyphs.css`.
- [ ] Running the generator twice produces a zero-byte git diff.
- [ ] Open `kb/ships/glyphs/index.html` in a browser and toggle the theme; glyphs must be legible in both light and dark.
- [ ] Spot-check by eye that the faction languages are distinguishable: Crimson reads angular, Nebula reads as a smooth needle, Outer Rim reads visibly asymmetric, Voidborn reads organic.

## Notes for the reviewer

**This is the taste gate.** The contact sheet exists so the whole design language can be judged at once. Expect the first output to need tuning — chamfer depth, jitter amount, archetype aspect ratios and the lobe wave frequency are all single constants in `pkg/shipglyph/style.go`, `infer.go` and `parts.go`. Tuning those is the *point* of this phase, not a defect in it.

Two questions were deliberately deferred from the design to be settled by eye here:

1. Whether pip geometry along curved region boundaries needs a path-following layout. Pips are not rendered in P1 at all; this is P3's problem, and the region paths emitted here are what it will follow.
2. Whether glyphs should normalise to a common box or scale by hull size. P1 normalises everything to the same `Size` box. If the sheet reads better with a dreadnought visibly larger than a shuttle, that is a one-line change to how `usable` is computed in `Render`.
