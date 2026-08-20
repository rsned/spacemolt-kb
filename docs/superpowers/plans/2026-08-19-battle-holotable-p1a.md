# Battle Holotable P1a — Static Frame Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render one tick of a real SpaceMolt battle as a top-down holotable — zone rings, side spokes, real SVG hull silhouettes at correct scale and heading, shield/hull state arcs, and targeting lines — so the visual language can be judged before any playback or FX work is built on it.

**Architecture:** A Go generator (`cmd/generate-battle-holotable`) reads a replay JSON exported by `bin/battle-export`, resolves each `ship_class` to its shipped SVG footprint and its catalog `scale`, and writes three sibling files into `kb/battles/`: the replay JSON, a hull pack, and a thin HTML page. A dependency-free `holotable.js` fetches those two JSONs and draws to a canvas. All geometry lives in pure exported functions tested under `node --test`; the renderer never reads a database and never learns the API's shape.

**Tech Stack:** Go 1.25 (`modernc.org/sqlite`, stdlib `html/template`, `encoding/json`, `encoding/xml`), vanilla ES2020 JavaScript with no dependencies, Node 22 built-in test runner (`node --test`), canvas 2D, inline SVG path data.

**Spec:** `docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md` — read the "P1 splits" section and Findings 3 and 4 before starting.

## Global Constraints

- **Repo:** all work is in `kb` (`github.com/rsned/spacemolt-kb`). Nothing in this plan touches the `spacemolt` repo; P0 already landed `pkg/battlereplay` and `bin/battle-export` there.
- **Zero JS dependencies.** No Three.js, no npm packages, no bundler. `holotable.js` is a plain script.
- **The renderer is data-only.** `holotable.js` must never contain a ship name, a scale number, a catalog lookup, or an API field name that is not already in the replay model. Everything ship-specific arrives in the hull pack.
- **Projection is true top-down.** No tilt, no foreshortening, no depth sort. Ring circles stay circles. All projection goes through one `project()` function so P3 can replace it.
- **Do not flip the Y axis.** `bearing_mean` in the replay model was computed with `atan2(y, x)` in model space; canvas Y is also down. Leaving Y unflipped keeps ship headings, side spokes, and ring geometry in one consistent convention. Flipping Y in `project()` alone would silently mirror every heading.
- **Never draw a placeholder box and never throw on a missing asset.** An unresolved `ship_class` draws a marked chevron; a station draws the station glyph.
- **Sleep constants:** not applicable — no sleeps in this work.
- **Verification gate for every Go task:** `go build ./...`, `go test ./...`, and `golangci-lint run` with no new findings. For JS tasks: `node --test tests/js/`.
- **Commit style:** conventional commits, staged explicitly by path. Never `git add -A` — this repo carries large generated-HTML churn under `kb/`.

## File Structure

| File | Responsibility |
|---|---|
| `data/battles/<id>.json` | Exported replay model fixture (Task 1). Input to the generator, committed so the page is reproducible. |
| `pkg/footprint/footprint.go` | Parse one hy3d SVG into `Footprint{Ship, D, Height, Aspect, FrameAmbiguous, KBMatch}`. No I/O. |
| `pkg/footprint/check.go` | `Check()` — the asset-contract lint. Pure over a parsed `Footprint`. |
| `pkg/footprint/footprint_test.go` | Unit tests over synthetic SVGs + a corpus test over all 395 shipped files. |
| `cmd/generate-battle-holotable/main.go` | CLI wiring: flags, DB open, read replay, call builders, write three files. |
| `cmd/generate-battle-holotable/hullpack.go` | `BuildHullPack()` — replay + footprint dir + DB → the hull pack. The resolution logic. |
| `cmd/generate-battle-holotable/hullpack_test.go` | Tests for class collection, fallback marking, station detection, scale lookup. |
| `cmd/generate-battle-holotable/page.go` | The HTML template and `RenderPage()`. |
| `kb/battles/holotable.js` | The renderer. Pure geometry exported for tests; draw functions; DOM init guarded. |
| `tests/js/holotable.test.js` | `node --test` suite over the exported pure functions. |
| `kb/battles/<id>.html` | Generated page. Not hand-edited. |
| `kb/battles/<id>-hulls.json` | Generated hull pack. Not hand-edited. |

---

### Task 1: Export the reference replays

**Files:**
- Create: `data/battles/a2619bbe328676445828b4e1007fe9aa.json`
- Create: `data/battles/b131fd5aae68420107dd20e93d15d3ba.json`
- Create: `data/battles/README.md`

**Interfaces:**
- Consumes: `bin/battle-export` from the `spacemolt` repo (already built).
- Produces: two replay-model JSON fixtures every later task reads. The schema is `pkg/battlereplay.Replay` — top level keys `battle_id`, `centre{x,y}`, `bounds{x_min,x_max,y_min,y_max}`, `zones[]`, `sides[{side_id,bearing_mean,radius_mean,count,won}]`, `participants[{player_id,username,kind,side_id,ship_class,max_hull,max_shield,max_fuel,first_tick,last_tick,destroyed_at_tick}]`, `frames[{tick,ships[{player_id,x,y,zone,hull,shield,fuel,stance,target_id,stale}],shots,moves,kills,repairs,chatter}]`.

- [ ] **Step 1: Export the primary battle**

`a2619bbe…` is the acceptance artifact: 42 participants, 30 ticks, two sides, 14 kills, a station with an empty `ship_class`, and two ship classes with no art. It exercises every draw path in one frame.

Run from the `spacemolt` repo (that is where the binary and credentials live):

```bash
cd /home/robert/spacemolt/spacemolt
mkdir -p /home/robert/spacemolt/kb/data/battles
bin/battle-export \
  --agent explorer-7 \
  --battle a2619bbe328676445828b4e1007fe9aa \
  --out /home/robert/spacemolt/kb/data/battles/a2619bbe328676445828b4e1007fe9aa.json
```

`explorer-7`, `databot`, and `craftsman-boss` are the idle agents with credentials and no fleet worker — a login from an agent that has a running worker collides with it. Battle logs are readable by any logged-in agent; you do not need to have been in the battle.

- [ ] **Step 2: Verify the primary export's shape**

Run:

```bash
cd /home/robert/spacemolt/kb
python3 - <<'PY'
import json
m = json.load(open("data/battles/a2619bbe328676445828b4e1007fe9aa.json"))
print("frames      ", len(m["frames"]))
print("participants", len(m["participants"]))
print("kills       ", sum(len(f.get("kills") or []) for f in m["frames"]))
print("zones       ", m["zones"])
print("sides       ", [(s["side_id"], round(s["bearing_mean"],1), s["count"]) for s in m["sides"]])
print("centre      ", m["centre"])
print("classes     ", sorted({p["ship_class"] for p in m["participants"]}))
print("destroyed   ", sum(1 for p in m["participants"] if p.get("destroyed_at_tick")))
PY
```

Expected: 30 frames, 42 participants, 14 kills, 4 zones, 2 sides, and a class list containing an empty string (the station) plus `anamnesis` and `silent_tide` among the resolvable names. If the counts differ, stop and report — the spec's golden numbers came from this exact battle and a mismatch means the adapter changed.

- [ ] **Step 3: Export the four-side battle**

`b131fd5a…` proves "no fixed side count" by render rather than by assertion — four sides at bearings 82/121/152/271°.

```bash
cd /home/robert/spacemolt/spacemolt
bin/battle-export \
  --agent explorer-7 \
  --battle b131fd5aae68420107dd20e93d15d3ba \
  --out /home/robert/spacemolt/kb/data/battles/b131fd5aae68420107dd20e93d15d3ba.json
```

Wait at least 35 seconds between the two exports. The client aborts if two connections die within 30 s (its session-contention guard), and back-to-back logins can trip it.

- [ ] **Step 4: Verify the four-side export**

Run the same Python block from Step 2 with the `b131fd5a…` path.
Expected: 158 frames, 5 participants, 4 sides with bearings near 82, 121, 152, 271.

- [ ] **Step 5: Answer spec open question 1 — is x/y quantised per zone?**

The spec flags this because it decides whether P1b interpolates linearly or eases. Measure it now while the data is in hand:

```bash
cd /home/robert/spacemolt/kb
python3 - <<'PY'
import json, math, collections
m = json.load(open("data/battles/a2619bbe328676445828b4e1007fe9aa.json"))
c = m["centre"]
byzone = collections.defaultdict(list)
for f in m["frames"]:
    for s in f["ships"]:
        if s.get("stale"):
            continue
        r = math.hypot(s["x"] - c["x"], s["y"] - c["y"])
        byzone[s["zone"]].append(r)
for z, rs in sorted(byzone.items(), key=lambda kv: sum(kv[1])/len(kv[1])):
    rs.sort()
    mean = sum(rs)/len(rs)
    spread = rs[-1] - rs[0]
    print(f"{z:9s} n={len(rs):5d} mean={mean:.3f} min={rs[0]:.3f} max={rs[-1]:.3f} spread={spread:.3f}")
PY
```

Record the output in `data/battles/README.md`. Interpretation: a spread near zero within each zone means radius is a function of zone (P1b must ease, or ships will teleport between rings); a wide spread means x/y drifts continuously and linear interpolation is honest. Write down which it is — do not guess.

- [ ] **Step 6: Write the fixtures README**

Create `data/battles/README.md`:

```markdown
# Battle replay fixtures

Exported by `bin/battle-export` from the `spacemolt` repo, which holds the
credentials — battle reads require a logged-in session, so a static page
cannot fetch these for itself.

| File | Battle | Ticks | Participants | Why it is here |
|---|---|---|---|---|
| `a2619bbe….json` | Node Beta | 30 | 42 | Primary acceptance artifact. Two sides, 14 kills, a station with an empty `ship_class`, and two classes with no art (`anamnesis`, `silent_tide`) — every draw path in one frame. |
| `b131fd5a….json` | Kitalpha | 158 | 5 | Four sides at bearings 82/121/152/271°. Proves the radial layout generalises past two sides. |

Re-export with:

    bin/battle-export --agent explorer-7 --battle <id> --out data/battles/<id>.json

Use an idle agent (`explorer-7`, `databot`, `craftsman-boss`); a login from an
agent with a running fleet worker collides with it. Leave 35 s between exports.

## Measured: does x/y drift within a zone?

<paste the Step 5 output here, and state the conclusion in one sentence>
```

- [ ] **Step 7: Check the fixtures are not gitignored, then commit**

`kb` has broad ignore patterns; confirm before committing.

```bash
cd /home/robert/spacemolt/kb
git check-ignore -v data/battles/a2619bbe328676445828b4e1007fe9aa.json || echo "not ignored - good"
```

If it reports a matching rule, add a negation to `.gitignore` (e.g. `!data/battles/*.json`) and include that in the commit.

```bash
git add data/battles/
git commit -m "test(battles): add Node Beta and Kitalpha replay fixtures

Node Beta is the acceptance artifact: 42 participants, a station with an
empty ship_class, and two classes with no art, so one frame exercises the
hull, station and fallback draw paths. Kitalpha carries four sides, which
is what proves the radial layout is not a two-side special case.

Also records whether radius is a function of zone, which decides how P1b
interpolates."
```

---

### Task 2: SVG footprint parser and asset-contract lint

**Files:**
- Create: `pkg/footprint/footprint.go`
- Create: `pkg/footprint/check.go`
- Create: `pkg/footprint/footprint_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `type Footprint struct { Ship, D, ArtStem, KBMatch string; Width, Height, Aspect float64; FrameAmbiguous bool }`
  - `func Parse(data []byte) (Footprint, error)`
  - `func Check(f Footprint, filename string) []string` — returns human-readable problems, empty slice when the file satisfies the contract.

This is the spec's "Contract" verification item: a lint over the asset directory so a pipeline regression fails loudly instead of producing subtly wrong hulls.

- [ ] **Step 1: Write the failing parser test**

Create `pkg/footprint/footprint_test.go`:

```go
package footprint

import (
	"math"
	"testing"
)

const goodSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1020 628"
  data-ship="dirk" data-art-stem="crimson_dirk" data-kb-match="stripped"
  data-aspect="1.6447" data-frame-ambiguous="false">
<title>dirk</title>
<path d="M10 10L1010 10L1010 618L10 618Z" fill-rule="evenodd"/>
</svg>`

func TestParseReadsTheRootAttributes(t *testing.T) {
	f, err := Parse([]byte(goodSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if f.Ship != "dirk" {
		t.Errorf("Ship = %q, want dirk", f.Ship)
	}
	if f.ArtStem != "crimson_dirk" {
		t.Errorf("ArtStem = %q, want crimson_dirk", f.ArtStem)
	}
	if f.KBMatch != "stripped" {
		t.Errorf("KBMatch = %q, want stripped", f.KBMatch)
	}
	if f.Width != 1020 {
		t.Errorf("Width = %v, want 1020", f.Width)
	}
	if f.Height != 628 {
		t.Errorf("Height = %v, want 628", f.Height)
	}
	if math.Abs(f.Aspect-1.6447) > 1e-6 {
		t.Errorf("Aspect = %v, want 1.6447", f.Aspect)
	}
	if f.FrameAmbiguous {
		t.Error("FrameAmbiguous = true, want false")
	}
	if f.D == "" {
		t.Error("D is empty; the path data must be captured")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/footprint/ -run TestParseReadsTheRootAttributes -v`
Expected: FAIL — the package does not compile because `Parse` and `Footprint` do not exist.

- [ ] **Step 3: Write the parser**

Create `pkg/footprint/footprint.go`:

```go
// Package footprint reads the KB pipeline's top-down ship footprint SVGs and
// checks them against the asset contract the battle holotable draws against.
//
// The contract, from the holotable design doc: one closed path with
// fill-rule="evenodd", bow pointing at +X, hull length normalised to 1000
// units inside a 1020-wide viewBox with a 10-unit margin, and the filename
// equal to data-ship equal to the ships-catalog id.
package footprint

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// Footprint is one parsed hy3d SVG.
type Footprint struct {
	// Ship is data-ship, which is also the catalog id and the battle log's
	// ship_class. It is the only join key the battle log provides.
	Ship string
	// D is the single path's geometry, drawn as-is by the renderer.
	D string
	// ArtStem is the original art asset name, kept for tracing a bad hull
	// back to the source image.
	ArtStem string
	// KBMatch records how the art was joined to the catalog: verbatim,
	// stripped, fuzzy, or none. Suspect "fuzzy" first if a hull looks wrong.
	KBMatch string

	Width  float64
	Height float64
	// Aspect is hull length over hull width, margins excluded.
	Aspect float64
	// FrameAmbiguous is the pipeline saying it is unsure of the hull's frame.
	FrameAmbiguous bool
}

// svgRoot mirrors only the attributes the contract cares about.
type svgRoot struct {
	ViewBox        string `xml:"viewBox,attr"`
	Ship           string `xml:"data-ship,attr"`
	ArtStem        string `xml:"data-art-stem,attr"`
	KBMatch        string `xml:"data-kb-match,attr"`
	Aspect         string `xml:"data-aspect,attr"`
	FrameAmbiguous string `xml:"data-frame-ambiguous,attr"`
	Paths          []struct {
		D        string `xml:"d,attr"`
		FillRule string `xml:"fill-rule,attr"`
	} `xml:"path"`
}

// Parse reads one footprint SVG. It does not validate the contract; use Check
// for that, so a caller can render a slightly off asset while still reporting
// it.
func Parse(data []byte) (Footprint, error) {
	var root svgRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return Footprint{}, fmt.Errorf("parse svg: %w", err)
	}

	f := Footprint{
		Ship:           root.Ship,
		ArtStem:        root.ArtStem,
		KBMatch:        root.KBMatch,
		FrameAmbiguous: root.FrameAmbiguous == "true",
	}

	fields := strings.Fields(root.ViewBox)
	if len(fields) != 4 {
		return Footprint{}, fmt.Errorf("viewBox %q: want 4 numbers, got %d", root.ViewBox, len(fields))
	}
	var err error
	if f.Width, err = strconv.ParseFloat(fields[2], 64); err != nil {
		return Footprint{}, fmt.Errorf("viewBox width %q: %w", fields[2], err)
	}
	if f.Height, err = strconv.ParseFloat(fields[3], 64); err != nil {
		return Footprint{}, fmt.Errorf("viewBox height %q: %w", fields[3], err)
	}
	if root.Aspect != "" {
		if f.Aspect, err = strconv.ParseFloat(root.Aspect, 64); err != nil {
			return Footprint{}, fmt.Errorf("data-aspect %q: %w", root.Aspect, err)
		}
	}

	// Collect every path so Check can complain about a second one; the
	// renderer only ever draws the first.
	for _, p := range root.Paths {
		if f.D == "" {
			f.D = p.D
		}
	}
	f.pathCount = len(root.Paths)

	return f, nil
}
```

Add the unexported field to the struct — put `pathCount int` as the last field of `Footprint` with a comment:

```go
	// pathCount is how many <path> elements the file carried. The contract is
	// exactly one; Check reports anything else.
	pathCount int
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/footprint/ -run TestParseReadsTheRootAttributes -v`
Expected: PASS

- [ ] **Step 5: Write the failing lint test**

Append to `pkg/footprint/footprint_test.go`:

```go
func TestCheckAcceptsAContractCompliantFile(t *testing.T) {
	f, err := Parse([]byte(goodSVG))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if problems := Check(f, "dirk.svg"); len(problems) != 0 {
		t.Errorf("Check reported %v, want none", problems)
	}
}

func TestCheckRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name     string
		svg      string
		filename string
		want     string // substring the report must mention
	}{
		{
			name:     "filename disagrees with data-ship",
			svg:      goodSVG,
			filename: "stiletto.svg",
			want:     "data-ship",
		},
		{
			name: "viewBox is not 1020 wide",
			svg: `<svg viewBox="0 0 999 628" data-ship="dirk" data-aspect="1.6447">` +
				`<path d="M0 0Z"/></svg>`,
			filename: "dirk.svg",
			want:     "width",
		},
		{
			name: "two paths instead of one",
			svg: `<svg viewBox="0 0 1020 628" data-ship="dirk" data-aspect="1.6447">` +
				`<path d="M0 0Z"/><path d="M1 1Z"/></svg>`,
			filename: "dirk.svg",
			want:     "path",
		},
		{
			name: "data-aspect disagrees with the viewBox height",
			svg: `<svg viewBox="0 0 1020 628" data-ship="dirk" data-aspect="9.9">` +
				`<path d="M0 0Z"/></svg>`,
			filename: "dirk.svg",
			want:     "aspect",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse([]byte(tt.svg))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			problems := Check(f, tt.filename)
			if len(problems) == 0 {
				t.Fatalf("Check reported no problems, want one mentioning %q", tt.want)
			}
			joined := strings.ToLower(strings.Join(problems, "; "))
			if !strings.Contains(joined, tt.want) {
				t.Errorf("Check reported %v, want a problem mentioning %q", problems, tt.want)
			}
		})
	}
}
```

Add `"strings"` to the test file's imports.

- [ ] **Step 6: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/footprint/ -run TestCheck -v`
Expected: FAIL — `Check` is not defined.

- [ ] **Step 7: Write the lint**

Create `pkg/footprint/check.go`:

```go
package footprint

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// canonicalWidth is the viewBox width every footprint carries: 1000 units of
// hull length plus a 10-unit margin on each side.
const canonicalWidth = 1020

// marginTotal is the vertical margin the hull is inset by, summed over both
// edges. data-aspect is measured with it removed.
const marginTotal = 20

// aspectTolerance is how far data-aspect may sit from 1000/(H-20). Four of the
// shipped files miss by a rounding hair; anything past this is a real defect.
const aspectTolerance = 0.01

// Check reports every way f departs from the asset contract. An empty result
// means the file is good. It returns all problems rather than the first so a
// corpus run gives one complete picture instead of needing repeated passes.
func Check(f Footprint, filename string) []string {
	var problems []string

	stem := strings.TrimSuffix(filepath.Base(filename), ".svg")
	if f.Ship == "" {
		problems = append(problems, "data-ship is missing; it is the only join key the battle log provides")
	} else if f.Ship != stem {
		problems = append(problems, fmt.Sprintf("filename stem %q != data-ship %q", stem, f.Ship))
	}

	if f.Width != canonicalWidth {
		problems = append(problems, fmt.Sprintf("viewBox width %v, want %v", f.Width, canonicalWidth))
	}
	if f.Height <= marginTotal {
		problems = append(problems, fmt.Sprintf("viewBox height %v leaves no hull inside the margins", f.Height))
	}

	switch f.pathCount {
	case 1:
		// The contract: one closed path, fill-rule evenodd for holes.
	case 0:
		problems = append(problems, "no path element; there is nothing to draw")
	default:
		problems = append(problems, fmt.Sprintf("%d path elements, want exactly 1", f.pathCount))
	}

	if f.Aspect > 0 && f.Height > marginTotal {
		want := 1000 / (f.Height - marginTotal)
		if math.Abs(f.Aspect-want) > aspectTolerance {
			problems = append(problems, fmt.Sprintf(
				"data-aspect %.4f but the viewBox implies %.4f", f.Aspect, want))
		}
	}

	return problems
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/footprint/ -v`
Expected: PASS for all cases.

- [ ] **Step 9: Add the corpus test over all 395 shipped assets**

Append to `pkg/footprint/footprint_test.go`:

```go
// footprintDir is the KB pipeline's output the holotable draws against.
const footprintDir = "../../data/footprints/hy3d-svg"

func TestShippedAssetsSatisfyTheContract(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(footprintDir, "*.svg"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) < 300 {
		t.Fatalf("found %d svg files in %s; the asset set is missing or the path moved",
			len(files), footprintDir)
	}

	var bad int
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: read: %v", filepath.Base(path), err)
			bad++
			continue
		}
		f, err := Parse(data)
		if err != nil {
			t.Errorf("%s: parse: %v", filepath.Base(path), err)
			bad++
			continue
		}
		for _, p := range Check(f, path) {
			t.Errorf("%s: %s", filepath.Base(path), p)
			bad++
		}
	}
	t.Logf("checked %d footprints, %d problems", len(files), bad)
}
```

Add `"os"` and `"path/filepath"` to the test imports.

- [ ] **Step 10: Run the corpus test**

Run: `cd /home/robert/spacemolt/kb && go test ./pkg/footprint/ -run TestShippedAssets -v`
Expected: PASS, logging `checked 395 footprints, 0 problems`.

If it fails, do not loosen the lint to make it green. Report which files fail and how — the spec says `data-aspect` matches on 391 of 395, so up to four aspect complaints are the known state and should be recorded in the test log, not silenced. If any *other* kind of problem appears, that is a genuine pipeline regression and must be reported before continuing.

- [ ] **Step 11: Lint and commit**

```bash
cd /home/robert/spacemolt/kb
go build ./... && go test ./pkg/footprint/ && golangci-lint run ./pkg/footprint/
git add pkg/footprint/
git commit -m "feat(footprint): parse hy3d footprints and lint the asset contract

The holotable draws these paths directly, so a pipeline regression would
surface as a subtly wrong hull rather than an error. Check reports every
departure at once — filename/data-ship disagreement, a viewBox that is not
1020 wide, a second path, an aspect that contradicts the height — and a
corpus test runs it over all 395 shipped files."
```

---

### Task 3: Hull pack builder

**Files:**
- Create: `cmd/generate-battle-holotable/hullpack.go`
- Create: `cmd/generate-battle-holotable/hullpack_test.go`

**Interfaces:**
- Consumes: `footprint.Parse`, `footprint.Footprint` from Task 2; the replay fixtures from Task 1.
- Produces:
  - `type Replay struct { BattleID string; Participants []Participant; ... }` — a minimal local mirror of the replay JSON, decoding only what the generator needs. Deliberately not an import of `pkg/battlereplay`: that lives in the other repo, and the generator only needs `ship_class` and `kind`.
  - `type Hull struct { Ship, D string; Height, Aspect float64; Scale int; Kind string; FrameAmbiguous bool; KBMatch string }`
  - `func BuildHullPack(rep Replay, dir string, scales map[string]int) (map[string]Hull, error)`
  - Hull `Kind` is one of `"hull"` (art resolved), `"station"`, or `"missing"` (no art — the renderer draws a marked chevron).

- [ ] **Step 1: Write the failing test**

Create `cmd/generate-battle-holotable/hullpack_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFootprint drops a minimal contract-compliant SVG into dir.
func writeFootprint(t *testing.T, dir, ship string, height float64) {
	t.Helper()
	svg := `<svg viewBox="0 0 1020 ` + ftoa(height) + `" data-ship="` + ship +
		`" data-aspect="` + ftoa(1000/(height-20)) + `" data-kb-match="verbatim">` +
		`<path d="M10 10L1010 10L1010 ` + ftoa(height-10) + `L10 ` + ftoa(height-10) + `Z"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, ship+".svg"), []byte(svg), 0o644); err != nil {
		t.Fatalf("write footprint: %v", err)
	}
}

func TestBuildHullPackResolvesArtStationsAndMisses(t *testing.T) {
	dir := t.TempDir()
	writeFootprint(t, dir, "dirk", 628)
	writeFootprint(t, dir, "vigil", 400)

	rep := Replay{
		BattleID: "test",
		Participants: []Participant{
			{PlayerID: "p1", ShipClass: "dirk", Kind: "player"},
			{PlayerID: "p2", ShipClass: "dirk", Kind: "player"},   // duplicate class
			{PlayerID: "p3", ShipClass: "vigil", Kind: "pirate"},
			{PlayerID: "p4", ShipClass: "anamnesis", Kind: "player"}, // no art
			{PlayerID: "p5", ShipClass: "", Kind: "station"},          // the station
		},
	}
	scales := map[string]int{"dirk": 2, "vigil": 4, "anamnesis": 3}

	pack, err := BuildHullPack(rep, dir, scales)
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}

	// One entry per distinct class, not per participant.
	if len(pack) != 4 {
		t.Fatalf("pack has %d entries, want 4 (dirk, vigil, anamnesis, station)", len(pack))
	}

	dirk, ok := pack["dirk"]
	if !ok {
		t.Fatal("dirk missing from pack")
	}
	if dirk.Kind != "hull" {
		t.Errorf("dirk Kind = %q, want hull", dirk.Kind)
	}
	if dirk.D == "" {
		t.Error("dirk has no path data")
	}
	if dirk.Height != 628 {
		t.Errorf("dirk Height = %v, want 628", dirk.Height)
	}
	if dirk.Scale != 2 {
		t.Errorf("dirk Scale = %d, want 2", dirk.Scale)
	}

	miss, ok := pack["anamnesis"]
	if !ok {
		t.Fatal("anamnesis missing from pack; a class with no art must still get an entry")
	}
	if miss.Kind != "missing" {
		t.Errorf("anamnesis Kind = %q, want missing", miss.Kind)
	}
	if miss.D != "" {
		t.Error("anamnesis has path data but has no art file")
	}
	if miss.Scale != 3 {
		t.Errorf("anamnesis Scale = %d, want 3 — scale comes from the catalog, not the art", miss.Scale)
	}

	station, ok := pack[""]
	if !ok {
		t.Fatal("the station's empty ship_class must still get an entry")
	}
	if station.Kind != "station" {
		t.Errorf("station Kind = %q, want station", station.Kind)
	}
}

func TestBuildHullPackDefaultsScaleToOneWhenTheCatalogIsSilent(t *testing.T) {
	dir := t.TempDir()
	writeFootprint(t, dir, "dirk", 628)

	rep := Replay{Participants: []Participant{{ShipClass: "dirk", Kind: "player"}}}

	pack, err := BuildHullPack(rep, dir, map[string]int{}) // catalog knows nothing
	if err != nil {
		t.Fatalf("BuildHullPack: %v", err)
	}
	if got := pack["dirk"].Scale; got != 1 {
		t.Errorf("Scale = %d, want 1 — an unknown scale must not render a zero-size hull", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-battle-holotable/ -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the hull pack builder**

Create `cmd/generate-battle-holotable/hullpack.go`:

```go
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rsned/spacemolt-kb/pkg/footprint"
)

// Replay is the part of the exported replay model the generator reads. It is a
// local mirror rather than an import of the spacemolt repo's pkg/battlereplay:
// the generator only needs to know which hulls appear, and the page passes the
// rest of the JSON through untouched.
type Replay struct {
	BattleID     string        `json:"battle_id"`
	SystemName   string        `json:"system_name"`
	TickCount    int           `json:"tick_count"`
	Participants []Participant `json:"participants"`
}

// Participant is one combatant's identity in the replay model.
type Participant struct {
	PlayerID  string `json:"player_id"`
	Username  string `json:"username"`
	Kind      string `json:"kind"`
	SideID    int    `json:"side_id"`
	ShipClass string `json:"ship_class"`
}

// Hull is one drawable ship class, everything the renderer needs to draw it and
// nothing else. The renderer never reads a catalog or an SVG file; it reads
// this.
type Hull struct {
	// Ship is the class id, empty for a station.
	Ship string `json:"ship"`
	// D is the SVG path geometry, empty when Kind is not "hull".
	D string `json:"d,omitempty"`
	// Height is the source viewBox height. With width fixed at 1020, the draw
	// transform needs it to find the hull centre at (510, Height/2).
	Height float64 `json:"height,omitempty"`
	Aspect float64 `json:"aspect,omitempty"`
	// Scale is the catalog hull scale, 1..5, and sets relative draw size.
	Scale int `json:"scale"`
	// Kind is how to draw it: "hull", "station", or "missing".
	Kind string `json:"kind"`
	// FrameAmbiguous forwards the pipeline's own uncertainty about the hull's
	// frame, so a debug view can surface it instead of silently trusting it.
	FrameAmbiguous bool `json:"frame_ambiguous,omitempty"`
	// KBMatch is the art-to-catalog join provenance. "fuzzy" was inferred by
	// hand and is the first thing to suspect if a hull looks wrong.
	KBMatch string `json:"kb_match,omitempty"`
}

// hull kinds.
const (
	kindHull    = "hull"
	kindStation = "station"
	kindMissing = "missing"
)

// defaultScale keeps a hull the catalog has no row for from rendering at zero
// size. A ship drawn slightly wrong is debuggable; one drawn invisibly is not.
const defaultScale = 1

// BuildHullPack resolves every distinct ship class in rep to a drawable Hull.
// Classes with no art are included with Kind "missing" rather than omitted, so
// the renderer can draw a marked chevron and the operator can see the gap.
func BuildHullPack(rep Replay, dir string, scales map[string]int) (map[string]Hull, error) {
	pack := make(map[string]Hull)

	for _, p := range rep.Participants {
		if _, seen := pack[p.ShipClass]; seen {
			continue
		}

		scale := scales[p.ShipClass]
		if scale <= 0 {
			scale = defaultScale
		}

		// A station carries an empty ship_class and has no footprint art; the
		// renderer draws it as a fixed glyph.
		if p.ShipClass == "" || p.Kind == kindStation {
			pack[p.ShipClass] = Hull{Ship: p.ShipClass, Kind: kindStation, Scale: scale}
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, p.ShipClass+".svg"))
		if errors.Is(err, fs.ErrNotExist) {
			pack[p.ShipClass] = Hull{Ship: p.ShipClass, Kind: kindMissing, Scale: scale}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read footprint for %q: %w", p.ShipClass, err)
		}

		f, err := footprint.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("parse footprint for %q: %w", p.ShipClass, err)
		}

		pack[p.ShipClass] = Hull{
			Ship:           p.ShipClass,
			D:              f.D,
			Height:         f.Height,
			Aspect:         f.Aspect,
			Scale:          scale,
			Kind:           kindHull,
			FrameAmbiguous: f.FrameAmbiguous,
			KBMatch:        f.KBMatch,
		}
	}

	return pack, nil
}

// ftoa formats a float compactly for generated SVG and test fixtures.
func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-battle-holotable/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
go build ./... && go test ./cmd/generate-battle-holotable/ && golangci-lint run ./cmd/generate-battle-holotable/
git add cmd/generate-battle-holotable/
git commit -m "feat(holotable): resolve a battle's ship classes to a hull pack

One entry per distinct class rather than per participant, carrying the path
geometry, viewBox height, catalog scale and join provenance. Classes with no
art are included as 'missing' rather than dropped so the renderer draws a
marked chevron and the coverage gap stays visible, and a station's empty
ship_class resolves to the station glyph instead of a failed file read."
```

---

### Task 4: Generator CLI and page emit

**Files:**
- Create: `cmd/generate-battle-holotable/main.go`
- Create: `cmd/generate-battle-holotable/page.go`
- Modify: `cmd/generate-battle-holotable/hullpack_test.go` (append page tests)

**Interfaces:**
- Consumes: `BuildHullPack`, `Replay`, `Hull` from Task 3.
- Produces:
  - `func LoadScales(db *sql.DB) (map[string]int, error)` — reads `ships(id, scale)`.
  - `func RenderPage(rep Replay) ([]byte, error)` — the HTML.
  - Three output files per battle in `kb/battles/`: `<id>.json`, `<id>-hulls.json`, `<id>.html`.

- [ ] **Step 1: Write the failing page test**

Append to `cmd/generate-battle-holotable/hullpack_test.go`:

```go
func TestRenderPageWiresTheDataFilesAndRenderer(t *testing.T) {
	rep := Replay{
		BattleID:   "a2619bbe328676445828b4e1007fe9aa",
		SystemName: "Node Beta",
		TickCount:  30,
	}
	got, err := RenderPage(rep)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	page := string(got)

	for _, want := range []string{
		"a2619bbe328676445828b4e1007fe9aa.json",      // the replay
		"a2619bbe328676445828b4e1007fe9aa-hulls.json", // the hull pack
		"holotable.js",                                 // the renderer
		"Node Beta",                                    // the heading
		"<canvas",                                      // something to draw on
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not mention %q", want)
		}
	}

	// The renderer is data-only: the page must not inline ship data.
	if strings.Contains(page, "data-ship") {
		t.Error("page inlines SVG attributes; hull data belongs in the hull pack")
	}
}

func TestRenderPageEscapesTheBattleID(t *testing.T) {
	rep := Replay{BattleID: `"><script>alert(1)</script>`, SystemName: "x"}
	got, err := RenderPage(rep)
	if err != nil {
		t.Fatalf("RenderPage: %v", err)
	}
	if strings.Contains(string(got), "<script>alert(1)</script>") {
		t.Error("battle id was interpolated unescaped")
	}
}
```

Add `"strings"` to the test imports.

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-battle-holotable/ -run TestRenderPage -v`
Expected: FAIL — `RenderPage` is not defined.

- [ ] **Step 3: Write the page renderer**

Create `cmd/generate-battle-holotable/page.go`:

```go
package main

import (
	"bytes"
	"fmt"
	"html/template"
)

// pageTemplate is the whole page. It carries no ship data: the renderer fetches
// the replay and the hull pack beside it, which keeps holotable.js editable
// without regenerating anything.
const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Holotable — {{.SystemName}} ({{.BattleID}})</title>
<style>
  :root { color-scheme: dark; }
  body {
    margin: 0; background: #05080d; color: #9fd4e8;
    font: 13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  header { padding: 10px 16px; border-bottom: 1px solid #123; }
  h1 { margin: 0; font-size: 15px; font-weight: 600; letter-spacing: .04em; }
  .meta { color: #4d7a8c; }
  #table { display: block; width: 100vw; height: calc(100vh - 46px); }
  #status { padding: 16px; color: #c86; }
</style>
</head>
<body>
<header>
  <h1>{{.SystemName}}</h1>
  <div class="meta">battle {{.BattleID}} &middot; {{.TickCount}} ticks &middot; tick <span id="tick">—</span></div>
</header>
<canvas id="table"></canvas>
<div id="status"></div>
<script>
  window.HOLOTABLE = {
    replayURL: {{.ReplayURL}},
    hullsURL: {{.HullsURL}},
  };
</script>
<script src="holotable.js"></script>
</body>
</html>
`

// pageData is what the template renders against.
type pageData struct {
	BattleID   string
	SystemName string
	TickCount  int
	ReplayURL  string
	HullsURL   string
}

// RenderPage produces the holotable page for one battle. The page is thin by
// design: it names two data files and loads the shared renderer.
func RenderPage(rep Replay) ([]byte, error) {
	tmpl, err := template.New("holotable").Parse(pageTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	data := pageData{
		BattleID:   rep.BattleID,
		SystemName: rep.SystemName,
		TickCount:  rep.TickCount,
		ReplayURL:  rep.BattleID + ".json",
		HullsURL:   rep.BattleID + "-hulls.json",
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}
```

`html/template` escapes `{{.ReplayURL}}` correctly inside the script block as a JS string literal, which is why the URLs are passed through the template rather than concatenated.

- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && go test ./cmd/generate-battle-holotable/ -run TestRenderPage -v`
Expected: PASS

- [ ] **Step 5: Write the CLI**

Create `cmd/generate-battle-holotable/main.go`:

```go
// Command generate-battle-holotable turns an exported battle replay into a
// holotable page: the replay JSON, a hull pack naming every ship class that
// appears, and a thin HTML page that loads both plus the shared renderer.
//
//	go run ./cmd/generate-battle-holotable --replay data/battles/<id>.json
//
// Design: docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	replayPath := flag.String("replay", "", "exported replay model JSON (required)")
	outDir := flag.String("out", "kb/battles", "directory to write the page and its data into")
	footprints := flag.String("footprints", "data/footprints/hy3d-svg", "directory of hy3d footprint SVGs")
	dbPath := flag.String("db", "spacemolt-knowledge.db", "knowledge database, for ship scale")
	flag.Parse()

	if *replayPath == "" {
		fmt.Fprintln(os.Stderr, "usage: generate-battle-holotable --replay data/battles/<id>.json")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*replayPath)
	if err != nil {
		log.Fatalf("read replay: %v", err)
	}

	var rep Replay
	if err := json.Unmarshal(raw, &rep); err != nil {
		log.Fatalf("decode replay: %v", err)
	}
	if rep.BattleID == "" {
		log.Fatalf("replay %s has no battle_id", *replayPath)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	scales, err := LoadScales(db)
	if err != nil {
		log.Fatalf("load ship scales: %v", err)
	}

	pack, err := BuildHullPack(rep, *footprints, scales)
	if err != nil {
		log.Fatalf("build hull pack: %v", err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output directory: %v", err)
	}

	// The replay is copied verbatim: the renderer consumes the model the
	// adapter already normalised, and re-encoding here would be a second place
	// for the shape to drift.
	writeFile(filepath.Join(*outDir, rep.BattleID+".json"), raw)

	packJSON, err := json.Marshal(pack)
	if err != nil {
		log.Fatalf("encode hull pack: %v", err)
	}
	writeFile(filepath.Join(*outDir, rep.BattleID+"-hulls.json"), packJSON)

	page, err := RenderPage(rep)
	if err != nil {
		log.Fatalf("render page: %v", err)
	}
	writeFile(filepath.Join(*outDir, rep.BattleID+".html"), page)

	var missing, ambiguous int
	for _, h := range pack {
		if h.Kind == kindMissing {
			missing++
			log.Printf("no footprint art for ship_class %q", h.Ship)
		}
		if h.FrameAmbiguous {
			ambiguous++
			log.Printf("footprint for %q is flagged frame-ambiguous", h.Ship)
		}
	}
	log.Printf("%s: %d participants, %d ship classes, %d without art, %d frame-ambiguous",
		rep.BattleID, len(rep.Participants), len(pack), missing, ambiguous)
}

// writeFile writes or dies; every caller here treats a write failure as fatal.
func writeFile(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(data))
}

// LoadScales reads the catalog hull scale for every ship. Scale drives relative
// draw size, so a scale-1 cobble and a scale-4 junk_convoy share a table
// correctly rather than rendering the same size.
func LoadScales(db *sql.DB) (map[string]int, error) {
	rows, err := db.Query(`SELECT id, scale FROM ships`)
	if err != nil {
		return nil, fmt.Errorf("query ships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	scales := make(map[string]int)
	for rows.Next() {
		var id string
		var scale int
		if err := rows.Scan(&id, &scale); err != nil {
			return nil, fmt.Errorf("scan ship: %w", err)
		}
		scales[id] = scale
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ships: %w", err)
	}
	return scales, nil
}
```

- [ ] **Step 6: Run the generator against the real fixture**

Run:

```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable \
  --replay data/battles/a2619bbe328676445828b4e1007fe9aa.json
```

Expected log lines: three `wrote …` lines, a `no footprint art for ship_class "anamnesis"` and one for `"silent_tide"`, and a summary reading `42 participants, N ship classes, 2 without art`.

Confirm the hull pack looks right:

```bash
python3 - <<'PY'
import json
p = json.load(open("kb/battles/a2619bbe328676445828b4e1007fe9aa-hulls.json"))
for cls, h in sorted(p.items()):
    print(f"{cls!r:22} kind={h['kind']:8} scale={h['scale']} "
          f"h={h.get('height','-')} match={h.get('kb_match','-')} d={len(h.get('d',''))}")
PY
```

Every `kind: hull` entry must have a non-empty `d` and a `height`; the two missing classes and the station must not.

- [ ] **Step 7: Run it for the four-side battle too**

```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable \
  --replay data/battles/b131fd5aae68420107dd20e93d15d3ba.json
```

- [ ] **Step 8: Confirm the outputs are committable, then commit**

```bash
cd /home/robert/spacemolt/kb
git check-ignore -v kb/battles/a2619bbe328676445828b4e1007fe9aa.html || echo "not ignored - good"
go build ./... && go test ./cmd/generate-battle-holotable/ && golangci-lint run ./cmd/generate-battle-holotable/
git add cmd/generate-battle-holotable/ kb/battles/
git commit -m "feat(holotable): generate the battle page, replay copy and hull pack

The page is deliberately thin — it names two JSON files and loads the shared
renderer — so holotable.js stays editable without regenerating anything and
the renderer never learns a ship name. The replay is copied verbatim rather
than re-encoded, so there is only one place the model shape can drift.

Logs classes with no art and frame-ambiguous footprints, since both are
things to see before judging a render rather than after."
```

---

### Task 5: Renderer geometry core

**Files:**
- Create: `kb/battles/holotable.js`
- Create: `tests/js/holotable.test.js`

**Interfaces:**
- Consumes: the replay model and hull pack shapes from Tasks 1 and 3.
- Produces, all exported for tests:
  - `fitView(bounds, width, height, margin)` → `{scale, ox, oy}`
  - `project(x, y, view)` → `{px, py}`
  - `zoneRings(frames, centre, opts)` → `[{zone, meanRadius, rInner, rOuter}]` ordered inner to outer
  - `headingOf(ship, shipsById, centre)` → radians
  - `hullPixels(scale, opts)` → pixel length of a hull
  - `hullState(ship, participant, tick)` → `{hull: fraction|null, shield: fraction|null, dead: bool}` where `null` means unknown

This task is where correctness lives; everything after it is drawing.

- [ ] **Step 1: Write the failing geometry tests**

Create `tests/js/holotable.test.js`:

```js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const ht = require('../../kb/battles/holotable.js');

test('fitView centres the model bounds in the canvas', () => {
  const bounds = {x_min: 0, x_max: 2, y_min: 0, y_max: 2};
  const v = ht.fitView(bounds, 400, 400, 20);
  // 360 usable px over 2 model units.
  assert.strictEqual(v.scale, 180);
  // Model centre (1,1) must land at canvas centre (200,200).
  const p = ht.project(1, 1, v);
  assert.ok(Math.abs(p.px - 200) < 1e-9, `px ${p.px}`);
  assert.ok(Math.abs(p.py - 200) < 1e-9, `py ${p.py}`);
});

test('fitView uses the tighter axis so nothing is cropped', () => {
  const bounds = {x_min: 0, x_max: 4, y_min: 0, y_max: 1};
  const v = ht.fitView(bounds, 400, 400, 0);
  assert.strictEqual(v.scale, 100, 'the wide axis must set the scale');
  const right = ht.project(4, 0.5, v);
  assert.ok(right.px <= 400, `right edge ${right.px} escaped the canvas`);
});

test('fitView survives degenerate bounds', () => {
  // A battle where every participant sits at one point must not divide by zero.
  const v = ht.fitView({x_min: 1, x_max: 1, y_min: 1, y_max: 1}, 400, 400, 20);
  assert.ok(Number.isFinite(v.scale), `scale ${v.scale} is not finite`);
  assert.ok(v.scale > 0, `scale ${v.scale} must be positive`);
  const p = ht.project(1, 1, v);
  assert.ok(Number.isFinite(p.px) && Number.isFinite(p.py));
});

test('project does not flip Y, so headings stay in the model convention', () => {
  const v = ht.fitView({x_min: 0, x_max: 2, y_min: 0, y_max: 2}, 400, 400, 0);
  const lo = ht.project(1, 0.5, v);
  const hi = ht.project(1, 1.5, v);
  assert.ok(hi.py > lo.py,
    'a larger model y must map to a larger canvas y; flipping would mirror every heading');
});

test('zoneRings orders bands by measured radius, not by array order', () => {
  const centre = {x: 0, y: 0};
  const frames = [{
    tick: 1,
    ships: [
      {player_id: 'a', x: 0.5, y: 0, zone: 'engaged'},
      {player_id: 'b', x: 0.7, y: 0, zone: 'inner'},
      {player_id: 'c', x: 1.0, y: 0, zone: 'mid'},
      {player_id: 'd', x: 1.4, y: 0, zone: 'outer'},
    ],
  }];
  const rings = ht.zoneRings(frames, centre, {});
  assert.deepStrictEqual(rings.map(r => r.zone), ['engaged', 'inner', 'mid', 'outer']);
  assert.ok(Math.abs(rings[0].meanRadius - 0.5) < 1e-9);
  // Boundaries sit at the midpoints between adjacent means.
  assert.ok(Math.abs(rings[0].rOuter - 0.6) < 1e-9, `rOuter ${rings[0].rOuter}`);
  assert.strictEqual(rings[0].rInner, 0, 'the innermost band starts at the centre');
  assert.ok(rings[3].rOuter > rings[3].meanRadius, 'the outer ring must enclose its ships');
});

test('zoneRings ignores carried-forward states', () => {
  const frames = [{
    tick: 1,
    ships: [
      {player_id: 'a', x: 0.5, y: 0, zone: 'engaged'},
      {player_id: 'b', x: 99, y: 0, zone: 'engaged', stale: true},
    ],
  }];
  const rings = ht.zoneRings(frames, {x: 0, y: 0}, {});
  assert.ok(Math.abs(rings[0].meanRadius - 0.5) < 1e-9,
    'a stale state must not drag the measured radius');
});

test('headingOf points at the target when there is one', () => {
  const ships = new Map([
    ['a', {player_id: 'a', x: 0, y: 0, target_id: 'b'}],
    ['b', {player_id: 'b', x: 1, y: 0}],
  ]);
  const th = ht.headingOf(ships.get('a'), ships, {x: 0, y: 5});
  assert.ok(Math.abs(th - 0) < 1e-9, `heading ${th}, want 0 (bow toward +X)`);
});

test('headingOf falls back to the centre with no target', () => {
  const ships = new Map([['a', {player_id: 'a', x: 0, y: 0}]]);
  const th = ht.headingOf(ships.get('a'), ships, {x: 0, y: 1});
  assert.ok(Math.abs(th - Math.PI / 2) < 1e-9, `heading ${th}, want PI/2`);
});

test('headingOf falls back when the target is gone or co-located', () => {
  const ships = new Map([['a', {player_id: 'a', x: 0, y: 0, target_id: 'ghost'}]]);
  const th = ht.headingOf(ships.get('a'), ships, {x: 1, y: 0});
  assert.ok(Number.isFinite(th), 'a dangling target_id must not produce NaN');

  const stacked = new Map([
    ['a', {player_id: 'a', x: 2, y: 2, target_id: 'b'}],
    ['b', {player_id: 'b', x: 2, y: 2}],
  ]);
  const th2 = ht.headingOf(stacked.get('a'), stacked, {x: 0, y: 2});
  assert.ok(Number.isFinite(th2), 'a co-located target must not produce an arbitrary heading');
  assert.ok(Math.abs(th2 - Math.PI) < 1e-9, 'it must fall back to the centre');
});

test('hullPixels grows with catalog scale', () => {
  const small = ht.hullPixels(1, {});
  const big = ht.hullPixels(5, {});
  assert.ok(big > small, 'a scale-5 hull must draw larger than a scale-1 hull');
  assert.ok(small > 0);
});

test('hullState reports unknown rather than empty when hull reads zero alive', () => {
  const p = {max_hull: 75, max_shield: 40, destroyed_at_tick: 0};
  const s = ht.hullState({hull: 0, shield: 40}, p, 1);
  assert.strictEqual(s.hull, null,
    'a live ship reading hull 0 is unknown data, not a derelict');
  assert.strictEqual(s.dead, false);
  assert.ok(Math.abs(s.shield - 1) < 1e-9);
});

test('hullState reports dead once the ship is past its destruction tick', () => {
  const p = {max_hull: 75, max_shield: 40, destroyed_at_tick: 12};
  assert.strictEqual(ht.hullState({hull: 0, shield: 0}, p, 12).dead, true);
  assert.strictEqual(ht.hullState({hull: 0, shield: 0}, p, 20).dead, true);
  assert.strictEqual(ht.hullState({hull: 0, shield: 0}, p, 11).dead, false,
    'before its destruction tick the same zero is still unknown');
});

test('hullState reports a fraction when the data is real', () => {
  const p = {max_hull: 100, max_shield: 50};
  const s = ht.hullState({hull: 40, shield: 25}, p, 3);
  assert.ok(Math.abs(s.hull - 0.4) < 1e-9);
  assert.ok(Math.abs(s.shield - 0.5) < 1e-9);
});

test('hullState reports unknown when the maximum is missing', () => {
  const s = ht.hullState({hull: 10, shield: 0}, {max_hull: 0, max_shield: 0}, 1);
  assert.strictEqual(s.hull, null, 'no max means no fraction to draw');
  assert.strictEqual(s.shield, null);
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: FAIL — cannot find module `../../kb/battles/holotable.js`.

- [ ] **Step 3: Write the geometry core**

Create `kb/battles/holotable.js`:

```js
'use strict';
// Holotable — a top-down tactical replay of a SpaceMolt battle.
//
// The table is RADIAL: concentric zone bands around a centre, with each side
// holding a spoke that its ships advance inward and retreat outward along.
// Sides are not limited to two.
//
// Everything above the draw layer is a pure function exported for node --test.
// This file reads only the replay model and the hull pack; it never learns a
// ship name, a catalog value, or an API field the adapter did not normalise.
//
// Design: docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md

// HULL_PX_PER_SCALE sets how many pixels of hull length one unit of catalog
// scale buys. It is the first knob to turn after seeing a render: too small and
// the silhouettes are unreadable, too large and an engaged ball becomes soup.
const HULL_PX_PER_SCALE = 14;

// OUTER_RING_MARGIN pushes the outermost ring past the ships sitting on it, so
// the table has a rim rather than clipping its own outer band.
const OUTER_RING_MARGIN = 1.12;

// DEGENERATE_SPAN is the model-space span assumed when every participant sits
// at effectively one point, which would otherwise divide by zero.
const DEGENERATE_SPAN = 1;

// fitView maps model coordinates onto the canvas: uniform scale, model bounds
// centred, the tighter axis winning so nothing is cropped.
function fitView(bounds, width, height, margin) {
  const m = margin || 0;
  let spanX = bounds.x_max - bounds.x_min;
  let spanY = bounds.y_max - bounds.y_min;
  if (!(spanX > 0)) spanX = DEGENERATE_SPAN;
  if (!(spanY > 0)) spanY = DEGENERATE_SPAN;

  const scale = Math.min((width - 2 * m) / spanX, (height - 2 * m) / spanY);
  const midX = (bounds.x_min + bounds.x_max) / 2;
  const midY = (bounds.y_min + bounds.y_max) / 2;

  return {scale, ox: width / 2 - midX * scale, oy: height / 2 - midY * scale};
}

// project maps one model point to canvas pixels.
//
// Y is deliberately NOT flipped. The replay model's side bearings were computed
// with atan2(y, x) in model space and canvas Y also grows downward, so leaving
// it alone keeps ship headings, side spokes and ring geometry in one
// convention. Flipping here would silently mirror every heading.
//
// This is the single seam P3's 2.5D view replaces.
function project(x, y, view) {
  return {px: x * view.scale + view.ox, py: y * view.scale + view.oy};
}

// zoneRings measures each zone band's radius from the data rather than assuming
// fixed radii, and orders the bands by what it measures rather than trusting
// the order the zones arrived in. Boundaries sit at the midpoints between
// adjacent means.
function zoneRings(frames, centre, opts) {
  const options = opts || {};
  const outerMargin = options.outerMargin || OUTER_RING_MARGIN;

  const sums = new Map();
  for (const frame of frames) {
    for (const ship of frame.ships) {
      // A carried-forward state repeats a stale position and would drag the
      // measurement toward wherever the ship was last seen.
      if (ship.stale) continue;
      const r = Math.hypot(ship.x - centre.x, ship.y - centre.y);
      const acc = sums.get(ship.zone) || {sum: 0, n: 0};
      acc.sum += r;
      acc.n += 1;
      sums.set(ship.zone, acc);
    }
  }

  const rings = [];
  for (const [zone, acc] of sums) {
    rings.push({zone, meanRadius: acc.sum / acc.n});
  }
  rings.sort((a, b) => a.meanRadius - b.meanRadius);

  for (let i = 0; i < rings.length; i++) {
    rings[i].rInner = i === 0 ? 0 : (rings[i - 1].meanRadius + rings[i].meanRadius) / 2;
    rings[i].rOuter = i === rings.length - 1
      ? rings[i].meanRadius * outerMargin
      : (rings[i].meanRadius + rings[i + 1].meanRadius) / 2;
  }

  return rings;
}

// headingOf is the rotation to draw a ship at. Bow toward its target if it has
// a live one, else inward toward the centre — the axis its advance and retreat
// run along. It is never a mirror; on a radial table facing follows the ship's
// own geometry.
function headingOf(ship, shipsById, centre) {
  const target = ship.target_id ? shipsById.get(ship.target_id) : null;
  if (target) {
    const dx = target.x - ship.x;
    const dy = target.y - ship.y;
    // A co-located target gives no direction; fall through to the centre.
    if (dx !== 0 || dy !== 0) return Math.atan2(dy, dx);
  }

  const cx = centre.x - ship.x;
  const cy = centre.y - ship.y;
  if (cx === 0 && cy === 0) return 0; // sitting on the centre; any heading is arbitrary
  return Math.atan2(cy, cx);
}

// hullPixels converts catalog hull scale to drawn length, so a scale-1 cobble
// and a scale-4 junk_convoy share a table at their real relative sizes.
function hullPixels(scale, opts) {
  const options = opts || {};
  const perScale = options.hullPxPerScale || HULL_PX_PER_SCALE;
  return perScale * Math.max(1, scale || 1);
}

// hullState turns raw hull and shield readings into drawable fractions.
//
// A null fraction means UNKNOWN and must be drawn as such. The battle log reads
// hull 0 for some live participants, including on tick 1 with full shields and
// no damage taken, so a bare zero cannot be trusted to mean destroyed. The
// adapter's destroyed_at_tick is what actually settles it: past that tick the
// ship is dead, before it a zero is missing data.
function hullState(ship, participant, tick) {
  const destroyedAt = participant.destroyed_at_tick || 0;
  const dead = destroyedAt > 0 && tick >= destroyedAt;

  return {
    hull: fractionOf(ship.hull, participant.max_hull, dead),
    shield: fractionOf(ship.shield, participant.max_shield, dead),
    dead,
  };
}

// fractionOf returns value/max, or null when there is nothing trustworthy to
// draw. A zero on a live ship is unknown; a zero on a dead one is a real zero.
function fractionOf(value, max, dead) {
  if (!(max > 0)) return null;
  if (!(value > 0)) return dead ? 0 : null;
  return Math.min(1, value / max);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    fitView, project, zoneRings, headingOf, hullPixels, hullState,
    HULL_PX_PER_SCALE, OUTER_RING_MARGIN,
  };
}
```


- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
node --test tests/js/
git add kb/battles/holotable.js tests/js/holotable.test.js
git commit -m "feat(holotable): geometry core for the radial table

Ring radii are measured from the frames and ordered by what is measured,
rather than assuming four fixed bands in the order they arrived. Y is not
flipped in project(), because the model's side bearings use atan2 in model
space and canvas Y also grows downward — flipping there would mirror every
heading while looking correct in isolation.

hullState returns null for unknown rather than zero: the log reads hull 0
for live ships on tick 1, so destroyed_at_tick is what separates a derelict
from missing data."
```

---

### Task 6: Ground layer — rings, spokes, labels

**Files:**
- Modify: `kb/battles/holotable.js`
- Modify: `tests/js/holotable.test.js`

**Interfaces:**
- Consumes: `fitView`, `project`, `zoneRings` from Task 5.
- Produces:
  - `spokeEnd(bearingDeg, radius, centre)` → `{x, y}` in model space
  - `drawGround(ctx, view, centre, rings, sides, theme)` — draws bands, spokes and labels
  - `THEME` — the exported default palette object

- [ ] **Step 1: Write the failing spoke test**

Append to `tests/js/holotable.test.js`:

```js
test('spokeEnd places a side at its own bearing', () => {
  const centre = {x: 0, y: 0};
  const east = ht.spokeEnd(0, 2, centre);
  assert.ok(Math.abs(east.x - 2) < 1e-9, `x ${east.x}`);
  assert.ok(Math.abs(east.y - 0) < 1e-9, `y ${east.y}`);

  const south = ht.spokeEnd(90, 2, centre);
  assert.ok(Math.abs(south.x - 0) < 1e-9, `x ${south.x}`);
  assert.ok(Math.abs(south.y - 2) < 1e-9,
    `y ${south.y} — bearing must use the same atan2 convention as the model`);
});

test('spokeEnd handles a side straddling zero degrees', () => {
  // The adapter already averaged bearings as unit vectors; the renderer must
  // not reintroduce the wrap bug by treating degrees as a plain number line.
  const a = ht.spokeEnd(350, 1, {x: 0, y: 0});
  const b = ht.spokeEnd(-10, 1, {x: 0, y: 0});
  assert.ok(Math.abs(a.x - b.x) < 1e-9 && Math.abs(a.y - b.y) < 1e-9,
    '350 and -10 degrees are the same direction');
});

test('THEME defines every colour the ground layer draws with', () => {
  for (const key of ['bg', 'ring', 'ringLabel', 'spoke', 'grid']) {
    assert.strictEqual(typeof ht.THEME[key], 'string', `THEME.${key} missing`);
  }
  assert.ok(Array.isArray(ht.THEME.sides) && ht.THEME.sides.length >= 4,
    'side colours must cover at least four sides; three- and four-way battles occur');
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: FAIL — `ht.spokeEnd is not a function`.

- [ ] **Step 3: Implement the ground layer**

Insert into `kb/battles/holotable.js`, above the export guard:

```js
// THEME is the holo palette, carried over from the point-cloud demo: a dark
// field with cyan structure. Side colours must cover at least four sides —
// three- and four-way battles occur and the upper bound is unknown, so the list
// is cycled rather than indexed blindly.
const THEME = {
  bg: '#05080d',
  ring: 'rgba(90, 190, 220, 0.20)',
  ringLabel: 'rgba(120, 200, 225, 0.55)',
  spoke: 'rgba(90, 190, 220, 0.14)',
  grid: 'rgba(60, 130, 160, 0.10)',
  hullUnknown: 'rgba(160, 160, 170, 0.5)',
  targetLine: 'rgba(120, 210, 235, 0.16)',
  wreck: 'rgba(140, 90, 80, 0.55)',
  missingArt: 'rgba(230, 170, 90, 0.9)',
  // Six colours: four-way battles are confirmed and the upper bound on
  // sides is unknown, so the list is cycled rather than indexed blindly.
  sides: ['#4fd0e8', '#e8734f', '#7fe08a', '#d9a0e8', '#e8d24f', '#8f9ae8'],
};

// spokeEnd converts a side's mean bearing into a model-space point at radius r.
// The bearing is degrees in the same atan2 convention the adapter used, so this
// is a plain polar conversion and inherits the unit-vector averaging the
// adapter already did.
function spokeEnd(bearingDeg, radius, centre) {
  const rad = bearingDeg * Math.PI / 180;
  return {x: centre.x + Math.cos(rad) * radius, y: centre.y + Math.sin(rad) * radius};
}

// sideColour cycles the palette, because the number of sides has no known upper
// bound and running off the end must not produce undefined.
function sideColour(sideId, theme) {
  const palette = theme.sides;
  return palette[Math.abs(sideId) % palette.length];
}

// drawGround lays the table down: zone bands as true circles, one spoke per
// side, and a label per band.
function drawGround(ctx, view, centre, rings, sides, theme) {
  const c = project(centre.x, centre.y, view);

  // Bands, outermost first so inner rings draw over the fill.
  for (let i = rings.length - 1; i >= 0; i--) {
    const r = rings[i].rOuter * view.scale;
    ctx.beginPath();
    ctx.arc(c.px, c.py, r, 0, Math.PI * 2);
    ctx.strokeStyle = theme.ring;
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  // Spokes: each side's axis of advance and retreat.
  const outer = rings.length ? rings[rings.length - 1].rOuter : 1;
  for (const side of sides) {
    const end = spokeEnd(side.bearing_mean, outer, centre);
    const p = project(end.x, end.y, view);
    ctx.beginPath();
    ctx.moveTo(c.px, c.py);
    ctx.lineTo(p.px, p.py);
    ctx.strokeStyle = theme.spoke;
    ctx.lineWidth = 1;
    ctx.stroke();

    ctx.fillStyle = sideColour(side.side_id, theme);
    ctx.font = '11px ui-monospace, monospace';
    ctx.textAlign = 'center';
    ctx.fillText(`SIDE ${side.side_id} (${side.count})`, p.px, p.py - 6);
  }

  // Band labels along the +X axis, where a spoke is least likely to sit on top
  // of them for a two-side battle.
  ctx.fillStyle = theme.ringLabel;
  ctx.font = '10px ui-monospace, monospace';
  ctx.textAlign = 'left';
  for (const ring of rings) {
    const at = project(centre.x + ring.rOuter, centre.y, view);
    ctx.fillText(ring.zone.toUpperCase(), at.px + 4, c.py - 4);
  }
}
```

Add `spokeEnd`, `sideColour`, `drawGround`, and `THEME` to the `module.exports` list.


- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
node --test tests/js/
git add kb/battles/holotable.js tests/js/holotable.test.js
git commit -m "feat(holotable): draw the ground — zone bands, side spokes, labels

Side colours cycle rather than index, because the number of sides has no
known upper bound: four-way battles are confirmed and running off the end of
a fixed palette would draw undefined. spokeEnd is a plain polar conversion
in the adapter's own atan2 convention, so the unit-vector bearing averaging
is inherited rather than redone."
```

---

### Task 7: Ship layer — hulls, glyphs, state arcs

**Files:**
- Modify: `kb/battles/holotable.js`
- Modify: `tests/js/holotable.test.js`

**Interfaces:**
- Consumes: `project`, `headingOf`, `hullPixels`, `hullState`, `THEME`, `sideColour` from Tasks 5 and 6.
- Produces:
  - `hullTransform(hull, lengthPx)` → `{scale, cx, cy}` — the SVG-space to table transform
  - `drawShip(ctx, view, ship, participant, hull, opts)`
  - `drawStationGlyph(ctx, px, py, r, colour)`
  - `drawMissingGlyph(ctx, px, py, r, colour)`

- [ ] **Step 1: Write the failing transform test**

Append to `tests/js/holotable.test.js`:

```js
test('hullTransform centres a footprint and scales it to the drawn length', () => {
  // The contract: viewBox 1020 wide, 1000 units of hull, 10-unit margins, so
  // the hull centre is (510, height/2) and hull length is 1000.
  const t = ht.hullTransform({height: 628}, 40);
  assert.ok(Math.abs(t.scale - 0.04) < 1e-12, `scale ${t.scale}, want 40/1000`);
  assert.strictEqual(t.cx, 510, 'x centre is half the 1020 viewBox');
  assert.strictEqual(t.cy, 314, 'y centre is half the height');
});

test('hullTransform tolerates a footprint with no height', () => {
  const t = ht.hullTransform({}, 40);
  assert.ok(Number.isFinite(t.cy), 'a missing height must not produce NaN');
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: FAIL — `ht.hullTransform is not a function`.

- [ ] **Step 3: Implement the ship layer**

Insert into `kb/battles/holotable.js` above the export guard:

```js
// FOOTPRINT_WIDTH is the viewBox width every footprint carries: 1000 units of
// hull plus a 10-unit margin each side.
const FOOTPRINT_WIDTH = 1020;
// FOOTPRINT_HULL_LENGTH is the normalised hull length inside that viewBox.
const FOOTPRINT_HULL_LENGTH = 1000;

// hullTransform gives the numbers for drawing a footprint at a table length:
// translate to the ship, rotate to its heading, scale, then shift the
// footprint's own centre to the origin.
function hullTransform(hull, lengthPx) {
  const height = hull.height > 0 ? hull.height : FOOTPRINT_WIDTH;
  return {
    scale: lengthPx / FOOTPRINT_HULL_LENGTH,
    cx: FOOTPRINT_WIDTH / 2,
    cy: height / 2,
  };
}

// pathCache avoids rebuilding a Path2D per ship per frame; a 373-participant
// battle would otherwise parse the same handful of paths hundreds of times.
const pathCache = new Map();

function hullPath(hull) {
  if (!hull.d) return null;
  let p = pathCache.get(hull.ship);
  if (!p) {
    p = new Path2D(hull.d);
    pathCache.set(hull.ship, p);
  }
  return p;
}

// drawStationGlyph reproduces the official viewer's station mark — a filled
// hexagon with a circle at each corner inside two concentric rings — so a
// reader who has seen one viewer can read the other.
function drawStationGlyph(ctx, px, py, r, colour) {
  ctx.save();
  ctx.translate(px, py);

  ctx.beginPath();
  for (let i = 0; i < 6; i++) {
    const a = i * Math.PI / 3;
    const x = Math.cos(a) * r;
    const y = Math.sin(a) * r;
    if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
  }
  ctx.closePath();
  ctx.fillStyle = colour;
  ctx.globalAlpha = 0.35;
  ctx.fill();
  ctx.globalAlpha = 1;
  ctx.strokeStyle = colour;
  ctx.lineWidth = 1.5;
  ctx.stroke();

  for (let i = 0; i < 6; i++) {
    const a = i * Math.PI / 3;
    ctx.beginPath();
    ctx.arc(Math.cos(a) * r, Math.sin(a) * r, r * 0.18, 0, Math.PI * 2);
    ctx.fillStyle = colour;
    ctx.fill();
  }

  for (const mult of [1.3, 1.55]) {
    ctx.beginPath();
    ctx.arc(0, 0, r * mult, 0, Math.PI * 2);
    ctx.strokeStyle = colour;
    ctx.globalAlpha = 0.4;
    ctx.lineWidth = 1;
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  ctx.restore();
}

// drawMissingGlyph marks a ship class with no footprint art. It is deliberately
// unlike a hull — a dashed chevron — so a coverage gap reads as a gap rather
// than as a badly drawn ship.
function drawMissingGlyph(ctx, px, py, r, colour) {
  ctx.save();
  ctx.translate(px, py);
  ctx.beginPath();
  ctx.moveTo(r, 0);
  ctx.lineTo(-r * 0.6, r * 0.6);
  ctx.lineTo(-r * 0.25, 0);
  ctx.lineTo(-r * 0.6, -r * 0.6);
  ctx.closePath();
  ctx.strokeStyle = colour;
  ctx.setLineDash([3, 2]);
  ctx.lineWidth = 1.2;
  ctx.stroke();
  ctx.setLineDash([]);
  ctx.restore();
}

// ARC_SWEEP is how much of a circle a full state bar covers, leaving a gap at
// the bow so the arcs never look like a closed ring.
const ARC_SWEEP = Math.PI * 1.6;
const ARC_START = -Math.PI * 0.8;

// drawStateArcs puts shield outside and hull inside, as fractions of maximum.
// A null fraction is UNKNOWN and draws as a dashed grey ring rather than an
// empty bar, so missing data does not read as a derelict ship.
function drawStateArcs(ctx, px, py, r, state, theme) {
  const bands = [
    {frac: state.shield, radius: r * 1.35, colour: '#6fc8e8'},
    {frac: state.hull, radius: r * 1.15, colour: '#e8b96f'},
  ];

  for (const band of bands) {
    ctx.beginPath();
    if (band.frac === null) {
      ctx.arc(px, py, band.radius, ARC_START, ARC_START + ARC_SWEEP);
      ctx.strokeStyle = theme.hullUnknown;
      ctx.setLineDash([2, 3]);
    } else {
      ctx.arc(px, py, band.radius, ARC_START, ARC_START + ARC_SWEEP * band.frac);
      ctx.strokeStyle = band.colour;
    }
    ctx.lineWidth = 1.5;
    ctx.stroke();
    ctx.setLineDash([]);
  }
}

// drawShip draws one combatant: its silhouette at its heading, then its state.
function drawShip(ctx, view, ship, participant, hull, opts) {
  const theme = opts.theme;
  const p = project(ship.x, ship.y, view);
  const lengthPx = hullPixels(hull.scale, opts);
  const colour = opts.dead ? theme.wreck : sideColour(participant.side_id, theme);

  if (hull.kind === 'station') {
    drawStationGlyph(ctx, p.px, p.py, lengthPx * 0.6, colour);
    return;
  }

  const path = hullPath(hull);
  if (!path) {
    drawMissingGlyph(ctx, p.px, p.py, lengthPx * 0.5, theme.missingArt);
    drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
    return;
  }

  const t = hullTransform(hull, lengthPx);
  ctx.save();
  ctx.translate(p.px, p.py);
  ctx.rotate(opts.heading);
  ctx.scale(t.scale, t.scale);
  ctx.translate(-t.cx, -t.cy);

  ctx.fillStyle = colour;
  ctx.globalAlpha = opts.dead ? 0.25 : 0.45;
  ctx.fill(path, 'evenodd');
  ctx.globalAlpha = 1;
  ctx.strokeStyle = colour;
  // Undo the scale so the outline is a constant pixel width whatever the hull.
  ctx.lineWidth = 1.2 / t.scale;
  ctx.stroke(path);
  ctx.restore();

  drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
}
```

Add `hullTransform`, `drawShip`, `drawStationGlyph`, `drawMissingGlyph`, and `drawStateArcs` to `module.exports`.

`Path2D` does not exist in Node, so keep `hullPath` out of the exported pure surface and never call it from a test.

- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb
node --test tests/js/
git add kb/battles/holotable.js tests/js/holotable.test.js
git commit -m "feat(holotable): draw hulls, station glyphs and state arcs

Line width is divided by the hull scale so an outline is a constant pixel
weight whatever the ship's size, and Path2D objects are cached per class
because a 373-participant battle would otherwise reparse the same handful of
paths every frame.

A class with no art draws a dashed chevron, not an approximation of a ship,
so a coverage gap reads as a gap. An unknown state arc is a dashed ring
rather than an empty bar."
```

---

### Task 8: Assemble, render, and report

**Files:**
- Modify: `kb/battles/holotable.js`
- Modify: `tests/js/holotable.test.js`
- Create: `docs/holotable-p1a-findings.md`

**Interfaces:**
- Consumes: everything from Tasks 5–7.
- Produces:
  - `pickFrame(replay, want)` → the frame to draw
  - `busiestTick(replay)` → the tick with the most targeting activity
  - `initHolotable()` — DOM entry point, guarded

- [ ] **Step 1: Write the failing frame-selection test**

Append to `tests/js/holotable.test.js`:

```js
test('busiestTick finds the frame with the most ships actively targeting', () => {
  const replay = {frames: [
    {tick: 1, ships: [{player_id: 'a'}, {player_id: 'b'}]},
    {tick: 2, ships: [{player_id: 'a', target_id: 'b'}, {player_id: 'b', target_id: 'a'}]},
    {tick: 3, ships: [{player_id: 'a', target_id: 'b'}]},
  ]};
  assert.strictEqual(ht.busiestTick(replay), 2);
});

test('busiestTick falls back to the first frame when nobody ever targets', () => {
  const replay = {frames: [{tick: 7, ships: [{player_id: 'a'}]}]};
  assert.strictEqual(ht.busiestTick(replay), 7);
});

test('pickFrame returns the requested tick, else the busiest', () => {
  const replay = {frames: [
    {tick: 1, ships: [{player_id: 'a'}]},
    {tick: 2, ships: [{player_id: 'a', target_id: 'b'}]},
  ]};
  assert.strictEqual(ht.pickFrame(replay, 1).tick, 1);
  assert.strictEqual(ht.pickFrame(replay, null).tick, 2, 'no request means the busiest frame');
  assert.strictEqual(ht.pickFrame(replay, 999).tick, 2, 'an out-of-range tick falls back');
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: FAIL — `ht.busiestTick is not a function`.

- [ ] **Step 3: Implement frame selection and the DOM entry point**

Insert into `kb/battles/holotable.js` above the export guard:

```js
// busiestTick picks the most informative single frame: the one where the most
// ships are actively targeting something. P1a draws one frame, so it should be
// a frame where the fleet is visibly doing something rather than tick 1, where
// half the participants have not joined yet.
function busiestTick(replay) {
  let best = null;
  let bestCount = -1;
  for (const frame of replay.frames) {
    let count = 0;
    for (const ship of frame.ships) {
      if (ship.target_id) count++;
    }
    if (count > bestCount) {
      bestCount = count;
      best = frame.tick;
    }
  }
  return best;
}

// pickFrame resolves the frame to draw: an explicitly requested tick if it
// exists, else the busiest.
function pickFrame(replay, want) {
  if (want !== null && want !== undefined) {
    const found = replay.frames.find(f => f.tick === want);
    if (found) return found;
  }
  const tick = busiestTick(replay);
  return replay.frames.find(f => f.tick === tick) || replay.frames[0];
}

// TABLE_MARGIN keeps the outermost ring and its labels off the canvas edge.
const TABLE_MARGIN = 60;

// drawFrame renders one tick of the battle onto ctx.
function drawFrame(ctx, replay, hulls, frame, width, height) {
  const view = fitView(replay.bounds, width, height, TABLE_MARGIN);
  const rings = zoneRings(replay.frames, replay.centre, {});

  ctx.fillStyle = THEME.bg;
  ctx.fillRect(0, 0, width, height);

  drawGround(ctx, view, replay.centre, rings, replay.sides, THEME);

  const shipsById = new Map(frame.ships.map(s => [s.player_id, s]));
  const partById = new Map(replay.participants.map(p => [p.player_id, p]));

  // Targeting lines first, so hulls draw over them.
  ctx.strokeStyle = THEME.targetLine;
  ctx.lineWidth = 1;
  for (const ship of frame.ships) {
    const target = ship.target_id ? shipsById.get(ship.target_id) : null;
    if (!target) continue;
    const a = project(ship.x, ship.y, view);
    const b = project(target.x, target.y, view);
    ctx.beginPath();
    ctx.moveTo(a.px, a.py);
    ctx.lineTo(b.px, b.py);
    ctx.stroke();
  }

  for (const ship of frame.ships) {
    const participant = partById.get(ship.player_id);
    if (!participant) continue;
    const hull = hulls[participant.ship_class] || {kind: 'missing', scale: 1};
    const state = hullState(ship, participant, frame.tick);
    drawShip(ctx, view, ship, participant, hull, {
      theme: THEME,
      heading: headingOf(ship, shipsById, replay.centre),
      state,
      dead: state.dead,
    });
  }
}

// initHolotable wires the page: fetch both data files, size the canvas to the
// device, draw one frame.
async function initHolotable() {
  const cfg = window.HOLOTABLE;
  const status = document.getElementById('status');
  const canvas = document.getElementById('table');

  try {
    const [replay, hulls] = await Promise.all([
      fetch(cfg.replayURL).then(r => r.json()),
      fetch(cfg.hullsURL).then(r => r.json()),
    ]);

    const dpr = window.devicePixelRatio || 1;
    const width = canvas.clientWidth;
    const height = canvas.clientHeight;
    canvas.width = width * dpr;
    canvas.height = height * dpr;

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    const params = new URLSearchParams(window.location.search);
    const wanted = params.has('tick') ? Number(params.get('tick')) : null;
    const frame = pickFrame(replay, wanted);

    drawFrame(ctx, replay, hulls, frame, width, height);

    document.getElementById('tick').textContent = String(frame.tick);
    status.textContent = '';
  } catch (err) {
    status.textContent = 'Could not draw the battle: ' + err.message;
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initHolotable);
}
```

Add `busiestTick`, `pickFrame`, and `drawFrame` to `module.exports`.

- [ ] **Step 4: Run to verify it passes**

Run: `cd /home/robert/spacemolt/kb && node --test tests/js/holotable.test.js`
Expected: PASS

- [ ] **Step 5: Render both battles and look at them**

The page fetches its data, so it must be served rather than opened from disk:

```bash
cd /home/robert/spacemolt/kb/kb && python3 -m http.server 8099
```

Open `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html` and
`http://localhost:8099/battles/b131fd5aae68420107dd20e93d15d3ba.html`.

Check, in order — each of these is a specific failure the design predicted:

1. Rings are circles, four of them, ordered engaged/inner/mid/outer from the centre out.
2. The four-side battle shows four spokes at roughly 82/121/152/271°. If they are mirrored or collapsed, `project()` flipped Y or `spokeEnd` disagrees with the adapter's convention.
3. Hulls point somewhere meaningful — at their targets, or inward. If every ship of one side points the wrong way, suspect the Y convention before suspecting the art.
4. A scale-4 hull is visibly larger than a scale-1 hull.
5. The station draws as a hexagon-in-rings, not a chevron and not a hull.
6. `anamnesis` and `silent_tide` draw as dashed chevrons in the missing-art colour.
7. No ship shows an empty hull arc on a frame where it is alive — unknown reads dashed grey.

Add `?tick=N` to the URL to look at any other frame.

- [ ] **Step 6: Capture the findings**

Create `docs/holotable-p1a-findings.md` recording, with a screenshot of each battle:

```markdown
# Holotable P1a — first render findings

Rendered <date> from `a2619bbe…` (Node Beta, 42 participants) and
`b131fd5a…` (Kitalpha, four sides).

## Spec open questions this render answers

**Q1 — does x/y drift within a zone, or is it quantised?**
<the Task 1 Step 5 measurement, and what it means for P1b interpolation>

**Q2 — station placement.** Does the station read correctly as a table
participant at its own x/y, or should it anchor a side's baseline?
<answer from the render>

**Q4 — how does the hull-0 UNKNOWN treatment read?**
<does the dashed grey ring read as "no data" or as damage?>

## Tuning needed

- `HULL_PX_PER_SCALE` (currently 14): <too small / about right / too large>
- `OUTER_RING_MARGIN` (currently 1.12): <does the outer band clip its ships?>
- Side palette legibility: <are four sides distinguishable?>

## What P1b should change before adding playback

<list>
```

- [ ] **Step 7: Final verification and commit**

```bash
cd /home/robert/spacemolt/kb
go build ./... && go test ./... && node --test tests/js/ && golangci-lint run
git add kb/battles/holotable.js tests/js/holotable.test.js docs/holotable-p1a-findings.md
git commit -m "feat(holotable): assemble the static frame and record first findings

Draws the busiest frame by default rather than tick 1, where half the
participants have not joined yet, with ?tick=N to inspect any other.
Targeting lines draw under the hulls so they read as context rather than
clutter.

Records what the render answers about the spec's open questions: whether
radius is a function of zone, where the station belongs, and whether the
unknown-hull treatment reads as missing data rather than as damage."
```

---

## Self-Review

**Spec coverage.** Every P1a element in the ratified spec section maps to a task: SVG hulls (3, 7), zone rings (5, 6), side spokes (6), state arcs (7), targeting lines (8), station glyph (7), missing-art fallback (3, 7), plan-view projection (5), the asset-contract lint (2), the generator/page shape (3, 4), and both acceptance battles (1, 8). The spec's P1b items — transport, interpolation, chatter rail — are deliberately absent.

**Deferred deliberately, and why.** Pre-baking `pkg/shipglyph` output as real SVG assets stays in P1b per the spec: P1a's bar is only that a missing hull never draws a box and never throws, which the chevron satisfies.

**Known risk, flagged not hidden.** Tasks 6–8 are canvas drawing, so their tests cover the pure geometry underneath and the render itself is the acceptance artifact. Step 5 of Task 8 lists seven specific checks with the failure each one implies, so "look at it" is a real gate rather than a shrug.

**One correction carried into this plan.** The design presented in chat said the page would inline its data because `file://` blocks `fetch`. That was wrong — `bom-explorer.js` already fetches a sibling JSON, so the KB site is served over HTTP, and this plan follows that precedent instead. Every code block here is meant to be copied as written; there are no correction notes to apply while reading.
