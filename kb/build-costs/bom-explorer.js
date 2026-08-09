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

// topoOrder returns node ids ordered output-first. Every edge runs from a
// higher rank to a strictly lower one, so descending rank is a valid
// topological order; ties break by id so the result is deterministic.
function topoOrder(graph, ranks) {
  return [...graph.nodes.keys()].sort((a, b) => {
    const d = ranks.get(b) - ranks.get(a);
    return d !== 0 ? d : (a < b ? -1 : a > b ? 1 : 0);
  });
}

// rollUp computes how much of each item the build needs, in whole batches.
//
// Batch counts cannot be decided top-down, because a shared item's batch count
// depends on its TOTAL demand across every parent: ceil-ing per parent and
// summing over-counts. Walking output-first in topological order means an
// item's demand is final by the time its batches are computed.
function rollUp(graph, ranks, quantity) {
  const demand = new Map();
  const batches = new Map();
  const surplus = new Map();

  demand.set(graph.targetId, quantity);

  for (const id of topoOrder(graph, ranks)) {
    const node = graph.nodes.get(id);
    const need = demand.get(id) || 0;
    if (need === 0 || node.inputs.length === 0) continue;

    const perBatch = node.yield || 1;
    const runs = Math.ceil(need / perBatch);
    batches.set(id, runs);
    const made = runs * perBatch;
    if (made > need) surplus.set(id, made - need);

    for (const input of node.inputs) {
      demand.set(input.id, (demand.get(input.id) || 0) + runs * input.qty);
    }
  }

  return {demand, batches, surplus};
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

const BOX_W = 150;
const BOX_H = 46;
const BOX_H_SEL = 66;
const COL_GAP = 90;
const ROW_GAP = 14;
const MARGIN = 20;

// orderColumns groups nodes by rank and orders each column to reduce edge
// crossings, using a barycentre sweep: a node sorts to the mean vertical
// position of its consumers in the column to its right. Working right-to-left
// from the target (a single node) gives the sweep something to anchor on.
// A node with no consumer in the column to its right (nothing there takes it
// as an input, e.g. a leaf shared only with a farther-right column that has
// already been placed by an earlier iteration) has no barycentre to sort by
// and falls to the bottom, ordered by id among its fellow orphans.
// Ties break by id so repeated calls agree.
//
// One sweep only: a second right-to-left pass reorders each column against
// columns[col+1], which the same sweep already finalised, so it is provably
// a no-op — verified on overmind (135 nodes) by parametrising the pass count
// and diffing output. An alternating right-left-right sweep was also
// measured and produced identical orderings and identical edge-crossing
// counts on every large target tried, so it buys nothing either.
function orderColumns(graph, ranks) {
  const maxRank = Math.max(...ranks.values());
  const columns = [];
  for (let i = 0; i <= maxRank; i++) columns.push([]);
  for (const id of [...graph.nodes.keys()].sort()) columns[ranks.get(id)].push(id);

  // consumers[id] = ids of nodes that take id as an input.
  const consumers = new Map();
  for (const node of graph.nodes.values()) {
    for (const input of node.inputs) {
      if (!consumers.has(input.id)) consumers.set(input.id, []);
      consumers.get(input.id).push(node.id);
    }
  }

  for (let col = columns.length - 2; col >= 0; col--) {
    const rightPos = new Map();
    columns[col + 1].forEach((id, i) => rightPos.set(id, i));
    const bary = new Map();
    for (const id of columns[col]) {
      const positions = (consumers.get(id) || [])
        .map((c) => rightPos.get(c))
        .filter((p) => p !== undefined);
      bary.set(id, positions.length
        ? positions.reduce((a, b) => a + b, 0) / positions.length
        : Number.MAX_SAFE_INTEGER);
    }
    columns[col].sort((a, b) => {
      const d = bary.get(a) - bary.get(b);
      return d !== 0 ? d : (a < b ? -1 : a > b ? 1 : 0);
    });
  }

  return columns;
}

// boxHeight returns a node's height: taller when it carries a recipe selector.
function boxHeight(producers, id) {
  const ids = producers ? producers.get(id) : null;
  return ids && ids.length > 1 ? BOX_H_SEL : BOX_H;
}

// layout converts ordered columns into drawable geometry. Columns are placed
// left to right by rank, so a base ore consumed directly by the output spans
// the full width — expected, not a defect. Each column is vertically centred
// against the tallest so short columns do not hug the top.
//
// producers is optional; pass it to size boxes that carry a recipe selector.
function layout(graph, ranks, columns, producers) {
  const heights = columns.map((column) =>
    column.reduce((sum, id) => sum + boxHeight(producers, id) + ROW_GAP, -ROW_GAP));
  const tallest = Math.max(0, ...heights);

  const boxes = new Map();
  columns.forEach((column, col) => {
    let y = MARGIN + (tallest - heights[col]) / 2;
    column.forEach((id, row) => {
      const h = boxHeight(producers, id);
      boxes.set(id, {x: MARGIN + col * (BOX_W + COL_GAP), y, w: BOX_W, h, col, row});
      y += h + ROW_GAP;
    });
  });

  const width = MARGIN * 2 + columns.length * BOX_W + Math.max(0, columns.length - 1) * COL_GAP;
  const height = MARGIN * 2 + tallest;

  // Elbow polylines: out of the input's right edge, across to the midpoint of
  // the gutter immediately left of the consumer, vertically, then in.
  const edges = [];
  for (const node of graph.nodes.values()) {
    const to = boxes.get(node.id);
    if (!to) continue;
    for (const input of node.inputs) {
      const from = boxes.get(input.id);
      if (!from) continue;
      const x1 = from.x + from.w;
      const y1 = from.y + from.h / 2;
      const x2 = to.x;
      const y2 = to.y + to.h / 2;
      const mid = x2 - COL_GAP / 2;
      const points = y1 === y2
        ? [[x1, y1], [x2, y2]]
        : [[x1, y1], [mid, y1], [mid, y2], [x2, y2]];
      edges.push({from: input.id, to: node.id, qty: input.qty, points});
    }
  }

  return {width, height, boxes, edges};
}

// ---------------------------------------------------------------------------
// Selectable outputs and URL state
// ---------------------------------------------------------------------------

const QTY_MIN = 1;
const QTY_MAX = 99999;

// hasOwn tests real membership of a plain object parsed from JSON.
//
// A bare `data.targets[key]` lookup is truthy for every Object.prototype
// member — `__proto__`, `constructor`, `toString`, `hasOwnProperty` — so a
// hand-edited `?target=__proto__` would pass validation and then throw in
// buildGraph. The URL is the one place untrusted keys enter, so this is
// where it gets checked.
function hasOwn(obj, key) {
  return Object.prototype.hasOwnProperty.call(obj, key);
}

// selectableOutputs is everything the user may pick as an output: every ship
// and facility, plus every non-terminal item some recipe produces. Terminal
// items are excluded — the explorer treats them as raw inputs, so offering one
// as an output would render a single leaf box and no tables. That exclusion
// must use isTerminalItem, not merely "has no recipe": four ores DO have
// recipes (energy_crystal, exotic_crystal, void_crystal, hydrogen_gas) and
// still must not be selectable. Derived rather than shipped as a fourth list.
function selectableOutputs(data, producers) {
  const out = [];
  for (const [id, target] of Object.entries(data.targets)) {
    out.push({id, name: target.n, type: target.t});
  }
  for (const id of producers.keys()) {
    const item = data.items[id];
    if (!item) continue;
    if (isTerminalItem(data, producers, id)) continue;
    out.push({id, name: item.n, type: 'item'});
  }
  out.sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1
    : a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  return out;
}

// clampQty truncates to an integer inside [QTY_MIN, QTY_MAX]. Anything
// unparseable becomes QTY_MIN rather than blanking the page.
function clampQty(value) {
  const n = Math.trunc(Number(value));
  if (!Number.isFinite(n)) return QTY_MIN;
  return Math.min(QTY_MAX, Math.max(QTY_MIN, n));
}

// encodeState renders state as a query string with no leading '?'. Choices
// equal to the generated default and the default quantity are omitted, so the
// common URL is just target=<id>. Choice keys are sorted for stability.
function encodeState(data, producers, state) {
  const parts = [];
  if (state.target) parts.push('target=' + encodeURIComponent(state.target));
  const qty = clampQty(state.qty);
  if (qty !== QTY_MIN) parts.push('qty=' + qty);

  const pairs = [];
  for (const item of Object.keys(state.choices || {}).sort()) {
    const recipe = state.choices[item];
    const ids = producers.get(item);
    if (!ids || !ids.includes(recipe)) continue;
    if (recipe === data.defaults[item]) continue;
    if (!data.defaults[item] && ids.length < 2) continue;
    pairs.push(item + ':' + recipe);
  }
  // Item and recipe ids are [a-z0-9_] throughout the crafting data, so the
  // pairs need no escaping. Do NOT run them through encodeURIComponent: it
  // escapes the ':' separator to %3A and makes the URL unreadable.
  if (pairs.length) parts.push('r=' + pairs.join(','));

  return parts.join('&');
}

// decodeState parses a query string back into state. Unknown targets, unknown
// recipe ids, recipes that do not produce their item, and out-of-range
// quantities are all discarded in favour of the defaults — URLs are
// user-editable and a bad one must degrade, not break the page.
function decodeState(data, producers, query) {
  const params = new URLSearchParams(query || '');

  let target = params.get('target');
  if (target && !hasOwn(data.targets, target) && !hasOwn(data.items, target) &&
      !producers.has(target)) {
    target = null;
  }

  const qty = params.has('qty') ? clampQty(params.get('qty')) : QTY_MIN;

  const choices = {};
  for (const pair of (params.get('r') || '').split(',')) {
    if (!pair) continue;
    const idx = pair.indexOf(':');
    if (idx < 1) continue;
    const item = pair.slice(0, idx);
    const recipe = pair.slice(idx + 1);
    const ids = producers.get(item);
    if (!ids || !ids.includes(recipe)) continue;
    choices[item] = recipe;
  }

  return {target: target || null, qty, choices};
}

// ---------------------------------------------------------------------------
// DOM layer
// ---------------------------------------------------------------------------
// Everything below touches the document. It is skipped entirely under Node,
// which loads this file to test the pure functions above.

const MAX_SUGGESTIONS = 50;

// itemHref returns an item's KB catalog page, or '' when it has no category.
function itemHref(data, id) {
  const item = data.items[id];
  if (!item || !item.c) return '';
  return '../items/' + item.c + '/' + id + '.html';
}

// targetHref returns the catalog page for a graph node of any kind.
function targetHref(data, id) {
  const target = data.targets[id];
  if (target) return target.t === 'ship' ? '../ships/all.html' : '';
  return itemHref(data, id);
}

// displayName falls back to the raw id so an unknown item never renders blank.
function displayName(data, id) {
  if (data.targets[id]) return data.targets[id].n;
  return (data.items[id] && data.items[id].n) || id;
}

// leafKind labels a leaf: ores are mined, anything else with no recipe is a
// drop. Keeping them visually distinct stops a drop reading as mineable.
function leafKind(data, id) {
  const item = data.items[id];
  if (!item) return 'unknown';
  return item.c === 'ore' || item.c === 'material' ? 'ore' : 'drop';
}

function initExplorer() {
  const els = {
    target: document.getElementById('target-input'),
    list: document.getElementById('target-list'),
    qty: document.getElementById('qty-input'),
    status: document.getElementById('status'),
    result: document.getElementById('result'),
    recipe: document.getElementById('chart-recipe'),
    meta: document.getElementById('chart-meta'),
    chart: document.getElementById('chart'),
    base: document.getElementById('table-base'),
    direct: document.getElementById('table-direct'),
    directSub: document.getElementById('direct-sub'),
    surplus: document.getElementById('table-surplus'),
    surplusSection: document.getElementById('surplus-section'),
  };

  let data = null;
  let producers = null;
  let outputs = [];
  let state = {target: null, qty: QTY_MIN, choices: {}};
  let suggestions = [];
  let activeIndex = -1;

  fetch('recipe-graph.json')
    .then((r) => {
      if (!r.ok) throw new Error('HTTP ' + r.status);
      return r.json();
    })
    .then((doc) => {
      data = doc;
      producers = producersOf(data);
      outputs = selectableOutputs(data, producers);
      state = decodeState(data, producers, window.location.search.slice(1));
      if (state.target) els.target.value = displayName(data, state.target);
      els.qty.value = state.qty;
      render();
    })
    .catch((err) => {
      els.status.textContent = 'Could not load recipe-graph.json: ' + err.message;
    });

  // -- autocomplete ---------------------------------------------------------

  function matches(query) {
    const q = query.trim().toLowerCase();
    if (!q) return outputs.slice(0, MAX_SUGGESTIONS);
    const hits = [];
    for (const o of outputs) {
      if (o.name.toLowerCase().includes(q) || o.id.includes(q)) {
        hits.push(o);
        if (hits.length >= MAX_SUGGESTIONS) break;
      }
    }
    return hits;
  }

  function closeList() {
    els.list.classList.remove('open');
    els.target.setAttribute('aria-expanded', 'false');
    activeIndex = -1;
    // Clear the backing array too, not just the visible state. Leaving it
    // populated lets Escape-then-ArrowDown-then-Enter silently pick an option
    // the user can no longer see and believes they dismissed.
    suggestions = [];
  }

  function openList(query) {
    suggestions = matches(query);
    els.list.innerHTML = '';
    for (const [i, o] of suggestions.entries()) {
      const li = document.createElement('li');
      li.setAttribute('role', 'option');
      const name = document.createElement('span');
      name.textContent = o.name;
      const type = document.createElement('span');
      type.className = 'type';
      type.textContent = o.type;
      li.append(name, type);
      li.addEventListener('mousedown', (e) => {
        e.preventDefault();
        choose(i);
      });
      els.list.append(li);
    }
    els.list.classList.toggle('open', suggestions.length > 0);
    els.target.setAttribute('aria-expanded', String(suggestions.length > 0));
    activeIndex = -1;
  }

  function highlight(delta) {
    if (!suggestions.length) return;
    activeIndex = (activeIndex + delta + suggestions.length) % suggestions.length;
    [...els.list.children].forEach((li, i) => li.classList.toggle('active', i === activeIndex));
    els.list.children[activeIndex].scrollIntoView({block: 'nearest'});
  }

  function choose(index) {
    const picked = suggestions[index];
    if (!picked) return;
    els.target.value = picked.name;
    // A new output invalidates the old choice map: its items may not appear.
    state = {target: picked.id, qty: state.qty, choices: {}};
    closeList();
    render();
  }

  els.target.addEventListener('input', () => openList(els.target.value));
  els.target.addEventListener('focus', () => openList(els.target.value));
  els.target.addEventListener('blur', closeList);
  els.target.addEventListener('keydown', (e) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); highlight(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); highlight(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); choose(activeIndex >= 0 ? activeIndex : 0); }
    else if (e.key === 'Escape') closeList();
  });

  els.qty.addEventListener('change', () => {
    state.qty = clampQty(els.qty.value);
    els.qty.value = state.qty;
    render();
  });

  // -- rendering ------------------------------------------------------------

  function table(rows, headers) {
    if (!rows.length) return '<p class="empty">None.</p>';
    const head = '<thead><tr><th>' + headers.join('</th><th>') + '</th></tr></thead>';
    const body = rows.map((r) => '<tr><td>' + r[0] + '</td><td>' + r[1] + '</td></tr>').join('');
    return '<table>' + head + '<tbody>' + body + '</tbody></table>';
  }

  // href is interpolated unescaped: item/recipe ids are always [a-z0-9_] and
  // categories come from a closed set (see itemHref), so neither can ever
  // contain a quote or angle bracket. Only the display name, which comes
  // from free-text game data, needs escaping.
  function nameCell(id, suffix) {
    const href = targetHref(data, id);
    const name = escapeHTML(displayName(data, id));
    const label = href ? '<a href="' + href + '">' + name + '</a>' : name;
    return suffix ? label + '<span class="leafkind">' + suffix + '</span>' : label;
  }

  function byName(a, b) {
    return displayName(data, a) < displayName(data, b) ? -1
      : displayName(data, a) > displayName(data, b) ? 1 : 0;
  }

  function render() {
    if (!data) return;

    const url = encodeState(data, producers, state);
    window.history.replaceState(null, '', url ? '?' + url : window.location.pathname);

    if (!state.target) {
      els.status.hidden = false;
      els.status.textContent = 'Pick an output to see its bill of materials.';
      els.result.hidden = true;
      return;
    }

    const isTarget = hasOwn(data.targets, state.target);
    if (!isTarget && isTerminalItem(data, producers, state.target)) {
      els.status.hidden = false;
      els.status.textContent = leafKind(data, state.target) === 'ore'
        ? displayName(data, state.target) +
          ' is a raw material — the explorer stops at ores and raw materials, ' +
          'the same place the static build-cost pages stop, so there is nothing to expand.'
        : displayName(data, state.target) +
          ' — no recipe produces this; it is a drop.';
      els.result.hidden = true;
      return;
    }

    els.status.hidden = true;
    els.result.hidden = false;

    const graph = buildGraph(data, producers, state.target, state.choices);
    const ranks = rankNodes(graph);
    const columns = orderColumns(graph, ranks);
    const totals = rollUp(graph, ranks, state.qty);
    const node = graph.nodes.get(state.target);

    els.recipe.textContent = node.recipeId
      ? node.recipeId
      : displayName(data, state.target) + ' (' + node.kind + ' build materials)';
    els.meta.textContent = columns.length + ' tiers, ' + graph.nodes.size + ' items';

    // Base materials: every leaf, by total demand.
    const leaves = [...graph.nodes.values()].filter((n) => n.leaf).map((n) => n.id).sort(byName);
    els.base.innerHTML = table(
      leaves.map((id) => [
        nameCell(id, leafKind(data, id) === 'drop' ? 'drop' : ''),
        (totals.demand.get(id) || 0).toLocaleString(),
      ]),
      ['Material', 'Qty']);

    // Direct inputs: the target's own inputs, scaled by its batch count.
    const runs = totals.batches.get(state.target) || 1;
    const direct = [...node.inputs].sort((a, b) => byName(a.id, b.id));
    els.directSub.textContent = node.recipeId ? '(' + node.recipeId + ')' : '';
    els.direct.innerHTML = table(
      direct.map((i) => [nameCell(i.id, ''), (i.qty * runs).toLocaleString()]),
      ['Input', 'Qty']);

    // Surplus: only when the batch rounding over-produced something.
    const spare = [...totals.surplus.keys()].sort(byName);
    els.surplusSection.hidden = spare.length === 0;
    els.surplus.innerHTML = table(
      spare.map((id) => [nameCell(id, ''), totals.surplus.get(id).toLocaleString()]),
      ['Item', 'Qty']);

    renderChart(els.chart, data, producers, graph, ranks, columns, totals, onChoice);
  }

  function onChoice(itemId, recipeId) {
    state.choices = Object.assign({}, state.choices, {[itemId]: recipeId});
    render();
  }
}

// escapeHTML makes a value safe to interpolate into innerHTML. Item names come
// from game data, not user input, but the tables build markup as strings.
function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({'&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'}[c]));
}

// renderChart is defined in the SVG section (Task 8). Until then it draws
// nothing so the page works with tables alone.
function renderChart() {}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initExplorer);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes, topoOrder, rollUp,
    orderColumns, layout, selectableOutputs, clampQty, hasOwn, encodeState, decodeState,
    itemHref, leafKind, escapeHTML,
    QTY_MIN, QTY_MAX,
  };
}
