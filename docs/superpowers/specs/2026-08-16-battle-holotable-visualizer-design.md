# Battle Holotable Visualizer — Design

**Date:** 2026-08-16
**Status:** DRAFT — awaiting operator review
**Related:** `2026-07-27-ship-record-sheets-design.md` (shipglyph; superseded in
part, see Findings 3), `2026-08-07-footprint-fusion-design.md`,
`kb/data/mesh_bakeoff/` (mesh bake-off, evals in flight)

## Goal

A sci-fi bridge "holotable" view of a SpaceMolt battle: ships rendered as their
own silhouettes on a projected tactical table, replaying tick by tick from real
battle data, with weapon fire and damage rendered in the visual language of the
weapon that fired.

Two hosts, in order:

1. **KB site** — a page that takes a `battle_id`, loads that battle, and replays
   it. First and primary host.
2. **Human web client** — the same renderer embedded in the playable client,
   fetching live.

A third consumer falls out of the same assets: the **ship record sheet**
(BattleTech/AeroTech style — top-down outline as armour diagram, panels for
hull, shields, fuel, cargo, weapon loadout, damage), for both the KB ship pages
and an agent's live status view.

## Non-goals

- Physical accuracy. The game's combat space is not Newtonian and the data is
  not a physics trace; this is a legible tactical display, not a simulation.
- Recognisable-at-a-rivet ship fidelity. "Good guesstimation" remains the bar,
  inherited from the record-sheet spec.
- Rendering battles live as they happen in phase 1. Replay first; live polling
  is a later, small addition (see Phasing).
- Any server-side change. None is required — see Finding 2.

## Findings that shaped the design

### 1. `get_battle_log` is a complete reconstruction, and richer than the site

Measured against battle `a2619bbe328676445828b4e1007fe9aa` (Node Beta, 11 v 30
plus a station, 30 ticks, 42 participants, 10,293 damage). The whole battle
returned in **one call** at `limit: 200` (the max) — **1.5 MB**, `has_more:
false`. Budget roughly 50 KB per tick at ~28 live participants; a 200-tick
battle needs pagination via `tick_start`.

Per tick, one `ParticipantSnapshot` per combatant carries: `x`, `y`, `zone`,
`hull`/`max_hull`, `shield`/`max_shield`, `fuel`/`max_fuel`, `stance`,
`target_id`, `auto_pilot`, `kind` (player/pirate/police/drone/creature/station),
`side_id`, `ship_class`, `username`, `player_id`, `damage_dealt`,
`damage_taken`, `kill_count`, `flee_counter`, and the fitted `modules` list.

Event arrays per tick: `attacks`, `autopilot`, `zone_moves`, `regen`, `kills`,
`joins`, `flee`, `fuel`, `burns`, `commands`, `battle_ended`. In the sample
battle: 840 snapshots, 840 autopilot decisions, 405 zone moves, 371 attacks,
33 regen, 14 kills.

`AttackLogEntry` is a full pipeline, not a total: `hit_chance` with the actual
`hit_roll` and `hit_success`, per-weapon `WeaponFireDetail`
(`name`, `ammo_used`, `base_damage`, `damage_type`, `crit_chance`/`crit_roll`/
`crit_fired`, `after_disruption`, `instance_id`), then `raw_damage` →
`pre_hit_damage` → resists and multipliers → `shield_damage` + `hull_damage` =
`final_damage`, plus `zone_distance`, `splash`, `disrupted`.

**Weapon typing is present and sufficient for phase 3.** Observed
`damage_type`: energy (318), void (27), kinetic (24), explosive (2). Observed
`ammo_used`: `railgun`, `autocannon`, `missile`, `scrap_missiles`,
`scrap_torpedoes`, `scrap_shot`, `void_core_pack`, `em_charge_pack`, and
**`null` for 666 fires** — the beam signature (no ammo consumed).

`autopilot` carries `chosen_target` **and a `reason`** (`npc_hold_range`,
`npc_dry_retreat_retarget`, `npc_close_braced_stance`, `station_fire`, …). This
is the AI's own decision trace and appears nowhere on the public site.

### 2. The API is CORS-open to any origin, but battle reads require a login

- Endpoint `https://game.spacemolt.com/api/v1/get_battle_log`, header
  `X-Session-Id`.
- **CORS reflects any origin.** `localhost:8080` and `example.com` were each
  echoed back in `access-control-allow-origin`, with
  `access-control-allow-credentials: true`. No server change is needed for a
  browser client, now or later.
- **But an anonymous session is not enough.** `POST /session {}` returns a
  session id (30-minute TTL), and both `get_battle_log` and
  `get_battle_summary` then reject it with `not_authenticated` — a `login` is
  required.

Consequence, and it is the whole reason for the adapter split below: the KB
page **cannot** self-fetch without credentials, so phase 1 is fed by a small
credentialed helper writing `battle_<id>.json`. The in-client host later gets
live fetch for free, because the player session already exists and CORS is
already open.

### 3. The outline source has changed since the record-sheet spec

`2026-07-27-ship-record-sheets-design.md` concluded that there were **no 3D
models** and that **outline tracing was not viable**, and therefore built
`pkg/shipglyph` — procedural outlines from lore- and stat-derived descriptors.
Both premises have since been overturned by the mesh bake-off: image-to-3D now
produces per-ship meshes, 403 ships have `footprint.json` + `profile.json`, and
the KB pipeline's stated end state is **a water-tight top-down SVG per ship**.

This design therefore treats outlines as an **asset contract owned by the KB
pipeline**, not something the visualizer derives. It does not attempt to consume
`footprint.json` directly (those are `MultiPolygon` with fragments — the
`alchemist` first part is a 14-point sliver spanning 0.013 units — and cleaning
them is the pipeline's job, not the renderer's).

**The 2-D SVG is the KB pipeline's own acceptance criterion** (operator,
2026-08-16), so this design assumes a water-tight top-down SVG will exist for
every ship and draws against it directly. `shipglyph` is retained only as a
safety net — an unknown `ship_class` (a new hull shipped by the devs before the
pipeline has run, say) renders as a procedural glyph rather than a placeholder
box or an error. It is the exception, not the expected path.

### 4. The battlefield is RADIAL: concentric rings and one spoke per side

An earlier draft of this section read the reference battle as a 1-D engagement
axis, because with two sides the fleets sit on opposite arcs and x alone appears
to separate them. **That was wrong.** The official viewer draws concentric rings
labelled OUTER / MID / INNER / ENGAGED around a centre point, with each side
holding an angular sector, and the operator confirms the mechanic:

> each axis of advance/retreat is along the direction they are assigned, towards
> and away from center

So:

- **`zone` is a radial band measured from the table centre**, not a slice of an
  axis. Verified on the reference battle using the midpoint of the position
  bounds as centre — mean radius comes out monotonic and correctly ordered:
  engaged 0.58, inner 0.65, mid 0.80, outer 1.08. (Using the centroid of
  positions instead scrambles it, because the larger side drags the mean; the
  bounds midpoint is the right centre.)
- **Each side is assigned a bearing** — a spoke from the centre — and its ships
  advance inward and retreat outward along it. Bearings must be averaged as unit
  vectors, not as degrees: a side straddling 0°/360° would otherwise average to
  180°, the exact opposite of where it is.
- `x`/`y` are ordinary table coordinates; radius and bearing are derived from
  them relative to the centre.
- `zone_distance` on attacks is discrete, observed 0–5, and remains the
  authoritative range for "could this weapon reach".

**There is no fixed limit of two sides.** Three- and four-sided battles occur
and render as triangular/quadrilateral arrangements around the rings. Validated
end to end against `b131fd5aae68420107dd20e93d15d3ba` (Kitalpha, 158 ticks, 5
participants): four sides at bearings 82°, 121°, 152° and 271°, side 4 winning
with 2 of the 5. There may be an upper bound on sides, but it is unknown — so
nothing in the model or renderer may assume a count. In particular, "side 2
mirrors" is not a valid facing rule; facing follows the side's own spoke.

**`zone` + `zone_distance` are the authoritative tactical space; x/y are the
presentation layout.** Where they disagree, trust `zone`.

### 5. The existing holo demo is dependency-free canvas 2D

`kb/data/mesh_bakeoff/pointcloud_holo.html` is hand-rolled canvas 2D with its
own ISO projection, scan-line pulse, and idle rotation (`ISO`, `SCALE`,
`DETAIL_STEPS`, `IDLE_MS`, `SHIPS`) — **no Three.js, no external libraries**.
That is a significant asset: the same renderer can drop into a static KB page
and a React component with no build-system negotiation, and it establishes the
holo aesthetic (projection cone, ground rings, scan pulse) that this design
adopts wholesale.

Its weight is also the phase-2 warning: **4 MB for a single ship's point
cloud.** Point clouds do not scale to 42 simultaneous ships and are, per the
operator, a coolness demo rather than the intended end state.

## Architecture

Three layers, because there are two hosts and the data source differs between
them.

```
   ┌─────────────────────────────────────────────────────────┐
   │ HOSTS                                                   │
   │  kb/site/battles/<id>.html   (phase 1, file-fed)        │
   │  client web UI component     (later, live-fed)          │
   └───────────────┬─────────────────────────────────────────┘
                   │ replay model + asset resolver
   ┌───────────────▼─────────────────────────────────────────┐
   │ RENDERER  — canvas 2D, zero dependencies                │
   │  holotable presentation · ship draw · FX · timeline UI  │
   │  knows: ReplayModel, AssetResolver.  knows NOT: the API │
   └───────────────▲─────────────────────────────────────────┘
                   │
   ┌───────────────┴──────────────┐   ┌──────────────────────┐
   │ ADAPTER: file               │   │ ADAPTER: live         │
   │ battle_<id>.json on disk    │   │ POST get_battle_log   │
   │ (written by fetch helper)   │   │ with player session   │
   └─────────────────────────────┘   └──────────────────────┘
```

**The renderer must never import the API shape.** Everything it needs arrives as
the replay model below. This is what lets the KB host ship before the client
host exists, and what lets the client host add live polling without touching
rendering code.

### Replay model

Normalized once, at adapter boundary. Field names are ours, not the API's, so a
server-side rename is a one-file change.

```ts
ReplayModel {
  battleId, systemId, systemName, status,        // 'active' | 'completed'
  startTick, tickCount, hasStation,
  outcome, winningSide,
  bounds: {xMin, xMax, yMin, yMax},              // computed over all frames
  participants: Map<playerId, {
    playerId, username, shipClass, kind, sideId,
    maxHull, maxShield, maxFuel, modules[],
    firstTick, lastTick, destroyedAtTick|null,   // derived from kills
  }>,
  frames: Frame[]                                // one per tick, dense
}

Frame {
  tick,
  ships: Map<playerId, {
    x, y, zone, hull, shield, fuel, stance,
    targetId, autoPilot, damageDealt, damageTaken, killCount, fleeCounter,
  }>,
  shots: Shot[],                                 // from attacks, one per WEAPON fire
  moves: {playerId, from, to, reason}[],
  kills: {killerId, victimId}[],
  repairs: {playerId, shieldRegen, armorRepair, remoteRepair}[],
  chatter: {playerId, reason, chosenTarget}[],   // from autopilot
}

Shot {
  fromId, toId, weaponName, ammo|null, damageType,
  hit, crit, finalDamage, shieldDamage, hullDamage, zoneDistance, splash,
}
```

Two derivations are the adapter's job, not the renderer's:

- **`destroyedAtTick`** — the API gives `kills` events; the renderer needs to
  know when to stop drawing a hull and when to play an explosion.
- **Snapshot sparsity.** Snapshots appear only for participants present that
  tick (15 of 42 on tick 1, growing as sides engage). The adapter must decide
  per participant between *not yet joined* (do not draw) and *destroyed* (draw
  wreck), and carry the last known state forward for anything in between.

### Asset contract

Owned by the KB pipeline; the renderer only consumes. **Keyed by
`ship_class`** exactly as `ParticipantSnapshot.ship_class` emits it (lowercase,
e.g. `vigil`) — that is the only join key the battle log provides.

**Phase 1 — top-down SVG.** These now EXIST at
`kb/data/footprints/hy3d-svg/<name>.svg` (402 files as of 2026-08-16), and the
contract below is the operator's, ratified against the shipped assets — it
replaces an earlier draft here that guessed nose-up and centroid-origin. Both
guesses were wrong; the assets are the source of truth.

- **Bow-right.** The hull points toward +X in its own space, so drawing a ship
  is a rotation to its heading — bow toward the centre, along its side's spoke,
  or toward `target_id`. It is never a mirror: mirroring was an artifact of the
  mistaken linear reading, and on a radial table it would flip hulls that should
  simply be rotated.
- **Length-normalized to 1000 units.** The viewBox is `0 0 1020 <h>` — 1000
  units of hull along X plus a 10-unit margin each side — with height varying
  by aspect. Scale by hull scale/tier at draw time so a scale-1 `cobble` and a
  scale-4 `junk_convoy` share a table correctly.
- **One closed path, `fill-rule="evenodd"`** for holes, so a single fill yields
  the silhouette and a single stroke yields the outline glow. Confirmed on the
  shipped assets.
- **`data-ship`, `data-aspect`, `data-frame-ambiguous`, `data-adjustments`** on
  every root element. `data-frame-ambiguous="true"` is the pipeline telling the
  renderer it is unsure of the hull's frame — worth surfacing in a debug view
  rather than silently trusting.
- No fills, strokes, or colours baked in — the renderer themes them.

**Coordinate convention** (operator, ratified against the shipped assets):

- **Origin (0,0) is the top-left of the viewBox** — in ship terms the
  stern-side / starboard-side corner.
- **x increases stern → bow**, so the bow is at the right edge.
- **y increases starboard → port** (standard SVG y-down).
- The hull is **inset by a 10-unit margin on all sides**, so its own bbox spans
  `(10,10) → (W−10, H−10)`, with **W = 1020 always** (1000 units of hull length
  plus margins) and **H varying by ship width**.

Verified across all 395 files: W is 1020 on every one, and `data-aspect` equals
`1000/(H−20)` on 391 of them — i.e. aspect is hull length over hull width, with
the margins removed.

The renderer's transform follows directly: hull centre is `(510, H/2)` and hull
length is 1000, so drawing a ship of table-length `L` at table position `(x, y)`
with heading `θ` is translate `−(510, H/2)`, scale `L/1000`, rotate `θ`,
translate to `(x, y)`.

`θ` comes from the radial layout, in order of preference: toward `target_id` if
the ship has one, else inward along its own bearing from `Centre`
(`atan2(centre.y − y, centre.x − x)`), which is the axis its advance and retreat
run along. Bow-toward-centre is the sensible default because that is the
direction a ship closes and withdraws on.

**The join key is settled: the filename IS `data-ship`.** All 395 files satisfy
`filename == data-ship`, and `data-ship` is the ships-catalog id, which is also
exactly what the battle log's `ship_class` emits. An earlier draft of this spec
described a two-step faction-prefix resolver; the pipeline's rename made it
unnecessary. Index by filename (or equivalently by `data-ship`) and stop there.

Provenance is recorded per file in `data-kb-match`, and the 395 files split
**275 catalog-named / 120 art-only**:

- **verbatim** 59 — the art stem already was the catalog id, including the six
  genuinely faction-prefixed ids such as `crimson_devastator`.
- **stripped** 212 — a faction prefix removed to reach the catalog id
  (`outerrim_rapid_smelter` → `rapid_smelter.svg`).
- **fuzzy** 4 — name variants reconciled by hand: `huffenpuff`→`huffnpuff`,
  `first_step`→`first_step_dreadnought`, `meridian`→`meridian_freighter`,
  `survey`→`survey_vessel`. These are the only joins that were inferred rather
  than derived, so they are the ones to re-check if a hull ever looks wrong.
- **none** 120 — real art with no catalog row, keeping its stem name and
  flagged with a red "no KB ship" badge in the gallery. Likely unreleased or
  removed ships; note that several are hulls our own fleet still flies
  ([[reference_legacy_ship_classes_erased_by_refresh]]), so art-only does not
  mean unused.

Seven duplicate art files were skipped — the old two-view anomaly pairs, where
the verbatim-named art won (`precept` kept over `solarian_precept`).
`data-art-stem` preserves the original asset name for tracing.

**Coverage, measured 2026-08-17:** 275 of 338 catalog ships (81%) have art, and
63 do not. Against what actually matters for a replay — the hulls our fleet
flies — coverage is **433 of 435 ships, 99.5%**, with only `rubble` and
`scrutiny` missing at one ship each. For the reference battle, 14 of 16 classes
resolve; `anamnesis` and `silent_tide` do not, and the station has no
`ship_class` at all. The fallback therefore still has to exist, but it is a rare
path rather than the common one.

**Phase 2 — low-poly mesh** (`kb/data/ship_mesh/<ship_class>.json` or `.glb`):

- Target **hundreds of triangles**, decimated from the bake-off meshes. At
  holotable scale (a ship is tens of pixels across) that is ample, and it is
  what makes 42 simultaneous ships viable where point clouds are not.
- Same canonical orientation and normalization as the SVG.
- Intermediate step, cheap and useful: **extrude the top-down SVG against the
  side-view silhouette** to get a 2.5D hull without waiting for mesh
  decimation. The bake-off triage sheet already renders top/side/front, so the
  side silhouettes should be preserved as pipeline outputs rather than
  discarded as eval artifacts.

**Fallback:** any `ship_class` with no asset renders via `pkg/shipglyph` — never
a placeholder rectangle, never a blocking error. With the SVG set as a KB
acceptance criterion this should stay rare, but it must still be tested, since
the failure it guards against (a hull the devs ship before the pipeline runs)
appears in production rather than in development.

### Presentation

Adopted from `pointcloud_holo.html`: dark field, projection cone, concentric
ground rings, scan-line pulse, idle rotation. Additions specific to the table:

- **Zone bands as concentric rings** around `Centre`, labelled
  outer/mid/inner/engaged from the outside in — matching the official viewer, so
  a reader who has seen one can read the other.
- **One spoke per side**, anchored at the side's mean bearing, since advance and
  retreat run along it. Side rosters and labels hang off the outer end of their
  own spoke, which is what keeps a four-way battle legible.
- **Side identity by colour**, `kind` by outline treatment (station gets a
  fixed emplacement glyph rather than a hull; drones/creatures read distinctly).
- **Per-ship state ring**: shield as an outer arc, hull as an inner arc, both
  as fractions of max. This is the record sheet's armour diagram, reduced.
- **Targeting lines** from `targetId`, drawn faint and continuously — the
  single most legible signal of what the fleet is actually doing.
- **Chatter rail**: a scrolling column of `autopilot` reasons and `zone_moves`,
  which turns the AI's decisions into the bridge-log texture the aesthetic wants.
- **Transport**: play/pause, scrub, step, speed. Tick number and game tick both
  visible.

### Weapon and damage FX (phase 3)

Driven off `Shot`, resolved in this order:

| Condition | Treatment |
|---|---|
| `ammo` is `missile`/`scrap_missiles`/`scrap_torpedoes` | Blob traversing the gap over ~1 tick, with a trail |
| `ammo` is `null` and `damageType` is `energy` | Beam, drawn for a fraction of the tick |
| `ammo` is `railgun`/`autocannon`/`scrap_shot` | Dashed tracer stream, morse-like |
| `damageType` is `void` | Distortion/warp treatment, distinct palette |
| `damageType` is `explosive` | Short burst at the target |
| `hit === false` | Same treatment, but the tracer passes the target and fades — misses must be visible, they are most of the drama |
| `crit === true` | Amplified flash plus a hit marker |
| `finalDamage` splits into shield vs hull | Shield hits ripple the shield arc; hull hits scar the silhouette |

Destruction (`kills`) plays a stock explosion at the victim, after which the
hull renders as a wreck or is removed.

## Phasing

| Phase | Deliverable | Depends on |
|---|---|---|
| **P0** | Fetch helper (`battle_<id>.json`) + replay-model adapter + adapter tests against the captured sample battle | nothing — data verified today |
| **P1** | Canvas holotable drawing the **real SVG outlines**: zone bands, state rings, targeting lines, transport, chatter rail | P0 + KB SVG set |
| **P2** | Record-sheet view reusing the same assets and the same participant snapshot | P1 |
| **P3** | 2.5D: extrude top+side to a rotatable table ("lazy susan"), then low-poly meshes when decimation lands | P2 + side silhouettes preserved |
| **P4** | Weapon/damage-typed FX per the table above | P1 |
| **P5** | Client host: same renderer, live adapter, polling while `status === 'active'` | P1 + client-side typed command |

**P1 is the taste gate**, mirroring the record-sheet spec's own P1 rationale:
the whole visual language should be judgeable from one replay before any asset
or FX work is committed to it.

## Cross-repo work

Mostly KB, with one item in `spacemolt`:

- **`spacemolt`**: add a typed `GetBattleLog` (and `GetBattleSummary`) to the Go
  client. Today only passthrough exists — `command_coverage_test.go` marks
  `get_battle_log` a "stopgap". Follow CLAUDE.md's pattern: client method →
  `GameClient` interface → runner dispatch → classify as a **read** in
  `isActionCommand` → response struct in `serverapi`. Note the interface change
  breaks the `pkg/agent` and `pkg/skills` mocks, which `go build` will not
  catch — run `go test ./...`.
- **`kb`**: everything else — fetch helper, adapter, renderer, page, assets.

## Verification

- **Adapter**: golden test against the captured `a2619bbe…` battle. Assert 30
  frames, 42 participants, 14 kills, 371 attacks expanding to the right shot
  count, and that `destroyedAtTick` is set for exactly the 14 victims.
- **Sparsity**: assert a participant absent from early snapshots is not drawn
  before `firstTick`, and that a destroyed one stops updating.
- **Asset fallback**: assert an unknown `ship_class` resolves to a shipglyph and
  renders, rather than throwing or drawing a box.
- **Contract**: assert every shipped SVG is closed, nose-up, and centroid-origin
  — a lint over the asset directory, so a pipeline regression fails loudly.
- **Visual**: the sample battle replayed end to end is the acceptance artifact.

## Open questions

1. **Does `x`/`y` drift within a zone, or is it quantised per zone?** The sample
   shows continuous values and a clean advance, but one battle is not proof. If
   x is effectively a function of zone, interpolation between ticks should be
   eased rather than linear, or ships will slide unnaturally.
2. **Station placement.** `has_station` is true and the station appears as a
   participant with its own x/y; whether it should sit on the table as a hull or
   anchor a side's baseline is a presentation call best made from a first render.
3. **Tick cadence for playback.** Game ticks are ~10 s; replay wants to be much
   faster, but interpolation between two snapshots needs a chosen easing.
4. **`hull` reads 0 for some participants, including on the first tick.** In
   the reference export 802 of 840 ship-states carry `hull > 0`, but 38 do not,
   and 4 participants never report a positive hull at all — including a police
   ship on tick 1 with `max_hull: 75`, full shields, and no damage taken. Some
   of those zeros are genuine (a destroyed ship's last state), but not the
   tick-1 ones. Until this is understood the renderer should treat a 0/max hull
   as "unknown" rather than drawing an empty hull bar, or a third of the police
   wing will look derelict from the opening frame.
5. **The station has no `ship_class`** (empty string), so it is the first real
   consumer of the asset fallback and needs its own emplacement glyph rather
   than a hull.
6. **Sequencing of P0/P1 against the KB evals.** The SVG set is a KB acceptance
   criterion, so P1 draws against it directly — but P0 (fetch helper, adapter,
   golden tests) depends on nothing and can be built now. Whether P1 waits for
   the full set or starts against the subset already passing eval is a
   scheduling call, not a design one.
