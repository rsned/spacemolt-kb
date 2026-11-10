# Mission Template Families Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the ~24,500 templated missions on each KB mission category page into one card per formula, with chains first and hand-written one-offs grouped under station headings.

**Architecture:** All work is in `cmd/generate-items-kb`. A new pure-Go file, `missionfamilies.go`, partitions a category's missions into families and one-offs and summarises each family. `missions.go` loads one extra column, wires the new structures into `MissionCategoryInfo`, and swaps the Standalone/Procedural template sections for family cards and station-grouped one-offs. Per-mission detail pages are untouched.

**Tech Stack:** Go 1.24+ (range-over-int, `slices`, `maps`), `html/template`, SQLite via the existing `database/sql` loader. The hand-maintained `kb/missions/missions.css` gets the new styles.

**Spec:** `docs/superpowers/specs/2026-11-09-mission-template-families-design.md`

## Global Constraints

- Every per-mission detail page keeps its URL `kb/missions/<type>/<id>.html` and is still written.
- Category pages never list family variants individually, whether visible or inside a hidden `<details>`.
- Chain members are removed **before** family grouping.
- Family key = `type` + title text before the first `:` (trimmed). With no colon, procedural missions in a type form one family named after their shared title if they all share it, otherwise **"Odd jobs"**. Non-procedural missions with no colon key on their exact title.
- A group needs **≥ 3 members** to be a family. Anything smaller is a one-off, whatever its `procedural` flag says.
- One-offs whose id starts with `faction_` go into the **"Posted by player factions"** subsection.
- Server one-offs are grouped under station headings. Missions at 2+ stations go first, under **"Offered at many stations"**, with a station count. Missions with no recorded location go last, under **"Station not recorded"**.
- The **ONLY AT** badge appears only on: missions with `exclusive_base_id` set; chains containing such a mission or whose steps share one single offering station; families whose variants share one single offering station. Breakdown rows never get it.
- The reward line shows only the reward types a family actually pays: credits range, XP range per skill, and item names. Never render "0–0 cr".
- Breakdown dimension: by good (`ItemID`) if it varies, else by title if that varies, else by target station if it varies, else by offering station. Cap at **25 rows** plus "and N more".
- Every new Go file passes `golangci-lint run ./cmd/generate-items-kb/...` with no new findings.

## Review Focus

1. **Missions with zero locations.** They must not panic. A one-off goes to "Station not recorded", and a family whose variants have no locations shows 0 stations and no badge. *(Task 3 test)*
2. **Families that pay no credits** (all Trade Runs). The reward line shows XP only and never "0–0 cr". *(Task 2 test)*
3. **A hand-written chain step whose title prefix matches a procedural family** ("Supply Run" vs "Supply Run: …"). It stays in the chain, and the family count excludes it. *(Task 1 test)*
4. **Names containing `&`, `'` or `<`**, such as "Traders' Rest & Resort". They must be HTML-escaped on the cards and the station headings. *(Task 4 test)*
5. **Unknown 32-hex player-station ids** with no `pois` name. They fall back to `titleCase(baseID)` the same way detail pages do today, rather than rendering an empty heading. *(Task 3 test)*

---

### Task 1: Family grouping and exclusivity data

**Files:**
- Create: `cmd/generate-items-kb/missionfamilies.go`
- Create: `cmd/generate-items-kb/missionfamilies_test.go`
- Modify: `cmd/generate-items-kb/missions.go` (the `Mission` struct, the `loadMissions` SELECT/Scan, and `enrichMissionLocationNames`)

**Interfaces:**
- Produces:
  - `Mission.ExclusiveBaseID string` and `Mission.ExclusiveBaseName string`. These are loaded from `COALESCE(exclusive_base_id,'')`. The name is resolved in `enrichMissionLocationNames` from the same `baseNames` map, falling back to `titleCase(id)`.
  - `type MissionFamily struct { Key, Name, Type string; Variants []*Mission }`, with the summary fields added in Task 2.
  - `func familyKey(m *Mission, oddJobsName string) (key, name string)`
  - `func partitionFamilies(missions []*Mission, chained map[string]bool) (families []*MissionFamily, oneOffs, factionOneOffs []*Mission)`. Families are sorted by `len(Variants)` descending, then by `Name`. Both one-off slices are sorted with `compareMissions`.
  - `const familyMinVariants = 3`

- [ ] **Step 1: Write the failing tests** in `missionfamilies_test.go`, following the fixture style of `missionchains_test.go`:
  - `TestFamilyKeySplitsOnFirstColon`: "Trade Run: Fuel Cell to Korr" with type trading gives `("trading|Trade Run", "Trade Run")`. "HEXC supply: Whale Oil" gives name `"HEXC supply"`.
  - `TestPartitionFamiliesRemovesChainMembersFirst`:
    - Fixture: hand-written `supply_run` "Supply Run" (smuggling, chained) plus 3 procedural "Supply Run: X to Y".
    - Expect one family "Supply Run" with exactly 3 variants. `supply_run` appears in neither the families nor the one-offs.
  - `TestPartitionFamiliesThreshold`: two procedural "Border Job: …" missions become 2 one-offs and no family. Three "HEXC recon: Hex Star" with `Procedural=false` become 1 family.
  - `TestPartitionFamiliesOddJobs`:
    - Fixture: procedural no-colon delivery titles "Please hold" ×3 and "The chair inspection" ×3. Expect one family named "Odd jobs" with 6 variants.
    - Second fixture: procedural exploration "Anomalous Readings" ×4. Expect one family named "Anomalous Readings".
  - `TestPartitionFamiliesFactionOneOffs`: id `faction_abc` titled "ROCI Darksteel Supply Contract", alone, goes into `factionOneOffs` and not `oneOffs`.

- [ ] **Step 2: Run them and confirm they fail**

  Run: `go test ./cmd/generate-items-kb/ -run 'FamilyKey|PartitionFamilies'`
  Expected: FAIL (undefined: `familyKey`, `partitionFamilies`)

- [ ] **Step 3: Implement**
  - Add the two `Mission` fields, the extra SELECT column and Scan target, and the name resolution in `enrichMissionLocationNames`.
  - Write `familyKey` and `partitionFamilies` in `missionfamilies.go`.
  - In `partitionFamilies`, first bucket the no-colon procedural missions of the type. Pass `oddJobsName` as the shared title when every bucket member has the same title, otherwise "Odd jobs".

- [ ] **Step 4: Run the tests and confirm they pass**

  Run: `go test ./cmd/generate-items-kb/`
  Expected: PASS. The existing chain tests also still pass.

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/generate-items-kb/missionfamilies.go cmd/generate-items-kb/missionfamilies_test.go cmd/generate-items-kb/missions.go
  git commit -m "missions: group templated missions into families"
  ```

### Task 2: Family summaries (rewards, pattern, breakdown, single-station)

**Files:**
- Modify: `cmd/generate-items-kb/missionfamilies.go`
- Modify: `cmd/generate-items-kb/missionfamilies_test.go`

**Interfaces:**
- Consumes: `MissionFamily`, `Mission.Locations[].BaseID/BaseName`, `Mission.Objectives`, `Mission.RewardsSkillXP`, `Mission.RewardsItems`, `Mission.RewardsCredits`, `Mission.Difficulty`
- Produces:
  - `type IntRange struct{ Min, Max int }` with `func (r IntRange) String() string`. It renders "480–1,750" using `fmtValue`, and a single number when Min == Max.
  - `type RewardSummary struct { Credits *IntRange; XP []SkillXPRange; Items []string }` and `type SkillXPRange struct { Skill string; IntRange }`.
    - `Credits` is nil when no variant pays credits.
    - XP is sorted by skill. Items are distinct names, sorted.
    - This struct is the place a future `Reputation` field goes.
  - `func (s RewardSummary) Parts() []string` returns, e.g., `["480–1,750 cr", "20–40 trading XP", "items: Whale Oil"]`.
  - `type FamilyBreakdownRow struct { Label string; Variants, Stations int; Rewards RewardSummary }`
  - Fields filled by `func (f *MissionFamily) summarize()`:
    `Rewards RewardSummary; Difficulty IntRange; Stations int; OnlyAt string; Goods []string; GoodsMore int; Pattern string; BreakdownBy string; Breakdown []FamilyBreakdownRow; BreakdownMore int`
    - `Goods` is the top 4 item names by variant count. `GoodsMore` is the number of remaining distinct goods.
    - `BreakdownBy` is one of "good", "title", "destination" or "station".
  - `partitionFamilies` calls `summarize()` on every family it returns.

- [ ] **Step 1: Write the failing tests**
  - `TestRewardSummaryOmitsZeroCredits`: 3 Trade Runs with `RewardsCredits: 0` and XP `{trading:20}`, `{trading:40}`, `{trading:30}` give `Parts() == ["20–40 trading XP"]`.
  - `TestRewardSummaryCreditsAndXP`: credits 480, 1750 and 1125 plus XP give a first part of `"480–1,750 cr"`.
  - `TestFamilyPatternDeliverVaryingItemAndTarget`:
    - Fixture: 3 variants each with one `deliver_item` objective, different `ItemID`/`ItemName` and different `TargetBaseName`, quantities 5, 8, 5.
    - Expect `Pattern == "Deliver <N> <good> to <station>"`.
    - Second case: same item "Void Dust" and same target "Ironhearth Station" with quantity varying. Expect `"Deliver <N> Void Dust to Ironhearth Station"`.
  - `TestFamilyBreakdownByGoodCapsAt25`: 30 variants with 30 distinct items give `BreakdownBy == "good"`, `len(Breakdown) == 25` and `BreakdownMore == 5`. Rows are sorted by `Variants` descending, then `Label`.
  - `TestFamilyBreakdownByTitleForOddJobs`: an "Odd jobs" family with 2 titles × 3 gives `BreakdownBy == "title"` and 2 rows.
  - `TestFamilyOnlyAtSingleStation`: every variant located only at `treasure_cache_trading_post` ("Treasure Cache Trading Post") gives `OnlyAt == "Treasure Cache Trading Post"` and `Stations == 1`. Adding one variant at a second station makes `OnlyAt` empty and `Stations` 2.
  - `TestFamilyNoLocations`: no variant has locations, which gives `Stations == 0` and an empty `OnlyAt`.

- [ ] **Step 2: Run them and confirm they fail**

  Run: `go test ./cmd/generate-items-kb/ -run 'RewardSummary|FamilyPattern|FamilyBreakdown|FamilyOnlyAt|FamilyNoLocations'`
  Expected: FAIL

- [ ] **Step 3: Implement `summarize()` and its helpers**
  - **Pattern.** Build it from the variants' first objective that has an `ItemID`. If none has one, use the first objective.
  - Verb map:
    - `deliver_item` → "Deliver"
    - `sell_item` → "Sell"
    - `buy_item` → "Buy"
    - `mine_resource` → "Mine"
    - `kill_creature` → "Hunt"
    - `kill_pirate` → "Defeat"
    - `visit_system` → "Visit"
    - `dock_at_base` → "Dock at"
    - `traverse_wormhole` → "Traverse"
    - `survey_system` → "Survey"
    - default → `titleCase(type)`
  - **Placeholders.** Use `<N>`, `<good>` and `<station>` for fields that vary across variants, and the literal value for fields that don't. Omit " to …" when no variant has a target. If no variant has objectives, the pattern is empty.

- [ ] **Step 4: Run the tests and confirm they pass**

  Run: `go test ./cmd/generate-items-kb/`
  Expected: PASS

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/generate-items-kb/missionfamilies.go cmd/generate-items-kb/missionfamilies_test.go
  git commit -m "missions: summarise families (rewards, pattern, breakdown, single-station)"
  ```

### Task 3: One-offs by station, and chain/mission ONLY AT badges

**Files:**
- Modify: `cmd/generate-items-kb/missionfamilies.go`
- Modify: `cmd/generate-items-kb/missionchains.go` (adds the `OnlyAt` field to `MissionChain`)
- Modify: `cmd/generate-items-kb/missionfamilies_test.go`

**Interfaces:**
- Consumes: `Mission.Locations`, `Mission.ExclusiveBaseID/ExclusiveBaseName`, `MissionChain.Steps`
- Produces:
  - `type StationGroup struct { Heading string; Many bool; Missions []*Mission }`. `Many` marks the "Offered at many stations" group.
  - `func groupOneOffsByStation(oneOffs []*Mission) []StationGroup`. The order is: "Offered at many stations" (if any), then single-station groups sorted by heading, then "Station not recorded" (if any). Missions inside each group are sorted with `compareMissions`.
  - `func (m *Mission) StationCount() int`: the number of distinct `Locations[].BaseID`. The template uses it in the many-stations group.
  - `func (m *Mission) OnlyAt() string`: returns `ExclusiveBaseName` when `ExclusiveBaseID != ""`, otherwise "".
  - `MissionChain.OnlyAt string`, set by `func chainOnlyAt(c MissionChain) string`:
    - first, the `ExclusiveBaseName` of any step that has one
    - otherwise, the single shared base name when every step's distinct locations are exactly the same one base
    - otherwise ""

- [ ] **Step 1: Write the failing tests**
  - `TestGroupOneOffsByStationOrder`:
    - Fixture:
      - "First Haul" at 50 bases
      - "Gas Futures" and "Water Rights" at `grand_exchange_station` ("Grand Exchange Station")
      - "Scraping By" at `frontier_station`
      - "Ghost Job" with no locations
    - Expected headings: `["Offered at many stations", "Frontier Station", "Grand Exchange Station", "Station not recorded"]`.
    - The first group has `Many == true`, and First Haul's `StationCount()` is 50.
  - `TestGroupOneOffsUnknownBaseFallsBackToTitleCase`: a location with `BaseID: "b495c6003fc83e18f6d8cecbe6929133"` and an empty `BaseName` produces a non-empty heading equal to `titleCase(BaseID)`.
  - `TestChainOnlyAtExclusiveStep`: a 3-step chain where step 2 has `ExclusiveBaseID: "treasure_cache_trading_post"` and `ExclusiveBaseName: "Treasure Cache Trading Post"` returns "Treasure Cache Trading Post".
  - `TestChainOnlyAtSharedSingleStation`: all steps located only at `starfall_salvage_station` return its name. One step also at a second base returns "".

- [ ] **Step 2: Run them and confirm they fail**

  Run: `go test ./cmd/generate-items-kb/ -run 'GroupOneOffs|ChainOnlyAt'`
  Expected: FAIL

- [ ] **Step 3: Implement**
  - Write `groupOneOffsByStation`, the two `Mission` methods and `chainOnlyAt` in `missionfamilies.go`, and add the `OnlyAt` field to `MissionChain`.
  - Set `OnlyAt` inside `chainsForType` when it builds each output chain.
  - The heading for a single-station group is the location's `BaseName`, or `titleCase(BaseID)` when that is empty.

- [ ] **Step 4: Run the tests and confirm they pass**

  Run: `go test ./cmd/generate-items-kb/`
  Expected: PASS

- [ ] **Step 5: Commit**

  ```bash
  git add cmd/generate-items-kb/missionfamilies.go cmd/generate-items-kb/missionchains.go cmd/generate-items-kb/missionfamilies_test.go
  git commit -m "missions: group one-offs by station; ONLY AT for exclusive missions and chains"
  ```

### Task 4: Render the new category and index pages

**Files:**
- Modify: `cmd/generate-items-kb/missions.go`. This covers `MissionCategoryInfo`, `writeMissionPages`, `htmlMissionsTopTemplate`, `htmlMissionsCategoryTemplate`, and deleting `missionTable` usage from category pages if it becomes unused.
- Modify: `cmd/generate-items-kb/missionchains.go`. Delete `partitionMissions` and its test if nothing else calls them.
- Modify: `kb/missions/missions.css`. It is hand-maintained, and the generator preserves it.
- Create: `cmd/generate-items-kb/missions_render_test.go`

**Interfaces:**
- Consumes: `partitionFamilies`, `groupOneOffsByStation`, `MissionFamily` with its summary fields, `StationGroup`, `Mission.OnlyAt()`, `Mission.StationCount()`, `MissionChain.OnlyAt`
- Produces:
  - `MissionCategoryInfo` loses `Standalone` and `Procedural` and gains `Families []*MissionFamily`, `OneOffGroups []StationGroup` and `FactionOneOffs []*Mission`.
  - New methods for the index cards: `OneOffCount() int` (server one-offs plus faction one-offs), `FamilyCount() int` and `VariantCount() int`.
  - `func renderCategoryPage(cat MissionCategoryInfo) ([]byte, error)`. It is extracted so tests can render without touching disk, and `writeMissionPages` uses it.

- [ ] **Step 1: Write the failing tests** in `missions_render_test.go`
  - `TestCategoryPageCollapsesFamilies`:
    - Build a trading category of 2,000 procedural "Trade Run: Good<i> to Station<i%7>" missions, each with XP `{trading:20+i%3*10}`, a deliver objective and one location. Run it through `partitionFamilies`.
    - Render it, then assert:
      - the output has exactly 1 `class="family-card"`
      - it contains `"Trade Run"` and `"20–40 trading XP"`
      - it does **not** contain `"trade_1999.html"`
      - `len(out) < 60_000`
  - `TestCategoryPageSectionsOrderAndBadges`:
    - Fixture:
      - 1 chain containing an exclusive step at "Treasure Cache Trading Post"
      - 1 one-off at "Traders' Rest & Resort"
      - 1 faction one-off
      - 1 family
    - Assert:
      - the section headings appear in the order "Mission Chains" < "One-off Missions" < "Posted by player factions" < "Template Families"
      - the output contains `ONLY AT Treasure Cache Trading Post`
      - the station heading is escaped as `Traders&#39; Rest &amp; Resort`
  - `TestTopIndexShowsBreakdownCounts`: rendering the top template with one category gives text containing `chains`, `one-offs` and `families` along with the variant count.

- [ ] **Step 2: Run them and confirm they fail**

  Run: `go test ./cmd/generate-items-kb/ -run 'CategoryPage|TopIndex'`
  Expected: FAIL

- [ ] **Step 3: Implement the templates and wiring**
  - **Category page sections, in order:**
    1. **Mission Chains.** Keep the existing table and add an `ONLY AT` badge after the chain when `.OnlyAt` is set.
    2. **One-off Missions.** Use the existing `missionTable` per station group, under an `<h4>` station heading. The many-stations group adds a "Stations" column showing `StationCount`. A per-mission `ONLY AT` badge goes in the title cell.
    3. **Posted by player factions.**
    4. **Template Families.**
  - **Family card** (`<div class="family-card">`), following the approved mockup layout A:
    - the name and variant count
    - `Pattern` in a `<code class="family-formula">`
    - the reward line, joining `Rewards.Parts` with " · "
    - difficulty range, station count and goods chips (+GoodsMore)
    - the ONLY AT badge
    - a `<details><summary>Breakdown by {{.BreakdownBy}}</summary>` table with the columns Label / Variants / Stations / Rewards, plus an "and N more" row
  - **CSS.** Add `.family-card`, `.family-formula`, `.only-at` (the amber badge) and `.station-group h4` to `missions.css`, using `hsl(var(--x))` theme tokens (bare HSL triplets, per the KB CSS gotcha). Check the result in both themes.
  - **Top index cards.** Show `{{.ChainCount}} chains · {{.OneOffCount}} one-offs · {{.FamilyCount}} families ({{.VariantCount}} variants)`.

- [ ] **Step 4: Run all tests and lint**

  Run: `go build ./... && go test ./cmd/generate-items-kb/ && golangci-lint run ./cmd/generate-items-kb/...`
  Expected: build OK, PASS, and no new lint findings

- [ ] **Step 5: Commit the code**

  ```bash
  git add cmd/generate-items-kb kb/missions/missions.css
  git commit -m "missions: family cards and station-grouped one-offs on category pages"
  ```

- [ ] **Step 6: Regenerate the mission pages and verify on real data**
  - Run `go run ./cmd/generate-items-kb`.
  - Then, check:
    - `ls -l kb/missions/*/index.html`. Expected: `trading/index.html` under 200 KB (it was 13 MB), and every category index under 500 KB.
    - `ls kb/missions/trading/*.html | wc -l`. Expected: unchanged from before the regen, about 19,000 detail pages, give or take normal agent churn.
    - `grep -c 'family-card' kb/missions/smuggling/index.html`. Expected: 8 (the smuggling families).
    - Screenshot `kb/missions/smuggling/index.html` with headless Chrome in light and dark themes. Confirm the chain ONLY AT Treasure Cache badge, the station headings and the readable cards.
    - Run `go test ./internal/kbnav/`. Expected: PASS.

- [ ] **Step 7: Commit only the mission pages**, keeping them separate from unrelated regen drift as the runbook advises:

  ```bash
  git add kb/missions
  git commit -m "chore(kb): regenerate mission pages with template families"
  git checkout -- kb && git clean -fdq kb/   # discard the unrelated drift this run produced
  ```
