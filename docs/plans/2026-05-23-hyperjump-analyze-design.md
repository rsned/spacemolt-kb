# Hyperjump (Pathfinder Drive) Analysis Tool — Design

Date: 2026-05-23
Status: Approved, ready for implementation

## Context

A new server feature, the **Pathfinder Drive**, lets advanced pilots hyper-jump
directly toward a system by *heading* instead of plotting a multi-hop route. The
ship flies along a bearing from its current galactic coordinates until the ray
passes within a fixed tolerance of a system's center — that system becomes the
landing point.

We want a tool that uses the KB `systems` table (galactic `position_x` /
`position_y`) to answer:

1. **Q1** — the bearing (angle of travel) to every other system, for every system.
2. **Q2** — given the intersection tolerance, enumerate every system that
   *interrupts* a direct path between any two systems.
3. **Q3** — for clean (uninterrupted) paths, the *margin of error*: how far the
   heading can deviate before the jump no longer lands on the intended system.
4. **Bonus** — from a given origin (e.g. Sol at `(0,0)`), is there any heading
   that intersects no system at all (a "void escape direction")?

## Engine math (authoritative, from `engine.go:2085–2213`)

Coordinate system: `0°` points along `+X`; angle increases counter-clockwise
(`+Y` at `90°`).

```
bearing(A,B) = degrees(atan2(B.Y - A.Y, B.X - A.X))
pathfinderLandingMargin = 100.0   // galactic units (GU)
pathfinderSpeed         = 10.0    // GU/tick, tick = 10s
```

Ray-cast at jump time, heading direction `dir = (cos θ, sin θ)`, for every
system `S` except the origin:

```
rel  = S - origin
proj = rel · dir                      // forward projection
if proj <= 0: skip                    // behind the ship
perp = | relX*dirY - relY*dirX |      // perpendicular distance to ray
if perp > 100: skip                   // outside the corridor
```

Among all systems passing both checks, the one with the **lowest `proj`** (the
closest along the ray) is the landing system. Boundary is inclusive: `perp == 100`
lands, `perp == 101` misses. If nothing passes, `arrivalTick = 1<<62` (never) —
self-destruct is the only exit. Travel time `= ceil(proj/10) * 10` seconds.

## Key insight: everything is arcs on a circle

For a fixed origin A, translate so A is at the origin. Each other system C has
polar coordinates `(r_C, φ_C)` where `r_C = |C-A|`, `φ_C = bearing(A,C)`. For a
heading θ:

```
proj_C(θ)      = r_C * cos(θ - φ_C)
signedPerp_C(θ) = r_C * sin(θ - φ_C)
```

(algebraically identical to the dot/cross form above). C is *in the forward
corridor* exactly when θ lies in the arc

```
[ φ_C - α_C ,  φ_C + α_C ]   where   α_C = asin(min(1, 100 / r_C))
```

Systems within 100 GU (`r_C <= 100`) have `α_C = 90°` and block the entire
forward half-plane around their bearing.

This single "set of arcs per origin" structure answers all four questions:
- **Q1** = the `φ_C` values.
- **Q2** = for heading θ₀ = φ_B, which closer systems' arcs contain θ₀.
- **Q3** = angular distance from θ₀ to the nearest blocking arc edge.
- **Bonus** = whether the union of all arcs leaves any gap.

## Architecture

- `pkg/hyperjump/` — pure, dependency-free geometry and analysis (unit-tested).
- `cmd/hyperjump-analyze/` — thin CLI: loads systems from SQLite, runs the
  analysis, writes a stdout summary and an optional JSON artifact.

Follows existing KB conventions: `modernc.org/sqlite` pure-Go driver,
`database/sql`, Go 1.24, lint-clean under `golangci-lint`.

### Core geometry (pure functions)

```go
type Vec struct{ X, Y float64 }

func Bearing(a, b Vec) float64            // degrees in [0,360)
func Proj(rel Vec, headingDeg float64) float64
func SignedPerp(rel Vec, headingDeg float64) float64
func ArcHalfWidth(r, margin float64) float64 // asin(min(1, margin/r)) in degrees
```

### Data model

```go
type System struct {
    ID   string
    Name string
    Pos  Vec
}

type Pair struct {
    From, To      string
    Bearing       float64  // Q1, degrees
    Distance      float64  // proj to target (== |B-A|)
    Reachable     bool     // Q2: true if no interrupter
    LandsAt       string   // To if clean, else nearest interrupter
    Interrupters  []string // all stealing systems, nearest-first
    AngularMargin float64  // Q3, degrees (NaN/0 if not reachable)
    MarginLeft    float64  // signed CCW window edge
    MarginRight   float64  // signed CW window edge
    Clearance     float64  // GU, min perpendicular of any non-target system
}

type OriginReport struct {
    System       string
    Pairs        []Pair
    CoveragePct  float64    // fraction of 360° blocked by some system
    Gaps         []Gap      // void escape windows
}

type Gap struct{ StartDeg, EndDeg, WidthDeg, CenterDeg float64 }
```

## Algorithms

All brute force. N = 505 systems → ~254k directed pairs; the interrupter scan is
O(N³) ≈ 1.3e8 ops, sub-second in Go. No spatial index (YAGNI at this N; the
simple version is obviously correct and trivially testable).

### Q1 — bearings
For each ordered pair `(A,B)`, `Bearing(A,B)`.

### Q2 — interrupters
For directed pair A→B aiming at θ₀ = `Bearing(A,B)`, an interrupter is any C with
`0 < proj_C < distance(A,B)` and `|signedPerp_C(θ₀)| <= margin`. Sort by `proj`
ascending; `LandsAt` = first interrupter (or B if none). `Reachable = len(Interrupters)==0`.

### Q3 — angular margin (clean pairs only)
Aiming at θ₀ = φ_B. Find the maximal heading window around θ₀ that keeps B as the
landing system. On each side independently, the limit is the nearest of:
- **B leaving its own corridor:** `asin(min(1, margin / distance))`.
- **A nearer system entering its corridor:** for each C with `proj_C(θ₀) < distance`,
  the angular distance from θ₀ to the near edge of C's arc `[φ_C ± α_C]` on that side.

`MarginLeft` / `MarginRight` = the two signed edges; `AngularMargin = min(|left|,|right|)`.
`Clearance = min over non-target C of |signedPerp_C(θ₀)|`.

### Bonus — void coverage (per origin)
Build forward arcs `[φ_C - α_C, φ_C + α_C]` for all C, normalize onto `[0,360)`
(splitting wrap-around), merge, and report uncovered gaps. `CoveragePct` and
`Gaps` (widest first). A non-empty `Gaps` means escape headings exist.

## Outputs

One run produces two things:

1. **Summary report** (stdout) answering Q1–Q3 + bonus galaxy-wide: counts of
   clean vs blocked directed pairs, angular-margin distribution, how many systems
   have void escape directions, and (when `--system` is given) that origin's gaps.
2. **JSON** (`--out`), an array of `OriginReport` keyed by origin system. This is
   the artifact the later KB-site integration consumes to render a per-system
   page showing each system's reachable destinations, margins, and escape windows.

## CLI

`cmd/hyperjump-analyze`:

| Flag       | Default                                  | Purpose                          |
|------------|------------------------------------------|----------------------------------|
| `--db`     | `../spacemolt/data/spacemolt-knowledge.db` | SQLite KB path                   |
| `--margin` | `100`                                    | Landing margin in GU             |
| `--out`    | (none)                                   | Write JSON report to this path   |
| `--system` | (none)                                   | Restrict origin to one system id |

## Testing (TDD)

Pure-geometry functions get unit tests with hand-computed fixtures:
- `Bearing` on the four axes (`+X`=0, `+Y`=90, `-X`=180, `-Y`=270).
- A known interrupter exactly on a straight line (C between A and B).
- A system just inside vs just outside the corridor (`perp` = 99.9 vs 100.1).
- A synthetic 3-system galaxy with a hand-derived gap and angular margin.
- Arc merge / gap detection over wrap-around (arc straddling 0°/360°).

Regression against the real DB: Sol has a `~1.17°` void window centered on
heading `~15.5°`; total coverage `~99.32%` with 6 gaps. Assert these (with
tolerance) to lock the end-to-end pipeline.

## Future: site integration

A later phase renders the `OriginReport` JSON into the KB site — one page per
system listing direct-jump destinations (bearing, distance, reachable / blocked-by,
angular margin, clearance) and the system's void escape windows, alongside the
existing galaxy map and system pages.
