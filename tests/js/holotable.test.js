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

test('zoneRings orders bands by canonical zone order, and clears the flag when measurement agrees', () => {
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
  assert.strictEqual(rings.agreesWithMeasurement, true,
    'measured means already increase in canonical order, so nothing disagrees');
  assert.ok(Math.abs(rings[0].meanRadius - 0.5) < 1e-9);
  // Boundaries sit at the midpoints between adjacent means.
  assert.ok(Math.abs(rings[0].rOuter - 0.6) < 1e-9, `rOuter ${rings[0].rOuter}`);
  assert.strictEqual(rings[0].rInner, 0, 'the innermost band starts at the centre');
  assert.ok(rings[3].rOuter > rings[3].meanRadius, 'the outer ring must enclose its ships');
});

test('zoneRings keeps canonical order even when measured radius disagrees entirely', () => {
  const centre = {x: 0, y: 0};
  // Radii are scrambled relative to canonical order: sorting by measurement
  // alone would produce outer, inner, mid, engaged. zone is authoritative
  // over x/y (design spec Finding 4), so the ring order must not move.
  const frames = [{
    tick: 1,
    ships: [
      {player_id: 'a', x: 1.5, y: 0, zone: 'engaged'},
      {player_id: 'b', x: 0.5, y: 0, zone: 'inner'},
      {player_id: 'c', x: 0.9, y: 0, zone: 'mid'},
      {player_id: 'd', x: 0.2, y: 0, zone: 'outer'},
    ],
  }];
  const rings = ht.zoneRings(frames, centre, {});
  assert.deepStrictEqual(rings.map(r => r.zone), ['engaged', 'inner', 'mid', 'outer'],
    'ring order must stay canonical regardless of what x/y measures');
  assert.strictEqual(rings.agreesWithMeasurement, false,
    'a fully scrambled measurement must be flagged as a disagreement');
  assert.strictEqual(rings[0].rInner, 0);
  for (let i = 1; i < rings.length; i++) {
    assert.ok(rings[i].rOuter > rings[i - 1].rOuter,
      `ring ${rings[i].zone}.rOuter (${rings[i].rOuter}) must exceed ${rings[i - 1].zone}.rOuter (${rings[i - 1].rOuter})`);
    assert.ok(Number.isFinite(rings[i].rOuter) && Number.isFinite(rings[i].rInner));
  }
});

test('zoneRings enforces monotonic boundaries when a single band is internally inverted', () => {
  const centre = {x: 0, y: 0};
  // Mirrors a real recorded four-side battle (Kitalpha): "mid" measured a
  // SMALLER mean radius than "engaged", even though canonical order says mid
  // sits farther out. mid 0.268 < engaged 0.344 < inner 0.456 < outer 0.798.
  const frames = [{
    tick: 1,
    ships: [
      {player_id: 'e', x: 0.344, y: 0, zone: 'engaged'},
      {player_id: 'i', x: 0.456, y: 0, zone: 'inner'},
      {player_id: 'm', x: 0.268, y: 0, zone: 'mid'},
      {player_id: 'o', x: 0.798, y: 0, zone: 'outer'},
    ],
  }];
  const rings = ht.zoneRings(frames, centre, {});
  assert.deepStrictEqual(rings.map(r => r.zone), ['engaged', 'inner', 'mid', 'outer'],
    'ring order must stay canonical even when one band is internally inverted');
  assert.strictEqual(rings.agreesWithMeasurement, false,
    'mid measuring smaller than engaged must be flagged as a disagreement');
  assert.strictEqual(rings[0].rInner, 0);
  for (let i = 1; i < rings.length; i++) {
    assert.ok(rings[i].rOuter > rings[i - 1].rOuter,
      `ring ${rings[i].zone}.rOuter (${rings[i].rOuter}) must exceed ${rings[i - 1].zone}.rOuter (${rings[i - 1].rOuter})`);
    assert.ok(Number.isFinite(rings[i].rOuter) && Number.isFinite(rings[i].rInner));
  }
  // meanRadius itself is left untouched for diagnostics, even though the
  // boundary derived from it is not.
  assert.ok(Math.abs(rings[2].meanRadius - 0.268) < 1e-9,
    'the raw measured mean must not be rewritten, only the boundary');
});

test('zoneRings clamps a single ring whose ships sit exactly on the centre', () => {
  // centre is the position bounds' midpoint, so a one-ship frame (or every
  // ship stacked at one point) lands exactly there: meanRadius is 0.0, not
  // merely close to it, and the only ring must still get a real width.
  const centre = {x: 0, y: 0};
  const frames = [{
    tick: 1,
    ships: [{player_id: 'a', x: 0, y: 0, zone: 'engaged'}],
  }];
  const rings = ht.zoneRings(frames, centre, {});
  assert.strictEqual(rings.length, 1);
  assert.strictEqual(rings[0].meanRadius, 0);
  assert.ok(Number.isFinite(rings[0].rInner) && Number.isFinite(rings[0].rOuter));
  assert.ok(rings[0].rOuter > rings[0].rInner,
    `rOuter (${rings[0].rOuter}) must exceed rInner (${rings[0].rInner}); a zero-width ring cannot be drawn`);
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
