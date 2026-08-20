'use strict';

// holotable-player.js — motion for the battle holotable.
//
// The seam against holotable.js is "decides which frame, and when" versus
// "draws a frame". Nothing here draws; nothing in holotable.js knows this file
// exists. Design:
// docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md

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
      alpha: 1,
      // fuel is NOT interpolated: unlike hull/shield, a 0 reading is a real
      // zero (a stranded ship), not "no data" — lerpGuarded's zero-means-
      // unknown guard is the wrong semantics for it, and holotable.js does
      // not render a fuel gauge yet. Object.assign already carries prev's
      // fuel through above; when a fuel gauge exists, wire it with plain
      // lerp, not lerpGuarded.
      //
      // zone, stance and target_id are NOT copied from b either: Object.assign
      // already took prev's, and they must hold for the whole interval. A
      // target acquired next tick must not draw its line until the ship
      // arrives.
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
    els.scrub.addEventListener('input', () => {
      // Read the slider BEFORE pausing. setPlaying(false) calls syncControls(),
      // which writes state.frameIndex back into els.scrub.value — so reading it
      // afterwards returns the frame we were already on and the scrub is a no-op.
      const want = Number(els.scrub.value);
      setPlaying(false);
      seek(want);
    });
    els.speed.addEventListener('change', () => { state.speed = Number(els.speed.value); });
    cfg.win.addEventListener('keydown', onKey);
    cfg.win.addEventListener('resize', resize);

    resize();
    rebuildRail();
    syncControls();
  }

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

  return {start, setPlaying, seek, render, resize, bench, state};
}

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
    // busiestTick returns a TICK NUMBER, not a frame object — pickFrame looks it
    // up with find(f => f.tick === tick). indexOf against a number never matches.
    const i = replay.frames.findIndex(f => f.tick === ht.busiestTick(replay));
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

    const benchN = Number(params.get('bench') || 0);
    if (benchN > 0) {
      const r = player.bench(benchN);
      const line = `bench: ${r.frames} frames, ${r.msPerFrame.toFixed(2)} ms/frame ` +
        `(${(1000 / r.msPerFrame).toFixed(0)} fps ceiling), ${replay.participants.length} participants`;
      console.log(line);
      status.textContent = line;
      return;
    }

    if (params.get('play') === '1') player.setPlaying(true);
    status.textContent = '';
  } catch (err) {
    status.textContent = 'Could not draw the battle: ' + err.message;
  }
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initHolotable);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    interpolateFrame, lerp, lerpGuarded, clamp01,
    advance, MS_PER_TICK, MAX_DELTA_MS, SPEEDS,
    groupChatter, nameOf, railBlock, createPlayer, startIndex, RAIL_WINDOW_TICKS,
  };
}
