# Ship Record Sheets — Design

**Date:** 2026-07-27
**Status:** Approved, ready for implementation planning

## Goal

Give every ship in the game a BattleTech/AeroTech-style record sheet: a top-down
schematic outline of the hull with damage tracks, hardpoint markers, and panels
of vessel data. Rendered as SVG so it works both as a static page in the KB and
as a live, state-filled panel on the agent dashboard.

Fidelity target is "good guesstimation", not accuracy. A Crimson salvager should
read as a boxy armored tug; a Nebula liner should read as a needle. Nobody needs
to recognise a specific rivet.

## Non-goals

- Photorealism, or any attempt to reproduce the hero art exactly.
- 3D reconstruction from images.
- Per-location damage simulation. The game does not have it (see Findings).

## Findings that shaped the design

### 1. All 332 ships already have lore, and the lore is geometric

`ships.lore` is populated for 332/332 rows, averaging 656 characters. It
frequently describes structure directly. The `prayer`:

> "The Prayer-class is not a ship. It is cargo containers welded to an engine
> with a seat bolted to the outside. There is no cockpit."

Its hero render is exactly that: a 2x2 stack of mismatched shipping containers,
a ribbed engine cone on the tail, an open tube frame with a seat welded to the
nose. Checked against art for `reliquary` ("a cathedral of docking cradles,
gantries and umbilicals" — a rack of arched bays) and `crowbar` ("salvage tug,
two autocannons, tow rig an afterthought") — both match.

Lore quality varies from "reads out the blueprint" (`prayer`, `reliquary`) to
"no geometric content at all" (`paradox`, which is entirely about shields and
vibes). It is a strong primary source, not a complete one.

### 2. The hero art is Gemini output prompted from the ship descriptions

Not renders of 3D models. Two consequences:

- **The art carries no structural information the lore didn't already have.**
  Gemini read the same text. What it adds is one interpretation — invented
  proportions and massing. Art is a plausible reading, not ground truth, and
  descriptors should not be over-fitted to it.
- **There are no 3D models**, so orthographic top-down renders cannot be
  requested from the devs.

### 3. No hero image is top-down

Every sampled image is a 3/4 hero angle at a different yaw and pitch. Tracing
any of them yields a 3/4 silhouette, not a plan view — the plan-form width along
the spine is foreshortened away. **Outline tracing is not viable** against the
existing art.

### 4. Faction design languages are strong and consistent

| Faction | Language |
|---|---|
| crimson | Boxy red armored slabs, chamfered corners, blunt prow, dorsal turrets |
| nebula | Hyper-tapered white/gold needle, swept wings, one smooth sweep |
| solarian | Gray-blue and gold, ornate; clean wedge or symmetric rack of arched bays |
| outerrim | Welded scrap, mismatched plates, open racks, asymmetric bolt-ons |
| voidborn | Flowing iridescent lobes, no hard edges anywhere |

### 5. Damage is a scalar, not per-location

From the live client, `spacemolt/pkg/game/types.go:126`:

```go
Hull   float64 `json:"hull"`     MaxHull   float64
Shield float64 `json:"shield"`   MaxShield float64
Armor  float64 `json:"armor"`    // no MaxArmor — flat mitigation, not a pool
```

Per-region damage boxes would display information the game does not have.
Armor has no current/max pair, so **armor is a printed rating, not a pip track.**

### 6. Stat ranges

`base_hull` 40–4280, `base_shield` 0–2500, `base_armor` 0–150,
`cargo_capacity` 10–3600, total slots ≤ 20, `scale` 1–5 (evenly distributed),
6 factions (5 major + pirate, 5 rows with empty faction).

## Architecture

### Shape is per-class; state is per-instance

A ship's outline never changes. Only what is painted on it changes. Therefore:

**Bake all 332 SVGs at build time in Go with stable element IDs. The dashboard
never generates geometry — it fetches an SVG and sets attributes on known IDs.**

```
kb/ships/glyphs/war_wagon.svg
  <g id="hull">
    <path id="region-bow"/>  <path id="region-port"/>  <path id="region-star"/>
    <path id="region-stern"/> <path id="region-core"/>
  </g>
  <g id="pips-hull">   <rect id="pip-hull-000"/> ... </g>
  <g id="pips-shield"> <rect id="pip-shield-000"/> ... </g>
  <g id="hardpoints">  <circle id="hp-w1"/> <circle id="hp-d1"/> ... </g>
  <rect id="cargo-bay"/> <rect id="cargo-fill"/>
```

The KB inlines these at build time. The dashboard does
`svg.querySelector('#pip-hull-017').setAttribute('class','pip-lost')`.
No WASM, no TypeScript port, no duplicated geometry. The *artifact* is shared
rather than the code.

This also gives early taste control: the SVGs are on disk and hand-tweakable
before anything consumes them.

### Pipeline

```
lore text  ──────┐
hero art  ───────┼──▶  descriptor JSON  ──▶  pkg/shipglyph.Render(desc, style)  ──▶  SVG
ship stats ──────┤        (merged)
hand overlay ────┘
```

### Source precedence, split by what each source is good at

| Source | Topology (parts, arrangement) | Proportion (aspect, mass) | Coverage |
|---|---|---|---|
| Hand overlay | wins | wins | as needed |
| **Lore** | **wins** — authored intent | weak, rarely numeric | 332/332 |
| Generated top-downs *(deferred)* | good | **wins** | future |
| Hero art | good | usable, but it's Gemini's guess | ~18 now |
| Stats | fallback | fallback | 332/332 |

Merge is **field-by-field**, so `paradox` falls through silent lore to
class+faction defaults while `prayer` takes lore's explicit assembly.

Overlays live in `overlays/shipshapes/<id>.json`, following the existing
`overlays/` convention in this repo.

## Descriptor schema

JSON Schema Draft 2020-12. One file per ship, all fields optional — anything
absent is filled by the stat-inferred layer.

```jsonc
{
  "id": "prayer",
  "source": { "topology": "lore", "proportion": "stats" },
  "aspect": 3.2,              // length / max beam
  "symmetry": "bilateral",    // bilateral | asymmetric | radial
  "hull": [
    { "kind": "container_stack", "span": [0.15, 0.75], "grid": [2, 2] },
    { "kind": "engine_cone",     "span": [0.75, 1.00], "bells": 4 },
    { "kind": "open_frame",      "span": [0.00, 0.15], "seat": true }
  ],
  "appendages": [
    { "kind": "wing", "at": 0.62, "sweep": 38, "span": 0.55, "side": "both" }
  ],
  "mountZones": {
    "weapon":  [[0.10, 0.45]],
    "defense": [[0.35, 0.70]],
    "utility": [[0.60, 0.95]]
  },
  "greeble": "heavy"          // none | light | heavy
}
```

`t` runs 0 at the nose to 1 at the tail. `y` is centerline ± half-beam.

### Hull part kinds (initial kit)

| Kind | Params | Used for |
|---|---|---|
| `beam` | `points: [[t, halfwidth], ...]` | coherent single bodies — the workhorse |
| `box` | `halfwidth` | rectangular slab sections |
| `container_stack` | `grid: [w, h]` | `prayer`-likes, cheap haulers |
| `bay_rack` | `cells` | `reliquary`, carriers, drone racks |
| `engine_cone` | `bells` | sterns |
| `open_frame` | `seat` | exposed trusses |
| `drum` | `radius` | tankers, refineries, gas harvesters |
| `disc` | `radius` | saucers |
| `lobe_cluster` | `lobes` | voidborn organics |

### Appendage kinds

`wing`, `sponson`, `nacelle`, `outrigger`, `boom`, `drone_rack`, `tow_arm`,
`antenna_mast`.

### The half-beam curve is the workhorse primitive

Rather than a fixed catalogue of plan-forms, the `beam` part is a list of
`(t, halfwidth)` control points mirrored across the centerline. Examples read
off the art:

```
comet     (nebula needle):  [(0,.01) (.25,.06) (.5,.10) (.8,.07) (1,.03)]
war_wagon (crimson spine):  [(0,.14) (.12,.22) (.5,.24) (.75,.20) (1,.16)]
reliquary (solarian rack):  [(0,.30) (.2,.32) (.9,.32) (1,.28)]
```

## Renderer — `pkg/shipglyph`

### Faction supplies the interpolator

The same control points produce all five design languages purely by changing how
they are joined:

| Faction | Interpolation |
|---|---|
| crimson | Polyline, every vertex chamfered to a 45° cut |
| nebula | Catmull-Rom, high tension — one continuous sweep |
| solarian | Polyline plus regular perpendicular fluting notches |
| outerrim | Polyline plus seeded ±8% per-vertex jitter, asymmetric port/starboard |
| voidborn | Metaball union of lobes centered on the control points |
| pirate | outerrim jitter with crimson chamfers |

One geometry routine, six looks. Adding a faction is one function.

Note this is *why voidborn stops being a problem*: curves are generated, not
traced. `{"kind":"lobe_cluster","lobes":3}` renders as clean metaballs.

### Determinism

Seeded from `hash/fnv` FNV-1a of the ship id, matching the existing convention
in `cmd/generate-factions-kb/silhouette.go`. Same input always produces
byte-identical SVG, so regeneration produces clean diffs.

### Visual style

Blueprint line art: thin stroke, no fill, hairline internal panel lines. Leaves
maximum room for damage state to sit on top without fighting the artwork.

### Hardpoint placement

Slot **counts** come from the DB (`weapon_slots`, `defense_slots`,
`utility_slots`) and are authoritative. The descriptor supplies only the
`mountZones`. A deterministic placer distributes N markers along the relevant
zones, alternating port/starboard for symmetric pairs. So `magnate` (3W/6D/5U)
gets 14 correctly-counted markers whether or not its art is ever seen, and the
sheet's loadout list lines up 1:1 with the diagram.

### Region partition

The assembled outline polygon is cut into five regions: `bow` (t < 0.25),
`port`/`starboard` (0.25 ≤ t < 0.75, split by sign of y), `stern` (t ≥ 0.75),
and `core` (a centerline inset spanning the middle). Each becomes its own
`<path>` with a stable ID.

## The record sheet

### Damage model — honest snake track

One continuous pip track threading the regions in fixed order
**bow → port → starboard → stern → core**, filled from the bow end.

This reads as per-location damage while being a faithful rendering of the single
scalar `hull` value. The identical SVG serves the KB (all pips empty — a blank
sheet) and the dashboard (filled to `1 - hull/max_hull`).

### Pip scaling

`base_hull` spans 40–4280, so a dreadnought cannot have one box per point.
Fixed ~60-pip track: `pipValue = ceil(maxHull / 60)`,
`pipCount = ceil(maxHull / pipValue)`, and **the multiplier is printed on the
sheet** — the same solution BattleTech uses for large units. Shields get their
own track at `ceil(maxShield / 40)` per pip. Armor is a printed rating.

### Layout

```
┌─────────────────────────────┬──────────────────────────┐
│  ARMOR DIAGRAM              │  VESSEL DATA             │
│   top-down glyph, blueprint │  class/category/faction  │
│   line art, region labels   │  tier · scale · price    │
│   + hardpoint markers       │  speed · fuel · cpu/power│
│   + hull pip snake          │  shipyard tier·build time│
│                             ├──────────────────────────┤
│   ┌──────────┐              │  WEAPONS & MODULES       │
│   │ 3/4 art  │  profile     │  W ▢▢▢  D ▢▢▢▢▢▢  U ▢▢▢▢ │
│   │silhouette│  badge       │  rows = slots, filled    │
│   └──────────┘              │  from default_modules or │
├─────────────────────────────┤  live ship modules       │
│  SHIELDS  ▢▢▢▢▢▢ recharge/t ├──────────────────────────┤
│  ARMOR    rating 24         │  CARGO MANIFEST          │
│  HULL     ▢▢▢▢▢▢ ×72/pip    │  ▓▓▓▓░░░ 240/960         │
├─────────────────────────────┴──────────────────────────┤
│  REQUIREMENTS & PROVENANCE                             │
│  required_skills · piloting_required · prestige_lock   │
│  inherent_capabilities · flavor_tags                   │
└────────────────────────────────────────────────────────┘
```

Every field maps to a column that already exists in the `ships` table.

The profile badge is a chroma-key mask of the existing 3/4 hero art — a flood
fill from the corner pixel with tolerance (the purple is not a fixed hex;
`comet` and `war_wagon` differ). Decoration only, off the critical path.
`paradox`, `crowbar` and `principia` are staged hangar scenes rather than
purple-background and get no badge.

## Dashboard integration

The dashboard fetches `kb/ships/glyphs/<class_id>.svg` once per ship class,
caches it, and paints per-instance state onto the stable IDs:

| State | Applied to |
|---|---|
| `hull / max_hull` | `#pip-hull-NNN` class toggles |
| `shield / max_shield` | `#pip-shield-NNN` class toggles |
| fitted modules | `#hp-w1`…, filled vs empty marker |
| cargo used/capacity | `#cargo-fill` width |

No geometry code on the client.

## Phasing

| Phase | Deliverable |
|---|---|
| **P1** | Descriptor schema + stat-inferred layer + `pkg/shipglyph` renderer + **contact sheet of all 332 glyphs**, blueprint line art, grouped by faction |
| **P2** | Lore-derived descriptors, authored in batches into `overlays/shipshapes/`, replacing the inferred topology |
| **P3** | Record sheet rendered onto the 332 existing `kb/ships/<Category>/<id>.html` pages |
| **P4** | Dashboard consumption of the baked SVGs |
| **P5** *(deferred)* | Generated top-down reference images + CV proportion layer; VLM gap-fill for silent-lore ships; profile badges |

**P1 is the taste gate.** The contact sheet exists so the whole design language
can be judged at a glance — "crimson chamfers are too soft", "outerrim jitter is
too much" — before any of it is wired into a record sheet.

## Deferred: generated top-down references (P5)

Because the hero art is generated from prompts, the camera is not fixed by
anything. A second generation pass with a locked prompt — *directly overhead,
flat lay, technical schematic, orthographic, flat purple background* — would
produce genuinely traceable plan views for all 332 from lore already in hand,
using the local tooling in `~/sd-venv`.

The risk is that diffusion models drift toward 3/4 because that is their
training distribution. Mitigation: generate N variants and **auto-select by
bilateral symmetry score on the purple mask** — a true overhead view of a
bilaterally symmetric ship scores high, a drifted 3/4 view scores low. Failures
fall back to lore + stats, which already produce a complete glyph.

The same mask also yields proportion metrics for free: bounding-box aspect,
fill ratio (boxy slab vs spindly frame), convex-hull deficiency (kitbashed vs
smooth), and the width profile along the principal axis — which *is* the
half-beam control points.

This is strictly optional. The descriptors, renderer, contact sheet and record
sheet all work without it; it slots in later as a proportion refinement.

## Verification

- `go build ./...` and `go test ./...` pass.
- `golangci-lint` introduces no new findings.
- Renderer is deterministic: regenerating twice produces no diff.
- All 332 ships produce a glyph, including those with no overlay file.
- Contact sheet renders in both light and dark KB themes.

## Open questions

- Exact pip geometry along curved region boundaries — may need a path-following
  layout rather than a grid. Resolve during P1 against real outlines.
- Whether `scale` or `cargo_capacity` should drive rendered size on the contact
  sheet, or whether all glyphs normalise to a common box. Decide by eye at P1.
