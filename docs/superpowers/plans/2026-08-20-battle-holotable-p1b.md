# Battle Holotable P1b (Motion) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the static P1a holotable frame into a playable battle replay — transport controls, linear inter-tick interpolation, and a grouped chatter rail — without changing how a single frame looks.

**Architecture:** A `Player` drives a `requestAnimationFrame` loop whose pure core, `advance(state, dtMs, opts)`, returns the new `{frameIndex, t}` plus the tick boundaries it crossed. Each animation frame builds a synthetic frame with the pure `interpolateFrame(prev, next, t)` and hands it to the existing draw path. The static layer — rings, spokes, labels — bakes once into an offscreen canvas and blits, so the per-frame cost is ships only.

**Tech Stack:** Dependency-free ES2020 + canvas 2D (no build step, no bundler), Go 1.25 with `html/template` for the page shell, `node --test` on Node v22.16.0.

**Spec:** `docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md` — read the section **"P1b — motion (ratified 2026-08-20)"** before starting. The plan argues from it.

## Global Constraints

- **Repo:** all work is in `/home/robert/spacemolt/kb` (module `github.com/rsned/spacemolt-kb`). Nothing in this plan touches the sibling `spacemolt` repo except Task 9, which only *reads* from it by running an already-built binary.
- **JS test gate:** `node --test tests/js/*.test.js` — **never** `node --test tests/js/`. On Node v22.16.0 a directory argument makes the runner `require` the directory and report `pass 0 / fail 1` on a clean tree.
- **Baseline:** 113 JS tests pass and `go build ./...`, `go test ./...`, `golangci-lint run` are clean before Task 1. Every task must leave them that way.
- **No dependencies.** No npm packages, no CDN scripts, no bundler. `holotable.js` and `holotable-player.js` are plain files loaded by `<script src>`.
- **House module pattern:** every JS file ends with `if (typeof module !== 'undefined' && module.exports) { module.exports = {...}; }` and guards any `document` access with `if (typeof document !== 'undefined')`, so the same file loads in a browser and `require`s in a test. See the bottom of `kb/battles/holotable.js`.
- **Linear interpolation only.** The spec settled that radius is not a function of zone. Never ease between ring radii.
- **UNKNOWN never fades into a reading.** A `hull` or `shield` of `0` means "no data" under `fractionOf`. If either endpoint of an interpolation is `<= 0`, hold `prev`'s value for the whole interval.
- **No destruction FX.** Alpha only. No explosion, no kill flash, no persistent wreck glyph — those belong to P4.
- **Go lint:** `golangci-lint run` must report 0 issues. Run it after any `.go` change.
- **Binaries go in `bin/`**, never the repo root.
- **Commit staging is explicit.** `git add <paths>`, never `git add -A` — this repo carries a large amount of unrelated generated churn in `kb/` and `data/`.

---

## File Structure

| File | Responsibility |
|---|---|
| `kb/battles/holotable.js` *(modify)* | Geometry and drawing only. Gains `layoutTable`, an `alpha` channel through `drawShip`/`drawStationGlyph`, and a split of `drawFrame` into `drawStatic` + `drawShips`. |
| `kb/battles/holotable-player.js` *(create)* | Motion only: `interpolateFrame`, `advance`, `groupChatter`, the rail DOM, the transport wiring, the rAF loop, and page init. |
| `cmd/generate-battle-holotable/page.go` *(modify)* | Page shell: stage/rail layout, transport bar markup, second `<script>` tag. |
| `tests/js/holotable.test.js` *(modify)* | Repair two zone-order tests that pass by fixture coincidence; add a `layoutTable` threading test. |
| `tests/js/holotable-player.test.js` *(create)* | `interpolateFrame`, `advance`, `groupChatter`. |
| `scripts/make-stress-replay.js` *(create)* | Generates the ~400-hull × 600-tick synthetic stress replay. |
| `docs/holotable-p1b-findings.md` *(create)* | Measurements, screenshots, and the verdict. |

The seam between the two JS files is **"draws a frame"** versus **"decides which frame, and when"**. Nothing in `holotable.js` may import or reference the player; the player calls into the renderer.

---

## Task 1: `layoutTable`, and repair the parked zone-order tests

Extracts the per-frame table geometry into one pure function, which is both what the static-layer bake needs and what makes the `opts.zoneOrder` threading testable. Also repairs two existing tests that pass today for the wrong reason.

**Files:**
- Modify: `kb/battles/holotable.js` (`drawFrame` around line 669, exports at the bottom)
- Test: `tests/js/holotable.test.js` (lines 152-179, plus new tests)

**Interfaces:**
- Consumes: existing `zoneRings`, `deriveZoneOrder`, `outerRadius`, `tableBounds`, `fitView`, `TABLE_MARGIN`.
- Produces: `layoutTable(replay, width, height)` → `{rings, bounds, view}`. Tasks 5 and 7 both consume it.

**Why the two existing tests are weak.** `zoneRings` appends any zone name that `zoneOrder` does not mention (the second loop, "a zone name outside zoneOrder entirely… is appended rather than dropped"). The existing 5-band test passes `['engaged', 'inner', 'mid', 'outer', 'flank']` — the first four *are* `CANONICAL_ZONE_ORDER` and `flank` would be appended last anyway. So if `opts.zoneOrder` were dropped entirely, the fallback produces the identical order and the test still passes. The `deriveZoneOrder` test has the same defect: `['outer','mid','inner','engaged']` reversed is exactly `CANONICAL_ZONE_ORDER`, so "reversed the input" and "returned the constant" are indistinguishable.

- [ ] **Step 1: Repair the 5-band ordering test**

In `tests/js/holotable.test.js`, replace the test that begins at line 152 (`'zoneRings orders zones exactly as given via opts.zoneOrder, ...'`) with this. The only change is that `flank` moves to the **front** of `zoneOrder`, where the fallback could never put it:

```js
test('zoneRings orders zones exactly as given via opts.zoneOrder, including a band CANONICAL_ZONE_ORDER does not name', () => {
  const centre = {x: 0, y: 0};
  const frames = [{
    tick: 1,
    ships: [
      {player_id: 'a', x: 0.4, y: 0, zone: 'engaged'},
      {player_id: 'b', x: 0.6, y: 0, zone: 'inner'},
      {player_id: 'c', x: 0.8, y: 0, zone: 'mid'},
      {player_id: 'd', x: 1.0, y: 0, zone: 'outer'},
      {player_id: 'e', x: 1.2, y: 0, zone: 'flank'},
    ],
  }];
  // 'flank' leads deliberately. zoneRings appends any zone its zoneOrder does
  // not name, so a 'flank'-last expectation is satisfied by the
  // CANONICAL_ZONE_ORDER fallback too — the test would keep passing if the
  // opts.zoneOrder threading were deleted. Leading with it means only a
  // genuinely honoured zoneOrder can produce this order.
  const zoneOrder = ['flank', 'engaged', 'inner', 'mid', 'outer'];
  const rings = ht.zoneRings(frames, centre, {zoneOrder});
  assert.deepStrictEqual(rings.map(r => r.zone), zoneOrder,
    'a band named first in zoneOrder must lead, not be appended by the fallback');
});
```

- [ ] **Step 2: Repair the `deriveZoneOrder` reversal test**

Replace the test at line 170 (`'deriveZoneOrder reverses replay.zones, ...'`) with one whose reversal is not the canonical constant:

```js
test('deriveZoneOrder reverses replay.zones, since the adapter ships nearest-to-contact last', () => {
  // Deliberately not the canonical band names: reversing
  // ['outer','mid','inner','engaged'] yields exactly CANONICAL_ZONE_ORDER, so
  // that fixture cannot tell "reversed the input" from "returned the constant".
  assert.deepStrictEqual(
    ht.deriveZoneOrder({zones: ['far', 'near', 'contact']}),
    ['contact', 'near', 'far']);
  assert.notDeepStrictEqual(
    ht.deriveZoneOrder({zones: ['far', 'near', 'contact']}),
    ht.CANONICAL_ZONE_ORDER);
});
```

- [ ] **Step 3: Run both repaired tests — they must still pass**

Run: `node --test tests/js/*.test.js --test-name-pattern 'zoneOrder|deriveZoneOrder'`

Expected: PASS. These repairs sharpen existing assertions; they do not describe new behaviour. If either fails, the production code has a real bug — stop and report it rather than weakening the test.

- [ ] **Step 4: Write the failing `layoutTable` test**

Append to `tests/js/holotable.test.js`:

```js
test('layoutTable threads the replay zone order through to the rings', () => {
  // Non-canonical band names, so a dropped opts.zoneOrder cannot coincidentally
  // produce the right order: the fallback would append all three in data order.
  const replay = {
    zones: ['far', 'near', 'contact'],
    centre: {x: 0, y: 0},
    bounds: {x_min: -1, x_max: 1, y_min: -1, y_max: 1},
    frames: [{
      tick: 1,
      ships: [
        {player_id: 'a', x: 0.9, y: 0, zone: 'far'},
        {player_id: 'b', x: 0.5, y: 0, zone: 'near'},
        {player_id: 'c', x: 0.2, y: 0, zone: 'contact'},
      ],
    }],
  };
  const layout = ht.layoutTable(replay, 800, 800);
  assert.deepStrictEqual(layout.rings.map(r => r.zone), ['contact', 'near', 'far'],
    'rings must run contact-outward, which only deriveZoneOrder(replay) produces');
});

test('layoutTable fits a view that contains the outer ring, not just the ships', () => {
  // The ships span y in [-0.1, 0.1] but the rings reach far past them, so a
  // view fitted to ship bounds alone would clip the outermost ring.
  const replay = {
    zones: ['outer', 'mid', 'inner', 'engaged'],
    centre: {x: 0, y: 0},
    bounds: {x_min: -2, x_max: 2, y_min: -0.1, y_max: 0.1},
    frames: [{
      tick: 1,
      ships: [
        {player_id: 'a', x: 2, y: 0, zone: 'outer'},
        {player_id: 'b', x: 0.2, y: 0, zone: 'engaged'},
      ],
    }],
  };
  const layout = ht.layoutTable(replay, 800, 800);
  const outer = ht.outerRadius(layout.rings);
  const top = ht.project(0, -outer, layout.view);
  const bottom = ht.project(0, outer, layout.view);
  assert.ok(top.py >= 0, `outer ring escaped the top of the canvas at ${top.py}`);
  assert.ok(bottom.py <= 800, `outer ring escaped the bottom of the canvas at ${bottom.py}`);
});
```

- [ ] **Step 5: Run it to confirm it fails**

Run: `node --test tests/js/*.test.js --test-name-pattern 'layoutTable'`

Expected: FAIL — `ht.layoutTable is not a function`.

- [ ] **Step 6: Add `layoutTable` and have `drawFrame` use it**

In `kb/battles/holotable.js`, immediately **above** `function drawFrame(...)`, add:

```js
// layoutTable computes everything about a battle's table that does not change
// from tick to tick: the zone rings, the bounds that contain both the ships and
// the outermost ring, and the view that maps model space onto the canvas.
//
// It is pure and depends only on the replay and the canvas size, which is the
// whole point: P1b bakes the static layer once from this, so the whole-battle
// scan inside zoneRings stops happening per frame. It also puts the
// deriveZoneOrder threading somewhere a test can reach without a canvas.
function layoutTable(replay, width, height) {
  const rings = zoneRings(replay.frames, replay.centre, {zoneOrder: deriveZoneOrder(replay)});
  const bounds = tableBounds(replay.bounds, replay.centre, outerRadius(rings));
  const view = fitView(bounds, width, height, TABLE_MARGIN);
  return {rings, bounds, view};
}
```

Then in `drawFrame`, delete the three lines that computed `rings`, `bounds` and `view` and the `TODO(P1b)` paragraph about `zoneRings` being on the per-frame path, replacing them with a single call:

```js
  ctx.save();
  const layout = layoutTable(replay, width, height);
  const rings = layout.rings;
  const view = layout.view;
```

Leave the second `TODO(P1b)` comment (the `try/finally` one) alone — Task 5 removes it.

- [ ] **Step 7: Export it**

In the `module.exports` block at the bottom of `kb/battles/holotable.js`, add `layoutTable` to the last line alongside `tableBounds`:

```js
    busiestTick, pickFrame, drawFrame, tableBounds, layoutTable,
```

- [ ] **Step 8: Run the whole JS suite**

Run: `node --test tests/js/*.test.js`

Expected: PASS, **115 tests** — the 113 baseline plus the two new `layoutTable` tests. The two repairs sharpen existing tests and add no count.

- [ ] **Step 9: Verify the render did not change**

Run:
```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable --replay data/battles/a2619bbe328676445828b4e1007fe9aa.json
git diff --stat kb/battles/
```
Expected: `git diff --stat kb/battles/` reports **no changes** — this task is a pure refactor of the renderer, and the generator's output does not depend on it.

- [ ] **Step 10: Commit**

```bash
git add kb/battles/holotable.js tests/js/holotable.test.js
git commit -m "refactor(holotable): extract layoutTable, and repair two coincidental zone-order tests

layoutTable pulls the per-battle geometry — rings, bounds, view — out of
drawFrame into one pure function, which is what the P1b static-layer bake
needs and what makes the deriveZoneOrder threading reachable from a test.

The two repaired tests passed for the wrong reason: zoneRings appends any
band its zoneOrder does not name, so a flank-last expectation was satisfied
by the CANONICAL_ZONE_ORDER fallback too, and reversing the canonical band
names produces the canonical constant. Both fixtures now differ from what
the fallback could produce."
```

---

## Task 2: `interpolateFrame`

The heart of motion: a pure function producing a synthetic frame between two real ones, which the existing draw path already knows how to render.

**Files:**
- Create: `kb/battles/holotable-player.js`
- Test: `tests/js/holotable-player.test.js`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: `interpolateFrame(prev, next, t)` → `{tick, t, ships}`, where each ship carries every field of its source plus an `alpha` in `[0, 1]`. Tasks 5 and 7 consume it.

**The contract, from the spec:**

| Case | Treatment |
|---|---|
| Present in both frames | `x`, `y` lerp linearly |
| `zone`, `stance`, `target_id` | Hold `prev`'s value across the whole interval |
| `hull`, `shield`, `fuel` | Lerp, unless either endpoint is `<= 0` — then hold `prev` |
| Present in `next` only | Alpha `0 → 1`, held at `next`'s position |
| Present in `prev` only | Alpha `1 → 0`, held at `prev`'s position |

The returned frame carries `tick: prev.tick`. `hullState` reads `frame.tick` to decide whether a participant is past its `destroyed_at_tick`, and a ship must stay alive-but-fading across the interval that ends in its death, not flip to dead at the start of it.

- [ ] **Step 1: Write the failing tests**

Create `tests/js/holotable-player.test.js`:

```js
'use strict';
const test = require('node:test');
const assert = require('node:assert');
const hp = require('../../kb/battles/holotable-player.js');

const shipA = (over) => Object.assign({
  player_id: 'a', x: 0, y: 0, zone: 'outer', hull: 100, shield: 50, fuel: 80,
  stance: 'fire', target_id: 'b', auto_pilot: true,
}, over || {});

test('interpolateFrame lerps position linearly for a ship present in both frames', () => {
  const prev = {tick: 10, ships: [shipA({x: 0, y: 0})]};
  const next = {tick: 11, ships: [shipA({x: 2, y: 4})]};
  const mid = hp.interpolateFrame(prev, next, 0.5);
  assert.strictEqual(mid.ships[0].x, 1);
  assert.strictEqual(mid.ships[0].y, 2);
  assert.strictEqual(mid.ships[0].alpha, 1);
  assert.strictEqual(mid.tick, 10, 'the interpolated frame stays on the departing tick');
});

test('interpolateFrame holds discrete fields at their previous value', () => {
  const prev = {tick: 10, ships: [shipA({zone: 'outer', stance: 'fire', target_id: 'b'})]};
  const next = {tick: 11, ships: [shipA({zone: 'mid', stance: 'brace', target_id: 'c'})]};
  const mid = hp.interpolateFrame(prev, next, 0.99);
  assert.strictEqual(mid.ships[0].zone, 'outer');
  assert.strictEqual(mid.ships[0].stance, 'fire');
  assert.strictEqual(mid.ships[0].target_id, 'b',
    'a target acquired next tick must not draw its line until the ship arrives');
});

test('interpolateFrame lerps hull and shield when both endpoints are real', () => {
  const prev = {tick: 10, ships: [shipA({hull: 100, shield: 40})]};
  const next = {tick: 11, ships: [shipA({hull: 60, shield: 20})]};
  const mid = hp.interpolateFrame(prev, next, 0.5);
  assert.strictEqual(mid.ships[0].hull, 80);
  assert.strictEqual(mid.ships[0].shield, 30);
});

test('interpolateFrame holds hull when either endpoint reads zero, so UNKNOWN never fades into a reading', () => {
  // 0 means "no data" under fractionOf, not "destroyed". Lerping 100 -> 0 would
  // draw a hull arc steadily draining to empty on a ship nobody has shot.
  const dying = hp.interpolateFrame(
    {tick: 10, ships: [shipA({hull: 100})]},
    {tick: 11, ships: [shipA({hull: 0})]}, 0.5);
  assert.strictEqual(dying.ships[0].hull, 100);

  const waking = hp.interpolateFrame(
    {tick: 10, ships: [shipA({hull: 0})]},
    {tick: 11, ships: [shipA({hull: 100})]}, 0.5);
  assert.strictEqual(waking.ships[0].hull, 0, 'unknown holds until the boundary');
});

test('interpolateFrame fades in a ship that appears only in the next frame', () => {
  const prev = {tick: 10, ships: []};
  const next = {tick: 11, ships: [shipA({x: 7, y: 9})]};
  const mid = hp.interpolateFrame(prev, next, 0.25);
  assert.strictEqual(mid.ships.length, 1);
  assert.strictEqual(mid.ships[0].alpha, 0.25);
  assert.strictEqual(mid.ships[0].x, 7, 'an arriving hull holds at where it arrives');
  assert.strictEqual(mid.ships[0].y, 9);
});

test('interpolateFrame fades out a ship that is gone from the next frame', () => {
  const prev = {tick: 10, ships: [shipA({x: 3, y: 4})]};
  const next = {tick: 11, ships: []};
  const mid = hp.interpolateFrame(prev, next, 0.25);
  assert.strictEqual(mid.ships.length, 1);
  assert.strictEqual(mid.ships[0].alpha, 0.75);
  assert.strictEqual(mid.ships[0].x, 3, 'a departing hull holds at where it was last seen');
});

test('interpolateFrame with no next frame returns the previous frame at full opacity', () => {
  const prev = {tick: 10, ships: [shipA({x: 3})]};
  const only = hp.interpolateFrame(prev, null, 0.5);
  assert.strictEqual(only.ships.length, 1);
  assert.strictEqual(only.ships[0].alpha, 1);
  assert.strictEqual(only.ships[0].x, 3);
});

test('interpolateFrame clamps t outside [0,1]', () => {
  const prev = {tick: 10, ships: [shipA({x: 0})]};
  const next = {tick: 11, ships: [shipA({x: 10})]};
  assert.strictEqual(hp.interpolateFrame(prev, next, -3).ships[0].x, 0);
  assert.strictEqual(hp.interpolateFrame(prev, next, 4).ships[0].x, 10);
});

test('interpolateFrame preserves fields the renderer reads but does not interpolate', () => {
  const prev = {tick: 10, ships: [shipA({stale: true, auto_pilot: false})]};
  const next = {tick: 11, ships: [shipA({stale: true, auto_pilot: false})]};
  const mid = hp.interpolateFrame(prev, next, 0.5);
  assert.strictEqual(mid.ships[0].stale, true, 'zoneRings filters on stale; dropping it changes ring radii');
  assert.strictEqual(mid.ships[0].auto_pilot, false);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test tests/js/*.test.js --test-name-pattern 'interpolateFrame'`

Expected: FAIL — `Cannot find module '../../kb/battles/holotable-player.js'`.

- [ ] **Step 3: Create the player file with `interpolateFrame`**

Create `kb/battles/holotable-player.js`:

```js
'use strict';

// holotable-player.js — motion for the battle holotable.
//
// The seam against holotable.js is "decides which frame, and when" versus
// "draws a frame". Nothing here draws; nothing in holotable.js knows this file
// exists. Design:
// docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md

// clamp01 keeps an interpolation parameter inside the interval it names. A
// caller that overshoots (a long animation delta, a scrub past the end) should
// see the endpoint, never an extrapolated position off the table.
function clamp01(t) {
  if (!(t > 0)) return 0;
  return t > 1 ? 1 : t;
}

// lerp is the only interpolation in the renderer. Positions are linear on
// purpose: the spec settled that ring radius is not a function of zone, so
// easing between bands would invent motion the battle never had.
function lerp(a, b, t) {
  return a + (b - a) * t;
}

// lerpGuarded interpolates a state value unless either endpoint reads zero.
//
// A 0 hull or shield means "no data", not "empty" — fractionOf in holotable.js
// draws it as the dashed-grey UNKNOWN arc. Interpolating between unknown and a
// real reading would animate a ship's hull draining to nothing, or filling from
// nothing, on evidence that does not exist. Holding prev keeps the arc showing
// exactly one state per interval and snaps at the tick boundary, where the data
// actually changed.
function lerpGuarded(a, b, t) {
  if (!(a > 0) || !(b > 0)) return a;
  return lerp(a, b, t);
}

// interpolateFrame builds the frame to draw at time t between two real ticks.
//
// The result is shaped like a real frame — {tick, ships} — because the existing
// draw path consumes exactly those two fields, plus the new per-ship `alpha`.
// It carries prev's tick deliberately: hullState decides "destroyed" by
// comparing frame.tick against destroyed_at_tick, and a ship must stay
// alive-but-fading across the interval that ends in its death rather than
// flipping to dead at the start of it.
//
// Ships are matched by player_id. A frame is not a fixed roster — Node Beta
// runs 15,15,15,15,14,17,16 ships and then 40 — so presence in one frame and
// not the other is the normal case, not an error, and is what the alpha channel
// exists to absorb.
function interpolateFrame(prev, next, t) {
  const f = clamp01(t);
  if (!next) {
    return {tick: prev.tick, t: 0, ships: prev.ships.map(s => Object.assign({}, s, {alpha: 1}))};
  }

  const nextById = new Map(next.ships.map(s => [s.player_id, s]));
  const ships = [];

  for (const a of prev.ships) {
    const b = nextById.get(a.player_id);
    if (!b) {
      // Present in prev only: leaving or destroyed. Hold its last known
      // position and fade it out; nothing remains after (P4 owns destruction).
      ships.push(Object.assign({}, a, {alpha: 1 - f}));
      continue;
    }
    ships.push(Object.assign({}, a, {
      x: lerp(a.x, b.x, f),
      y: lerp(a.y, b.y, f),
      hull: lerpGuarded(a.hull, b.hull, f),
      shield: lerpGuarded(a.shield, b.shield, f),
      fuel: lerpGuarded(a.fuel, b.fuel, f),
      alpha: 1,
      // zone, stance and target_id are NOT copied from b: Object.assign already
      // took prev's, and they must hold for the whole interval. A target
      // acquired next tick must not draw its line until the ship arrives.
    }));
  }

  const prevIds = new Set(prev.ships.map(s => s.player_id));
  for (const b of next.ships) {
    if (prevIds.has(b.player_id)) continue;
    // Present in next only: arriving. Hold at where it arrives and fade it in.
    ships.push(Object.assign({}, b, {alpha: f}));
  }

  return {tick: prev.tick, t: f, ships};
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {interpolateFrame, lerp, lerpGuarded, clamp01};
}
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `node --test tests/js/*.test.js --test-name-pattern 'interpolateFrame'`

Expected: PASS, 9 tests.

- [ ] **Step 5: Run the whole suite**

Run: `node --test tests/js/*.test.js`

Expected: 0 failures.

- [ ] **Step 6: Commit**

```bash
git add kb/battles/holotable-player.js tests/js/holotable-player.test.js
git commit -m "feat(holotable): interpolateFrame — the pure core of P1b motion

Positions lerp linearly (radius is not a function of zone, so easing would
invent motion). Discrete fields hold prev's value for the interval so a
target acquired next tick does not draw its line early. Hull and shield hold
when either endpoint reads zero, because zero means no data and UNKNOWN must
never fade into a reading. Presence in one frame and not the other drives an
alpha channel — frames are not a fixed roster."
```

---

## Task 3: The `advance` clock

**Files:**
- Modify: `kb/battles/holotable-player.js`
- Test: `tests/js/holotable-player.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces: `advance(state, dtMs, opts)` → `{frameIndex, t, playing, crossed}` where `state` is `{frameIndex, t, playing, speed}`, `opts` is `{frameCount, msPerTick}`, and `crossed` is the ordered array of frame indices newly entered. Also exports `MS_PER_TICK`, `MAX_DELTA_MS`, `SPEEDS`. Task 7 consumes all of it.

`advance` never mutates its input state — it returns a new one. That is what lets a test step it repeatedly with no hidden accumulation, and it is why the rAF loop can be a two-line wrapper.

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/holotable-player.test.js`:

```js
const clock = (over) => Object.assign({frameIndex: 0, t: 0, playing: true, speed: 1}, over || {});
const opts = {frameCount: 10, msPerTick: 500};

test('advance does nothing at all while paused', () => {
  const out = hp.advance(clock({playing: false}), 400, opts);
  assert.strictEqual(out.frameIndex, 0);
  assert.strictEqual(out.t, 0);
  assert.deepStrictEqual(out.crossed, []);
});

test('advance moves t within a tick without crossing a boundary', () => {
  const out = hp.advance(clock(), 200, opts);
  assert.strictEqual(out.frameIndex, 0);
  assert.ok(Math.abs(out.t - 0.4) < 1e-9, `t ${out.t}`);
  assert.deepStrictEqual(out.crossed, []);
});

test('advance crosses one boundary and reports it', () => {
  // 0.8 + 100/500 = exactly 1.0, so this lands precisely on the boundary.
  const out = hp.advance(clock({t: 0.8}), 100, opts);
  assert.strictEqual(out.frameIndex, 1);
  assert.ok(Math.abs(out.t - 0) < 1e-9, `t ${out.t}`);
  assert.deepStrictEqual(out.crossed, [1]);
});

test('advance scales with speed', () => {
  const out = hp.advance(clock({speed: 4}), 200, opts);
  // 200ms at 4x is 800ms of tick time: one whole tick plus 0.6 of the next.
  assert.strictEqual(out.frameIndex, 1);
  assert.ok(Math.abs(out.t - 0.6) < 1e-9, `t ${out.t}`);
  assert.deepStrictEqual(out.crossed, [1]);
});

test('advance reports every boundary in order when one delta spans several ticks', () => {
  // At 4x, the 250ms clamp is 1000ms of tick time = exactly two ticks. Nothing
  // may be skipped: the chatter rail appends from `crossed` and a dropped index
  // is a tick of the battle log that never gets printed.
  const out = hp.advance(clock({speed: 4}), 250, opts);
  assert.deepStrictEqual(out.crossed, [1, 2]);
  assert.strictEqual(out.frameIndex, 2);
});

test('advance clamps an enormous delta, so a backgrounded tab does not leap the battle', () => {
  const out = hp.advance(clock(), 60000, opts);
  // 60s would be 120 ticks unclamped. MAX_DELTA_MS caps it at 250ms = 0.5 tick.
  assert.strictEqual(out.frameIndex, 0);
  assert.ok(Math.abs(out.t - 0.5) < 1e-9, `t ${out.t}`);
  assert.deepStrictEqual(out.crossed, []);
});

test('advance stops and parks on the last frame', () => {
  const out = hp.advance(clock({frameIndex: 9, t: 0.9}), 250, opts);
  assert.strictEqual(out.frameIndex, 9, 'must not run past the end');
  assert.strictEqual(out.t, 0);
  assert.strictEqual(out.playing, false);
});

test('advance does not mutate the state it is given', () => {
  const before = clock({t: 0.8});
  hp.advance(before, 200, opts);
  assert.strictEqual(before.frameIndex, 0, 'advance must be pure');
  assert.strictEqual(before.t, 0.8);
});

test('advance survives a one-frame replay', () => {
  const out = hp.advance(clock(), 250, {frameCount: 1, msPerTick: 500});
  assert.strictEqual(out.frameIndex, 0);
  assert.strictEqual(out.playing, false);
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test tests/js/*.test.js --test-name-pattern 'advance'`

Expected: FAIL — `hp.advance is not a function`.

- [ ] **Step 3: Implement the clock**

In `kb/battles/holotable-player.js`, above the `module.exports` block, add:

```js
// MS_PER_TICK is one game tick's screen time at 1x. Game ticks are ~10s of
// world time; playback is not a simulation and picks a watchable cadence.
const MS_PER_TICK = 500;

// MAX_DELTA_MS caps one animation delta. A backgrounded tab, a blocked main
// thread, or a laptop waking from sleep hands rAF a delta measured in seconds
// or minutes; without this the player leaps a hundred ticks and the chatter
// rail floods with the whole battle at once.
const MAX_DELTA_MS = 250;

const SPEEDS = [0.25, 0.5, 1, 2, 4];

// advance steps the playback clock and reports which tick boundaries it
// crossed. Pure: it returns a new state rather than mutating the one it is
// given, so the rAF loop is a two-line wrapper and tests can step it freely.
//
// `crossed` is the ordered list of frame indices newly entered. It is the
// chatter rail's only input, so ordering and completeness matter: at 4x a
// single clamped delta covers two whole ticks, and skipping one would silently
// drop a tick of the battle log.
function advance(state, dtMs, opts) {
  const frameCount = opts.frameCount;
  const msPerTick = opts.msPerTick || MS_PER_TICK;
  const out = {
    frameIndex: state.frameIndex,
    t: state.t,
    playing: state.playing,
    speed: state.speed,
    crossed: [],
  };
  if (!state.playing) return out;

  const dt = Math.min(dtMs > 0 ? dtMs : 0, MAX_DELTA_MS);
  out.t += (dt * state.speed) / msPerTick;

  while (out.t >= 1 && out.frameIndex < frameCount - 1) {
    out.frameIndex += 1;
    out.t -= 1;
    out.crossed.push(out.frameIndex);
  }

  if (out.frameIndex >= frameCount - 1) {
    out.frameIndex = frameCount - 1;
    out.t = 0;
    out.playing = false;
  }
  return out;
}
```

And extend the exports line:

```js
if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    interpolateFrame, lerp, lerpGuarded, clamp01,
    advance, MS_PER_TICK, MAX_DELTA_MS, SPEEDS,
  };
}
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `node --test tests/js/*.test.js --test-name-pattern 'advance'`

Expected: PASS, 9 tests.

- [ ] **Step 5: Run the whole suite**

Run: `node --test tests/js/*.test.js`

Expected: 0 failures.

- [ ] **Step 6: Commit**

```bash
git add kb/battles/holotable-player.js tests/js/holotable-player.test.js
git commit -m "feat(holotable): the playback clock, pure and testable

advance(state, dtMs, opts) returns a new clock state plus the ordered list
of tick boundaries crossed, so the rAF loop is a wrapper and the chatter
rail has an exact input. Deltas clamp at 250ms: a backgrounded tab would
otherwise hand rAF a delta measured in minutes and leap the whole battle.
Multiple boundaries inside one delta are all reported, in order — at 4x a
single clamped delta covers two ticks."
```

---

## Task 4: `groupChatter`

Shapes one tick's chatter into what the rail prints. Pure — no DOM — so the decision of *what to say* is tested separately from *how it looks*.

**Files:**
- Modify: `kb/battles/holotable-player.js`
- Test: `tests/js/holotable-player.test.js`

**Interfaces:**
- Consumes: nothing.
- Produces: `groupChatter(frame, participantsById)` → `{tick, counts, moves, kills}`, and `nameOf(id, participantsById)` → `string`. Task 7 consumes both.

**Why grouping.** Measured across the two fixtures, Node Beta emits **840 chatter entries over 30 ticks — 28 a tick** — drawn from only 18 distinct reasons, with single ships repeating `npc_hold_range` for ten ticks straight. Printed literally that is a blur at any watchable speed. Grouping identical reasons per tick with a count cuts it to 6.7 lines a tick while keeping the texture; Kitalpha (2.3/tick raw) barely changes.

**Wire shapes, verified against both fixtures — do not assume, these are measured:**

```
frame.chatter[] = {player_id, reason, chosen_target}
frame.moves[]   = {player_id, from, to, reason}
frame.kills[]   = {killer_id, victim_id}
participant     = {player_id, username, side_id, ship_class, ...}
```

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/holotable-player.test.js`:

```js
const parts = new Map([
  ['a', {player_id: 'a', username: 'Ashen Assault', side_id: 1}],
  ['b', {player_id: 'b', username: 'Kestrel', side_id: 2}],
  ['c', {player_id: 'c', username: 'Harrow', side_id: 2}],
]);

test('groupChatter collapses identical reasons into one counted line', () => {
  const frame = {
    tick: 1615393,
    chatter: [
      {player_id: 'a', reason: 'npc_hold_range', chosen_target: 'b'},
      {player_id: 'b', reason: 'npc_hold_range', chosen_target: 'a'},
      {player_id: 'c', reason: 'npc_retreat', chosen_target: null},
    ],
    moves: [], kills: [],
  };
  const g = hp.groupChatter(frame, parts);
  assert.strictEqual(g.tick, 1615393);
  assert.deepStrictEqual(g.counts, [
    {reason: 'npc_hold_range', n: 2},
    {reason: 'npc_retreat', n: 1},
  ]);
});

test('groupChatter orders counts by frequency, then alphabetically, so the rail is deterministic', () => {
  const frame = {
    tick: 5,
    chatter: [
      {player_id: 'a', reason: 'zulu'},
      {player_id: 'b', reason: 'alpha'},
      {player_id: 'c', reason: 'mike'},
      {player_id: 'a', reason: 'mike'},
    ],
    moves: [], kills: [],
  };
  const g = hp.groupChatter(frame, parts);
  assert.deepStrictEqual(g.counts.map(c => c.reason), ['mike', 'alpha', 'zulu'],
    'mike leads on count 2; alpha and zulu tie at 1 and break alphabetically');
});

test('groupChatter names ships by username, not by player hash', () => {
  const frame = {
    tick: 5,
    chatter: [],
    moves: [{player_id: 'b', from: 'outer', to: 'mid', reason: 'advance'}],
    kills: [{killer_id: 'a', victim_id: 'c'}],
  };
  const g = hp.groupChatter(frame, parts);
  assert.deepStrictEqual(g.moves, [{name: 'Kestrel', from: 'outer', to: 'mid', reason: 'advance'}]);
  assert.deepStrictEqual(g.kills, [{victim: 'Harrow', killer: 'Ashen Assault'}]);
});

test('groupChatter falls back to a short id for a participant it has never heard of', () => {
  // Defensive: a chatter or kill entry can name a player_id that is not in the
  // participant roster. Printing "undefined" in the bridge log would be worse
  // than printing a stub.
  const frame = {
    tick: 5, chatter: [],
    moves: [],
    kills: [{killer_id: 'deadbeefcafe0000', victim_id: 'a'}],
  };
  const g = hp.groupChatter(frame, parts);
  assert.strictEqual(g.kills[0].killer, 'deadbe');
  assert.strictEqual(g.kills[0].victim, 'Ashen Assault');
});

test('groupChatter tolerates a frame missing any of its event arrays', () => {
  const g = hp.groupChatter({tick: 7}, parts);
  assert.deepStrictEqual(g, {tick: 7, counts: [], moves: [], kills: []});
});
```

- [ ] **Step 2: Run to verify failure**

Run: `node --test tests/js/*.test.js --test-name-pattern 'groupChatter'`

Expected: FAIL — `hp.groupChatter is not a function`.

- [ ] **Step 3: Implement it**

In `kb/battles/holotable-player.js`, above the `module.exports` block:

```js
// nameOf resolves a player_id for the chatter rail. The roster is the only
// place a readable name exists; an id that is not in it still gets something
// printable, because "undefined destroyed Kestrel" is worse than a stub.
function nameOf(id, participantsById) {
  const p = participantsById.get(id);
  if (p && p.username) return p.username;
  return String(id || '?').slice(0, 6);
}

// groupChatter shapes one tick for the rail.
//
// The spec's original description — a scrolling column of autopilot reasons —
// does not survive the data: Node Beta emits 840 chatter entries across 30
// ticks (28 a tick) drawn from just 18 distinct reasons, with single ships
// repeating npc_hold_range for ten ticks running. Printed literally that is a
// blur. Identical reasons collapse to one counted line, which cuts it to 6.7
// lines a tick and leaves Kitalpha (2.3 raw) essentially untouched.
//
// Moves and kills are never collapsed — they are the events a reader is
// actually watching for, and there are few of them (405 moves and 14 kills
// across all of Node Beta).
function groupChatter(frame, participantsById) {
  const counts = new Map();
  for (const c of frame.chatter || []) {
    counts.set(c.reason, (counts.get(c.reason) || 0) + 1);
  }

  // Sorted by frequency, then by name. The secondary key is not decoration:
  // Map iteration order is insertion order, so without it two ticks with the
  // same reasons in a different arrival order would print in a different order
  // and the rail would shimmer.
  const ordered = Array.from(counts, ([reason, n]) => ({reason, n}))
    .sort((a, b) => (b.n - a.n) || (a.reason < b.reason ? -1 : a.reason > b.reason ? 1 : 0));

  return {
    tick: frame.tick,
    counts: ordered,
    moves: (frame.moves || []).map(m => ({
      name: nameOf(m.player_id, participantsById),
      from: m.from,
      to: m.to,
      reason: m.reason,
    })),
    kills: (frame.kills || []).map(k => ({
      victim: nameOf(k.victim_id, participantsById),
      killer: nameOf(k.killer_id, participantsById),
    })),
  };
}
```

Extend the exports:

```js
    advance, MS_PER_TICK, MAX_DELTA_MS, SPEEDS,
    groupChatter, nameOf,
```

- [ ] **Step 4: Run to verify the tests pass**

Run: `node --test tests/js/*.test.js --test-name-pattern 'groupChatter'`

Expected: PASS, 5 tests.

- [ ] **Step 5: Sanity-check the shaping against the real fixture**

Run:
```bash
cd /home/robert/spacemolt/kb
node -e '
const hp = require("./kb/battles/holotable-player.js");
const r = JSON.parse(require("fs").readFileSync("data/battles/a2619bbe328676445828b4e1007fe9aa.json"));
const by = new Map(r.participants.map(p => [p.player_id, p]));
let raw = 0, lines = 0;
for (const f of r.frames) {
  const g = hp.groupChatter(f, by);
  raw += (f.chatter || []).length;
  lines += g.counts.length + g.moves.length + g.kills.length;
}
console.log("raw", raw, "rail lines", lines, "per tick", (lines / r.frames.length).toFixed(1));
const g = hp.groupChatter(r.frames[7], by);
console.log(JSON.stringify(g).slice(0, 400));
'
```

Expected: `raw 840` and a rail-line count far below it — roughly 20 a tick once moves are included (the 6.7 figure counts grouped reasons alone; Node Beta also averages 13.5 zone moves a tick, and those print individually by design). Record the actual number in your report; Task 8 puts it in the findings doc.

- [ ] **Step 6: Run the whole suite and commit**

Run: `node --test tests/js/*.test.js` — expected 0 failures.

```bash
git add kb/battles/holotable-player.js tests/js/holotable-player.test.js
git commit -m "feat(holotable): groupChatter — collapse the chatter firehose into a bridge log

Node Beta emits 28 chatter entries a tick from 18 distinct reasons, with
ships repeating one reason for ten ticks running. Identical reasons collapse
into a counted line, ordered by frequency then alphabetically so the rail
does not shimmer between ticks with the same content. Moves and kills are
never collapsed and are named by username."
```

---

## Task 5: The alpha channel, and splitting `drawFrame`

Teaches the renderer to draw a partly-faded ship, and separates the layer that never changes from the layer that changes every frame. After this task `drawFrame` still exists with its old signature and old behaviour — it just delegates.

**Files:**
- Modify: `kb/battles/holotable.js` (`drawStationGlyph` ~line 465, `drawShip` ~line 559, `drawFrame` ~line 669, exports)
- Test: none — the draw layer has no unit tests by design (see `docs/holotable-p1a-findings.md`). Verification is that the generated pages are byte-identical.

**Interfaces:**
- Consumes: `layoutTable` from Task 1.
- Produces: `drawStatic(ctx, replay, layout, width, height)`, `drawShips(ctx, replay, hulls, frame, layout)`, `alphaOf(ship)`. Task 7 consumes all three.

- [ ] **Step 1: Give `drawStationGlyph` an alpha parameter**

In `kb/battles/holotable.js`, change the signature and the four absolute `globalAlpha` assignments inside it. The station is the only glyph that sets its own alpha, so it is the only one that has to multiply rather than inherit:

```js
function drawStationGlyph(ctx, px, py, r, colour, alpha) {
  // The station sets its own per-element opacity, so a caller's fade has to be
  // multiplied in rather than inherited from ctx.globalAlpha — an absolute
  // assignment here would make a fading station snap back to full opacity.
  const a = alpha === undefined ? 1 : alpha;
  ctx.save();
  ctx.translate(px, py);
```

Then, inside the same function:

- `ctx.globalAlpha = 0.35;` → `ctx.globalAlpha = 0.35 * a;`
- the first `ctx.globalAlpha = 1;` (just before `ctx.strokeStyle = colour;`) → `ctx.globalAlpha = a;`
- `ctx.globalAlpha = 0.4;` (in the concentric-ring loop) → `ctx.globalAlpha = 0.4 * a;`
- the second `ctx.globalAlpha = 1;` (end of that loop) → `ctx.globalAlpha = a;`

- [ ] **Step 2: Add `alphaOf` and thread alpha through `drawShip`**

Immediately above `function drawShip(...)`, add:

```js
// alphaOf reads a ship's fade. Real frames carry no alpha — only the frames
// interpolateFrame builds do — so an absent value means fully opaque. This
// keeps the P1a path (draw one real frame) working untouched.
function alphaOf(ship) {
  return ship.alpha === undefined ? 1 : ship.alpha;
}
```

Then in `drawShip`, insert at the top of the function body, immediately after `const theme = opts.theme;`:

```js
  const a = opts.alpha === undefined ? 1 : opts.alpha;
  // Fully transparent is not "draw it with no ink" — it is "do not draw". A
  // ship at alpha 0 still costs a Path2D fill, a stroke and two arcs, and at
  // ~400 hulls the frames on either side of every arrival and death are
  // entirely made of them.
  if (!(a > 0)) return;
```

Change the station branch to pass the alpha through:

```js
  if (hull.kind === 'station') {
    drawStationGlyph(ctx, p.px, p.py, lengthPx * 0.6, colour, a);
    return;
  }
```

Wrap the missing-art branch so its chevron and arcs inherit the fade:

```js
  const path = hullPath(hull);
  if (!path) {
    ctx.save();
    ctx.globalAlpha = a;
    drawMissingGlyph(ctx, p.px, p.py, lengthPx * 0.5, theme.missingArt);
    drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
    ctx.restore();
    return;
  }
```

In the hull branch, multiply the two existing assignments and fade the arcs:

- `ctx.globalAlpha = opts.dead ? 0.25 : 0.45;` → `ctx.globalAlpha = a * (opts.dead ? 0.25 : 0.45);`
- `ctx.globalAlpha = 1;` → `ctx.globalAlpha = a;`

and replace the final line of the function:

```js
  ctx.save();
  ctx.globalAlpha = a;
  drawStateArcs(ctx, p.px, p.py, lengthPx * 0.5, opts.state, theme);
  ctx.restore();
```

- [ ] **Step 3: Split `drawFrame` into a static layer and a ship layer**

Replace the whole `drawFrame` function (including both `TODO(P1b)` comment blocks — this task is what they were waiting for) with:

```js
// drawStatic paints everything that does not change from tick to tick: the
// field, the zone rings, the side spokes and their labels.
//
// It is separate from the ships because P1b bakes it once into an offscreen
// canvas and blits it every frame. drawGround measures nothing per-frame, but
// it is a substantial amount of stroking and text at ~400 hulls' worth of
// canvas, and none of it changes until the window resizes.
function drawStatic(ctx, replay, layout, width, height) {
  ctx.save();
  try {
    ctx.fillStyle = THEME.bg;
    ctx.fillRect(0, 0, width, height);
    drawGround(ctx, layout.view, replay.centre, layout.rings, replay.sides, THEME);
  } finally {
    // try/finally, unlike the rest of this file's plain save/restore pairs:
    // P1b reuses one context across every frame, so an unbalanced save from a
    // mid-frame throw survives into the next frame instead of vanishing with
    // a discarded canvas.
    ctx.restore();
  }
}

// drawShips paints one tick's combatants over an already-painted static layer:
// targeting lines first, then hulls on top of them.
function drawShips(ctx, replay, hulls, frame, layout) {
  ctx.save();
  try {
    const view = layout.view;
    const shipsById = new Map(frame.ships.map(s => [s.player_id, s]));
    const partById = new Map(replay.participants.map(p => [p.player_id, p]));

    ctx.save();
    ctx.strokeStyle = THEME.targetLine;
    ctx.lineWidth = 1;
    for (const ship of frame.ships) {
      const target = ship.target_id ? shipsById.get(ship.target_id) : null;
      if (!target) continue;
      // A line is only as present as its faintest end: a line to a hull that
      // is fading out must fade with it, or the targeting layer outlives the
      // ships it describes.
      const lineAlpha = Math.min(alphaOf(ship), alphaOf(target));
      if (!(lineAlpha > 0)) continue;
      ctx.globalAlpha = lineAlpha;
      const a = project(ship.x, ship.y, view);
      const b = project(target.x, target.y, view);
      ctx.beginPath();
      ctx.moveTo(a.px, a.py);
      ctx.lineTo(b.px, b.py);
      ctx.stroke();
    }
    ctx.restore();

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
        alpha: alphaOf(ship),
      });
    }
  } finally {
    ctx.restore();
  }
}

// drawFrame renders one tick of the battle onto ctx, static layer and all.
// P1a's entry point, kept for the single-frame path and for anything that just
// wants a whole picture; the player calls drawStatic and drawShips separately
// so it can bake the first and repeat only the second.
function drawFrame(ctx, replay, hulls, frame, width, height) {
  const layout = layoutTable(replay, width, height);
  drawStatic(ctx, replay, layout, width, height);
  drawShips(ctx, replay, hulls, frame, layout);
}
```

- [ ] **Step 4: Export the new functions**

In the `module.exports` block, extend the last line:

```js
    busiestTick, pickFrame, drawFrame, tableBounds, layoutTable,
    drawStatic, drawShips, alphaOf,
```

- [ ] **Step 5: Run the JS suite**

Run: `node --test tests/js/*.test.js`

Expected: 0 failures, count unchanged from Task 4.

- [ ] **Step 6: Verify the generated pages are untouched**

Run:
```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable --replay data/battles/a2619bbe328676445828b4e1007fe9aa.json
go run ./cmd/generate-battle-holotable --replay data/battles/b131fd5aae68420107dd20e93d15d3ba.json
git status --porcelain kb/battles/
```
Expected: only `kb/battles/holotable.js` shows as modified. The three generated files per battle must be byte-identical — this task changes the renderer, not the generator.

- [ ] **Step 7: Commit**

```bash
git add kb/battles/holotable.js
git commit -m "feat(holotable): alpha channel through the ship layer, and split drawFrame

drawShip gains an alpha that multiplies into every opacity it sets, and
returns early at zero rather than paying for an invisible hull — at ~400
ships the frames around every arrival and death are made of them. The
station has to multiply rather than inherit, since it sets its own
per-element opacity. A targeting line takes the fainter of its two ends.

drawFrame splits into drawStatic (field, rings, spokes, labels — baked once
by the player) and drawShips (one tick), and keeps its old signature by
delegating to both. Both TODO(P1b) comments are now discharged: zoneRings
left the per-frame path in the layoutTable refactor, and the save/restore
pairs here are try/finally because P1b reuses one context across frames."
```

---

## Task 6: Page shell — stage, rail, and transport bar

Adds the markup and layout the player will drive. After this task the page still renders exactly the P1a static frame; the controls are present but inert.

**Files:**
- Modify: `cmd/generate-battle-holotable/page.go`
- Regenerate: `kb/battles/a2619bbe328676445828b4e1007fe9aa.html`, `kb/battles/b131fd5aae68420107dd20e93d15d3ba.html`

**Interfaces:**
- Consumes: nothing.
- Produces: DOM element ids Task 7 binds to — `stage`, `table`, `rail`, `chatter`, `transport`, `stepBack`, `playPause`, `stepFwd`, `scrub`, `speed`, `readout`, `tick`, `status`.

**Preserve two things from P1a.** `#status` keeps `order: -1` and `:not(:empty)` padding, so a fetch failure lands above the header in the flex flow without reserving a blank strip on success — that combination was a three-way composition bug the final P1a review caught. And `#table` must never go back to a `calc(100vh - <header height>)` magic number.

- [ ] **Step 1: Replace the `<style>` block's layout rules**

In `cmd/generate-battle-holotable/page.go`, inside `pageTemplate`, replace the `#table` rule and its comment (everything from `/* #table takes whatever the flex column leaves it` through `#status:not(:empty) { padding: 16px; }`) with:

```css
  /* The flex column is header / stage / transport / status. Nothing carries a
     "100vh minus the header's height" magic number, which drifts every time
     the header's own CSS changes. */
  #stage { flex: 1; display: flex; min-height: 0; }
  #table { display: block; flex: 1; min-width: 0; }
  #rail {
    width: 300px; flex: none; overflow-y: auto;
    border-left: 1px solid #123; padding: 8px 10px;
  }
  #chatter { margin: 0; padding: 0; list-style: none; }
  #chatter li { margin: 0 0 10px; }
  #chatter .tickno { color: #4d7a8c; }
  #chatter .count  { color: #7fb3c8; }
  #chatter .move   { color: #9fd4e8; }
  #chatter .kill   { color: #e08a6a; }
  #transport {
    display: flex; align-items: center; gap: 10px;
    padding: 8px 16px; border-top: 1px solid #123;
  }
  #transport button {
    background: #0b131c; color: #9fd4e8; border: 1px solid #1d3644;
    font: inherit; padding: 3px 10px; cursor: pointer;
  }
  #transport button:hover { background: #12212e; }
  #transport select {
    background: #0b131c; color: #9fd4e8; border: 1px solid #1d3644; font: inherit;
  }
  #scrub { flex: 1; min-width: 80px; }
  /* order: -1 keeps a fetch failure visible above the header, in the flex
     flow rather than fixed/absolute, so it shrinks the stage instead of
     overflowing the viewport — see initHolotable's catch in
     holotable-player.js. :not(:empty) means the div carries no padding (and
     so no height) while it has no text, so a successful load never shows a
     blank strip. */
  #status { color: #c86; order: -1; }
  #status:not(:empty) { padding: 16px; }
  /* Narrow viewports put the rail under the table rather than squeezing both. */
  @media (max-width: 900px) {
    #stage { flex-direction: column; }
    #rail {
      width: auto; height: 180px;
      border-left: none; border-top: 1px solid #123;
    }
  }
```

- [ ] **Step 2: Replace the `<body>` content**

Replace everything between `<body>` and `</body>` in `pageTemplate` with:

```html
<header>
  <h1>{{.SystemName}}</h1>
  <div class="meta">battle {{.BattleID}} &middot; {{.TickCount}} ticks &middot; tick <span id="tick">&mdash;</span></div>
</header>
<div id="stage">
  <canvas id="table"></canvas>
  <aside id="rail"><ol id="chatter"></ol></aside>
</div>
<div id="transport">
  <button id="stepBack" type="button" title="Step back (left arrow)">&#9664;&#9664;</button>
  <button id="playPause" type="button" title="Play / pause (space)">&#9654;</button>
  <button id="stepFwd" type="button" title="Step forward (right arrow)">&#9654;&#9654;</button>
  <input id="scrub" type="range" min="0" max="0" step="1" value="0" aria-label="Scrub through the battle">
  <label>speed <select id="speed"></select></label>
  <span id="readout" class="meta">&mdash;</span>
</div>
<div id="status"></div>
<script>
  window.HOLOTABLE = {
    replayURL: {{.ReplayURL}},
    hullsURL: {{.HullsURL}},
  };
</script>
<script src="holotable.js"></script>
<script src="holotable-player.js"></script>
```

The `speed` select is populated by the player from `SPEEDS`, so the option list lives in exactly one place.

- [ ] **Step 3: Build and lint**

Run:
```bash
cd /home/robert/spacemolt/kb
go build ./... && go test ./cmd/generate-battle-holotable/ && golangci-lint run ./cmd/generate-battle-holotable/
```
Expected: build OK, tests pass, 0 lint issues.

- [ ] **Step 4: Regenerate both pages**

Run:
```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable --replay data/battles/a2619bbe328676445828b4e1007fe9aa.json
go run ./cmd/generate-battle-holotable --replay data/battles/b131fd5aae68420107dd20e93d15d3ba.json
git diff --stat kb/battles/
```
Expected: both `.html` files changed; the `.json` and `-hulls.json` files unchanged.

- [ ] **Step 5: Verify the page still draws**

Run:
```bash
cd /home/robert/spacemolt/kb/kb && python3 -m http.server 8099 &
sleep 2
curl -s http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html | grep -c 'holotable-player.js'
kill %1
```
Expected: `1`. The page is unchanged in behaviour — `holotable.js` still owns `initHolotable` until Task 7, and `holotable-player.js` touches no DOM yet, so the static frame renders exactly as before with an inert transport bar beneath it.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-battle-holotable/page.go kb/battles/a2619bbe328676445828b4e1007fe9aa.html kb/battles/b131fd5aae68420107dd20e93d15d3ba.html
git commit -m "feat(holotable): page shell for playback — stage, chatter rail, transport bar

The flex column becomes header / stage / transport / status, with the stage
a row of canvas plus a 300px rail that stacks below the canvas under 900px.
#status keeps its order:-1 and :not(:empty) padding so a fetch failure still
lands above the header without reserving a blank strip on success, and
nothing regains a 100vh-minus-header magic number.

Controls are inert until the player lands; the speed options are populated
from SPEEDS so the list exists in one place."
```

---

## Task 7: The player

The integration task: the rAF loop, the offscreen static layer, the transport wiring, the keyboard, the rail DOM, and page init — which moves here out of `holotable.js`.

**Files:**
- Modify: `kb/battles/holotable-player.js` (add the DOM half)
- Modify: `kb/battles/holotable.js` (remove `fetchJSON`, `initHolotable`, and the `DOMContentLoaded` listener)

**Interfaces:**
- Consumes: `interpolateFrame`, `advance`, `groupChatter`, `nameOf`, `MS_PER_TICK`, `SPEEDS` (Tasks 2-4); `layoutTable`, `drawStatic`, `drawShips`, `busiestTick`, `pickFrame` (Tasks 1 and 5, reached through the browser global `window.holotable` — see Step 1).
- Produces: nothing later tasks consume.

**The cross-file reference problem.** `holotable.js` exports through `module.exports` for tests, but in the browser there is no module system: both files load as plain scripts into the same global scope. `holotable.js`'s functions are top-level `function` declarations, so they are already globals in the browser and `holotable-player.js` can call them directly. In Node, `holotable-player.js` must `require` them. Handle it with one guarded lookup at the top rather than sprinkling checks.

- [ ] **Step 1: Add the renderer bridge to the top of `holotable-player.js`**

Immediately after the `'use strict';` and the file comment, add:

```js
// The two files load as plain scripts in the browser — no module system — so
// holotable.js's top-level function declarations are already globals here. In
// Node they have to be required. One lookup, resolved once, rather than a
// guard at every call site.
const ht = (typeof require !== 'undefined' && typeof window === 'undefined')
  ? require('./holotable.js')
  : {
      // Only what the player actually calls. holotable.js exports more, but an
      // unused name here would be a browser ReferenceError waiting for someone
      // to rename it in the other file.
      layoutTable, drawStatic, drawShips, busiestTick,
    };
```

Note the object-literal branch only evaluates in a browser, where those names are defined by the earlier `<script>`. Guarding on `typeof window === 'undefined'` rather than on `require` alone matters: a bundler or a browser test runner can define `require`.

- [ ] **Step 2: Add the rail rendering**

Append to `holotable-player.js`, above the exports block:

```js
// RAIL_WINDOW_TICKS bounds the rail's DOM. A 620-tick battle would otherwise
// build thousands of elements, and a scrub would rebuild all of them; keeping
// the last 40 makes both the append path and the seek path cost the same
// whether the battle is 30 ticks or 620.
const RAIL_WINDOW_TICKS = 40;

// railBlock builds one tick's entry. Grouped reasons first (the texture), then
// the events a reader is actually watching for.
function railBlock(doc, g) {
  const li = doc.createElement('li');

  const head = doc.createElement('div');
  head.className = 'tickno';
  head.textContent = 'TICK ' + g.tick;
  li.appendChild(head);

  for (const c of g.counts) {
    const row = doc.createElement('div');
    row.className = 'count';
    row.textContent = '  ×' + c.n + '  ' + c.reason;
    li.appendChild(row);
  }
  for (const m of g.moves) {
    const row = doc.createElement('div');
    row.className = 'move';
    row.textContent = '  →  ' + m.name + '  ' + m.from + ' → ' + m.to + '  (' + m.reason + ')';
    li.appendChild(row);
  }
  for (const k of g.kills) {
    const row = doc.createElement('div');
    row.className = 'kill';
    row.textContent = '  †  ' + k.victim + ' destroyed by ' + k.killer;
    li.appendChild(row);
  }
  return li;
}
```

Every field goes in through `textContent`, never `innerHTML`: `reason` and `username` are server-supplied strings.

- [ ] **Step 3: Add the player**

Append to `holotable-player.js`, above the exports block:

```js
// createPlayer wires one battle to one canvas. Everything stateful in P1b lives
// in this closure: the clock, the cached layout, and the baked static layer.
function createPlayer(cfg) {
  const doc = cfg.doc;
  const replay = cfg.replay;
  const hulls = cfg.hulls;
  const els = cfg.els;
  const frames = replay.frames;
  const participantsById = new Map(replay.participants.map(p => [p.player_id, p]));

  const state = {frameIndex: cfg.startIndex || 0, t: 0, playing: false, speed: 1};

  let ctx = null;
  let layout = null;
  let width = 0;
  let height = 0;
  // The static layer — field, rings, spokes, labels — is identical on every
  // frame of a battle and is a substantial amount of stroking and text. Baking
  // it once into an offscreen canvas turns it into a single drawImage per
  // frame, and rebuilding it is tied to resize, the only thing that changes it.
  let staticCanvas = doc.createElement('canvas');
  let lastNow = 0;
  let raf = 0;

  function resize() {
    const dpr = (cfg.win && cfg.win.devicePixelRatio) || 1;
    width = els.canvas.clientWidth;
    height = els.canvas.clientHeight;
    if (!(width > 0) || !(height > 0)) return;

    els.canvas.width = width * dpr;
    els.canvas.height = height * dpr;
    ctx = els.canvas.getContext('2d');
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    layout = ht.layoutTable(replay, width, height);

    staticCanvas.width = width * dpr;
    staticCanvas.height = height * dpr;
    const sctx = staticCanvas.getContext('2d');
    sctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ht.drawStatic(sctx, replay, layout, width, height);

    render();
  }

  function render() {
    if (!ctx || !layout) return;
    const frame = interpolateFrame(frames[state.frameIndex], frames[state.frameIndex + 1] || null, state.t);
    ctx.drawImage(staticCanvas, 0, 0, width, height);
    ht.drawShips(ctx, replay, hulls, frame, layout);
  }

  function syncControls() {
    els.scrub.value = String(state.frameIndex);
    els.playPause.textContent = state.playing ? '⏸' : '▶';
    els.tick.textContent = String(frames[state.frameIndex].tick);
    els.readout.textContent = (state.frameIndex + 1) + ' / ' + frames.length;
  }

  function appendRail(frame) {
    // "Stuck to the bottom" is measured before the append, not after: appending
    // changes scrollHeight, so checking afterwards always reads as scrolled up.
    const stuck = els.rail.scrollTop + els.rail.clientHeight >= els.rail.scrollHeight - 4;
    els.chatter.appendChild(railBlock(doc, groupChatter(frame, participantsById)));
    while (els.chatter.childElementCount > RAIL_WINDOW_TICKS) {
      els.chatter.removeChild(els.chatter.firstChild);
    }
    if (stuck) els.rail.scrollTop = els.rail.scrollHeight;
  }

  function rebuildRail() {
    els.chatter.textContent = '';
    const from = Math.max(0, state.frameIndex - RAIL_WINDOW_TICKS + 1);
    for (let i = from; i <= state.frameIndex; i++) {
      els.chatter.appendChild(railBlock(doc, groupChatter(frames[i], participantsById)));
    }
    els.rail.scrollTop = els.rail.scrollHeight;
  }

  function step(delta) {
    setPlaying(false);
    seek(state.frameIndex + delta);
  }

  function seek(index) {
    const clamped = Math.max(0, Math.min(frames.length - 1, index));
    if (clamped === state.frameIndex && state.t === 0) return;
    state.frameIndex = clamped;
    state.t = 0;
    rebuildRail();
    syncControls();
    render();
  }

  function setPlaying(on) {
    if (on && state.frameIndex >= frames.length - 1) {
      // Pressing play on the parked last frame restarts, rather than doing
      // nothing — the alternative is a button that silently ignores a click.
      state.frameIndex = 0;
      state.t = 0;
      rebuildRail();
    }
    state.playing = !!on;
    syncControls();
    if (state.playing) {
      lastNow = 0;
      raf = cfg.win.requestAnimationFrame(loop);
    } else if (raf) {
      cfg.win.cancelAnimationFrame(raf);
      raf = 0;
    }
  }

  function loop(now) {
    if (!state.playing) return;
    const dt = lastNow ? now - lastNow : 0;
    lastNow = now;

    const out = advance(state, dt, {frameCount: frames.length, msPerTick: MS_PER_TICK});
    for (const i of out.crossed) appendRail(frames[i]);
    state.frameIndex = out.frameIndex;
    state.t = out.t;
    state.playing = out.playing;

    syncControls();
    render();
    if (state.playing) raf = cfg.win.requestAnimationFrame(loop);
  }

  function onKey(e) {
    if (e.target && (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT')) return;
    const jump = e.shiftKey ? 10 : 1;
    if (e.key === ' ') { e.preventDefault(); setPlaying(!state.playing); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); step(-jump); }
    else if (e.key === 'ArrowRight') { e.preventDefault(); step(jump); }
    else if (e.key === 'Home') { e.preventDefault(); step(-frames.length); }
    else if (e.key === 'End') { e.preventDefault(); step(frames.length); }
  }

  function start() {
    els.scrub.max = String(frames.length - 1);
    for (const s of SPEEDS) {
      const opt = doc.createElement('option');
      opt.value = String(s);
      opt.textContent = s + '×';
      if (s === 1) opt.selected = true;
      els.speed.appendChild(opt);
    }

    els.playPause.addEventListener('click', () => setPlaying(!state.playing));
    els.stepBack.addEventListener('click', () => step(-1));
    els.stepFwd.addEventListener('click', () => step(1));
    els.scrub.addEventListener('input', () => { setPlaying(false); seek(Number(els.scrub.value)); });
    els.speed.addEventListener('change', () => { state.speed = Number(els.speed.value); });
    cfg.win.addEventListener('keydown', onKey);
    cfg.win.addEventListener('resize', resize);

    resize();
    rebuildRail();
    syncControls();
  }

  return {start, setPlaying, seek, render, resize, state};
}
```

- [ ] **Step 4: Move page init out of `holotable.js`**

Delete from `kb/battles/holotable.js`: the `fetchJSON` function, the `initHolotable` function, and the `if (typeof document !== 'undefined') { document.addEventListener('DOMContentLoaded', initHolotable); }` block. The `module.exports` block needs no change — neither function was ever exported.

Then append to `holotable-player.js`, above the exports block:

```js
// fetchJSON fetches and parses one data file, naming the URL and HTTP status on
// failure — without this, a missing file surfaces to initHolotable's catch as
// "Unexpected end of JSON input" from the JSON parser, not as the 404 that
// actually caused it.
async function fetchJSON(url) {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.json();
}

// startIndex resolves where playback begins. P1b starts at the beginning,
// because a player should; ?tick=N still seeks, and ?tick=busiest keeps P1a's
// busiest-frame default reachable so the screenshot commands in
// docs/holotable-p1a-findings.md stay reproducible.
function startIndex(replay, params) {
  const raw = params.get('tick');
  if (raw === 'busiest') {
    const i = replay.frames.indexOf(ht.busiestTick(replay));
    return i < 0 ? 0 : i;
  }
  if (raw !== null && raw !== '') {
    const i = replay.frames.findIndex(f => f.tick === Number(raw));
    if (i >= 0) return i;
  }
  return 0;
}

// initHolotable wires the page: fetch both data files, build the player, and
// draw the opening frame paused.
async function initHolotable() {
  const cfg = window.HOLOTABLE;
  const status = document.getElementById('status');

  try {
    const [replay, hulls] = await Promise.all([
      fetchJSON(cfg.replayURL),
      fetchJSON(cfg.hullsURL),
    ]);

    const els = {
      canvas: document.getElementById('table'),
      rail: document.getElementById('rail'),
      chatter: document.getElementById('chatter'),
      playPause: document.getElementById('playPause'),
      stepBack: document.getElementById('stepBack'),
      stepFwd: document.getElementById('stepFwd'),
      scrub: document.getElementById('scrub'),
      speed: document.getElementById('speed'),
      readout: document.getElementById('readout'),
      tick: document.getElementById('tick'),
    };

    const params = new URLSearchParams(window.location.search);
    const player = createPlayer({
      doc: document, win: window, replay, hulls, els,
      startIndex: startIndex(replay, params),
    });
    player.start();

    if (params.get('play') === '1') player.setPlaying(true);
    status.textContent = '';
  } catch (err) {
    status.textContent = 'Could not draw the battle: ' + err.message;
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initHolotable);
}
```

Extend the exports one last time:

```js
    groupChatter, nameOf, railBlock, createPlayer, startIndex, RAIL_WINDOW_TICKS,
```

- [ ] **Step 5: Run the JS suite**

Run: `node --test tests/js/*.test.js`

Expected: 0 failures. Requiring `holotable-player.js` in Node must not throw — that is what the `typeof window === 'undefined'` guard in Step 1 buys, and it is the single most likely thing to break in this task.

- [ ] **Step 6: Verify it plays**

Run:
```bash
cd /home/robert/spacemolt/kb/kb && python3 -m http.server 8099
```
Then open `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html` and confirm, by eye:

1. The page opens **paused on frame 1** with the transport bar populated and the rail showing tick 1615386.
2. Play runs the battle smoothly; hulls slide rather than jump between ticks.
3. The rail scrolls, groups reasons with `×N`, and prints moves and kills on their own lines.
4. The 24-ship arrival at frame 8 fades in rather than popping.
5. Scrubbing seeks, rebuilds the rail, and does not leave the canvas blank.
6. `?tick=busiest` still opens on the P1a screenshot frame.
7. Resizing the window re-fits the table without clipping the outer ring.

Report anything that fails this list rather than fixing it silently — items 2, 4 and 7 are the ones that would invalidate the design rather than the code.

- [ ] **Step 7: Commit**

```bash
git add kb/battles/holotable.js kb/battles/holotable-player.js
git commit -m "feat(holotable): playback — transport, rAF loop, and the chatter rail

createPlayer owns everything stateful: the clock, the cached layout, and the
static layer baked once into an offscreen canvas so a frame costs one
drawImage plus the ships. Page init moves out of holotable.js, which is now
purely geometry and drawing.

The rail keeps a 40-tick window so append and seek cost the same on a 30-tick
battle and a 620-tick one, and measures stuck-to-bottom before appending,
since appending is what changes scrollHeight. Playback starts paused at frame
0; ?tick=N seeks and ?tick=busiest keeps P1a's frame reachable."
```

---

## Task 8: Stress fixture, bench, and findings

Proves playback holds at a scale neither fixture reaches — Node Beta is 42 ships over 30 ticks, Kitalpha is 5 over 158, and real battles reach **373 participants over 264 ticks**.

**Files:**
- Create: `scripts/make-stress-replay.js`
- Modify: `kb/battles/holotable-player.js` (add `bench` to the player)
- Modify: `.gitignore`
- Create: `docs/holotable-p1b-findings.md`

**Interfaces:**
- Consumes: `createPlayer` from Task 7.
- Produces: a `bench(n)` method on the player object.

**Deliberately no timing assertion in `node --test`.** A wall-clock gate in a unit test is flaky by construction — it fails on a loaded CI box and passes on a fast laptop, teaching everyone to re-run it. The number goes in the findings doc, where a human reads it.

- [ ] **Step 1: Write the stress generator**

Create `scripts/make-stress-replay.js`:

```js
'use strict';

// make-stress-replay.js — build a synthetic replay big enough to stress the
// holotable renderer: ~400 hulls over 600 ticks, against real ship classes so
// the hull pack resolves real footprint art.
//
// Neither shipped fixture reaches this. Node Beta is 42 ships over 30 ticks and
// Kitalpha is 5 over 158; real battles reach 373 participants over 264 ticks
// (24MB), which is too expensive to export just to time a render loop.
//
//   node scripts/make-stress-replay.js
//
// Writes data/battles/ffffffffffffffffffffffffffffffff.json — a valid 32-hex
// battle id, which the generator requires, and an obviously fake one.

const fs = require('fs');

const SOURCE = 'data/battles/a2619bbe328676445828b4e1007fe9aa.json';
const OUT = 'data/battles/ffffffffffffffffffffffffffffffff.json';
const BATTLE_ID = 'ffffffffffffffffffffffffffffffff';
const FANOUT = 10;      // 42 source participants x 10 = 420 hulls
const TICKS = 600;
const ZONES = ['outer', 'mid', 'inner', 'engaged'];
const ZONE_RADIUS = {outer: 1.05, mid: 0.8, inner: 0.65, engaged: 0.55};
const SIDES = 4;

const src = JSON.parse(fs.readFileSync(SOURCE, 'utf8'));

const participants = [];
for (let copy = 0; copy < FANOUT; copy++) {
  for (let i = 0; i < src.participants.length; i++) {
    const p = src.participants[i];
    participants.push({
      player_id: `p${copy}_${i}`,
      username: `${p.username} ${copy}`,
      kind: p.kind,
      side_id: (participants.length % SIDES) + 1,
      ship_class: p.ship_class,
      max_hull: p.max_hull || 200,
      max_shield: p.max_shield || 100,
      max_fuel: p.max_fuel || 150,
      modules: [],
      first_tick: 1,
      last_tick: TICKS,
      destroyed_at_tick: 0,
      killed_by: '',
    });
  }
}

// Bearings are spread evenly per side and jittered per ship, so hulls do not
// stack into one line and the draw cost is realistic.
const bearing = (i) => ((participants[i].side_id - 1) * (360 / SIDES) + (i % 17) * 1.7) * Math.PI / 180;

const frames = [];
for (let t = 0; t < TICKS; t++) {
  const ships = [];
  for (let i = 0; i < participants.length; i++) {
    // Each ship walks inward and back out over a 40-tick cycle, so zone
    // membership churns and ring measurement sees every band every frame.
    const phase = ((t + i) % 40) / 40;
    const zone = ZONES[Math.floor(phase * ZONES.length)];
    const r = ZONE_RADIUS[zone] + ((i % 11) - 5) * 0.01;
    const b = bearing(i);
    ships.push({
      player_id: participants[i].player_id,
      x: Math.cos(b) * r,
      y: Math.sin(b) * r,
      zone,
      hull: participants[i].max_hull - (t % 100),
      shield: Math.max(0, participants[i].max_shield - (t % 50)),
      fuel: participants[i].max_fuel,
      stance: (i % 3 === 0) ? 'brace' : 'fire',
      // Every third ship targets someone, which is what makes the targeting
      // layer cost anything.
      target_id: (i % 3 === 0) ? participants[(i + 7) % participants.length].player_id : null,
      auto_pilot: true,
    });
  }
  frames.push({
    tick: 1000000 + t,
    ships,
    shots: [],
    moves: [{player_id: participants[t % participants.length].player_id, from: 'outer', to: 'mid', reason: 'advance'}],
    kills: [],
    repairs: [],
    chatter: ships.slice(0, 40).map((s, i) => ({
      player_id: s.player_id,
      reason: ['npc_hold_range', 'npc_close_firing', 'npc_retreat', 'hold'][i % 4],
      chosen_target: s.target_id,
    })),
  });
}

const replay = {
  schema: src.schema,
  battle_id: BATTLE_ID,
  system_id: 'stress',
  system_name: 'STRESS TEST',
  status: 'complete',
  start_tick: frames[0].tick,
  end_tick: frames[frames.length - 1].tick,
  tick_count: frames.length,
  total_ticks: frames.length,
  has_station: false,
  outcome: 'synthetic',
  winning_side: 0,
  total_damage: 0,
  bounds: {x_min: -1.2, x_max: 1.2, y_min: -1.2, y_max: 1.2},
  centre: {x: 0, y: 0},
  zones: ZONES,
  sides: Array.from({length: SIDES}, (_, i) => ({
    side_id: i + 1,
    count: participants.filter(p => p.side_id === i + 1).length,
    bearing_mean: i * (360 / SIDES),
    radius_mean: 0.8,
  })),
  participants,
  frames,
};

fs.writeFileSync(OUT, JSON.stringify(replay));
console.log(`wrote ${OUT}: ${participants.length} participants, ${frames.length} ticks, ${(fs.statSync(OUT).size / 1e6).toFixed(1)}MB`);
```

- [ ] **Step 2: Generate the fixture and its page**

Run:
```bash
cd /home/robert/spacemolt/kb
node scripts/make-stress-replay.js
go run ./cmd/generate-battle-holotable --replay data/battles/ffffffffffffffffffffffffffffffff.json
```
Expected: `420 participants, 600 ticks`, then the generator reports the hull classes it resolved. Record both output lines in your report.

- [ ] **Step 3: Keep the fixture out of git**

**Do not touch the repo-root `.gitignore`** — it already carries an unrelated
uncommitted change (`git diff .gitignore` shows five added lines), and
committing it would sweep up someone else's work. Use per-directory ignore
files instead, which is also where the explanation belongs.

Create `data/battles/.gitignore`:

```gitignore
# Synthetic holotable stress fixture — 420 hulls x 600 ticks, tens of MB.
# Regenerate with `node scripts/make-stress-replay.js`.
ffffffffffffffffffffffffffffffff.json
```

Create `kb/battles/.gitignore`:

```gitignore
# Generated page for the synthetic stress fixture. Regenerate with
# `go run ./cmd/generate-battle-holotable --replay data/battles/ffffffffffffffffffffffffffffffff.json`.
ffffffffffffffffffffffffffffffff.*
```

Verify: `git status --porcelain data/battles/ kb/battles/` lists the two new
`.gitignore` files and neither fixture file.

- [ ] **Step 4: Add `bench` to the player**

In `kb/battles/holotable-player.js`, inside `createPlayer`, add above the `return` statement:

```js
  // bench times the per-frame cost with the clock taken out of the loop: n
  // renders spread across the battle and across the interval between ticks, so
  // interpolation, the static blit and the ship layer are all exercised.
  // Deliberately not a test assertion — a wall-clock gate in `node --test`
  // fails on a loaded box and passes on a fast laptop, which teaches everyone
  // to re-run it rather than to read it.
  function bench(n) {
    const t0 = cfg.win.performance.now();
    for (let i = 0; i < n; i++) {
      state.frameIndex = i % Math.max(1, frames.length - 1);
      state.t = (i % 10) / 10;
      render();
    }
    const total = cfg.win.performance.now() - t0;
    return {frames: n, totalMs: total, msPerFrame: total / n};
  }
```

and add it to the returned object:

```js
  return {start, setPlaying, seek, render, resize, bench, state};
```

Then in `initHolotable`, immediately after `player.start();`, add:

```js
    const benchN = Number(params.get('bench') || 0);
    if (benchN > 0) {
      const r = player.bench(benchN);
      const line = `bench: ${r.frames} frames, ${r.msPerFrame.toFixed(2)} ms/frame ` +
        `(${(1000 / r.msPerFrame).toFixed(0)} fps ceiling), ${replay.participants.length} participants`;
      console.log(line);
      status.textContent = line;
      return;
    }
```

The early `return` skips the `status.textContent = ''` below it, which is what leaves the measurement on screen.

- [ ] **Step 5: Measure**

Run:
```bash
cd /home/robert/spacemolt/kb/kb && python3 -m http.server 8099
```
Open each and read the line at the top of the page:

- `http://localhost:8099/battles/ffffffffffffffffffffffffffffffff.html?bench=300` — 420 participants
- `http://localhost:8099/battles/a2619bbe328676445828b4e1007fe9aa.html?bench=300` — 42 participants

Record both `ms/frame` figures. For reference, 60 fps is a 16.7 ms budget and 30 fps is 33.3 ms; playback at `MS_PER_TICK = 500` only needs a new interpolated position twice a second, so anything under 33 ms is comfortable and anything over 100 ms needs reporting rather than silent acceptance.

- [ ] **Step 6: Check playback by eye at scale**

Open `http://localhost:8099/battles/ffffffffffffffffffffffffffffffff.html` (no `bench`) and play it at 1× and 4×. Confirm the rail keeps up, the transport stays responsive, and scrubbing to tick 500 is immediate. Report anything that stutters.

- [ ] **Step 7: Write the findings doc**

Create `docs/holotable-p1b-findings.md`. Follow the shape of `docs/holotable-p1a-findings.md`: a Screenshots section, a "How to see this yourself" section with the exact serve command and URLs, then the findings. It must record, with real numbers rather than adjectives:

- ms/frame at 420 participants and at 42, and what that extrapolates to.
- The real rail-line-per-tick count on Node Beta, from Task 4 Step 5.
- Whether interpolation reads as motion or as sliding — the one judgement P1b exists to make.
- Whether the 24-hull arrival at Node Beta frame 8 reads as arrival with the fade.
- Anything on the Task 7 Step 6 list that failed.

Add screenshots to `docs/img/holotable-p1b/` and reference them.

- [ ] **Step 8: Run every gate and commit**

Run:
```bash
cd /home/robert/spacemolt/kb
go build ./... && go test ./... && golangci-lint run && node --test tests/js/*.test.js
```
Expected: all clean, 0 JS failures.

```bash
git add scripts/make-stress-replay.js kb/battles/holotable-player.js data/battles/.gitignore kb/battles/.gitignore docs/holotable-p1b-findings.md docs/img/holotable-p1b/
git commit -m "test(holotable): synthetic stress fixture, in-page bench, and P1b findings

420 hulls over 600 ticks against real ship classes — neither shipped fixture
reaches that, and exporting a real 373-participant battle costs a live run.
?bench=N times the render path with the clock taken out of the loop; the
number lives in the findings doc rather than in a test assertion, because a
wall-clock gate in node --test fails on a loaded box and teaches everyone to
re-run it."
```

---

## Task 9: Acceptance against a real large battle

The synthetic fixture proves the renderer; this proves the whole path on data the game actually produced. Runs last because it is the only step that touches a live service.

**Files:**
- Modify: `.gitignore`, `docs/holotable-p1b-findings.md`

**This task runs a live export.** Read every caution before starting — they are measured, not theoretical:

- **Pick the export agent from `ps aux`, never from a list.** A login collides with that agent's running worker and dies `session_replaced`. Two such deaths inside 30 seconds trip the client's session-contention guard and abort the run. As of 2026-08-19, `explorer-7` and `databot` were **not** idle and `craftsman-boss` was — but re-check, do not assume.
- Battle logs are readable by **any** logged-in agent. You do not need a participant.
- The WebSocket read limit is **10 MB** (`SetReadLimit`, `pkg/game/client.go`). At ~370 participants a tick is ~90 KB, so `--limit 10`. Each oversized frame costs a reconnect; the exporter halves on failure with a 35 s backoff.
- If the export fails twice, **stop and report**. Do not retry a third time — that is what trips the guard.

- [ ] **Step 1: Find an idle agent**

Run:
```bash
ps aux | grep -E 'bin/worker|play_as' | grep -v grep | awk '{for(i=11;i<=NF;i++) printf "%s ", $i; print ""}'
```
Cross-reference against `ls /home/robert/spacemolt/spacemolt/data/agents/` and pick one that does **not** appear in the running list. Name your choice in your report.

- [ ] **Step 2: Export the battle**

Run:
```bash
cd /home/robert/spacemolt/spacemolt
bin/battle-export --agent <idle-agent> --battle c79f7810a59437b029a6168526782fe4 --limit 10 \
  --out /home/robert/spacemolt/kb/data/battles/c79f7810a59437b029a6168526782fe4.json
```

If `bin/battle-export` is missing, build it first: `go build -o bin/battle-export ./cmd/tools/battle-export` — binaries go in `bin/`, never the repo root.

Expected: a file of roughly 24 MB. If the export dies with `session_replaced`, the agent was not idle — pick another and try **once** more.

- [ ] **Step 3: Keep it out of git and generate its page**

Append to the `data/battles/.gitignore` and `kb/battles/.gitignore` created in
Task 8 — again, never the repo-root `.gitignore`:

`data/battles/.gitignore`:
```gitignore
# Real large-battle acceptance fixture — 24MB, regenerate with bin/battle-export.
c79f7810a59437b029a6168526782fe4.json
```

`kb/battles/.gitignore`:
```gitignore
c79f7810a59437b029a6168526782fe4.*
```

Then:
```bash
cd /home/robert/spacemolt/kb
go run ./cmd/generate-battle-holotable --replay data/battles/c79f7810a59437b029a6168526782fe4.json
```
Record the generator's summary line — participants, ship classes, how many without art.

- [ ] **Step 4: Measure and watch it**

Serve as before and open both:
- `.../c79f7810a59437b029a6168526782fe4.html?bench=300` — record ms/frame beside the synthetic figure.
- `.../c79f7810a59437b029a6168526782fe4.html` — play it. Note the initial load time for a 24 MB fetch separately from the frame cost; they are different problems with different fixes.

- [ ] **Step 5: Record the result and commit**

Add a "Real large battle" section to `docs/holotable-p1b-findings.md` covering: the export command that worked and which agent, the load time, the ms/frame, how it compares to the synthetic figure, and any shape the real data has that the synthetic fixture does not.

```bash
git add data/battles/.gitignore kb/battles/.gitignore docs/holotable-p1b-findings.md
git commit -m "docs(holotable): P1b acceptance against a real 373-participant battle

Records the export, the load time for 24MB, and the measured frame cost
beside the synthetic figure — the synthetic fixture stresses the renderer
but cannot produce a data shape only a real battle has."
```

---

## Definition of done

- `go build ./...`, `go test ./...`, `golangci-lint run` clean.
- `node --test tests/js/*.test.js` — 0 failures, count at or above the 113-test baseline plus the new tests.
- Both shipped battle pages play, scrub, step, and change speed; the rail groups and scrolls; the outer ring never clips at any window size.
- `?tick=busiest` still reaches P1a's screenshot frame.
- Both `TODO(P1b)` comments are gone from `holotable.js`, discharged rather than deleted.
- `docs/holotable-p1b-findings.md` records real numbers for frame cost at 42, 420, and 373 participants.
