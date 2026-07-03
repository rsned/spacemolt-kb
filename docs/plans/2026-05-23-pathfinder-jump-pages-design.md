# Pathfinder Jump Routes — KB Site Pages (Phase 2) — Design

Date: 2026-05-23
Status: Approved, ready for implementation
Builds on: 2026-05-23-hyperjump-analyze-design.md (phase 1, `pkg/hyperjump`)

## Goal

Render the phase-1 hyper-jump analysis into the KB site as a separate
**Pathfinder Jump Routes** page per system, linked from the main system page, with
two SVG visualizations.

## Decisions (from brainstorming)

- Separate page `kb/systems/<id>/jumps.html`, linked under the existing "Jump
  Connections" card on the system detail page (`Pathfinder Jump Routes →`).
- Page sections, top to bottom:
  1. Header + back-link.
  2. Two SVG graphics side by side.
  3. **Direct Connections** — reachable destinations: `system_id` (linked),
     heading, distance, angular margin.
  4. **Station Destinations** — all ~38 station systems: `system_id`, heading,
     direct/interrupted badge (mirrors the arrow map).
  5. **Interrupted paths** — placeholder note; full listing deferred to a later
     phase to save disk (only the ~466 non-station blocked paths are skipped).
- Graphic A — **station-arrow starburst**: arrows only to the ~38 station
  destinations; bright white = direct, lighter gray = interrupted; labeled with
  name + heading.
- Graphic B — **coverage wheel**: 360° donut, covered arcs vs void-escape gaps,
  adaptive boundary labels.

## Architecture & data flow

No JSON round-trip. `cmd/generate-items-kb` already loads systems + POIs, so:

1. Build `[]hyperjump.System` from the loaded systems (`HasStation` = any POI with
   `type == "station"`).
2. Run `hyperjump.Analyze(systems, 100)` once; index `OriginReport`s by system ID.
3. Pass each system's report into the new jump-page template + render funcs.

`pkg/hyperjump` stays pure (analysis only). A new package `pkg/jumpmap` renders
SVG, mirroring how `pkg/systemmap` renders the orbital view.

### Coordinate convention

Both graphics use the engine convention: `0° = +X (right)`, angle increases
counter-clockwise. SVG y is flipped so +Y points up:

```
screenX = cx + r*cos(θ)
screenY = cy - r*sin(θ)
```

## pkg/jumpmap

```go
// RenderStationArrows draws the Sun-centered starburst of station-destination
// headings. names maps system id -> display name.
func RenderStationArrows(r hyperjump.OriginReport, names map[string]string) string

// RenderCoverageWheel draws the 360° coverage/void donut for an origin.
func RenderCoverageWheel(r hyperjump.OriginReport) string
```

Unexported helpers: `polar(cx, cy, r, deg) (x, y)`, arc/wedge path builder, and a
greedy label de-collision that pushes near-colliding labels to outer concentric
radii with leader lines.

### Graphic A — station-arrow starburst

- Sun glyph at center (reuse star styling).
- One arrow per station destination at its bearing, **fixed length** to a label
  ring (distances span 300–10,000+ GU; true scale is unreadable — distance goes in
  the label).
- Color: white = `Reachable`, light gray otherwise. Arrowhead at tip.
- Tip label: `Name heading°`. De-collision: sort by angle; labels within the
  text-height threshold get pushed to a 2nd/3rd radius with a short leader line.

### Graphic B — coverage wheel

- Full donut. Covered arcs (heading hits some system) in a muted fill; gap arcs
  (void escape, never lands) in a bright contrasting color.
- Covered arcs = complement of `r.Gaps` on [0,360); draw full disk covered, then
  overlay gap wedges.
- Adaptive labels: if `len(gaps) <= ~12`, label each gap's start & end heading;
  else label only gaps above a width threshold, capped. Faint 0/90/180/270 ticks.
- Center text: `% blocked / % open` + gap count. No-gap system (e.g. grumium):
  solid wheel, center "no escape".

## Generator changes (cmd/generate-items-kb/main.go)

- Build hyperjump systems + run `Analyze`; map reports by ID.
- `JumpPageData{ System *System; Report hyperjump.OriginReport; Names map[string]string }`.
- Template funcs `stationArrows`, `coverageWheel`.
- `jumpDetailTemplate`; write `<id>/jumps.html` in the per-system loop and in
  `writeOneSystemPage`.
- Add `Pathfinder Jump Routes →` link to `systemDetailTemplate` under the Jump
  Connections card.
- CSS: small block appended to `system.css` (arrow/wheel colors, label text).

## Testing (TDD)

- `RenderStationArrows`: report with 1 direct + 1 blocked station → 2 arrows,
  correct white/gray, both labels present, non-station dests excluded.
- `RenderCoverageWheel`: report with a known gap → gap wedge + boundary labels;
  no-gap report → solid wheel + "no escape".
- `polar` helper: 0°→right, 90°→up (y flipped).
- Label de-collision: two near-identical headings → different label radii.
- Generator: jump page renders for a sample system without template errors
  (temp-DB pattern).

## Verification

`go build/test ./...`, `golangci-lint`, generate the site, spot-check Sol's
`jumps.html`.

## Out of scope (later phases)

- Full interrupted-path listing per page.
- Interactive filtering / client-side JS.
