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

// activeRecipeId resolves which recipe makes itemId under the current choices:
// an explicit choice, else the generated default, else the item's only recipe.
// Returns null for leaves. A choice naming a recipe that does not produce the
// item is ignored rather than trusted — URLs are user-editable.
function activeRecipeId(data, producers, choices, itemId) {
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

    const inputs = data.recipes[recipeId].i.map(([itemId, qty]) => ({id: itemId, qty}));
    nodes.set(id, {
      id, kind: 'item', recipeId, yield: yieldOf(data, recipeId, id), inputs, leaf: false, cycle: false,
    });

    const next = new Set(stack).add(id);
    for (const input of inputs) {
      if (next.has(input.id)) {
        // Cycle: stop before recursing, and mark the node we would re-enter.
        if (nodes.has(input.id)) nodes.get(input.id).cycle = true;
        continue;
      }
      visit(input.id, next);
    }
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
    if (stack.has(id)) return 0; // cycle backstop; expansion already flagged it
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
  module.exports = {producersOf, activeRecipeId, yieldOf, buildGraph, rankNodes};
}
