'use strict';
const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const pc = require('../../cmd/generate-production-costs/assets/prodcost.js');

// The fixture and golden are written by TestWriteJSGolden in pkg/prodcost
// (go test ./pkg/prodcost -run TestWriteJSGolden -update).
const testdata = path.join(__dirname, '../../pkg/prodcost/testdata');
const load = (name) => JSON.parse(fs.readFileSync(path.join(testdata, name), 'utf8'));
const data = () => load('js-fixture.json');
const golden = load('js-golden.json');

const near = (a, b) => Math.abs(a - b) < 1e-9;
const key = (o) => `${o.item}|${o.station}|${o.bid}`;
const goKey = (o) => `${o.ItemID}|${o.StationID}|${o.Bid}`;

test('total matches Go CostUnder for every item at default settings', () => {
  const d = data();
  const ids = Object.keys(golden.totals);
  assert.ok(ids.length >= 5);
  for (const id of ids) {
    const want = golden.totals[id];
    const got = pc.total(d, id, {}, {});
    assert.ok(near(got.credits, want.credits), `${id} credits ${got.credits} != ${want.credits}`);
    assert.ok(near(got.ticks, want.ticks), `${id} ticks ${got.ticks} != ${want.ticks}`);
    assert.strictEqual(got.partial, want.partial, `${id} partial`);
  }
});

test('rank matches Go Opportunities at default settings', () => {
  const got = pc.rank(data(), {}, {});
  assert.strictEqual(got.length, golden.opportunities.length);
  const byKey = new Map(got.map((o) => [key(o), o]));
  for (const w of golden.opportunities) {
    const o = byKey.get(goKey(w));
    assert.ok(o, `missing ${goKey(w)}`);
    for (const [js, gf] of [['cost', 'CostPerUnit'], ['margin', 'MarginPerUnit'], ['total', 'Total'],
      ['makeHours', 'MakeHours'], ['haulHours', 'HaulHours'], ['hours', 'Hours'], ['profitPerHour', 'ProfitPerHour']]) {
      assert.ok(near(o[js], w[gf]), `${goKey(w)} ${js} ${o[js]} != ${w[gf]}`);
    }
    assert.strictEqual(o.route, w.RouteLabel, `${goKey(w)} route`);
    assert.strictEqual(o.policy, w.Policy, `${goKey(w)} policy`);
    assert.strictEqual(o.unreachable, w.Unreachable, `${goKey(w)} unreachable`);
    assert.strictEqual(o.partial, w.Partial, `${goKey(w)} partial`);
  }
});

test('rank puts partial rows after complete ones, unreachable last', () => {
  const got = pc.rank(data(), {}, {});
  const reach = got.filter((o) => !o.unreachable);
  const firstPartial = reach.findIndex((o) => o.partial);
  assert.ok(firstPartial > 0, 'fixture needs a partial row behind a complete one');
  assert.ok(reach.slice(firstPartial).every((o) => o.partial));
  // In Go's order the partial plasma_injector row is first (highest profit/hr).
  assert.ok(golden.opportunities[0].Partial);
  assert.ok(got[got.length - 1].unreachable);
  for (let i = 1; i < firstPartial; i++) {
    assert.ok(reach[i - 1].profitPerHour >= reach[i].profitPerHour);
  }
});

test('rawMode buy raises copper_wiring by 8 x the ore buy-now price', () => {
  const d = data();
  const base = pc.total(d, 'copper_wiring', {}, {}).credits;
  const price = d.items.copper_ore.asks[0].price;
  const got = pc.total(d, 'copper_wiring', {}, {rawMode: 'buy'});
  assert.ok(near(got.credits, base + 8 * price), `${got.credits} != ${base} + 8 x ${price}`);
  assert.strictEqual(got.short, false);
});

test('haulFuelPerJump raises an opportunity cost by trips x jumps x fuel / depth', () => {
  const d = data();
  const find = (rows) => rows.find((o) => o.item === 'copper_wiring' && o.station === 'bidder');
  const base = find(pc.rank(d, {}, {}));
  const fueled = find(pc.rank(d, {}, {haulFuelPerJump: 100}));
  // copper_wiring is hand only: home -> bidder (s3) and back is 2 x 3 jumps;
  // 40 units x size 0.5 fit one 100-unit hold.
  const trips = 1;
  const jumps = 6;
  assert.ok(near(fueled.cost, base.cost + trips * jumps * 100 / base.depth), `${fueled.cost} vs ${base.cost}`);
  assert.ok(near(fueled.margin, base.margin - 15));
});

test('mineFuelPerUnit charges every raw unit in the tree', () => {
  const d = data();
  const got = pc.total(d, 'harness', {}, {mineFuelPerUnit: 1});
  // harness (crft) = 3 copper_wiring (8 ore each) + 1 steel_plate (3 ore, hand).
  assert.ok(near(got.credits, 12 + 3 * 8 + 3), `${got.credits}`);
});

test('a buy choice uses the ask book and its haul fuel; a short book is flagged', () => {
  const d = data();
  // iron_ore: 50 @ 2 at far (2 hops), island unreachable (skipped), then 6 at near (1 hop).
  const one = pc.total(d, 'steel_plate', {iron_ore: 'buy'}, {haulFuelPerJump: 10});
  assert.ok(near(one.credits, 3 * (3 * 2 + 20) / 3), `${one.credits}`);
  const many = pc.total(d, 'steel_plate', {iron_ore: 'buy'}, {qty: 100});
  // 300 ore: 50 @ 2 + 250 @ 6.
  assert.ok(near(many.credits, 3 * (50 * 2 + 250 * 6) / 300), `${many.credits}`);
  const harness = pc.total(d, 'harness', {harness: 'buy'}, {qty: 10});
  assert.strictEqual(harness.short, true);
  assert.ok(near(harness.credits, 100));
});

test('a route choice overrides the policy for that item everywhere', () => {
  const d = data();
  const label = 'ABC L2 @ Dock (labor 20 + fee 4)';
  const steel = d.items.steel_plate.routes.find((r) => r.label === label);
  const got = pc.total(d, 'harness', {steel_plate: label}, {});
  assert.ok(near(got.credits, 12 + (steel.labor + steel.fee) / steel.out), `${got.credits}`);
  // Same label twice (plasma_injector has two hand recipes): recipe/label picks one.
  const alt = pc.total(d, 'plasma_injector', {plasma_injector: 'make_plasma_injector_alt/hand'}, {});
  assert.ok(near(alt.ticks, 2));
});

test('priceUnpricedAtAsk prices an unpriced input at its ask', () => {
  const d = data();
  const got = pc.total(d, 'plasma_injector', {}, {priceUnpricedAtAsk: true});
  assert.ok(near(got.credits, 40), `${got.credits}`);
  assert.strictEqual(got.partial, false);
});

test('stepOptions lists mine/buy for raw items and every route otherwise', () => {
  const d = data();
  assert.deepStrictEqual(pc.stepOptions(d, 'iron_ore').map((o) => o.value), ['', 'mine', 'buy']);
  const plasma = pc.stepOptions(d, 'plasma_injector').map((o) => o.value);
  assert.deepStrictEqual(plasma, ['', 'make_plasma_injector/hand', 'make_plasma_injector_alt/hand']);
  assert.deepStrictEqual(pc.stepOptions(d, 'mystery_part').map((o) => o.value), ['', 'buy']);
});

test('treeHTML renders one select per step and expands an item once', () => {
  const html = pc.treeHTML(data(), 'harness', {}, {});
  // harness, copper_wiring, copper_ore, steel_plate, iron_ore.
  assert.strictEqual((html.match(/<select/g) || []).length, 5);
  assert.ok(!html.includes('<script'));
});
