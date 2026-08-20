# Holotable P1a — first render findings

Rendered 2026-08-19 from `a2619bbe…` (Node Beta, 42 participants) and
`b131fd5a…` (Kitalpha, four sides).

## Screenshots

Chrome's capture tooling was intermittently unreliable this session
(occasional timeouts and one corrupted tiled capture, always fixed by a
fresh navigate — never traced to the page itself; see the task report for
detail). Enough clean captures came through to keep in the repo:

- `img/holotable-p1a/node-beta-full.jpg` — Node Beta, full table, tick
  1615393 (the busiest frame). Both sides, the station, the outer chevrons.
- `img/holotable-p1a/node-beta-hull-scale-2x.jpg` — Node Beta rendered at
  2× canvas resolution (scrolled right, hence the visible scrollbar — a
  debugging aid for this capture, not a rendering defect). Shows the
  scale-4/5 hulls next to the scale-1 majority (constants judgement
  below).
- `img/holotable-p1a/node-beta-missing-art-and-unknown-hull.jpg` — closeup
  of two `anamnesis`/`silent_tide` dashed chevrons and, just below the
  large green hull, the one hull-0-alive ship as a small dot (check 7).
- `img/holotable-p1a/kitalpha-full.jpg` — Kitalpha, full table, tick
  1417934 (its only frame with any targeting at all). Four sides, the
  zone/measurement disagreement discussed below.

### How to see this yourself

```
cd kb/.claude/worktrees/battle-holotable/kb && python3 -m http.server 8099
```

Then open, in a browser:

- `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html?tick=1615393`
- `http://localhost:8099/battles/b131fd5aae68420107dd20e93d15d3ba.html?tick=1417934`

Both `?tick=` values are already the default (the busiest frame each page
picks on its own) — they're spelled out here only so a reader lands on
exactly the frame this document describes, not whatever frame a future
`pickFrame` change might pick instead.

## Spec open questions this render answers

**Q1 — does x/y drift within a zone, or is it quantised?**

Already measured in Task 1 (`data/battles/README.md`): every zone has a
spread (0.85–1.39) comparable to its own mean radius, and adjacent zones'
min/max ranges overlap heavily (`engaged` reaches 1.479 while `outer` starts
as low as 0.778). Radius is **not** a function of zone — x/y drifts
continuously within a zone rather than sitting on a fixed ring. This render
confirms it visually: Kitalpha (see below) has ships whose x/y radius lands
in a completely different band than their own zone label. **P1b should
interpolate ship positions linearly between frames, not ease between
discrete zone radii** — a zone-radius easing would fight the real data on
every single frame, not just at transition instants.

**Q2 — station placement.** Does the station read correctly as a table
participant at its own x/y, or should it anchor a side's baseline?

It reads correctly as an ordinary table participant. Node Beta's station
(`node_beta_industrial_station`, `ship_class: ""`) resolves through
`hulls['']` to `{kind: 'station'}` and draws via `drawStationGlyph` — a
filled hexagon with corner dots inside two concentric rings — sitting well
outside the outermost zone ring, at its own x/y, side-coloured green (side
2, the side it belongs to). It is visually unambiguous as "a different kind
of thing than a ship" without needing to anchor anything: no side spoke
touches it, and it does not sit on any zone ring. **No change needed** —
station-as-participant is the right model; do not special-case it as a
side anchor.

**Q4 — how does the hull-0 UNKNOWN treatment read?**

Node Beta has exactly one live ship reading hull 0 in the rendered frame:
`051be674…`, a `[POLICE]` `vigil` (scale 1, shield 140/140, hull 0/75,
`stance: brace`, not past its `destroyed_at_tick`). At native
`HULL_PX_PER_SCALE=14` it is small enough that the dashed-grey unknown ring
is hard to distinguish from a normal hull-state arc without zooming in —
but zoomed in, it is unambiguous: a real hull silhouette (this class has
art) surrounded by a **dashed** grey ring rather than a solid orange one,
clearly a different visual category from "derelict with 0% hull drawn as an
empty solid arc." It reads as "no data," not as damage, once you can see it
— the risk is legibility at table scale on a *small* hull, not the
encoding itself. See Tuning below.

Kitalpha has no zero-hull-alive ships in the rendered frame; all four ships
present show real hull/shield fractions.

## What the render actually shows

**Node Beta** (`?tick=1615393`, the busiest frame — 8 ships targeting at
once): two sides, red (`SIDE 1`, 11 ships) and green (`SIDE 2`, 31 ships,
including the station). Four concentric rings labelled ENGAGED / INNER /
MID / OUTER outward from centre, `agreesWithMeasurement: true` — the
measured zone means already increase in canonical order for this battle.
Side spokes read at the ships' own mean bearings (211° for side 1, 4.7° for
side 2), each side clustered along its spoke with real spread. Targeting
lines (thin cyan) connect ship pairs and draw under the hulls, reading as
depth/context rather than clutter. Four `anamnesis`/`silent_tide`
participants (all side 2, all `kind: 'missing'` in the hull pack) draw as
dashed orange chevrons, clearly distinct from the filled hull silhouettes
around them. Hull scale is visible: three scale-4 and one scale-5
participant are conspicuously larger than the eighteen scale-1 ships
sharing the table.

**Kitalpha** (`?tick=1417934`, also the busiest frame — its only frame with
any targeting at all): four sides at bearings 120.7°/81.9°/152.3°/271.7°,
matching the spec's 82/121/152/271° (label order differs only because the
adapter numbers sides by first-seen order, not by bearing). Confirmed by
direct pixel projection, not eyeballing: side 3's one ship projects to
canvas (215, 933) against a centre at (1026, 613) — down-and-left, matching
152°; side 2's ship at (1015, 934) is down-and-slightly-right, matching
82°; side 4's two ships at (1013, 153) and (1020, 354) are both
nearly-straight-up, matching 271°. No mirroring, no collapse. Side 1's one
ship (`kind: pirate`, bearing 120.7°) is not present in this particular
frame — it is not alive/spawned at this tick, not a rendering bug (its
absence is consistent across the frame's ship list, which is expected: not
every side has a live ship on every tick of a 158-tick battle).

**Kitalpha's zone/measurement disagreement, read on screen.** This battle
is the recorded case where raw measurement contradicts canonical order
(`mid` mean 0.268 < `engaged` mean 0.344 < `inner` mean 0.456 < `outer`
mean 0.798), and `zoneRings` correctly reports
`agreesWithMeasurement: false` while still drawing the rings in canonical
order with enforced-monotonic boundaries: engaged [0, 0.4], inner [0.4,
0.495], mid [0.495, 0.666], outer [0.666, 0.894]. Checking where the four
rendered ships' actual x/y radii land against those bands:

| ship | zone label | actual radius | its own band | inside its band? |
|---|---|---|---|---|
| `170f8c…` (side 2) | engaged | 0.364 | [0, 0.4] | yes |
| `b698d9…` (side 4) | mid | 0.294 | [0.495, 0.666] | **no** — plots inside the `engaged` band |
| `3ac93a…` (side 4) | outer | 0.522 | [0.666, 0.894] | **no** — plots inside the `mid` band |
| `2bcb65…` (side 3) | outer | 0.989 | [0.666, 0.894] | **no** — plots *past* the outer ring entirely |

Three of the four ships on screen sit visibly outside the ring band their
own zone label names — this is exactly the failure mode the spec's Finding
4 (`CANONICAL_ZONE_ORDER` comment) predicted, now confirmed on the actual
render rather than just in the aggregate numbers. It does not look broken
— the rings still read as four clean, correctly-ordered circles, and the
ships still read as "somewhere on the table" — but a viewer who tries to
use ring position to reason about a ship's tactical zone on Kitalpha will
be misled for 3 of 4 ships. This is **consistent with a small-N artifact**
— each zone here is a mean of one or two ships, so the "mean radius" is
really just "that ship's radius," with no reason to sit near a boundary
computed from different ships in different zones — but that explanation
rests on a sample of two battles and is not confirmed. Node Beta, with
tens of ships per zone, does not show the same ring/zone mismatch, but its
`agreesWithMeasurement` is also `true`, so it doesn't isolate which factor
matters: density or agreement. **Confirming the small-N story requires a
battle with both high per-zone density *and* `agreesWithMeasurement:
false`**, which has not yet been observed. The alternative hypothesis —
that zone and x/y radius are only loosely coupled in general, and it is
Node Beta's agreement that wants explaining rather than Kitalpha's
disagreement — is not excluded by what's rendered here.

## Tuning needed

- `HULL_PX_PER_SCALE` (currently 14): **about right for the common case,
  too small for the unknown-hull edge case.** On Node Beta's 42-ship table,
  scale-1 hulls are legible as distinct silhouettes and scale-4/5 hulls
  read as conspicuously larger without turning an engaged cluster into
  soup. The one place it strains is the dashed-grey UNKNOWN ring on a
  scale-1 hull (the `vigil` at hull 0) — at 14px it is small enough that
  the dashed pattern needs a zoom to read as "dashed" rather than "faint."
  Not a P1a blocker (P1a's bar was "draws, doesn't box, doesn't throw" —
  met), but worth a P1b look: either a slightly larger minimum radius for
  the state-arc bands specifically, or accept it as a "hover for detail"
  problem once P1b adds interaction.
- `OUTER_RING_MARGIN` (currently 1.12): does not clip *on Node Beta* — the
  outermost ships (the station at the top, the missing-art chevrons in the
  bottom band) sit comfortably inside the canvas with room for their
  labels; nothing touches the frame edge, and `TABLE_MARGIN=60` (Task 8's
  own constant, layered on top) does the same job for the canvas edge
  itself. **This is a corrected claim** — the first draft of this doc
  stated it as general, but Kitalpha's first render clipped: rings ran off
  the canvas top and bottom, the page grew scrollbars, and 3 of the 4 side
  labels (each anchored at its spoke's outer end) were pushed off-screen
  entirely. The cause was not `OUTER_RING_MARGIN` itself — it was that
  `fitView` was only ever handed the ships' own position bounds
  (`replay.bounds`), and Kitalpha's outer ring radius (0.894) is 42%
  larger than the ships' vertical half-extent (0.628), so however
  `OUTER_RING_MARGIN` scaled it, no view fit to ship positions alone could
  have contained it. Fixed in `drawFrame` by computing the rings first and
  unioning `centre ± outerRadius` into the fitted bounds
  (`tableBounds`, covered by three new unit tests) before calling
  `fitView` — verified on the re-render: all four Kitalpha side labels are
  now on-canvas, no scrollbars, and the outer ring's projected edge lands
  exactly on `TABLE_MARGIN` from the canvas edge (60px, computed
  numerically, not just eyeballed). See
  `img/holotable-p1a/kitalpha-full.jpg` for the corrected render.
  (Separately, the page-level scrollbars in the original Kitalpha capture
  were also fixed: `#table`'s CSS width was `100vw`, which counts the
  scrollbar's own width and can trigger a second, horizontal one once a
  vertical scrollbar appears from any cause — changed to `width: 100%`
  plus `overflow: hidden` on `body` in `cmd/generate-battle-holotable/page.go`,
  and both committed battle pages regenerated through the actual
  generator, not hand-edited.)
- Side palette legibility: yes, distinguishable. Node Beta's two sides
  (cyan-ish red `#e8734f` vs the base `#4fd0e8`... actually rendered as a
  clear red/orange vs. green split — see screenshot) read instantly apart
  even before reading the `SIDE N` labels. Kitalpha's four sides (red,
  cyan/white, green, and a fourth muted tone) are also each individually
  identifiable, though with only 1–2 ships per side and a five-participant
  battle this is a low bar; the palette has not yet been tested on a
  four-plus-side battle with real per-side density.

## What P1b should change before adding playback

- **Interpolate x/y linearly between frames, not by zone.** Q1's answer is
  unambiguous: zone is not a radius. Any tween that eases between "zone
  radii" will visibly fight the real per-ship x/y on every frame of every
  battle, not just Kitalpha.
- **Do not use ring position as a zone indicator when
  `agreesWithMeasurement` is false**, regardless of whether the eventual
  cause turns out to be small-N noise or a looser zone/radius coupling
  than Node Beta suggests. Kitalpha shows the consequence concretely: 3 of
  4 ships plot outside their own zone's ring band. If P1b adds any
  UI affordance that implies "this ring = this zone, ships near it are in
  that zone," it will be actively misleading on exactly the battles where
  a viewer most needs to trust it (small skirmishes). Either surface
  `agreesWithMeasurement` in the UI when false, or don't lean on ring
  position as a zone cue at all — use the explicit zone label per ship
  instead.
- **Consider a size floor on the state-arc bands independent of hull
  scale**, so the UNKNOWN dashed ring stays legible at `HULL_PX_PER_SCALE`
  on the smallest hulls, not just the common ones.
- **The targeting-line and hull draw order (lines under hulls) reads
  well** and should carry forward unchanged into playback — with dozens of
  simultaneous target lines on Node Beta, having them draw first and get
  partially occluded by hulls keeps the read on "who is where" instead of
  "a web of lines."
