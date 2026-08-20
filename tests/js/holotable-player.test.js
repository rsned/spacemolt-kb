'use strict';
const test = require('node:test');
const assert = require('node:assert');
const hp = require('../../kb/battles/holotable-player.js');
const {makeHarness} = require('./fake-dom.js');

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

  // t=0.5 is the fixed point of any symmetric easing curve, so the midpoint
  // alone cannot tell linear interpolation from ease-in-out. A quarter point
  // can: eased curves are below the line here, linear is exactly on it.
  const quarter = hp.interpolateFrame(prev, next, 0.25);
  assert.strictEqual(quarter.ships[0].x, 0.5);
  assert.strictEqual(quarter.ships[0].y, 1);
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

// startIndex — no fake DOM needed, but untested until now. busiestTick
// returns a TICK NUMBER, not a frame index; a caller that used it as an
// array offset directly (rather than looking up the frame whose .tick
// matches) would silently resolve to the wrong frame, or an out-of-range
// one, on any replay whose ticks don't happen to equal their index.

test('startIndex resolves ?tick=busiest by the winning frame\'s tick VALUE, not its list position', () => {
  const replay = {frames: [
    {tick: 100, ships: [{player_id: 'a'}]},
    {tick: 205, ships: [{player_id: 'a', target_id: 'b'}, {player_id: 'b'}]},
    {tick: 310, ships: [{player_id: 'a'}]},
  ]};
  // busiestTick(replay) === 205 here (frame 1 has the only live targeting).
  // Using that return value as an array index would look up frames[205]
  // (undefined) rather than the frame whose .tick === 205 (index 1).
  assert.strictEqual(hp.startIndex(replay, new URLSearchParams('tick=busiest')), 1);
});

test('startIndex resolves an explicit ?tick=N by matching tick value', () => {
  const replay = {frames: [{tick: 100, ships: []}, {tick: 205, ships: []}, {tick: 310, ships: []}]};
  assert.strictEqual(hp.startIndex(replay, new URLSearchParams('tick=310')), 2);
});

test('startIndex falls back to frame 0 for a tick not present in the replay', () => {
  const replay = {frames: [{tick: 100, ships: []}, {tick: 205, ships: []}]};
  assert.strictEqual(hp.startIndex(replay, new URLSearchParams('tick=999')), 0);
});

test('startIndex with no tick param starts at frame 0', () => {
  const replay = {frames: [{tick: 100, ships: []}, {tick: 205, ships: []}]};
  assert.strictEqual(hp.startIndex(replay, new URLSearchParams()), 0);
});

// createPlayer — driven through its doc/win/els injection seam with the
// fakes in fake-dom.js. This is the seam Task 7 built and nothing tested:
// the scrub bug fixed in 290c3560c (a complete no-op, only caught by a live
// browser check) is exactly the class of regression these are meant to
// catch without needing a browser.

test('scrubbing seeks to the dragged frame, even though pausing writes the slider first (regression: 290c3560c)', () => {
  // setPlaying(false) calls syncControls(), which writes state.frameIndex
  // back into els.scrub.value. The scrub handler must read the dragged
  // value BEFORE pausing, or the read sees the frame it was already on and
  // the whole drag is silently a no-op — which is exactly what shipped
  // until 290c3560c.
  const {doc, win, els, replay, hulls} = makeHarness(6);
  const player = hp.createPlayer({doc, win, els, replay, hulls});
  player.start();

  player.setPlaying(true);
  assert.strictEqual(player.state.frameIndex, 0, 'sanity: playback has not moved yet');

  els.scrub.value = '4'; // simulate the user dragging the slider to frame 4
  els.scrub.dispatch('input');

  assert.strictEqual(player.state.frameIndex, 4,
    'the seek must land on the dragged frame; a read-after-pause bug would leave this at 0');
  assert.strictEqual(player.state.playing, false, 'scrubbing pauses playback');
});

test('setPlaying(true) while already playing does not start a second animation loop', () => {
  const {doc, win, els, replay, hulls} = makeHarness(10);
  const player = hp.createPlayer({doc, win, els, replay, hulls});
  player.start();

  player.setPlaying(true);
  const firstRaf = win.rafCb;
  player.setPlaying(true); // e.g. a second external call while already playing
  assert.strictEqual(win.rafCb, firstRaf,
    'requestAnimationFrame must not be called again; a second call would double the playback rate');
});

test('the play-on-parked-last-frame restart rebuilds the rail from frame 0', () => {
  const {doc, win, els, replay, hulls} = makeHarness(6);
  const player = hp.createPlayer({doc, win, els, replay, hulls});
  player.start();

  player.seek(5); // the last frame
  player.setPlaying(true);
  assert.strictEqual(player.state.frameIndex, 0, 'pressing play parked on the end restarts, not no-ops');
  assert.strictEqual(els.chatter.childElementCount, 1, 'the rail resets to just the opening frame');
});

test('the rail keeps only the last RAIL_WINDOW_TICKS entries as playback advances past it', () => {
  // 50 ticks, MS_PER_TICK is 500ms fixed inside the loop. MAX_DELTA_MS caps
  // any single delta at 250ms, so each rAF step below (250ms) advances
  // exactly half a tick; two steps cross one tick. Advance well past
  // RAIL_WINDOW_TICKS (40) and check the rail trimmed to the window rather
  // than growing unbounded.
  const {doc, win, els, replay, hulls} = makeHarness(50);
  const player = hp.createPlayer({doc, win, els, replay, hulls});
  player.start();
  assert.strictEqual(els.chatter.childElementCount, 1, 'start() seeds the rail with just the opening frame');

  player.setPlaying(true);
  // Prime the clock with a non-zero `now`: the loop treats a falsy lastNow
  // as "no previous frame", and 0 is falsy, so priming at now=0 would make
  // the *next* call read a zero delta too and silently lose one step.
  let now = 1000;
  win.rafCb(now);
  for (let i = 0; i < 90; i++) {
    now += 250;
    win.rafCb(now);
  }

  assert.strictEqual(player.state.frameIndex, 45, 'sanity: 90 half-tick steps landed on frame 45');
  assert.strictEqual(els.chatter.childElementCount, hp.RAIL_WINDOW_TICKS,
    'the rail must not grow past its window');
  // 46 blocks were ever appended (frame 0 from start(), then crossings 1..45);
  // the oldest RAIL_WINDOW_TICKS-1 of those were trimmed, so the window now
  // runs from frame 6 (tick 1006) through frame 45 (tick 1045).
  assert.strictEqual(els.chatter.firstChild.firstChild.textContent, 'TICK 1006',
    'the oldest surviving block must be the one at the trailing edge of the window');
  assert.strictEqual(els.chatter.children[els.chatter.children.length - 1].firstChild.textContent, 'TICK 1045',
    'the newest block must be the most recently crossed tick');
});
