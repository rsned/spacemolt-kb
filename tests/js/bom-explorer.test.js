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
