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
