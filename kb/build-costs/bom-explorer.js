'use strict';
// Interactive Bill of Materials explorer.
//
// Loaded as a classic script by kb/build-costs/explorer.html and as a
// CommonJS module by tests/js/bom-explorer.test.js. Everything above the
// export guard at the bottom is pure: no DOM, no globals, no fetch.

// ---------------------------------------------------------------------------
// Graph model
// ---------------------------------------------------------------------------

// producersOf indexes which recipes produce each item. Built once per data
// load and threaded through the model functions so no function rebuilds it.
function producersOf(data) {
  const producers = new Map();
  for (const [recipeId, recipe] of Object.entries(data.recipes)) {
    for (const [itemId] of recipe.o) {
      if (!producers.has(itemId)) producers.set(itemId, []);
      producers.get(itemId).push(recipeId);
    }
  }
  for (const ids of producers.values()) ids.sort();
  return producers;
}

// isTerminalItem reports whether expansion stops at itemId: ores and raw
// materials always do, as does anything no recipe produces.
//
// The category test is load-bearing, not belt-and-braces. Four items are ores
// that ALSO have a recipe (energy_crystal, exotic_crystal, void_crystal,
// hydrogen_gas); without it they would be expanded, the base-material totals
// would stop agreeing with the static build-cost pages, and
// circuit_board -> power_cell -> energy_crystal -> circuit_board would become
// a reachable cycle. This mirrors isTerminal in pkg/bom/calculator.go.
function isTerminalItem(data, producers, itemId) {
  const item = data.items[itemId];
  if (item && (item.c === 'ore' || item.c === 'material')) return true;
  const ids = producers.get(itemId);
  return !ids || ids.length === 0;
}

// activeRecipeId resolves which recipe makes itemId under the current choices:
// an explicit choice, else the generated default, else the item's only recipe.
// Returns null for terminal items. A choice naming a recipe that does not
// produce the item is ignored rather than trusted — URLs are user-editable.
function activeRecipeId(data, producers, choices, itemId) {
  if (isTerminalItem(data, producers, itemId)) return null;
  const ids = producers.get(itemId);
  if (!ids || ids.length === 0) return null;
  const chosen = choices[itemId];
  if (chosen && ids.includes(chosen)) return chosen;
  const fallback = data.defaults[itemId];
  if (fallback && ids.includes(fallback)) return fallback;
  return ids[0];
}

// yieldOf returns how many units of itemId one batch of the recipe produces.
function yieldOf(data, recipeId, itemId) {
  for (const [id, qty] of data.recipes[recipeId].o) {
    if (id === itemId) return qty;
  }
  return 1;
}

// buildGraph expands targetId into a DAG of nodes under the given choices.
//
// One node per distinct item, never one per path: an item consumed by three
// parents is a single node with three incoming edges. Ships and facilities are
// sinks — they expand their build-materials list but have no recipe.
function buildGraph(data, producers, targetId, choices) {
  const nodes = new Map();

  function visit(id, stack) {
    if (nodes.has(id)) {
      // Already expanded. Only flag a cycle if it is on the current path.
      if (stack.has(id)) nodes.get(id).cycle = true;
      return;
    }

    const target = data.targets[id];
    if (target) {
      const inputs = target.bm.map(([itemId, qty]) => ({id: itemId, qty}));
      nodes.set(id, {
        id, kind: target.t, recipeId: null, yield: 1, inputs, leaf: false, cycle: false,
      });
      const next = new Set(stack).add(id);
      for (const input of inputs) visit(input.id, next);
      return;
    }

    const recipeId = activeRecipeId(data, producers, choices, id);
    if (recipeId === null) {
      nodes.set(id, {
        id, kind: 'item', recipeId: null, yield: 1, inputs: [], leaf: true, cycle: false,
      });
      return;
    }

    // Build the input list, OMITTING any edge that would close a cycle.
    //
    // Dropping the edge rather than merely declining to recurse is what makes
    // the graph acyclic by construction, and that is the only way the layering
    // invariant can hold: no ranking of a cyclic graph can put every input
    // strictly below its consumer, so leaving the edge in place would
    // guarantee at least one backwards arrow. The node keeps cycle:true so the
    // renderer can say so.
    const next = new Set(stack).add(id);
    const inputs = [];
    let cycle = false;
    for (const [itemId, qty] of data.recipes[recipeId].i) {
      if (next.has(itemId)) {
        cycle = true;
        continue;
      }
      inputs.push({id: itemId, qty});
    }
    nodes.set(id, {
      id, kind: 'item', recipeId, yield: yieldOf(data, recipeId, id), inputs, leaf: false, cycle,
    });

    for (const input of inputs) visit(input.id, next);
  }

  visit(targetId, new Set());
  return {targetId, nodes};
}

// rankNodes assigns each node its column: leaves are 0, and every other node
// is one past the highest-ranked of its inputs.
//
// This is longest-path layering, and it is what guarantees the visual's core
// property: every input has a strictly lower rank than its consumer, so all
// arrows run left to right and none is ever within a column. The target always
// attains the maximum rank, so the output is always rightmost.
function rankNodes(graph) {
  const ranks = new Map();

  function rank(id, stack) {
    if (ranks.has(id)) return ranks.get(id);
    // Defence in depth only: buildGraph already drops cycle-closing edges, so
    // the graph it hands us is acyclic and this branch cannot fire. It stays
    // so a future caller that builds a graph some other way still terminates.
    // Note it cannot repair the invariant on a genuinely cyclic graph — no
    // ranking can — which is why the cycle is broken during construction.
    if (stack.has(id)) return 0;
    const node = graph.nodes.get(id);
    if (!node || node.inputs.length === 0) {
      ranks.set(id, 0);
      return 0;
    }
    const next = new Set(stack).add(id);
    let best = 0;
    for (const input of node.inputs) {
      best = Math.max(best, rank(input.id, next) + 1);
    }
    ranks.set(id, best);
    return best;
  }

  for (const id of graph.nodes.keys()) rank(id, new Set());
  return ranks;
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes};
}
