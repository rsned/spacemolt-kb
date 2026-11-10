# Mission template families — collapsing generated missions on KB category pages

Date: 2026-11-09 · Status: APPROVED by owner 2026-11-09

## Why

The KB has ~25,000 mission templates and the category index pages list every one.
`kb/missions/trading/index.html` is 13 MB of 19,082 near-identical "Trade Run: X to Y"
rows, and delivery and smuggling are close behind. About 180 missions are hand-written
(one-offs and chain steps). The rest come from roughly 14 server formulas plus a few
player-faction formulas. The owner wants to see the **important hand-written content
first**, then a **high-level view of each formula** ("Deliver X to Y, rewards
250–10,000 cr") rather than every permutation, and to spot anything **unique to a
single station**.

Success: a category page reads as chains, then one-offs, then a short stack of family
cards, and answers "what kinds of missions are here, and what do they roughly pay?" at
a glance. The trading page drops from 13 MB to a size that loads instantly.

## Goals

1. Each category page (`kb/missions/<type>/index.html`) has three sections in order:
   **Chains**, **One-off missions**, **Template families**.
2. A **family** is one card per formula, showing:
   - family name (the title prefix, e.g. "Contraband Run") and variant count
   - the formula as a readable pattern, e.g. `Deliver <N> <good> to <station>`
   - **reward line broken out by reward type**, showing only the types that family
     actually pays: credit range (min–max), skill XP (range per skill, e.g.
     "20–40 trading XP"), and item rewards if any
   - difficulty range, number of offering stations, number of distinct goods
   - a few chips naming goods (e.g. first 4 + "+N")
   - a disclosure ("show breakdown") that opens a **compact breakdown table: one row per
     good** (or per destination where goods don't vary), each with its reward range and
     how many stations offer it. **Not** the full variant list.
3. **Station exclusivity is shown two ways** (owner decision):
   - **One-offs are grouped under station headings.** Most hand-written missions are
     tied to one station (115 of ~137 server one-offs have exactly one offering
     station), so grouping makes exclusivity obvious without badging every row. One-offs
     offered at 2+ stations go in a first group, **"Offered at many stations"**, whose
     rows list the station count (e.g. First Haul — 50 stations). Station groups follow,
     sorted by station name.
   - **A bold "ONLY AT <station>" badge** is reserved for (a) missions the server marks
     exclusive (`mission_templates.exclusive_base_id` non-empty — 4 today: `supply_run`
     at Treasure Cache, `replacement_survey_lens` and `closing_the_loop` at Starfall,
     `frontier_extension` at Alpha Centauri), (b) any **chain** containing such a
     mission, or whose steps are all offered at the same single station (e.g. the 9-step
     Treasure Cache smuggling chain), and (c) **families** whose variants are all offered
     at one station. Breakdown rows do not get badges (nearly every procedural variant
     has exactly one offering station, so it carries no signal there); the breakdown's
     station-count column covers it.
4. The top-level `kb/missions/index.html` category counts stay meaningful (e.g. show
   "N chains · N one-offs · N families (M variants)" per category instead of one raw total).

## Non-goals

- **Reputation rewards.** The server sends `reputation` and `pirate_rep` on mission
  rewards (`serverapi.MissionRewards`), but the mission scraper
  (`spacemolt/pkg/knowledge/mission_catalog.go`) drops them, so the DB has none.
  Capturing them is a **separate follow-up** in the game-client repo; the reward line
  should be built so a rep entry can be added later without redesign.
- Searching or filtering individual variants on the category page.
- Changing mission data capture, the DB schema, or mission chain detection.

## Anti-goals

- Re-listing thousands of variants in any form (including a giant hidden `<details>`
  block — it would keep the page at 13 MB).
- Breaking existing links: **every per-mission detail page keeps its current URL**
  (`kb/missions/<type>/<id>.html`); they stay reachable from search, chain links and
  wherever else they are linked today.
- Mislabeling templated player-faction missions as hand-written one-offs.

## Constraints and facts (from the knowledge DB, 2026-11-09)

- Source tables: `mission_templates` (has `procedural` flag, `title`, `type`,
  `difficulty`, `rewards_credits`, `rewards_skill_xp` JSON, `rewards_items` JSON,
  `chain_next`), `mission_objectives` (`type`, `item_id`, `quantity`,
  `target_base_id`), `mission_template_locations` (mission_id × base_id offering it).
- Family title prefixes today (procedural=1): Trade Run 19,082 · Shipyard Supply 3,840 ·
  Contraband Run 392 · Station Need 103 (new hunting missions, growing toward ~800) ·
  Supply Run 72 · Black Market 40 · Wreck Recovery 39 · Courier Run 39 · Quiet Delivery
  37 · Special Delivery 35 · Off the Books 30 · Border Job 19. Exploration has 223
  procedural "Anomalous Readings" (no colon prefix) and delivery has 72 procedural
  missions with no colon prefix.
- **Player-faction formulas are flagged procedural=0** but are clearly templated:
  "HEXC recon: Hex Star" ×708, "HEXC supply: <item>" ×~150. They must be grouped as
  families.
- Reward mix: 19,154 missions pay XP only (all Trade Runs — profit comes from the
  market; `rewards_credits` is 0), 4,940 credits only, 1,062 both.
- Many station ids are 32-hex player stations; `enrichMissionLocationNames` already
  resolves names via `pois`.
- Generator: `cmd/generate-items-kb/missions.go` (`writeMissionPages`,
  `partitionMissions`, `buildMissionChains`, templates `htmlMissionsCategoryTemplate`,
  `missionTableTemplate`). Today's sections are Mission Chains / Standalone /
  Procedural; this design replaces Standalone/Procedural rendering.
- Chains are global and may cross types (existing behaviour, keep it).

## Decided (by the owner)

- Layout **A — family cards** with a disclosure (over a single summary table).
- The disclosure shows a **breakdown**, not the variant list; dropping the full
  variant list from category pages is approved.
- Reward line shows each reward type the family pays (credits, XP, items; rep later).
- One-offs are grouped under station headings; the bold "ONLY AT" badge is kept for
  the server's explicit exclusive missions, single-station chains and single-station
  families (see Goal 3).
- Reputation capture is a separate follow-up.

## Calls I made (owner may override)

- **Grouping order: chains first.** Chain members (`buildMissionChains`) are removed
  before family grouping, so hand-written chain steps never join a family (e.g.
  hand-written `supply_run`, titled "Supply Run", stays in its chain and does not merge
  into the procedural "Supply Run: …" family; the three "Nebula: Consulting …" chain
  steps stay a chain).
- **Family key** = mission `type` + title text before the first colon. Missions with
  no colon are grouped by identical title (catches "Anomalous Readings";
  "HEXC recon: Hex Star" already has a colon). Any remaining group with **≥ 3 members**
  sharing a key is a family regardless of the `procedural` flag; everything else is a
  one-off.
- **Procedural flavour-titled missions merge into one family per type.** Delivery has 72
  procedural missions in 12 whimsical titles × 6 ("A reasonable number of spoons",
  "Please hold", "The chair inspection" …) with no colon. Rather than 12 near-identical
  cards, all `procedural=1` missions with no colon prefix in a type form **one family**,
  named "Odd jobs" for delivery (exploration's 223 "Anomalous Readings" keep their title
  as the name, since it is a single title). The breakdown for "Odd jobs" is by title.
- **Player-faction one-offs** (`id LIKE 'faction_%'`, 23 today, e.g. "ROCI Darksteel
  Supply Contract") sit in their own subsection **"Posted by player factions"** after the
  station-grouped server one-offs, and are counted separately on the top-level index.
- "HEXC supply" and "HEXC supply contract" stay separate families (different
  prefixes); no merging of player-faction prefixes.
- Families sorted by variant count, descending.
- Formula pattern is **derived** per family from the objective type + which fields
  vary (item / quantity / target station), e.g. deliver_item with varying item and
  target → `Deliver <N> <good> to <station>`. A small hand-maintained override map
  keyed by family name is acceptable where the derived text reads badly.
- Breakdown dimension: by good (`objective.item_id`) when it varies, else by target
  station, else by offering station.
- Single-station tests (chains, families) use `mission_template_locations` distinct
  `base_id` count == 1; server exclusivity uses `exclusive_base_id`.
- One-offs table keeps today's columns (title, difficulty, reward, chain link); the
  station heading replaces an "Offered at" column, except in the "many stations" group,
  which shows a station count.

## Left to the builder

- Exact card styling (reuse `missions.css` conventions and existing difficulty badge).
- How the breakdown table is capped if a family has hundreds of goods (sort by
  count, show top N + "and N more").
- Tests: unit tests for family grouping, reward-range aggregation and the "only at"
  rule on small fixtures; an assertion that the trading index is far below its
  current size.
- Regeneration and the commit split (template change vs regenerated pages), per the
  KB regeneration runbook.
