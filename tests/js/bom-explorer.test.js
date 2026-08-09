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
