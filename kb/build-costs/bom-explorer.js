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
        id, kind: target.t, recipeId: null, yield: 1, inputs, dropped: [], leaf: false, cycle: false,
      });
      const next = new Set(stack).add(id);
      for (const input of inputs) visit(input.id, next);
      return;
    }

    const recipeId = activeRecipeId(data, producers, choices, id);
    if (recipeId === null) {
      nodes.set(id, {
        id, kind: 'item', recipeId: null, yield: 1, inputs: [], dropped: [],
        leaf: true, cycle: false,
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
    // dropped names the omitted inputs: the craft script needs them to explain
    // why the item cannot be scheduled rather than emitting a doomed command.
    const next = new Set(stack).add(id);
    const inputs = [];
    const dropped = [];
    let cycle = false;
    for (const [itemId, qty] of data.recipes[recipeId].i) {
      if (next.has(itemId)) {
        cycle = true;
        dropped.push(itemId);
        continue;
      }
      inputs.push({id: itemId, qty});
    }
    nodes.set(id, {
      id, kind: 'item', recipeId, yield: yieldOf(data, recipeId, id), inputs, dropped,
      leaf: false, cycle,
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

// baseMaterialsMap returns the flattened base materials as a plain
// {itemId: qty} object — the same rows the base-materials table renders, in
// machine-readable form.
//
// Keys are item ids, not display names: ids are what the rest of the toolchain
// (crafting.db, the build-cost pages, the game's own commands) takes, and they
// are stable where names are free-text game data. Insertion is sorted because
// JSON.stringify emits string keys in insertion order, so a re-render of the
// same build produces a byte-identical blob and two builds diff cleanly.
function baseMaterialsMap(graph, totals) {
  const out = {};
  const leaves = [...graph.nodes.values()].filter((n) => n.leaf).map((n) => n.id).sort();
  for (const id of leaves) out[id] = totals.demand.get(id) || 0;
  return out;
}

// baseMaterialsJSON renders the map as the pretty-printed blob shown at the
// bottom of the page. Wrapped in a "materials" key so the document has room to
// grow another field without breaking anything already parsing it.
function baseMaterialsJSON(graph, totals) {
  return JSON.stringify({materials: baseMaterialsMap(graph, totals)}, null, 2);
}

// craftWaves groups the build's craft steps into waves that can be launched
// together, indexed by wave number.
//
// The wave IS the node's rank, with no further analysis: rankNodes computes
// rank(x) = max(rank(input) + 1) over the inputs, so the greatest input rank is
// exactly rank(x) - 1. Every item therefore has an input in the immediately
// preceding wave (it can never start earlier) and none in its own or a later
// one (a wave is safe to launch all at once). That is an identity of the
// ranking, not a property of any particular recipe data.
//
// Leaves carry no command — they are the raw materials you must already hold.
// Neither does a ship or facility target: no recipe in the game produces one,
// so its final assembly is out of band (see craftScriptText).
//
// qty is the exact demand, NOT runs * yield. The server takes an output
// quantity and rounds it up to whole production runs itself, so asking for 320
// and asking for 321 both run 107 batches of a yield-3 recipe. Printing the
// need is honest about what the build requires; made records what the rounding
// will actually produce so the renderer can disclose the surplus.
function craftWaves(graph, ranks, totals) {
  const waves = [[]];

  for (const [id, node] of graph.nodes) {
    if (node.leaf || !node.recipeId) continue;
    const rank = ranks.get(id);
    while (waves.length <= rank) waves.push([]);
    // The || 0 is load-bearing, not defensive: rollUp skips any node with no
    // inputs, so a cycle-cut node has no batches entry at all, and made would
    // be NaN without it. Removing it silently corrupts that one case.
    const runs = totals.batches.get(id) || 0;
    waves[rank].push({
      id,
      recipeId: node.recipeId,
      qty: totals.demand.get(id) || 0,
      yield: node.yield,
      runs,
      made: runs * node.yield,
      cycle: node.cycle,
      dropped: node.dropped || [],
    });
  }

  // Sorted by id, not by name: ids are stable where free-text names are not,
  // so the same build renders byte-identically run to run and two builds diff
  // cleanly.
  for (const wave of waves) {
    wave.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));
  }
  return waves;
}

const SURPLUS_COL = 44; // column the surplus comment aligns to, when it fits

// craftScriptText renders a wave model as a runnable list of craft commands.
//
// The header states the two things that most often make a first attempt fail —
// the one-mutation-per-tick rate limit, and that inputs are escrowed from
// station storage rather than cargo — so the script stays correct pasted
// somewhere with no access to the rest of the page.
//
// meta is {target, kind, qty, baseMaterials}: the target id, whether it is an
// item, ship or facility, the build quantity, and the baseMaterialsMap output
// used for the prerequisites list.
function craftScriptText(data, waves, meta) {
  const out = [];
  const name = displayName(data, meta.target);

  out.push('# Build: ' + name + ' x' + meta.qty);
  out.push('# Generated by the Spacemolt KB Bill of Materials explorer.');
  out.push('#');
  out.push('# Crafting is rate limited to 1 mutation per tick (10s) and jobs run');
  out.push('# asynchronously over later ticks. Everything inside a wave is');
  out.push('# independent: launch it together or shard it across agents, then let');
  out.push('# the whole wave finish before starting the next.');
  out.push('#');
  out.push('# Must be docked at a base with crafting + storage service.');
  out.push('# Inputs are escrowed from STATION STORAGE, not cargo - deposit them first.');

  const base = Object.keys(meta.baseMaterials || {});
  if (base.length) {
    out.push('#');
    out.push('# Raw materials required first (' + base.length + '):');
    for (const id of base) out.push('#   ' + id + ' ' + meta.baseMaterials[id]);
  }

  // Wave 0 holds only the raw materials, which carry no command — except for a
  // node whose recipe lost an input to cycle-breaking, which ranks 0 while
  // still needing a recipe. Say so rather than emitting a command that cannot
  // succeed, or silently dropping a step the reader still has to solve.
  for (const job of waves[0] || []) {
    out.push('#');
    out.push('# ' + displayName(data, job.id) + ' cannot be scheduled: recipe ' +
      job.recipeId + ' needs ' + job.dropped.join(', ') +
      ', which was dropped to break a cycle. Pick another recipe for it.');
  }

  for (let w = 1; w < waves.length; w++) {
    const wave = waves[w];
    if (!wave.length) continue;
    out.push('');
    out.push('# wave ' + w + ' - ' + wave.length + (wave.length === 1 ? ' craft' : ' crafts'));
    for (const job of wave) out.push(craftLine(job));

    // The same wave again as bulk payloads. Both forms are emitted because
    // consumers differ: several agents sharding a wave between them want the
    // plain lines, a single agent queueing it all wants one action.
    const chunks = bulkChunks(wave);
    chunks.forEach((chunk, i) => {
      out.push(chunks.length > 1 ? '# bulk ' + (i + 1) + '/' + chunks.length + ':' : '# bulk:');
      out.push('craft jobs=' + JSON.stringify(
        chunk.map((job) => ({recipe_id: job.recipeId, quantity: job.qty}))));
    });
  }

  // No recipe in the game produces a ship or a facility - 0 of 335 ships and 0
  // of 2650 facilities appear as a recipe output - so the final assembly is
  // always out of band. Naming the actual command keeps the script honest
  // about where it stops.
  if (meta.kind === 'ship') {
    out.push('');
    out.push('# Final: ' + name + ' is a ship - no craft recipe produces it.');
    out.push('# Dock at a shipyard and:');
    out.push('#   commission_ship ' + meta.target + ' provide_materials=true');
    out.push('# That commission enters a sourcing state; feed it one item type at a');
    out.push('# time with supply_commission <commission_id> <item_id> <quantity>.');
  } else if (meta.kind === 'facility') {
    out.push('');
    out.push('# Final: ' + name + ' is a facility - no craft recipe produces it.');
    out.push('# With the materials above in station storage:');
    out.push('#   facility build ' + meta.target);
    out.push('#   (or: facility faction_build ' + meta.target + ' to build it for your faction)');
  }

  return out.join('\n') + '\n';
}

// MaxCraftBulkJobs in the game client (pkg/game/crafting.go): the server
// accepts at most this many jobs in one bulk craft action.
const BULK_MAX = 50;

// bulkChunks splits a wave into payloads the server will accept. Bulk mode is
// the whole point of grouping by wave: at 1 mutation per tick, issuing a
// 29-craft wave as separate commands costs nearly five minutes, where one bulk
// action costs one tick.
function bulkChunks(wave) {
  const chunks = [];
  for (let i = 0; i < wave.length; i += BULK_MAX) chunks.push(wave.slice(i, i + BULK_MAX));
  return chunks;
}

// craftLine renders one command, with the surplus disclosed inline when whole
// production runs make more than the build actually needs.
function craftLine(job) {
  const cmd = 'craft ' + job.recipeId + ' ' + job.qty;
  if (job.made <= job.qty) return cmd;
  const note = '# yield ' + job.yield + ' -> ' + job.runs + ' runs makes ' + job.made +
    ', ' + (job.made - job.qty) + ' spare';
  return cmd.padEnd(SURPLUS_COL) + note;
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

const ROUTE_PREFIX = '__route:';
const ROUTE_SLOT_H = 8;
const ROUTE_GAP = 4; // waypoints pack tighter than boxes — see gapBetween.

// isRoutingNode reports whether an id is an invisible edge waypoint rather
// than a real item. Real item ids are [a-z0-9_] throughout the crafting
// data, so the '__route:' prefix cannot collide with one.
function isRoutingNode(id) {
  return typeof id === 'string' && id.startsWith(ROUTE_PREFIX);
}

// isDummyId is the check layout uses to decide whether an id is a waypoint.
// dummies (from withRoutingNodes) is the primary source of truth; isRoutingNode
// is the fallback, so a call site that only has a graph and no dummies Set
// (every pre-routing call, including the existing tests) still behaves
// correctly on a graph that happens to contain no waypoints at all.
function isDummyId(id, dummies) {
  return (dummies && dummies.has(id)) || isRoutingNode(id);
}

// withRoutingNodes returns an augmented COPY of the graph in which every edge
// spanning more than one column is replaced by a chain through one invisible
// waypoint per intervening column.
//
// Without this, a long edge runs flat at its source's height until the
// gutter just before its consumer, passing through any box in between — and
// since edges paint before boxes, it renders behind them. Giving the edge a
// slot in each column it crosses lets the packer route it between boxes
// instead.
//
// The original graph is never mutated: rollUp runs on it and must not see a
// waypoint, so this always builds a fresh Map of nodes.
function withRoutingNodes(graph, ranks) {
  const nodes = new Map();
  const outRanks = new Map(ranks);
  const dummies = new Set();

  for (const [id, node] of graph.nodes) {
    nodes.set(id, Object.assign({}, node, {inputs: []}));
  }

  for (const [id, node] of graph.nodes) {
    for (const input of node.inputs) {
      const from = ranks.get(input.id);
      const to = ranks.get(id);
      if (to - from <= 1) {
        nodes.get(id).inputs.push({id: input.id, qty: input.qty});
        continue;
      }
      // Chain: input.id -> w(from+1) -> ... -> w(to-1) -> id, one waypoint
      // per intervening column, each carrying the original quantity forward.
      let prev = input.id;
      for (let col = from + 1; col < to; col++) {
        const wid = ROUTE_PREFIX + input.id + '>' + id + ':' + col;
        if (!nodes.has(wid)) {
          nodes.set(wid, {
            id: wid, kind: 'route', recipeId: null, yield: 1,
            inputs: [], dropped: [], leaf: false, cycle: false,
          });
          outRanks.set(wid, col);
          dummies.add(wid);
        }
        nodes.get(wid).inputs.push({id: prev, qty: input.qty});
        prev = wid;
      }
      nodes.get(id).inputs.push({id: prev, qty: input.qty});
    }
  }

  return {graph: {targetId: graph.targetId, nodes}, ranks: outRanks, dummies};
}

// curvePath renders a polyline as a smooth cubic path. Control handles are
// horizontal, so every segment leaves and enters its endpoints going
// left-to-right, which preserves the reading direction the layout encodes.
function curvePath(points) {
  if (!points.length) return '';
  let d = 'M' + points[0][0] + ',' + points[0][1];
  for (let i = 1; i < points.length; i++) {
    const [x0, y0] = points[i - 1];
    const [x1, y1] = points[i];
    const dx = (x1 - x0) / 2;
    d += ' C' + (x0 + dx) + ',' + y0 + ' ' + (x1 - dx) + ',' + y1 + ' ' + x1 + ',' + y1;
  }
  return d;
}

// boxHeight returns a node's height: a routing waypoint gets a slim slot,
// otherwise taller when it carries a recipe selector.
function boxHeight(producers, id, dummies) {
  if (isDummyId(id, dummies)) return ROUTE_SLOT_H;
  const ids = producers ? producers.get(id) : null;
  return ids && ids.length > 1 ? BOX_H_SEL : BOX_H;
}

// gapBetween returns the vertical gap to leave below `aId` before `bId` in the
// same column. Waypoints pack tighter than real boxes — see ROUTE_GAP — so
// the gap shrinks whenever either neighbour is a routing node.
function gapBetween(aId, bId, dummies) {
  return (isDummyId(aId, dummies) || isDummyId(bId, dummies)) ? ROUTE_GAP : ROW_GAP;
}

// columnHeight sums a column's box heights plus the gaps between them.
function columnHeight(column, producers, dummies) {
  let h = 0;
  column.forEach((id, i) => {
    if (i > 0) h += gapBetween(column[i - 1], id, dummies);
    h += boxHeight(producers, id, dummies);
  });
  return h;
}

// layout converts ordered columns into drawable geometry. Columns are placed
// left to right by rank, so a base ore consumed directly by the output spans
// the full width — expected, not a defect. Each column is vertically centred
// against the tallest so short columns do not hug the top.
//
// producers is optional; pass it to size boxes that carry a recipe selector.
// dummies is the Set returned by withRoutingNodes; pass it when graph has
// been routed so waypoints get their slim slot instead of a full box height.
function layout(graph, columns, producers, dummies) {
  const heights = columns.map((column) => columnHeight(column, producers, dummies));
  const tallest = Math.max(0, ...heights);

  const boxes = new Map();
  columns.forEach((column, col) => {
    let y = MARGIN + (tallest - heights[col]) / 2;
    column.forEach((id, row) => {
      const h = boxHeight(producers, id, dummies);
      boxes.set(id, {x: MARGIN + col * (BOX_W + COL_GAP), y, w: BOX_W, h, col, row});
      const next = column[row + 1];
      y += h + (next ? gapBetween(id, next, dummies) : 0);
    });
  });

  const width = MARGIN * 2 + columns.length * BOX_W + Math.max(0, columns.length - 1) * COL_GAP;
  const height = MARGIN * 2 + tallest;

  // One edge per ORIGINAL source -> consumer pair, even when routing broke it
  // into a chain of adjacent-column hops: walk each real consumer's inputs,
  // and whenever an input is a waypoint, follow the chain back to the real
  // source, collecting each waypoint's slot centre as a point on the path.
  // This is what keeps a single label per edge (see renderChart) instead of
  // one per segment.
  const edges = [];
  for (const node of graph.nodes.values()) {
    if (isDummyId(node.id, dummies)) continue;
    const to = boxes.get(node.id);
    if (!to) continue;
    for (const input of node.inputs) {
      const waypoints = [];
      let fromId = input.id;
      while (isDummyId(fromId, dummies)) {
        waypoints.unshift(fromId);
        fromId = graph.nodes.get(fromId).inputs[0].id;
      }
      const from = boxes.get(fromId);
      if (!from) continue;

      const points = [[from.x + from.w, from.y + from.h / 2]];
      for (const wid of waypoints) {
        const wbox = boxes.get(wid);
        points.push([wbox.x + wbox.w / 2, wbox.y + wbox.h / 2]);
      }
      points.push([to.x, to.y + to.h / 2]);

      edges.push({from: fromId, to: node.id, qty: input.qty, points});
    }
  }

  return {width, height, boxes, edges};
}

const LABEL_LEAD = 18;    // distance in from the source along the first segment
const LABEL_STAGGER = 10; // extra lead per edge when 3+ share a source
const LABEL_GAP = 5;      // vertical clearance between the line and its label

// labelLayout computes each edge's label anchor point.
//
// Every edge leaving a box starts at the same point — the box's right-edge
// midpoint — so a fixed offset stacks every label from a shared source in
// nearly the same place (the reported bug: two edges out of one ore box, two
// labels landing on top of each other). A source with a single outgoing
// edge keeps today's placement exactly as it was: a fixed distance in, just
// above the line. A source with several instead keeps each edge's label on
// the side its line actually departs toward — above one that departs upward
// (dy < 0), below one that departs downward (dy > 0) — and, since the
// above/below split alone cannot separate three or more edges, also
// staggers how far each label sits along its segment.
//
// dy comes from points[1] - points[0], the edge's ACTUAL departure
// direction: after routing, points[1] is the nearest waypoint rather than
// the final consumer, which is exactly the direction the line leaves the
// box, not the direction it eventually arrives from.
function labelLayout(edges) {
  const bySource = new Map();
  edges.forEach((edge, i) => {
    if (!bySource.has(edge.from)) bySource.set(edge.from, []);
    bySource.get(edge.from).push(i);
  });

  return edges.map((edge, i) => {
    const [x0, y0] = edge.points[0];
    const [x1, y1] = edge.points[1];
    const dx = x1 - x0;
    const dy = y1 - y0;
    const len = Math.hypot(dx, dy) || 1;

    const group = bySource.get(edge.from);
    const shared = group.length > 1;
    const idx = group.indexOf(i);

    // Clamped so the label never runs past its own segment.
    const lead = shared ? LABEL_LEAD + idx * LABEL_STAGGER : LABEL_LEAD;
    const t = Math.min(lead, len - 2) / len;
    const side = shared && dy > 0 ? 1 : -1;

    return {from: edge.from, to: edge.to, x: x0 + dx * t, y: y0 + dy * t + side * LABEL_GAP};
  });
}

// edgesByNode maps each node id that appears as an edge endpoint to every
// edge touching it, whether as source or consumer. Pure, so the hover/focus
// highlight (renderChart, DOM-only) can be built and tested as "which edges
// touch this node" — a property of the edge list alone — without any DOM.
function edgesByNode(edges) {
  const map = new Map();
  for (const edge of edges) {
    if (!map.has(edge.from)) map.set(edge.from, []);
    map.get(edge.from).push(edge);
    if (!map.has(edge.to)) map.set(edge.to, []);
    map.get(edge.to).push(edge);
  }
  return map;
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
const COPY_LABEL_MS = 1500; // how long the copy button shows its outcome

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
    reset: document.getElementById('reset-choices'),
    chart: document.getElementById('chart'),
    base: document.getElementById('table-base'),
    direct: document.getElementById('table-direct'),
    directSub: document.getElementById('direct-sub'),
    surplus: document.getElementById('table-surplus'),
    surplusSection: document.getElementById('surplus-section'),
    json: document.getElementById('json-blob'),
    copy: document.getElementById('copy-json'),
    script: document.getElementById('craft-script'),
    copyScript: document.getElementById('copy-script'),
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

  els.reset.addEventListener('click', () => {
    state.choices = {};
    render();
  });

  // The blobs are selectable text, so copying is a convenience, not the only
  // route to them: clipboard access is permission-gated and absent on insecure
  // origins, so a failure says so rather than pretending it worked.
  function wireCopy(button, source) {
    button.addEventListener('click', () => {
      const done = (msg) => {
        button.textContent = msg;
        setTimeout(() => { button.textContent = 'copy'; }, COPY_LABEL_MS);
      };
      if (!navigator.clipboard) {
        done('select and copy');
        return;
      }
      navigator.clipboard.writeText(source.textContent)
        .then(() => done('copied'))
        .catch(() => done('copy failed'));
    });
  }
  wireCopy(els.copy, els.json);
  wireCopy(els.copyScript, els.script);

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
    // rollUp runs on the ORIGINAL graph, before routing, so demand and batch
    // counts are never computed against an invisible waypoint.
    const totals = rollUp(graph, ranks, state.qty);
    const routed = withRoutingNodes(graph, ranks);
    const columns = orderColumns(routed.graph, routed.ranks);
    const node = graph.nodes.get(state.target);

    els.recipe.textContent = node.recipeId
      ? node.recipeId
      : displayName(data, state.target) + ' (' + node.kind + ' build materials)';
    els.meta.textContent = columns.length + ' tiers, ' + graph.nodes.size + ' items';
    // Hidden rather than merely disabled when there is nothing to reset, so
    // it does not invite a no-op click.
    els.reset.hidden = Object.keys(state.choices).length === 0;

    // Base materials: every leaf, by total demand.
    const leaves = [...graph.nodes.values()].filter((n) => n.leaf).map((n) => n.id).sort(byName);
    els.base.innerHTML = table(
      leaves.map((id) => [
        nameCell(id, leafKind(data, id) === 'drop' ? 'drop' : ''),
        (totals.demand.get(id) || 0).toLocaleString(),
      ]),
      ['Material', 'Qty']);

    // The same leaf totals as the table above, machine-readable. textContent,
    // never innerHTML: this is a <pre> and the blob must render as literal
    // JSON.
    els.json.textContent = baseMaterialsJSON(graph, totals);

    // The same build as commands. kind decides how the script closes: a ship
    // or facility has no recipe of its own, so it hands off to the shipyard or
    // the facility build action instead of ending on a craft line.
    const kind = isTarget ? data.targets[state.target].t : 'item';
    els.script.textContent = craftScriptText(data, craftWaves(graph, ranks, totals), {
      target: state.target,
      kind,
      qty: state.qty,
      baseMaterials: baseMaterialsMap(graph, totals),
    });

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

    renderChart(els.chart, data, producers, routed.graph, columns, totals, onChoice, routed.dummies);
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

const SVG_NS = 'http://www.w3.org/2000/svg';
const XHTML_NS = 'http://www.w3.org/1999/xhtml';

function svgEl(name, attrs) {
  const el = document.createElementNS(SVG_NS, name);
  for (const [k, v] of Object.entries(attrs || {})) el.setAttribute(k, String(v));
  return el;
}

// edgeHue maps a source item id to a stable hue. Deterministic, so the same
// item keeps its colour across re-renders and between page loads, and needs
// no fixed palette that could run out as the item list grows.
function edgeHue(id) {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = (h * 31 + id.charCodeAt(i)) % 360;
  return h;
}

// renderChart draws the layered graph. Columns run left to right by rank, so
// every arrow points rightwards and a base ore feeding the output directly
// spans the full width.
function renderChart(container, data, producers, graph, columns, totals, onChoice, dummies) {
  container.innerHTML = '';
  const {width, height, boxes, edges} = layout(graph, columns, producers, dummies);

  // role="group", NOT role="img": this SVG contains the feature's only
  // interactive controls (the recipe <select> inside each foreignObject), and
  // role="img" tells assistive tech to treat the whole subtree as one flat
  // image, which can make those selects unreachable.
  const svg = svgEl('svg', {
    width, height, viewBox: '0 0 ' + width + ' ' + height,
    xmlns: SVG_NS, role: 'group', 'aria-label': 'Production chain',
  });

  const defs = svgEl('defs');
  // One arrowhead marker per distinct hue actually used this render — a
  // single shared marker cannot take two fills, and the arrowhead must match
  // the line it caps. renderChart clears the container every render, so no
  // marker from a previous chart outlives this one.
  const hues = new Set(edges.map((e) => edgeHue(e.from)));
  for (const hue of hues) {
    const marker = svgEl('marker', {
      id: 'bx-arrow-' + hue, viewBox: '0 0 10 10', refX: 9, refY: 5,
      markerWidth: 6, markerHeight: 6, orient: 'auto-start-reverse',
    });
    marker.append(svgEl('path', {d: 'M 0 0 L 10 5 L 0 10 z', fill: 'hsl(' + hue + ' var(--edge-s) var(--edge-l))'}));
    defs.append(marker);
  }
  svg.append(defs);

  // Edges first so boxes paint over them.
  //
  // Label position comes from labelLayout, not a fixed offset per edge: a
  // fixed offset stacks every label from a shared source in nearly the same
  // spot, since every edge leaving a box starts at the same point. See
  // labelLayout's own comment for the above/below/staggered rule.
  //
  // Each edge's <path> is also filed under edgesByNode so hovering or
  // focusing a node box (below) can find exactly the paths touching it,
  // without a DOM query on every hover.
  const labelPositions = labelLayout(edges);
  const touchingByNode = edgesByNode(edges); // node id -> the edges touching it
  const edgePathEls = new Map(); // edge -> its <path> element
  edges.forEach((edge, i) => {
    const hue = edgeHue(edge.from);
    const edgePath = svgEl('path', {
      d: curvePath(edge.points),
      fill: 'none', stroke: 'hsl(' + hue + ' var(--edge-s) var(--edge-l))', 'stroke-width': 1.5,
      'marker-end': 'url(#bx-arrow-' + hue + ')',
    });
    svg.append(edgePath);
    edgePathEls.set(edge, edgePath);

    const pos = labelPositions[i];
    const label = svgEl('text', {
      x: pos.x, y: pos.y, 'text-anchor': 'start',
      fill: 'var(--dim)', 'font-size': 11, 'font-family': 'system-ui,sans-serif',
    });
    // edge.qty is the PER-BATCH recipe input quantity; scale by the consuming
    // node's batch count so this label shows the actual total consumed,
    // matching the demand-scaled totals shown in the boxes rather than the
    // bare recipe stoichiometry.
    const labelQty = edge.qty * (totals.batches.get(edge.to) || 1);
    label.textContent = labelQty.toLocaleString();
    svg.append(label);
  });

  // Hovering or focusing a node box highlights the edges touching it (as
  // either source or consumer — direct edges only, not the whole upstream
  // chain) and dims the rest, plus dims unrelated boxes a little more
  // gently. Dimming is opacity-only, via a class, never a stroke-colour
  // change: edges are hsl() built from theme custom properties and must
  // stay theme-correct in both.
  const boxGroups = new Map(); // node id -> its <g>
  function highlight(nodeId) {
    const touching = new Set(touchingByNode.get(nodeId) || []);
    const related = new Set([nodeId]);
    for (const edge of touching) { related.add(edge.from); related.add(edge.to); }
    for (const [edge, el] of edgePathEls) el.classList.toggle('bx-dim', !touching.has(edge));
    for (const [id, g] of boxGroups) g.classList.toggle('bx-box-dim', !related.has(id));
  }
  function clearHighlight() {
    for (const el of edgePathEls.values()) el.classList.remove('bx-dim');
    for (const g of boxGroups.values()) g.classList.remove('bx-box-dim');
  }

  for (const [id, box] of boxes) {
    if (isRoutingNode(id)) continue; // waypoints occupy a slot but draw nothing.
    const node = graph.nodes.get(id);
    const stroke = id === graph.targetId ? 'var(--accent)'
      : node.leaf ? 'var(--muted2)' : 'var(--node-border)';

    // Wrapped in a <g>: the foreignObject below covers the rect entirely, so
    // a hover/focus listener on the rect alone would not fire reliably.
    const nodeGroup = svgEl('g', {class: 'bx-node'});
    nodeGroup.addEventListener('mouseenter', () => highlight(id));
    nodeGroup.addEventListener('mouseleave', clearHighlight);
    // focusin/focusout (not focus/blur) because they bubble: the actually
    // focusable elements are the name link and, on multi-recipe items, the
    // recipe <select>, both descendants of this <g> rather than the <g>
    // itself.
    nodeGroup.addEventListener('focusin', () => highlight(id));
    nodeGroup.addEventListener('focusout', clearHighlight);
    boxGroups.set(id, nodeGroup);
    svg.append(nodeGroup);

    nodeGroup.append(svgEl('rect', {
      x: box.x, y: box.y, width: box.w, height: box.h, rx: 5,
      fill: 'var(--panel2)', stroke, 'stroke-width': id === graph.targetId ? 2 : 1,
    }));

    const fo = svgEl('foreignObject', {x: box.x, y: box.y, width: box.w, height: box.h});
    const div = document.createElementNS(XHTML_NS, 'div');
    div.setAttribute('style',
      'height:100%;box-sizing:border-box;padding:4px 6px;font:12px system-ui,sans-serif;' +
      'color:var(--text);display:flex;flex-direction:column;gap:2px;overflow:hidden');

    const nameEl = document.createElementNS(XHTML_NS, 'div');
    const href = targetHref(data, id);
    if (href) {
      const a = document.createElementNS(XHTML_NS, 'a');
      a.setAttribute('href', href);
      a.setAttribute('style', 'color:var(--link);text-decoration:none');
      a.textContent = displayName(data, id);
      nameEl.append(a);
    } else {
      nameEl.textContent = displayName(data, id);
    }
    nameEl.setAttribute('style', 'font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis');
    div.append(nameEl);

    const qtyEl = document.createElementNS(XHTML_NS, 'div');
    qtyEl.setAttribute('style', 'color:var(--muted);font-size:11px');
    qtyEl.textContent = node.cycle
      ? 'cycle — not expanded'
      : '×' + (totals.demand.get(id) || 0).toLocaleString();
    div.append(qtyEl);

    const recipeIds = producers.get(id);
    if (recipeIds && recipeIds.length > 1) {
      const select = document.createElementNS(XHTML_NS, 'select');
      // Sighted users infer this control's purpose from the item name above
      // it; nothing ties the two together programmatically, so name it.
      select.setAttribute('aria-label', 'Recipe for ' + displayName(data, id));
      select.setAttribute('style',
        'width:100%;font:11px system-ui,sans-serif;background:var(--panel);color:var(--text);' +
        'border:1px solid var(--border);border-radius:3px;padding:1px 2px');
      for (const rid of recipeIds) {
        const option = document.createElementNS(XHTML_NS, 'option');
        option.setAttribute('value', rid);
        if (rid === node.recipeId) option.setAttribute('selected', 'selected');
        option.textContent = rid;
        select.append(option);
      }
      select.addEventListener('change', (e) => onChoice(id, e.target.value));
      div.append(select);
    }

    fo.append(div);
    nodeGroup.append(fo);
  }

  container.append(svg);
}

if (typeof document !== 'undefined') {
  document.addEventListener('DOMContentLoaded', initExplorer);
}

if (typeof module !== 'undefined' && module.exports) {
  module.exports = {
    producersOf, isTerminalItem, activeRecipeId, yieldOf, buildGraph, rankNodes, topoOrder, rollUp,
    baseMaterialsMap, baseMaterialsJSON, craftWaves, craftScriptText, bulkChunks,
    orderColumns, isRoutingNode, withRoutingNodes, curvePath, layout, labelLayout, edgesByNode,
    selectableOutputs, clampQty, hasOwn, encodeState, decodeState,
    itemHref, leafKind, escapeHTML, displayName,
    QTY_MIN, QTY_MAX,
  };
}
