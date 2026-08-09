# Craft Script Output — Design

**Date:** 2026-08-09
**Status:** Approved for planning

## Goal

Add a third output to the Bill of Materials explorer: the whole build rendered
as an ordered list of `craft <recipe_id> <quantity>` commands, grouped into
waves that can be launched in parallel.

The chart answers "what goes into this, through which recipes". The JSON blob
(added 2026-08-09, commit 6072c2339) answers "how much raw material". This
answers "what do I actually type, in what order, and what can I run at once".

## Scope

**In scope:** wave grouping derived from the existing rank layering; one
`craft` line per non-leaf node; a bulk `jobs=[...]` payload per wave; a raw
materials preamble; a closing line naming the real final step for ship and
facility targets; a copy button.

**Out of scope:** prices, station selection, travel, storage deposits beyond a
header warning, skill gating, facility ownership, job scheduling against real
tick timing, and any live query of the player's inventory. The script is a
recipe for what to type, not a plan validated against a game session.

## The core claim: waves are the columns

`rankNodes` assigns `rank = 1 + max(rank of inputs)` — longest-path layering
from the leaves. That is exactly an ASAP schedule under unlimited parallelism:
an item can start only once every input is complete, and its rank is one past
the last of them.

Two consequences, both verified rather than assumed:

1. **A wave never depends on itself.** Every edge runs from a strictly lower
   rank to a higher one (already enforced and tested — `buildGraph` drops
   cycle-closing edges precisely so this holds).
2. **A wave can never start early.** A rank-*k* node has at least one input at
   rank exactly *k−1*. This is definitional, not empirical: `rankNodes`
   computes `rank(x) = max(rank(input) + 1)` over the inputs, so the greatest
   input rank *is* `k−1`. No recipe shape can break it. The only escape is a
   node with no inputs, which is the cycle-cut case handled below.
   Independently measured across `overmind` (132 nodes),
   `void_shield_matrix` (12) and `steel_plate` (3) as a check on the reading
   of the code: zero violations.

So the answer to "what can run in parallel" is "each column of the chart, left
to right" — no new analysis, and the schedule is already on screen. A user
hypothesis that some column-4 items might run alongside column 3 is ruled out
by (2).

The finer truth the wave barrier gives up: an item needs only *its own*
inputs, not all of the previous wave. Strict waves are therefore a conservative
schedule. This was a deliberate choice — the barrier form is simpler to follow,
simpler to verify against the chart, and shards across agents without
coordination.

Reference shape, `overmind` at quantity 1: 132 nodes, 93 craft steps, 10 waves,
widths 29 / 14 / 12 / 12 / 11 / 8 / 4 / 2 / 1 after 39 raw materials in wave 0.

## Game semantics this depends on

Established from the game server's own generated docs in
`/home/robert/spacemolt/spacemolt/server_docs/` and the client in
`pkg/game/crafting.go`. These are facts about the server, not choices:

| Fact | Source | Consequence here |
|---|---|---|
| `craft` payload is `{recipe_id, quantity, deliver_to?}` | `pkg/game/crafting.go:105-124` | The command form is `craft <recipe_id> <quantity>`. |
| `quantity` is **output items**, "rounded up to a whole number of production runs, so a recipe that yields several items per run may produce a few extra" | `openapi.json` `/craft`, `quantity` property | Print the exact need. The server derives runs itself and lands on the same count the explorer's tables already assume. |
| Crafting is async: it queues a job that runs over later ticks | `openapi.json` `/craft` | A wave is "launch these", not "these are done". The header says so. |
| Rate limited to 1 mutation per tick (10 s) | `openapi.json` `/craft` | 93 separate lines cost ~15 minutes of issuing. This is why the bulk form is not optional. |
| `jobs=[...]` queues up to 50 jobs in one action | `openapi.json` `/craft`; `MaxCraftBulkJobs`, `pkg/game/crafting.go:136` | Bulk blocks chunk at 50. |
| Must be docked at a base with crafting + storage service | `openapi.json` `/craft` | Header warning. |
| Inputs are escrowed from **station storage**, not cargo | `openapi.json` `/craft` | Header warning — the most likely way a reader's first attempt fails. |
| Ships: `commission_ship(ship_class, provide_materials?, fund_from_faction?)` | `openapi.json` `/commission_ship` | Ship closing line. |
| A provide-materials commission enters a *sourcing* state fed by `supply_commission` per item type | `openapi.json` `/supply_commission` | Ship closing line points at it. |
| Facilities: `facility build <facility_type>` / `facility faction_build <facility_type>` | `openapi.json` `/facility` actions | Facility closing line. |
| No target is produced by any recipe (0 of 335 ships, 0 of 2650 facilities) | `recipe-graph.json` | The final assembly step is *always* out of band for ship and facility targets. |

`quantity` being output units makes the exact-need and whole-batch forms
numerically identical: `ceil(320/3)` and `ceil(321/3)` are both 107 runs. The
exact need is printed because it is what the reader asked for; the surplus is
disclosed in a comment.

## Architecture

Two pure functions in `kb/build-costs/bom-explorer.js`, beside
`baseMaterialsMap`, plus DOM wiring. No new data file, no generator change:
everything derives from the graph already in memory.

### `craftWaves(graph, ranks, totals)`

Returns an array indexed by wave number. Element 0 is empty except for the
cycle-cut case below — wave 0 is raw materials, which carry no command. Each
element is an array of:

```js
{id, recipeId, qty, yield, runs, made, cycle}
```

- `qty` is `totals.demand.get(id)` — the exact need.
- `runs` is `totals.batches.get(id)`, `made` is `runs * yield`. `made > qty`
  drives the surplus comment.
- Excluded: every leaf (`node.leaf`), and the target itself when it has no
  `recipeId` (a ship or facility sink).
- Each wave is sorted by item id, so regeneration is byte-identical and two
  builds diff cleanly.

**Cycle-cut nodes.** A node whose recipe lost an input to cycle-breaking has
`inputs: []`, so it ranks 0 while still being non-leaf. It carries
`cycle: true` and is emitted in wave 0's own list — the one case where wave 0
is non-empty. No real target produces one today; a user-chosen recipe can.

### `craftScriptText(data, waves, meta)`

Renders the text. `meta` carries `{target, kind, qty, baseMaterials}` — the
target id, its kind, the build quantity, and the `baseMaterialsMap` output for
the preamble. `kind` is `data.targets[target].t` (`ship` / `facility`) when the
target is a sink, and `item` otherwise.

Kept separate from `craftWaves` so tests assert on structure rather than
string formatting, and so a future consumer can render the same waves
differently.

### Output format

```
# Build: Overmind x1
#
# Rate limited to 1 mutation per tick (10s); jobs run async over later ticks.
# Everything inside a wave is independent - launch together or shard across
# agents; finish the wave before starting the next.
# Must be docked at a base with crafting + storage service.
# Inputs are escrowed from STATION STORAGE, not cargo - deposit them first.
#
# Raw materials required first (39):
#   iron_ore 28044
#   carbon_ore 18724
#   ...

# wave 1 - 29 crafts
craft refine_iron 28044
craft purify_argon 320          # yield 3 -> 107 runs makes 321, 1 spare
...
# bulk: craft jobs=[{"recipe_id":"refine_iron","quantity":28044}, ...]

# wave 2 - 14 crafts
...

# Final: Overmind is a ship - no craft recipe produces it.
# Dock at a shipyard and:
#   commission_ship overmind provide_materials=true
# That commission enters a sourcing state; feed it one item type at a
# time with supply_commission <commission_id> <item_id> <quantity>.
```

A wave over 50 jobs emits several bulk blocks, labelled `# bulk 1/2:` and
`# bulk 2/2:`; a wave at or under 50 emits a single unnumbered `# bulk:`.

Closing line by target kind:

- **item** — ends at its own craft line, no closing block.
- **ship** — `commission_ship <class> provide_materials=true` at a shipyard,
  plus a note that sourcing is fed per item type via `supply_commission`.
- **facility** — `facility build <type>`, or `facility faction_build <type>`.

A cycle-cut node renders as a commented-out line naming the input that was
dropped, not a command that would fail.

### UI

A new section beside the JSON blob in `kb/build-costs/explorer.html`: an `<h2>`
with a copy button and a `<pre>`, scrolling at the same max-height. Filled via
`textContent` in `render()`, so it tracks target, quantity and recipe choices
like every other output. Reuses the existing `.copy-btn` and `#json-blob`
styling rather than introducing a second visual treatment.

## Testing

Appended to `tests/js/bom-explorer.test.js`, over the fixture world:

1. Waves are indexed by rank; leaves are absent; a ship sink is absent.
2. **The load-bearing invariant:** no item in a wave depends on an item in the
   same or a later wave. Asserted over every emitted command against the
   graph's own edges, not against a re-derivation of ranks.
3. `qty` equals `totals.demand` for every command.
4. A surplus comment appears exactly where `runs * yield !== qty`, and nowhere
   else — the fixture's `smelt_steel` (5 ore → 2 plate) covers both sides.
5. Bulk blocks chunk at 50 jobs; a 51-item wave produces two numbered blocks.
6. The bulk payload parses as JSON and its `{recipe_id, quantity}` pairs match
   the plain lines of the same wave exactly.
7. A cycle-cut node is commented out, not emitted as a command.
8. Three closing variants: item, ship, facility.
9. Round trip: the `craft` lines recovered from the rendered text match the
   set of emitted commands exactly — that is, every non-leaf node except the
   sink target and any cycle-cut node. Nothing dropped, nothing invented.

## Risks

- **The script is not validated against a live game.** It does not know what is
  already in storage, what recipes the player's skills allow, or whether a
  facility is reachable. The header says what it assumes; it cannot say more.
- **`bom-explorer.js` grows to ~1250 lines.** Judged still coherent: the new
  functions share the graph model and sit in their own delimited section.
  Splitting would add a second `<script>` tag and a second `require` for no
  present benefit. Revisit if a fourth output lands.
- **Server semantics are read from generated docs, not server source**, which
  is not in either repo. The rounding rule rests on the OpenAPI text plus
  live-captured response fixtures, which agree with each other.
