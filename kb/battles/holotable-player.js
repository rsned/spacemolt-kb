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

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    interpolateFrame, lerp, lerpGuarded, clamp01,
    advance, MS_PER_TICK, MAX_DELTA_MS, SPEEDS,
    groupChatter, nameOf,
  };
}
