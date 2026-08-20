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
