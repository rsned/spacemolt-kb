# Craft Script Output Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third output to the Bill of Materials explorer — the whole build rendered as ordered `craft <recipe_id> <quantity>` commands grouped into waves that can be launched in parallel, plus a bulk `jobs=[...]` payload per wave.

**Architecture:** Two new pure functions in `kb/build-costs/bom-explorer.js` beside the existing `baseMaterialsMap`: `craftWaves` turns the already-computed graph + ranks + roll-up into a structured wave model, and `craftScriptText` renders that model to text. A small additive change to `buildGraph` records which inputs were dropped to break a cycle, so the one unschedulable case can explain itself. Then DOM wiring in `explorer.html` + `initExplorer`. No generator change, no new data file — everything derives from the graph already in memory.

**Tech Stack:** Plain ES2020 JavaScript, no build step, no dependencies. `kb/build-costs/bom-explorer.js` is loaded both as a classic browser script and as a CommonJS module by the tests. Tests are `node --test`.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-08-09-craft-script-design.md`. Read it before starting.
- **Test command is `node --test 'tests/js/*.test.js'`** — quote the glob. The directory form `node --test tests/js/` fails on this machine's Node 22 with `Cannot find module '/home/robert/spacemolt/kb/tests/js'`. That is a pre-existing environment quirk, not something you introduced or should fix.
- **All new functions must be pure** — no `document`, no `window`, no `fetch`, no globals. They go ABOVE the `// DOM layer` divider comment in `bom-explorer.js` (currently around line 630). Everything pure is exported at the bottom for tests.
- **Never mutate the graph** passed in. `rollUp` runs on the same graph and must not see your changes.
- **Item and recipe ids are always `[a-z0-9_]`** throughout the crafting data. Display names are free-text game data and are the only thing needing escaping — but this feature writes into a `<pre>` via `textContent`, so nothing needs HTML escaping.
- **Comment style:** this file explains *why*, not *what*, in full sentences. Match it. A comment that just restates the code will be rejected in review.
- **Commit after every task.** Run the full test suite before each commit.
- Do NOT run `git push` unless the user asks.

---

### Task 1: Record dropped cycle inputs, and build the wave model

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (`buildGraph`, around lines 70-126; new `craftWaves` after `baseMaterialsJSON`, around line 228; exports at the bottom)
- Test: `tests/js/bom-explorer.test.js` (append)

**Interfaces:**
- Consumes: existing `buildGraph(data, producers, targetId, choices)`, `rankNodes(graph)`, `rollUp(graph, ranks, quantity)`.
- Produces:
  - Every node object gains `dropped: string[]` — the input item ids omitted to break a cycle, empty for normal nodes.
  - `craftWaves(graph, ranks, totals) -> Array<Array<{id, recipeId, qty, yield, runs, made, cycle, dropped}>>`, indexed by wave number. Task 2 onward consumes this.

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/bom-explorer.test.js`:

```js
test('buildGraph records which inputs it dropped to break a cycle', () => {
  const data = fixture();
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});

  // Naming the dropped input is what lets the craft script explain why this
  // item cannot be scheduled, instead of emitting a command that would fail.
  assert.deepStrictEqual(g.nodes.get('steel_plate').dropped, ['frame']);
  assert.deepStrictEqual(g.nodes.get('widget').dropped, [], 'normal nodes drop nothing');
});

test('craftWaves indexes craft steps by rank and excludes leaves', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const waves = bx.craftWaves(g, ranks, bx.rollUp(g, ranks, 1));

  assert.deepStrictEqual(waves[0], [], 'wave 0 is raw materials, which carry no command');
  assert.deepStrictEqual(waves[1].map((j) => j.id), ['steel_plate']);
  assert.deepStrictEqual(waves[2].map((j) => j.id), ['frame']);
  assert.deepStrictEqual(waves[3].map((j) => j.id), ['widget']);
  // hauler is a ship sink at rank 4: it has no recipe, so it gets no command
  // and no trailing empty wave.
  assert.strictEqual(waves.length, 4);
});

test('craftWaves carries the exact demand, not the batched amount', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  // smelt_steel yields 2; asking for 3 runs 2 batches and makes 4.
  const g = bx.buildGraph(data, producers, 'steel_plate', {});
  const ranks = bx.rankNodes(g);
  const waves = bx.craftWaves(g, ranks, bx.rollUp(g, ranks, 3));

  const job = waves[1][0];
  assert.strictEqual(job.recipeId, 'smelt_steel');
  assert.strictEqual(job.qty, 3, 'the command asks for what is needed');
  assert.strictEqual(job.yield, 2);
  assert.strictEqual(job.runs, 2);
  assert.strictEqual(job.made, 4, 'the server rounds up to whole runs');
});

test('craftWaves puts a cycle-cut node in wave 0 flagged, not silently at rank 0', () => {
  const data = fixture();
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'widget', {steel_plate: 'cycle_steel'});
  const ranks = bx.rankNodes(g);
  const waves = bx.craftWaves(g, ranks, bx.rollUp(g, ranks, 1));

  // It lost its only input, so it ranks 0 alongside the ores while still
  // needing a recipe. It must be visible, not dropped on the floor.
  const job = waves[0].find((j) => j.id === 'steel_plate');
  assert.ok(job, 'the unschedulable node still appears');
  assert.strictEqual(job.cycle, true);
  assert.deepStrictEqual(job.dropped, ['frame']);
});

test('craftWaves sorts each wave by id so regeneration is byte-identical', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const waves = bx.craftWaves(g, ranks, bx.rollUp(g, ranks, 1));

  for (const wave of waves) {
    const ids = wave.map((j) => j.id);
    assert.deepStrictEqual(ids, [...ids].sort());
  }
});

test('no craft step depends on one in its own or a later wave', () => {
  // The load-bearing invariant: this is what makes "launch a wave in
  // parallel" safe. Asserted against the graph's own edges, not against a
  // re-derivation of the ranks that produced the waves.
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const waves = bx.craftWaves(g, ranks, bx.rollUp(g, ranks, 1));

  const waveOf = new Map();
  waves.forEach((wave, w) => wave.forEach((job) => waveOf.set(job.id, w)));

  for (const [id, w] of waveOf) {
    for (const input of g.nodes.get(id).inputs) {
      const iw = waveOf.get(input.id);
      if (iw === undefined) continue; // a raw material, available from the start
      assert.ok(iw < w, `${input.id} (wave ${iw}) must precede ${id} (wave ${w})`);
    }
  }
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `node --test 'tests/js/*.test.js'`
Expected: 6 failures, all `bx.craftWaves is not a function` except the first, which fails on `dropped` being `undefined`. The 53 existing tests still pass.

- [ ] **Step 3: Add the `dropped` field to `buildGraph`**

In `kb/build-costs/bom-explorer.js`, `buildGraph` currently builds its input list like this:

```js
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
```

Replace with:

```js
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
```

Add `dropped: []` to the other two `nodes.set` calls in `buildGraph` so every node has the field and no consumer needs an existence check. The target branch:

```js
      nodes.set(id, {
        id, kind: target.t, recipeId: null, yield: 1, inputs, dropped: [], leaf: false, cycle: false,
      });
```

and the terminal branch:

```js
      nodes.set(id, {
        id, kind: 'item', recipeId: null, yield: 1, inputs: [], dropped: [],
        leaf: true, cycle: false,
      });
```

Extend `buildGraph`'s existing comment about dropping cycle-closing edges with one sentence:

```js
    // ... existing paragraph ends "...so the renderer can say so."
    // dropped names the omitted inputs: the craft script needs them to explain
    // why the item cannot be scheduled rather than emitting a doomed command.
```

Also add `dropped: []` to the routing-node literal in `withRoutingNodes`, for the same uniformity reason:

```js
          nodes.set(wid, {
            id: wid, kind: 'route', recipeId: null, yield: 1,
            inputs: [], dropped: [], leaf: false, cycle: false,
          });
```

- [ ] **Step 4: Write `craftWaves`**

Insert immediately after `baseMaterialsJSON` and before the `// Layout` divider:

```js
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
```

- [ ] **Step 5: Export it**

In the `module.exports` block at the bottom, add `craftWaves` to the line that already carries `baseMaterialsMap, baseMaterialsJSON,`:

```js
    baseMaterialsMap, baseMaterialsJSON, craftWaves,
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `node --test 'tests/js/*.test.js'`
Expected: PASS, 59 tests.

- [ ] **Step 7: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): model the build as parallelizable craft waves

craftWaves groups craft steps by rank. The wave is the rank with no
further analysis: rank(x) = max(rank(input)+1) means every item has an
input in the preceding wave and none in its own, so a wave is safe to
launch all at once.

buildGraph now also records which inputs it dropped to break a cycle,
so the one unschedulable case can explain itself rather than emitting a
command that would fail."
```

---

### Task 2: Render the script header, prerequisites and wave lines

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (new `craftScriptText` after `craftWaves`; export `displayName`)
- Test: `tests/js/bom-explorer.test.js` (append)

**Interfaces:**
- Consumes: `craftWaves` output from Task 1; `baseMaterialsMap(graph, totals)`; the existing `displayName(data, id)`.
- Produces: `craftScriptText(data, waves, meta) -> string`, where `meta` is `{target, kind, qty, baseMaterials}`. `kind` is `'item'`, `'ship'` or `'facility'`. Tasks 3 and 4 extend this same function.

Note on `displayName`: it currently sits below the `// DOM layer` divider but touches no DOM — same as `itemHref` and `leafKind`, which are already exported and tested. Export it; do not move or duplicate it. Function declarations hoist, so `craftScriptText` may call it despite being defined earlier in the file.

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/bom-explorer.test.js`:

```js
// script builds the rendered craft script for a target, the argument shape
// every craftScriptText test needs.
function script(data, producers, target, choices, qty, kind) {
  const g = bx.buildGraph(data, producers, target, choices);
  const ranks = bx.rankNodes(g);
  const totals = bx.rollUp(g, ranks, qty);
  const waves = bx.craftWaves(g, ranks, totals);
  return bx.craftScriptText(data, waves, {
    target, kind, qty, baseMaterials: bx.baseMaterialsMap(g, totals),
  });
}

test('craftScriptText heads the script with the build and its constraints', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'hauler', {}, 2, 'ship');

  assert.match(text, /^# Build: Hauler x2$/m);
  // The two ways a reader's first attempt actually fails. Both come from the
  // server's own /craft docs.
  assert.match(text, /1 mutation per tick/);
  assert.match(text, /STATION STORAGE, not cargo/);
});

test('craftScriptText lists the raw materials before the first wave', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'hauler', {}, 1, 'ship');

  assert.match(text, /# Raw materials required first \(3\):/);
  assert.match(text, /^#   iron_ore 70$/m);
  assert.match(text, /^#   energy_crystal 2$/m);
  assert.match(text, /^#   drop_core 4$/m);
  // The preamble must come before any command.
  assert.ok(text.indexOf('#   iron_ore 70') < text.indexOf('craft '));
});

test('craftScriptText emits one craft line per step, grouped by wave', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'hauler', {}, 1, 'ship');

  assert.match(text, /^# wave 1 - 1 craft$/m);
  assert.match(text, /^craft smelt_steel 28/m);
  assert.match(text, /^# wave 2 - 1 craft$/m);
  assert.match(text, /^craft weld_frame 8/m);
  assert.match(text, /^# wave 3 - 1 craft$/m);
  assert.match(text, /^craft assemble_widget 4/m);
  // Order matters: waves must appear in ascending order.
  assert.ok(text.indexOf('# wave 1') < text.indexOf('# wave 2'));
  assert.ok(text.indexOf('# wave 2') < text.indexOf('# wave 3'));
});

test('craftScriptText discloses the surplus only where rounding overproduces', () => {
  const data = fixture();
  const producers = bx.producersOf(data);

  // smelt_steel yields 2. Asking for 3 runs 2 batches and makes 4.
  const odd = script(data, producers, 'steel_plate', {}, 3, 'item');
  assert.match(odd, /^craft smelt_steel 3\s+# yield 2 -> 2 runs makes 4, 1 spare$/m);

  // Asking for 4 divides evenly, so there is nothing to disclose.
  const even = script(data, producers, 'steel_plate', {}, 4, 'item');
  assert.match(even, /^craft smelt_steel 4$/m);
  assert.ok(!even.includes('spare'));
});

test('craftScriptText comments out a cycle-cut step instead of emitting it', () => {
  const data = fixture();
  data.recipes.cycle_steel = {n: 'Cycle Steel', c: 'X', i: [['frame', 1]], o: [['steel_plate', 1]]};
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'widget', {steel_plate: 'cycle_steel'}, 1, 'item');

  // A command here would be doomed: the input it needs was dropped to break a
  // cycle, so nothing upstream ever produces it.
  assert.ok(!/^craft cycle_steel/m.test(text), 'must not emit a doomed command');
  assert.match(text, /cannot be scheduled/i);
  assert.match(text, /frame/, 'names the input that was dropped');
});

test('craftScriptText round-trips: every emitted line matches a wave entry', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const g = bx.buildGraph(data, producers, 'hauler', {});
  const ranks = bx.rankNodes(g);
  const totals = bx.rollUp(g, ranks, 3);
  const waves = bx.craftWaves(g, ranks, totals);
  const text = bx.craftScriptText(data, waves, {
    target: 'hauler', kind: 'ship', qty: 3, baseMaterials: bx.baseMaterialsMap(g, totals),
  });

  const emitted = text.split('\n')
    .filter((l) => l.startsWith('craft ') && !l.startsWith('craft jobs='))
    .map((l) => l.split('#')[0].trim().split(/\s+/).slice(1))
    .map(([recipe, qty]) => recipe + ' ' + qty);
  const expected = waves.flat().filter((j) => !j.cycle)
    .map((j) => j.recipeId + ' ' + j.qty);

  assert.deepStrictEqual(emitted.sort(), expected.sort(),
    'nothing dropped, nothing invented');
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `node --test 'tests/js/*.test.js'`
Expected: 6 failures, `bx.craftScriptText is not a function`.

- [ ] **Step 3: Implement `craftScriptText`**

Insert after `craftWaves`. This version handles the header, preamble, wave lines and the cycle case; Tasks 3 and 4 add the bulk blocks and the closing.

```js
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
  }

  return out.join('\n') + '\n';
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
```

- [ ] **Step 4: Export the new functions**

Add `craftScriptText` to the exports line from Task 1, and add `displayName` to the line that already carries `itemHref, leafKind, escapeHTML,`:

```js
    baseMaterialsMap, baseMaterialsJSON, craftWaves, craftScriptText,
```
```js
    itemHref, leafKind, escapeHTML, displayName,
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `node --test 'tests/js/*.test.js'`
Expected: PASS, 65 tests.

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): render craft waves as a runnable script

Header, raw-material prerequisites, and one craft line per step grouped
by wave. Quantities are the exact need; where whole production runs
overproduce, the line discloses the surplus inline.

A step whose recipe lost an input to cycle-breaking is explained rather
than emitted, since the command could not succeed."
```

---

### Task 3: Bulk job blocks

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (`craftScriptText`, plus a new `bulkChunks` helper)
- Test: `tests/js/bom-explorer.test.js` (append)

**Interfaces:**
- Consumes: the wave arrays from Task 1, inside `craftScriptText` from Task 2.
- Produces: `bulkChunks(wave)` — splits a wave into arrays of at most `BULK_MAX` (50) jobs. Exported for tests.

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/bom-explorer.test.js`:

```js
// wideFixture builds a world whose target consumes `n` distinct craftable
// components, so wave 1 has exactly n entries. Needed to exercise the 50-job
// bulk cap, which the small fixture cannot reach.
function wideFixture(n) {
  const data = {items: {ore: {n: 'Ore', c: 'ore'}}, recipes: {}, targets: {}, defaults: {}};
  const inputs = [];
  for (let i = 0; i < n; i++) {
    const id = 'part_' + String(i).padStart(3, '0');
    data.items[id] = {n: 'Part ' + i, c: 'component'};
    data.recipes['make_' + id] = {n: 'Make ' + id, c: 'C', i: [['ore', 2]], o: [[id, 1]]};
    inputs.push([id, 1]);
  }
  data.items.assembly = {n: 'Assembly', c: 'component'};
  data.recipes.assemble = {n: 'Assemble', c: 'C', i: inputs, o: [['assembly', 1]]};
  return data;
}

test('bulkChunks splits a wave at the 50-job cap', () => {
  const wave = Array.from({length: 51}, (_, i) => ({id: 'x' + i, recipeId: 'r' + i, qty: 1}));
  const chunks = bx.bulkChunks(wave);
  assert.strictEqual(chunks.length, 2);
  assert.strictEqual(chunks[0].length, 50, 'the server accepts at most 50 jobs per action');
  assert.strictEqual(chunks[1].length, 1);
  assert.strictEqual(bx.bulkChunks(wave.slice(0, 50)).length, 1, 'exactly 50 is one chunk');
});

test('craftScriptText emits a parseable bulk payload matching its plain lines', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'hauler', {}, 1, 'ship');

  const bulk = text.split('\n').find((l) => l.startsWith('craft jobs='));
  const jobs = JSON.parse(bulk.slice('craft jobs='.length));
  assert.deepStrictEqual(jobs, [{recipe_id: 'smelt_steel', quantity: 28}]);
  assert.match(text, /^# bulk:$/m, 'a single chunk is unnumbered');
});

test('craftScriptText numbers bulk blocks when a wave exceeds the cap', () => {
  const data = wideFixture(51);
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'assembly', {}, 1, 'item');

  assert.match(text, /^# bulk 1\/2:$/m);
  assert.match(text, /^# bulk 2\/2:$/m);

  const bulks = text.split('\n').filter((l) => l.startsWith('craft jobs='));
  assert.strictEqual(bulks.length, 3, 'wave 1 splits into two, wave 2 needs one');
  const all = bulks.flatMap((l) => JSON.parse(l.slice('craft jobs='.length)));
  assert.strictEqual(all.length, 52, '51 parts plus the assembly, each exactly once');
  // The bulk form and the plain lines must agree, or one of them is wrong.
  const plain = text.split('\n')
    .filter((l) => l.startsWith('craft ') && !l.startsWith('craft jobs='))
    .map((l) => l.split('#')[0].trim().split(/\s+/));
  assert.deepStrictEqual(
    all.map((j) => j.recipe_id + ' ' + j.quantity).sort(),
    plain.map(([, r, q]) => r + ' ' + q).sort());
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `node --test 'tests/js/*.test.js'`
Expected: 3 failures — `bx.bulkChunks is not a function`, and the two `craftScriptText` tests failing because no `craft jobs=` line exists yet.

- [ ] **Step 3: Add `bulkChunks` and emit the blocks**

Add beside `craftLine`:

```js
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
```

In `craftScriptText`, replace the wave loop body's tail so it reads:

```js
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
```

- [ ] **Step 4: Export `bulkChunks`**

```js
    baseMaterialsMap, baseMaterialsJSON, craftWaves, craftScriptText, bulkChunks,
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `node --test 'tests/js/*.test.js'`
Expected: PASS, 68 tests.

- [ ] **Step 6: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): emit a bulk jobs payload per craft wave

At 1 mutation per tick, a 29-craft wave issued as separate commands
costs nearly five minutes; one bulk action costs one tick. Both forms
are emitted because consumers differ - agents sharding a wave want the
plain lines, one agent queueing it all wants the bulk payload.

Chunked at 50 jobs, the server's documented cap."
```

---

### Task 4: Closing line for ship and facility targets

**Files:**
- Modify: `kb/build-costs/bom-explorer.js` (`craftScriptText`)
- Test: `tests/js/bom-explorer.test.js` (append)

**Interfaces:**
- Consumes: `meta.kind` and `meta.target` inside `craftScriptText`.
- Produces: no new exports. This closes out `craftScriptText`.

Background (verified against `/home/robert/spacemolt/spacemolt/server_docs/openapi.json`): **no** target in the data is produced by any recipe — 0 of 335 ships, 0 of 2650 facilities. So a ship or facility build always ends with an out-of-band step. Ships use `commission_ship(ship_class, provide_materials?, fund_from_faction?)`, and a provide-materials commission enters a *sourcing* state fed per item type by `supply_commission`. Facilities use the `facility` command's `build` / `faction_build` actions with `facility_type`.

- [ ] **Step 1: Write the failing tests**

Append to `tests/js/bom-explorer.test.js`:

```js
test('a ship target closes by pointing at the commission, not a craft', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'hauler', {}, 1, 'ship');

  // No recipe in the game produces a ship, so the script must not pretend
  // otherwise - it hands off to the shipyard.
  assert.match(text, /is a ship - no craft recipe produces it/);
  assert.match(text, /^#   commission_ship hauler provide_materials=true$/m);
  assert.match(text, /supply_commission/, 'sourcing is fed per item type');
});

test('a facility target closes with the facility build action', () => {
  const data = fixture();
  data.targets.refinery = {n: 'Refinery', t: 'facility', bm: [['widget', 2]]};
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'refinery', {}, 1, 'facility');

  assert.match(text, /is a facility - no craft recipe produces it/);
  assert.match(text, /^#   facility build refinery$/m);
  assert.match(text, /facility faction_build refinery/);
});

test('an item target just ends at its own craft line', () => {
  const data = fixture();
  const producers = bx.producersOf(data);
  const text = script(data, producers, 'widget', {}, 1, 'item');

  assert.ok(!text.includes('# Final:'), 'an item needs no out-of-band step');
  assert.match(text, /^craft assemble_widget 1$/m);
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `node --test 'tests/js/*.test.js'`
Expected: 2 failures (ship and facility); the item test passes already, which is the point of including it — it guards against a closing block leaking onto item targets.

- [ ] **Step 3: Append the closing block**

In `craftScriptText`, immediately before `return out.join('\n') + '\n';`:

```js
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `node --test 'tests/js/*.test.js'`
Expected: PASS, 71 tests.

- [ ] **Step 5: Commit**

```bash
git add kb/build-costs/bom-explorer.js tests/js/bom-explorer.test.js
git commit -m "feat(bom-explorer): close the craft script with the real final step

No recipe produces a ship or facility - 0 of 335 and 0 of 2650 appear
as a recipe output - so the last step is always out of band. Ships get
commission_ship with provide_materials plus a pointer to
supply_commission; facilities get facility build / faction_build.
Item targets end at their own craft line."
```

---

### Task 5: Wire it into the page

**Files:**
- Modify: `kb/build-costs/explorer.html` (new section after the JSON blob section; one CSS rule)
- Modify: `kb/build-costs/bom-explorer.js` (`initExplorer`: element lookups, copy wiring, `render`)
- Test: manual browser verification (steps below)

**Interfaces:**
- Consumes: `craftWaves`, `craftScriptText`, `baseMaterialsMap`.
- Produces: nothing further.

- [ ] **Step 1: Add the markup**

In `kb/build-costs/explorer.html`, immediately after the closing `</section>` of the existing `json-out` section and before `<details class="recipe-note">`:

```html
  <section class="json-out">
    <h2>Craft script
      <button type="button" id="copy-script" class="copy-btn">copy</button>
    </h2>
    <pre id="craft-script"></pre>
  </section>
```

Reuses `.json-out` and `.copy-btn` deliberately: a second visual treatment for the same kind of output would be noise.

- [ ] **Step 2: Give the new `<pre>` the same styling**

The existing rule selects `#json-blob` by id. Change that selector to cover both:

```css
#json-blob,#craft-script{background:var(--panel);border:1px solid var(--border);border-radius:6px;padding:.6rem .8rem;margin:0;max-height:22rem;overflow:auto;font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--text)}
```

- [ ] **Step 3: Look up the new elements**

In `initExplorer`'s `els` object, after `copy: document.getElementById('copy-json'),`:

```js
    script: document.getElementById('craft-script'),
    copyScript: document.getElementById('copy-script'),
```

- [ ] **Step 4: Generalise the copy handler to serve both buttons**

There are now two copy buttons with identical behaviour. Replace the existing `els.copy.addEventListener('click', ...)` block with a helper and two calls:

```js
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
```

- [ ] **Step 5: Fill it in `render`**

Directly after the line that sets `els.json.textContent`:

```js
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
```

`isTarget` is already computed earlier in `render`; reuse it rather than calling `hasOwn` again.

- [ ] **Step 6: Verify the unit tests still pass**

Run: `node --test 'tests/js/*.test.js'`
Expected: PASS, 71 tests. (No new unit tests here — this step is DOM wiring, which the suite does not load.)

- [ ] **Step 7: Verify in a real browser**

```bash
cd kb && python3 -m http.server 8777 &
```

Open `http://localhost:8777/build-costs/explorer.html?target=overmind` and confirm:

1. The Craft script section renders below the JSON blob and is non-empty.
2. Wave headers run `# wave 1` through `# wave 9`, wave 1 says 29 crafts, and
   the preamble lists 39 raw materials.
3. `overmind` is a **ship**, so the closing block must say `commission_ship
   overmind provide_materials=true` and mention `supply_commission` — NOT
   `facility build`.
4. Every `craft jobs=` line parses as JSON. In the devtools console:
   ```js
   [...document.getElementById('craft-script').textContent.matchAll(/^craft jobs=(.*)$/gm)]
     .map(m => JSON.parse(m[1]).length)
   ```
   Expected: `[29, 14, 12, 12, 11, 8, 4, 2, 1]`.
5. Change quantity to 3 and confirm the numbers scale.
6. Switch a recipe in any node's dropdown and confirm the script changes.
7. Load a facility target (pick any entry the autocomplete labels `facility`)
   and confirm the closing block switches to `facility build <id>`.
8. Load a plain item target (e.g. `?target=steel_plate` is terminal, so use a
   craftable one such as `?target=circuit_board`) and confirm there is no
   `# Final:` block at all.
9. Click copy and confirm the label changes to `copied`.
10. Toggle the theme and confirm the `<pre>` stays readable in both.

Kill the server when done: `pkill -f "http.server 8777"`.

- [ ] **Step 8: Commit**

```bash
git add kb/build-costs/explorer.html kb/build-costs/bom-explorer.js
git commit -m "feat(bom-explorer): show the craft script on the page

New section beside the JSON blob, filled from the same graph so it
tracks target, quantity and recipe choices like every other output.

The copy handler is now shared by both buttons rather than duplicated."
```

---

## Verification

After Task 5, the whole feature is done. Final check:

```bash
node --test 'tests/js/*.test.js'   # 71 pass, 0 fail
git log --oneline -5               # five feature commits
```

Do NOT push unless the user asks.
