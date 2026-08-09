'use strict';
const test = require('node:test');
const assert = require('node:assert');
const bx = require('../../kb/build-costs/bom-explorer.js');

// A small world exercising every structural case:
//   iron_ore, energy_crystal  - ores (leaves)
//   drop_core                 - no recipe, not an ore (a drop; also a leaf)
//   steel_plate               - two recipes -> a choice, default smelt_steel
//   frame                     - one recipe, consumes steel_plate
//   widget                    - one recipe, consumes frame AND steel_plate (shared)
//   hauler                    - a ship sink consuming widget
function fixture() {
  return {
    items: {
      iron_ore: {n: 'Iron Ore', c: 'ore'},
      energy_crystal: {n: 'Energy Crystal', c: 'ore'},
      drop_core: {n: 'Drop Core', c: 'misc'},
      steel_plate: {n: 'Steel Plate', c: 'refined'},
      frame: {n: 'Frame', c: 'component'},
      widget: {n: 'Widget', c: 'component'},
    },
    recipes: {
      smelt_steel: {n: 'Smelt Steel', c: 'Refining', i: [['iron_ore', 5]], o: [['steel_plate', 2]]},
      cast_steel: {n: 'Cast Steel', c: 'Refining', i: [['iron_ore', 6]], o: [['steel_plate', 2]]},
      weld_frame: {n: 'Weld Frame', c: 'Components', i: [['steel_plate', 3]], o: [['frame', 1]]},
      assemble_widget: {
        n: 'Assemble Widget', c: 'Components',
        i: [['frame', 2], ['steel_plate', 1], ['drop_core', 1]], o: [['widget', 1]],
      },
    },
    targets: {
      hauler: {n: 'Hauler', t: 'ship', bm: [['widget', 4], ['energy_crystal', 2]]},
    },
    defaults: {steel_plate: 'smelt_steel'},
  };
}

test('producersOf maps each item to the recipes that make it, sorted', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.deepStrictEqual(producers.get('steel_plate'), ['cast_steel', 'smelt_steel']);
  assert.deepStrictEqual(producers.get('frame'), ['weld_frame']);
  assert.strictEqual(producers.get('iron_ore'), undefined);
});

test('activeRecipeId prefers an explicit choice over the default', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'steel_plate'), 'smelt_steel');
  assert.strictEqual(
    bx.activeRecipeId(data, producers, {steel_plate: 'cast_steel'}, 'steel_plate'), 'cast_steel');
});

test('activeRecipeId falls back to the sole recipe when there is no default', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'frame'), 'weld_frame');
});

test('activeRecipeId returns null for leaves', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'iron_ore'), null);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'drop_core'), null);
});

test('an ore stays terminal even when a recipe produces it', () => {
  const data = fixture();
  // Real data has four of these: energy_crystal, exotic_crystal, void_crystal,
  // hydrogen_gas. The Go flattener stops at them because of their category, and
  // this page must stop in the same place or its totals stop matching the
  // static build-cost pages.
  data.recipes.synthesise_crystal = {
    n: 'Synthesise Crystal', c: 'Refining',
    i: [['iron_ore', 4]], o: [['energy_crystal', 1]],
  };
  const producers = bx.producersOf(data);

  assert.strictEqual(bx.isTerminalItem(data, producers, 'energy_crystal'), true);
  assert.strictEqual(bx.activeRecipeId(data, producers, {}, 'energy_crystal'), null);

  const g = bx.buildGraph(data, producers, 'hauler', {});
  assert.strictEqual(g.nodes.get('energy_crystal').leaf, true, 'craftable ore must stay a leaf');
  assert.deepStrictEqual(g.nodes.get('energy_crystal').inputs, [], 'and must not be expanded');
});

test('a forced cycle still yields a graph with no backwards edge', () => {
  const data = fixture();
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});
  const ranks = bx.rankNodes(g);

  // The cycle-closing edge must be gone from the graph entirely, not merely
  // left unrecursed: no ranking of a cyclic graph can satisfy the invariant,
  // so an edge left in place would guarantee a backwards arrow.
  assert.strictEqual(g.nodes.get('steel_plate').cycle, true, 'the cutting node is flagged');
  assert.deepStrictEqual(g.nodes.get('steel_plate').inputs, [],
    'the cycle-closing edge is dropped, not retained');

  const violations = [];
  for (const node of g.nodes.values()) {
    for (const input of node.inputs) {
      if (!(ranks.get(input.id) < ranks.get(node.id))) {
        violations.push(`${input.id}(${ranks.get(input.id)}) -> ${node.id}(${ranks.get(node.id)})`);
      }
    }
  }
  assert.deepStrictEqual(violations, [], 'no edge may point backwards');
});

test('activeRecipeId ignores a choice naming a recipe that does not make the item', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(
    bx.activeRecipeId(data, producers, {steel_plate: 'weld_frame'}, 'steel_plate'), 'smelt_steel');
});

test('buildGraph creates one node per distinct item, not per path', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  // widget, frame, steel_plate, iron_ore, drop_core = 5.
  // steel_plate is reached via frame AND directly from widget, but is one node.
  assert.strictEqual(g.nodes.size, 5);
  assert.strictEqual(g.targetId, 'widget');
});

test('buildGraph marks leaves and records recipe yield', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  assert.strictEqual(g.nodes.get('iron_ore').leaf, true);
  assert.strictEqual(g.nodes.get('drop_core').leaf, true);
  assert.strictEqual(g.nodes.get('steel_plate').leaf, false);
  assert.strictEqual(g.nodes.get('steel_plate').yield, 2);
  assert.strictEqual(g.nodes.get('frame').yield, 1);
});

test('buildGraph expands a ship target from its build materials', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const hauler = g.nodes.get('hauler');
  assert.strictEqual(hauler.kind, 'ship');
  assert.strictEqual(hauler.recipeId, null);
  assert.strictEqual(hauler.yield, 1);
  assert.deepStrictEqual(hauler.inputs, [{id: 'widget', qty: 4}, {id: 'energy_crystal', qty: 2}]);
  assert.ok(g.nodes.has('widget'), 'ship target must expand its materials');
});

test('buildGraph follows an overridden recipe choice', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {steel_plate: 'cast_steel'});
  assert.strictEqual(g.nodes.get('steel_plate').recipeId, 'cast_steel');
  assert.deepStrictEqual(g.nodes.get('steel_plate').inputs, [{id: 'iron_ore', qty: 6}]);
});

test('buildGraph terminates and marks a node when the choice map forms a cycle', () => {
  const data = fixture();
  // Force a cycle: steel_plate made from a recipe consuming frame, which
  // consumes steel_plate. Not reachable through the UI; the backstop must hold.
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});
  const revisited = [...g.nodes.values()].filter((n) => n.cycle);
  assert.ok(revisited.length > 0, 'expected at least one node marked cycle');
});

test('rankNodes puts leaves at 0 and the target at the maximum', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  assert.strictEqual(ranks.get('iron_ore'), 0);
  assert.strictEqual(ranks.get('drop_core'), 0);
  assert.strictEqual(ranks.get('steel_plate'), 1);
  assert.strictEqual(ranks.get('frame'), 2);
  assert.strictEqual(ranks.get('widget'), 3);
  assert.strictEqual(Math.max(...ranks.values()), ranks.get(g.targetId));
});

test('every edge runs strictly left to right', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  for (const target of ['widget', 'hauler', 'frame', 'steel_plate']) {
    const g = bx.buildGraph(data, producers, target, {});
    const ranks = bx.rankNodes(g);
    for (const node of g.nodes.values()) {
      for (const input of node.inputs) {
        assert.ok(ranks.get(input.id) < ranks.get(node.id),
          `${target}: rank(${input.id})=${ranks.get(input.id)} must be < rank(${node.id})=${ranks.get(node.id)}`);
      }
    }
  }
});

test('rollUp rounds up to whole batches and reports surplus', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // smelt_steel: 5 iron_ore -> 2 steel_plate. Need 3 plates.
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches, surplus} = bx.rollUp(g, ranks, 3);

  assert.strictEqual(demand.get('steel_plate'), 3);
  assert.strictEqual(batches.get('steel_plate'), 2, 'ceil(3/2) = 2 batches');
  assert.strictEqual(demand.get('iron_ore'), 10, '2 batches x 5 ore');
  assert.strictEqual(surplus.get('steel_plate'), 1, '2 batches x 2 = 4 made, 3 needed');
});

test('rollUp batches a shared item once against summed demand', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // This is the case that separates the correct algorithm from the naive one,
  // so the numbers must actually diverge. Shrink both plate requirements to 1:
  //   1 widget  -> 1 frame + 1 steel_plate + 1 drop_core
  //   1 frame   -> 1 steel_plate
  //   total steel_plate demand = 1 (direct) + 1 (via frame) = 2
  //   batched ONCE against 2:  ceil(2/2) = 1 batch  -> 5 iron_ore
  //   batched PER PARENT:      ceil(1/2) + ceil(1/2) = 2 batches -> 10 iron_ore
  data.recipes.weld_frame.i = [['steel_plate', 1]];
  data.recipes.assemble_widget.i = [['frame', 1], ['steel_plate', 1], ['drop_core', 1]];

  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 1);

  assert.strictEqual(demand.get('frame'), 1);
  assert.strictEqual(demand.get('steel_plate'), 2, 'summed across both parents');
  assert.strictEqual(batches.get('steel_plate'), 1, 'ceil(2/2)=1, not ceil(1/2)+ceil(1/2)=2');
  assert.strictEqual(demand.get('iron_ore'), 5, '1 batch x 5 ore, not 10');
});

test('rollUp scales a ship target by quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 3);

  assert.strictEqual(demand.get('hauler'), 3);
  assert.strictEqual(demand.get('widget'), 12, '3 haulers x 4 widgets');
  assert.strictEqual(demand.get('energy_crystal'), 6, '3 haulers x 2 crystals');
  // A sink must carry a batches entry equal to its demand. Task 7 scales the
  // direct-inputs table by batches.get(target), so an absent entry would fall
  // back to 1 and show a ship's inputs at quantity 1 whatever was asked for.
  assert.strictEqual(batches.get('hauler'), 3, 'sinks get a batches entry');
});

test('rollUp reports no surplus when every yield divides evenly', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {surplus} = bx.rollUp(g, ranks, 4); // 2 batches x 2 = exactly 4
  assert.strictEqual(surplus.size, 0);
});

test('rollUp leaves have demand but no batches', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {demand, batches} = bx.rollUp(g, ranks, 1);
  assert.strictEqual(demand.get('iron_ore'), 5);
  assert.strictEqual(batches.has('iron_ore'), false);
});

test('orderColumns indexes columns by rank with the target alone on the right', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);

  assert.strictEqual(columns.length, 4, 'ranks 0..3');
  assert.deepStrictEqual(columns[3], ['widget']);
  assert.deepStrictEqual(columns[2], ['frame']);
  assert.deepStrictEqual(columns[1], ['steel_plate']);
  // Assert exact order, not just membership. These two are NOT tied:
  // iron_ore's consumer (steel_plate) sits in the adjacent column so it gets a
  // real barycentre of 0, while drop_core's only consumer (widget) is three
  // columns away, so it has no barycentre and sorts to the bottom. This pins
  // the distant-consumer behaviour that the sweep documents.
  assert.deepStrictEqual(columns[0], ['iron_ore', 'drop_core']);
});

test('orderColumns places every node exactly once', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);

  const placed = columns.flat();
  assert.strictEqual(placed.length, g.nodes.size);
  assert.strictEqual(new Set(placed).size, g.nodes.size, 'no duplicates');
  for (const id of g.nodes.keys()) assert.ok(placed.includes(id), `${id} missing`);
});

test('orderColumns is deterministic across repeated calls', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  assert.deepStrictEqual(bx.orderColumns(g, ranks), bx.orderColumns(g, ranks));
});

test('layout puts lower columns to the left and never overlaps boxes', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);
  const {boxes, width, height} = bx.layout(g, columns);

  assert.ok(boxes.get('iron_ore').x < boxes.get('steel_plate').x);
  assert.ok(boxes.get('steel_plate').x < boxes.get('frame').x);
  assert.ok(boxes.get('frame').x < boxes.get('widget').x);

  // No two boxes in the same column overlap vertically.
  for (const column of columns) {
    const sorted = column.map((id) => boxes.get(id)).sort((a, b) => a.y - b.y);
    for (let i = 1; i < sorted.length; i++) {
      assert.ok(sorted[i].y >= sorted[i - 1].y + sorted[i - 1].h,
        'boxes in a column must not overlap');
    }
  }

  // Canvas contains every box.
  for (const b of boxes.values()) {
    assert.ok(b.x >= 0 && b.y >= 0 && b.x + b.w <= width && b.y + b.h <= height);
  }
});

test('layout emits one edge per input with its quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const {edges} = bx.layout(g, bx.orderColumns(g, ranks));

  let total = 0;
  for (const n of g.nodes.values()) total += n.inputs.length;
  assert.strictEqual(edges.length, total);

  const direct = edges.find((e) => e.from === 'steel_plate' && e.to === 'widget');
  assert.ok(direct, 'expected the direct steel_plate -> widget edge');
  assert.strictEqual(direct.qty, 1);
  assert.ok(direct.points.length >= 2, 'edge must have a polyline');
});

test('layout handles the two-box refining degenerate case', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const {boxes, edges} = bx.layout(g, bx.orderColumns(g, ranks));

  assert.strictEqual(boxes.size, 2);
  assert.strictEqual(edges.length, 1);
  assert.strictEqual(edges[0].qty, 5);
});

test('scaling an edge by its consumer\'s batch count sums to the source\'s demand', () => {
  // edge.qty is the per-batch recipe input quantity, not the total consumed —
  // the renderer must multiply by totals.batches.get(edge.to) before display
  // (renderChart does this; it has no DOM-free seam to test directly, so this
  // pins the underlying arithmetic through the exported pure functions).
  // steel_plate feeds TWO different consumers here (frame and widget), each
  // via its own edge, so this also exercises multiple edges from one source.
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const ranks = bx.rankNodes(g);
  const columns = bx.orderColumns(g, ranks);
  const totals = bx.rollUp(g, ranks, 5);
  const {edges} = bx.layout(g, columns);

  const scaledLabel = (edge) => edge.qty * (totals.batches.get(edge.to) || 1);

  const toFrame = edges.find((e) => e.from === 'steel_plate' && e.to === 'frame');
  const toWidget = edges.find((e) => e.from === 'steel_plate' && e.to === 'widget');
  assert.strictEqual(scaledLabel(toFrame), toFrame.qty * totals.batches.get('frame'));
  assert.strictEqual(scaledLabel(toWidget), toWidget.qty * totals.batches.get('widget'));

  const outgoing = edges.filter((e) => e.from === 'steel_plate');
  const sum = outgoing.reduce((acc, e) => acc + scaledLabel(e), 0);
  assert.strictEqual(sum, totals.demand.get('steel_plate'),
    'a source item\'s scaled outgoing edges must sum to its total demand');
});

test('selectableOutputs spans craftable items, ships and facilities', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const out = bx.selectableOutputs(data, producers);
  const ids = out.map((o) => o.id);

  assert.ok(ids.includes('steel_plate'), 'craftable item must be selectable');
  assert.ok(ids.includes('widget'));
  assert.ok(ids.includes('hauler'), 'ship must be selectable');
  assert.ok(!ids.includes('iron_ore'), 'ores are not selectable outputs');
  assert.ok(!ids.includes('drop_core'), 'no-recipe drops are not selectable outputs');

  // A craftable ore is still terminal, so it is still not a selectable output.
  data.recipes.synthesise_crystal = {
    n: 'Synthesise Crystal', c: 'Refining',
    i: [['iron_ore', 4]], o: [['energy_crystal', 1]],
  };
  const withCraftableOre = bx.selectableOutputs(data, bx.producersOf(data)).map((o) => o.id);
  assert.ok(!withCraftableOre.includes('energy_crystal'),
    'an ore with a recipe is still terminal, so still not selectable');

  assert.strictEqual(out.find((o) => o.id === 'hauler').type, 'ship');
  assert.strictEqual(out.find((o) => o.id === 'steel_plate').type, 'item');
});

test('selectableOutputs sorts by display name', () => {
  const data = fixture();
  const out = bx.selectableOutputs(data, bx.producersOf(data));
  const names = out.map((o) => o.name);
  assert.deepStrictEqual(names, [...names].sort());
});

test('encodeState omits defaults and the default quantity', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(
    bx.encodeState(data, producers, {target: 'widget', qty: 1, choices: {}}),
    'target=widget');
  assert.strictEqual(
    bx.encodeState(data, producers, {target: 'widget', qty: 5, choices: {steel_plate: 'smelt_steel'}}),
    'target=widget&qty=5',
    'smelt_steel is the default and must not be serialised');
});

test('encodeState serialises non-default choices sorted by item', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const got = bx.encodeState(data, producers,
    {target: 'widget', qty: 1, choices: {steel_plate: 'cast_steel'}});
  assert.strictEqual(got, 'target=widget&r=steel_plate:cast_steel');
});

test('decodeState round-trips an encoded state', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const state = {target: 'widget', qty: 42, choices: {steel_plate: 'cast_steel'}};
  const decoded = bx.decodeState(data, producers, bx.encodeState(data, producers, state));
  assert.deepStrictEqual(decoded, state);
});

test('decodeState clamps quantity into range', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=0').qty, 1);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=-7').qty, 1);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=999999').qty, 99999);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=3.7').qty, 3);
  assert.strictEqual(bx.decodeState(data, producers, 'target=widget&qty=abc').qty, 1);
});

test('decodeState discards prototype-chain property names as targets', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // A bare bracket lookup is truthy for every Object.prototype member, so
  // these would pass validation and then throw in buildGraph. The page must
  // degrade, not break, on any hand-edited URL.
  for (const name of ['__proto__', 'constructor', 'toString', 'hasOwnProperty', 'valueOf']) {
    const state = bx.decodeState(data, producers, 'target=' + name);
    assert.strictEqual(state.target, null, `${name} must not validate as a target`);
  }
});

test('decodeState discards unknown targets and bogus choices', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.decodeState(data, producers, 'target=nonesuch').target, null);
  assert.strictEqual(bx.decodeState(data, producers, '').target, null);
  // weld_frame does not produce steel_plate, so the choice is dropped.
  assert.deepStrictEqual(
    bx.decodeState(data, producers, 'target=widget&r=steel_plate:weld_frame').choices, {});
  assert.deepStrictEqual(
    bx.decodeState(data, producers, 'target=widget&r=garbage').choices, {});
});

test('itemHref points at the catalog page and is empty without a category', () => {
  const data = fixture();
  assert.strictEqual(bx.itemHref(data, 'steel_plate'), '../items/refined/steel_plate.html');
  data.items.mystery = {n: 'Mystery', c: ''};
  assert.strictEqual(bx.itemHref(data, 'mystery'), '');
  assert.strictEqual(bx.itemHref(data, 'nonesuch'), '');
});

test('leafKind separates mined ores from no-recipe drops', () => {
  const data = fixture();
  assert.strictEqual(bx.leafKind(data, 'iron_ore'), 'ore');
  assert.strictEqual(bx.leafKind(data, 'drop_core'), 'drop');
});

test('escapeHTML neutralises markup', () => {
  assert.strictEqual(bx.escapeHTML('<a href="x">&</a>'),
    '&lt;a href=&quot;x&quot;&gt;&amp;&lt;/a&gt;');
});

test('decodeState admits a terminal item so render can explain it', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  assert.strictEqual(bx.decodeState(data, producers, 'target=iron_ore').target, 'iron_ore');
  assert.strictEqual(bx.decodeState(data, producers, 'target=drop_core').target, 'drop_core');
  assert.strictEqual(bx.decodeState(data, producers, 'target=nonesuch').target, null);
});

test('withRoutingNodes leaves the original graph untouched', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const before = g.nodes.size;

  bx.withRoutingNodes(g, ranks);

  assert.strictEqual(g.nodes.size, before, 'the original graph must not be mutated');
  for (const id of g.nodes.keys()) {
    assert.ok(!bx.isRoutingNode(id), 'no waypoint may leak into the original graph');
  }
});

test('withRoutingNodes inserts one waypoint per column a long edge crosses', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // hauler consumes energy_crystal (an ore, rank 0) directly, and hauler is the
  // target at the maximum rank, so that edge spans several columns.
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const span = ranks.get('hauler') - ranks.get('energy_crystal');
  assert.ok(span > 1, 'fixture must actually contain a long edge');

  const routed = bx.withRoutingNodes(g, ranks);
  // Scoped to the energy_crystal -> hauler chain specifically: the fixture's
  // widget subgraph independently contains two more long edges of its own
  // (steel_plate -> widget, drop_core -> widget), so counting every waypoint
  // in the whole graph would not equal this one edge's span - 1.
  const chainWaypoints = [...routed.graph.nodes.keys()]
    .filter((id) => bx.isRoutingNode(id) && id.startsWith('__route:energy_crystal>hauler:'));
  assert.strictEqual(chainWaypoints.length, span - 1,
    'one waypoint per intervening column');

  // The direct edge is gone, replaced by a chain.
  const hauler = routed.graph.nodes.get('hauler');
  assert.ok(!hauler.inputs.some((i) => i.id === 'energy_crystal'),
    'the long edge must be replaced by its chain, not left alongside it');
});

test('after routing, every edge connects adjacent columns only', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  for (const target of ['hauler', 'widget', 'frame']) {
    const g = bx.buildGraph(data, producers, target, {});
    const routed = bx.withRoutingNodes(g, bx.rankNodes(g));
    for (const node of routed.graph.nodes.values()) {
      for (const input of node.inputs) {
        assert.strictEqual(routed.ranks.get(node.id) - routed.ranks.get(input.id), 1,
          `${target}: ${input.id} -> ${node.id} must span exactly one column after routing`);
      }
    }
  }
});

test('routed edges carry the original quantity, not a per-segment one', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const routed = bx.withRoutingNodes(g, bx.rankNodes(g));
  // Walk the chain that replaced energy_crystal -> hauler and confirm every
  // segment reports the original quantity of 2.
  const chain = [...routed.graph.nodes.values()]
    .filter((n) => n.inputs.some((i) => i.id === 'energy_crystal' || bx.isRoutingNode(i.id)));
  const qtys = new Set();
  for (const n of chain) {
    for (const i of n.inputs) {
      if (i.id === 'energy_crystal' || bx.isRoutingNode(i.id)) qtys.add(i.qty);
    }
  }
  assert.ok(qtys.has(2), 'the original quantity must survive routing');
});

test('no laid-out edge passes through an unrelated box', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const routed = bx.withRoutingNodes(g, bx.rankNodes(g));
  const columns = bx.orderColumns(routed.graph, routed.ranks);
  const {boxes, edges} = bx.layout(routed.graph, columns, producers, routed.dummies);

  for (const e of edges) {
    for (const [id, b] of boxes) {
      if (id === e.from || id === e.to || bx.isRoutingNode(id)) continue;
      for (const [x, y] of e.points) {
        const inside = x > b.x && x < b.x + b.w && y > b.y && y < b.y + b.h;
        assert.ok(!inside, `edge ${e.from} -> ${e.to} has a point inside box ${id}`);
      }
    }
  }
});

test('curvePath emits a smooth path that starts and ends at the given points', () => {
  const d = bx.curvePath([[0, 0], [50, 20], [100, 20]]);
  assert.match(d, /^M0,0/, 'must start with a moveto at the first point');
  assert.match(d, /C/, 'must use cubic segments');
  assert.ok(d.trim().endsWith('100,20'), 'must end at the last point');
});

test('curvePath degenerates safely for a two-point path', () => {
  const d = bx.curvePath([[0, 0], [10, 10]]);
  assert.match(d, /^M0,0/);
  assert.ok(d.includes('10,10'));
});

// boxOf approximates a rendered label's bounding box the same way the
// project owner measured the real page: width = text.length * 6,
// height = 12, anchored at the label's (x, y) baseline (renderChart draws
// the <text> with that baseline, text-anchor "start", no dominant-baseline,
// so the glyphs sit above y and run rightward from x).
function boxOf(pos, text) {
  return {x: pos.x, y: pos.y - 12, w: text.length * 6, h: 12};
}

function boxesOverlap(a, b) {
  return a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y;
}

test('labelLayout separates two labels from a source that departs both up and down', () => {
  // ore feeds two siblings in the same column: part_a sorts above ore's row,
  // part_b below, so ore's two outgoing edges depart in opposite directions
  // — the exact shape of the reported bug (one source, two consumers, labels
  // landing on top of each other).
  const data = {
    items: {
      ore: {n: 'Ore', c: 'ore'},
      part_a: {n: 'Part A', c: 'component'},
      part_b: {n: 'Part B', c: 'component'},
    },
    recipes: {
      make_a: {n: 'A', c: 'C', i: [['ore', 1]], o: [['part_a', 1]]},
      make_b: {n: 'B', c: 'C', i: [['ore', 4]], o: [['part_b', 1]]},
      assemble: {n: 'Asm', c: 'C', i: [['part_a', 1], ['part_b', 1]], o: [['final', 1]]},
    },
    targets: {},
    defaults: {},
  };
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'final', {});
  const ranks = bx.rankNodes(g);
  const routed = bx.withRoutingNodes(g, ranks);
  const columns = bx.orderColumns(routed.graph, routed.ranks);
  const {edges} = bx.layout(routed.graph, columns, producers, routed.dummies);

  const fromOre = edges.filter((e) => e.from === 'ore');
  assert.strictEqual(fromOre.length, 2, 'fixture must give ore two outgoing edges');

  const positions = bx.labelLayout(edges);
  const boxes = fromOre.map((e) => {
    const pos = positions[edges.indexOf(e)];
    return boxOf(pos, String(e.qty));
  });
  assert.ok(!boxesOverlap(boxes[0], boxes[1]),
    `ore's two edge labels overlap: ${JSON.stringify(boxes)}`);
});

test('labelLayout leaves a source with one outgoing edge unchanged', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'frame', {});
  const routed = bx.withRoutingNodes(g, bx.rankNodes(g));
  const columns = bx.orderColumns(routed.graph, routed.ranks);
  const {edges} = bx.layout(routed.graph, columns, producers, routed.dummies);

  // frame has exactly one input (steel_plate), so this is the single-edge
  // case: the label must sit at the original fixed lead/offset, not the
  // grouped stagger.
  const positions = bx.labelLayout(edges);
  const edge = edges.find((e) => e.to === 'frame');
  const pos = positions[edges.indexOf(edge)];
  const [x0, y0] = edge.points[0];
  const [x1, y1] = edge.points[1];
  const dx = x1 - x0, dy = y1 - y0;
  const len = Math.hypot(dx, dy) || 1;
  const t = Math.min(18, len - 2) / len;
  assert.strictEqual(pos.x, x0 + dx * t);
  assert.strictEqual(pos.y, y0 + dy * t - 5);
});

test('edgesByNode maps each node to every edge touching it, as source or consumer', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {});
  const routed = bx.withRoutingNodes(g, bx.rankNodes(g));
  const columns = bx.orderColumns(routed.graph, routed.ranks);
  const {edges} = bx.layout(routed.graph, columns, producers, routed.dummies);

  const byNode = bx.edgesByNode(edges);
  // steel_plate: iron_ore->steel_plate (in), steel_plate->frame (out),
  // steel_plate->widget (out).
  assert.strictEqual(byNode.get('steel_plate').length, 3);
  // frame: steel_plate->frame (in), frame->widget (out).
  assert.strictEqual(byNode.get('frame').length, 2);
  // widget: three inputs, no outputs (it's the target).
  assert.strictEqual(byNode.get('widget').length, 3);
  assert.ok(!byNode.has('nonexistent_node'));

  // Every returned edge must actually touch the node it's filed under.
  for (const [id, touching] of byNode) {
    for (const e of touching) assert.ok(e.from === id || e.to === id);
  }
});
