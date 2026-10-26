# KB Guides Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `kb/guides/` section generated from hand-written markdown, starting with a "Building a Player Station" guide. Its build steps get build costs, upkeep and cross-links from the facility catalog, and a sticky sidebar shows the running station totals.

**Architecture:** `pkg/catalog` gains the facility cost/upkeep fields and an item loader. A new reusable `pkg/stationplan` does the maths (running build bill of materials, upkeep per cycle/day, power and life-support balance) and the link resolution. The future station planner reuses it. A new `cmd/generate-guides` reads `guides/*.md` (YAML frontmatter + goldmark body, the same pattern as `cmd/generate-factions-kb/overlay.go`) and writes `kb/guides/`.

**Tech Stack:** Go 1.25 (repo `go.mod`), `gopkg.in/yaml.v3`, `github.com/yuin/goldmark` (safe mode), `html/template`, `internal/kbnav`.

**Spec:** `docs/superpowers/specs/2026-10-25-kb-guides-section-design.md`

## Global Constraints

- Authors never type catalog numbers. Build cost, materials, time, upkeep and supply/draw all come from `catalog_facilities.json` in the latest snapshot (`data/snapshots/latest`). The only hand-typed figures are a non-facility step's `facts`, each with a `basis`.
- Upkeep per day = per cycle × **86.4** (100 ticks/cycle, 10 s/tick), rounded to the nearest integer (`math.Round`).
- Sidebar totals use **catalog** values. Measured values appear on the step card only, labelled `measured (<date>)`.
- `maintenance_fuel` is counted as the item id **`fuel`**.
- A step with alternatives: the **first** facility id is the default and the only one counted in the totals.
- Never emit a broken link. An unresolvable id renders as plain text and is reported on stderr.
- Facility page href from `kb/guides/`: `../facilities/<category>/<id>.html`. Item page href: `../items/<category>/<id>.html` (category from `catalog_items.json`).
- `golangci-lint run ./...` clean; `go test ./...` green.
- Page chrome: `kbnav.Header("../")`, `../smui.css` and `../system.css`, like `kb/wildlife/index.html`.

## Review Focus

- **Unknown facility id in a guide step:** generation fails with an error naming the guide file and step title, and writes nothing. → test in Task 4.
- **`[[id]]` that matches no facility or item:** renders as the bare id in plain text and is listed as a warning, with no `<a>`. → test in Task 3.
- **A step with no facility** (the fuel placeholder): renders its card and notes, and the totals carry over unchanged from the previous step. → test in Task 2.
- **A facility with no upkeep** (empty `maintenance_inputs` and no fuel): the card says "No upkeep" instead of an empty table. → test in Task 4.
- **A step whose draw exceeds supply:** the sidebar marks power or life support as a deficit at that step. → test in Task 2.

---

### Task 1: Catalog fields and item loader

**Files:**
- Modify: `pkg/catalog/catalog.go` (`Facility` struct ~line 31; add `Item`, `LoadItems`)
- Test: `pkg/catalog/catalog_test.go` (create it if absent)

**Interfaces:**
- Produces:
  - `catalog.Facility` adds: `Description string "json:\"description\""`, `BuildCost int "json:\"build_cost\""`, `BuildTime int "json:\"build_time\""`, `MaintenanceInputs []Material "json:\"maintenance_inputs\""`, `MaintenanceFuel int "json:\"maintenance_fuel\""`, `PowerSupply int "json:\"power_supply\""`, `PowerDraw int "json:\"power_draw\""`, `LifeSupportSupply int "json:\"life_support_supply\""`, `LifeSupportDraw int "json:\"life_support_draw\""`, `ServiceType string "json:\"service_type\""`. Keep the existing fields unchanged.
  - `type Item struct { ID, Name, Category string }` (json `id`, `name`, `category`)
  - `func LoadItems(root string) ([]Item, error)` reads `catalog_items.json` (top-level `items`) via the existing `loadCatalog`.

- [ ] **Step 1: Failing tests:** `TestLoadFacilities_UpkeepFields`. Write a temp snapshot dir `<root>/20261018/catalog_facilities.json` holding the real `atmospheric_recycler` entry (build_cost 600000, build_time 150, maintenance_inputs [sunspindle 1, water_ice 3, purified_water 1, oxygen_gas 2, catalytic_scrubber_cartridge 1], life_support_supply 12) and `bunker_fed_reactor` (maintenance_fuel 55, power_supply 12). Assert every field round-trips. `TestLoadItems`: `flex_polymer` → category `refined`.
- [ ] **Step 2: Run** `go test ./pkg/catalog/` → FAIL (unknown fields / undefined `LoadItems`).
- [ ] **Step 3: Implement** the fields and `LoadItems`.
- [ ] **Step 4: Run** `go test ./pkg/catalog/ ./cmd/generate-build-costs/ ./cmd/generate-bom-explorer/` → PASS (existing users unaffected).
- [ ] **Step 5: Commit** `feat(catalog): facility upkeep/supply fields and item loader`

### Task 2: `pkg/stationplan`: running totals

**Files:**
- Create: `pkg/stationplan/totals.go`, `pkg/stationplan/totals_test.go`

**Interfaces:**
- Consumes: `catalog.Facility` (Task 1)
- Produces:
  - `const CyclesPerDay = 86.4`
  - `func PerDay(perCycle int) int` (rounded)
  - `type StepInput struct { FacilityID string; ExtraCredits int }`. Empty `FacilityID` means a non-facility step; `ExtraCredits` covers e.g. the founding fee.
  - `type Totals struct { Credits int; Materials map[string]int; UpkeepPerCycle map[string]int; PowerSupply, PowerDraw, LifeSupportSupply, LifeSupportDraw int }`
  - `func (t Totals) PowerDeficit() bool` (draw > supply) and `func (t Totals) LifeSupportDeficit() bool`
  - `func Accumulate(facilities map[string]catalog.Facility, steps []StepInput) ([]Totals, error)` returns one cumulative `Totals` per step. An unknown id gives an error containing the id.

- [ ] **Step 1: Failing tests** (catalog values from the spec):
  - `TestPerDay`: 1→86, 2→173, 3→259, 55→4752.
  - `TestAccumulate_StationBuildOrder`: steps `{"",5000000}`, `{"locker_bay",0}`, `{"faction_workshop",0}`, `{"atmospheric_recycler",0}`. After the last step: Credits **6,600,000**; Materials steel_plate **3,600**, circuit_board 400, water_recycler 200; UpkeepPerCycle steel_plate 1, flex_polymer 1, circuit_board 1, focused_crystal 1, sunspindle 1, water_ice 3, purified_water 1, oxygen_gas 2, catalytic_scrubber_cartridge 1; LifeSupportSupply 12.
  - `TestAccumulate_NonFacilityStepCarriesTotals`: totals[2] equals totals[1] when step 2 is `{"",0}`.
  - `TestAccumulate_FuelIsAnItem`: a bunker_fed_reactor step gives `UpkeepPerCycle["fuel"] == 55` and PowerSupply 12.
  - `TestAccumulate_DeficitFlag`: with a fixture facility drawing power 6 and nothing supplying it, `PowerDeficit()` is true. After adding `stellar_collector_grid` (supply 20), it is false.
  - `TestAccumulate_UnknownFacility`: error mentions `no_such_facility`.
  - Each returned `Totals` is independent (mutating `totals[0].Materials` doesn't change `totals[1]`).
- [ ] **Step 2: Run** `go test ./pkg/stationplan/` → FAIL.
- [ ] **Step 3: Implement.** Deep-copy the maps per step.
- [ ] **Step 4: Run** → PASS; `golangci-lint run ./pkg/stationplan/...` clean.
- [ ] **Step 5: Commit** `feat(stationplan): cumulative build BoM, upkeep and power/life-support balance`

### Task 3: `pkg/stationplan`: cross-link resolution

**Files:**
- Create: `pkg/stationplan/links.go`, `pkg/stationplan/links_test.go`

**Interfaces:**
- Consumes: `catalog.Facility`, `catalog.Item` (Task 1)
- Produces:
  - `type Linker struct{ … }`; `func NewLinker(facs []catalog.Facility, items []catalog.Item, prefix string) *Linker` (`prefix` is the relative path to the kb root, `"../"` for `kb/guides/`)
  - `func (l *Linker) Href(id string) (string, bool)`: facility first, then item
  - `func (l *Linker) Name(id string) string`: display name, or the id if unknown
  - `func (l *Linker) ExpandRefs(md string) (string, []string)`: replaces each `[[id]]` with the markdown link `[Name](href)`, or the bare id when unresolvable. Returns the unresolved ids (deduplicated, in order of appearance).

- [ ] **Step 1: Failing tests:** `TestHref_FacilityAndItem` (`locker_bay` → `../facilities/service/locker_bay.html`; `flex_polymer` → `../items/refined/flex_polymer.html`). `TestExpandRefs` (`"Stock [[flex_polymer]] first"` → `"Stock [Flex Polymer](../items/refined/flex_polymer.html) first"`). `TestExpandRefs_Unknown` (`"[[nope]] and [[nope]]"` → `"nope and nope"`, unresolved `["nope"]`, and no `](` in the output).
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement** (regexp `\[\[([a-z0-9_]+)\]\]`).
- [ ] **Step 4: Run** → PASS; lint clean.
- [ ] **Step 5: Commit** `feat(stationplan): KB cross-link resolver with [[id]] expansion`

### Task 4: `cmd/generate-guides`

**Files:**
- Create: `cmd/generate-guides/main.go` (flags and orchestration), `cmd/generate-guides/guide.go` (frontmatter model and parse), `cmd/generate-guides/render.go` (templates and CSS), `cmd/generate-guides/generate_test.go`, `cmd/generate-guides/testdata/guides/test-guide.md`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces:
  - Flags: `-guides` (default `guides`), `-snapshots` (default `data/snapshots`), `-out` (default `kb/guides`).
  - Frontmatter schema (YAML between `---` lines; reuse the `splitFrontmatter` approach from `cmd/generate-factions-kb/overlay.go`):
    ```yaml
    title: Building a Player Station
    summary: One line for the guides index.
    intro: |            # markdown, rendered above the steps
    steps:
      - title: Found the station
        facts:          # non-facility steps only
          - {label: Founding fee, value: "5,000,000 cr", credits: 5000000, basis: "measured 2026-10-25"}
        notes: |        # markdown; [[id]] refs allowed
      - title: Power
        facility: stellar_collector_grid
        alternatives: [fuel_cell_generator, bunker_fed_reactor, scrap_reactor]
        notes: |
        measured:       # optional
          date: "2026-10-26"
          source: "Conjunction Junction storage diff"
          upkeep_per_cycle: {circuit_board: 2}
    ```
    The markdown body after the frontmatter renders **below** the steps (the reference section).
  - `func generate(guidesDir, snapshotsDir, outDir string) (warnings []string, err error)` writes `outDir/<slug>.html` per guide (slug = file name without `.md`) plus `outDir/index.html`.
- Card contents per step: title (facility name linked); "what it gives" line (service_type, power or life-support supply); **Build**: credits, materials (linked), build time in ticks; an **Upkeep** table (item linked, /cycle, /day), or "No upkeep"; notes; measured block if present; alternatives as compact rows (linked name, build credits, total upkeep/day, supply). **Sidebar** (sticky ≥ 900 px wide, a summary block above the steps below that): cumulative upkeep/day per item (linked), power `supply/draw` and life support `supply/draw` with a ⚠ deficit marker, build credits spent, and a collapsible `<details>` with the cumulative materials. The sidebar shows totals after the last step, and each card's footer shows "station so far: N items/day, power x/y" for that step.

- [ ] **Step 1: Failing tests** against `testdata/guides/test-guide.md` plus a temp snapshot built from the spec's facilities (locker_bay, atmospheric_recycler, stellar_collector_grid, fuel_cell_generator) and items:
  - `TestGenerate_WritesGuideAndIndex`: `test-guide.html` and `index.html` exist. The index links `test-guide.html` with the title and summary.
  - `TestGenerate_CardShowsCostsAndLinks`: the HTML contains `href="../facilities/service/locker_bay.html"`, `500,000`, `1,600`, `href="../items/refined/flex_polymer.html"`, and the per-day `86`.
  - `TestGenerate_NoUpkeepText`: a facility with no inputs or fuel renders `No upkeep`.
  - `TestGenerate_SidebarTotalsAndDeficit`: the sidebar contains the life support `12` supply. A fixture step drawing power with no supply renders the `⚠` deficit marker.
  - `TestGenerate_MeasuredShownBesideCatalog`: `measured (2026-10-26)` appears on that card.
  - `TestGenerate_UnknownFacilityFails`: a guide naming `no_such_facility` returns an error containing both the file name and the step title, and `-out` gets no `test-guide.html`.
  - `TestGenerate_UnknownRefWarns`: `[[nope]]` in notes → `warnings` contains `nope`, and the HTML has no `href` for it.
- [ ] **Step 2: Run** `go test ./cmd/generate-guides/` → FAIL.
- [ ] **Step 3: Implement.** Render markdown with `goldmark.New()` (safe) **after** `ExpandRefs`. Use `kbnav.Header("../")`. Format numbers with thousands separators.
- [ ] **Step 4: Run** → PASS; `golangci-lint run ./cmd/generate-guides/...` clean.
- [ ] **Step 5: Commit** `feat(guides): generate-guides renders hand-written guides with catalog-driven build steps`

### Task 5: Site navigation

**Files:**
- Modify: `internal/kbnav/kbnav.go` (`Items`: add `{"guides", "Guides"}` after `{"", "Home"}`), plus `internal/kbnav/*_test.go` where the expected list is pinned
- Modify: `kb/index.html`: the hard-coded nav (~line 210) gets `<a href="guides/index.html">Guides</a>` in the same position, and a new `<a href="guides/index.html" class="cat">` card after Wildlife (label `13 — Guides`, stat `1 guide`, glyph `&#x1F4D8;`, description "Hand-written walkthroughs the official docs leave thin, starting with building your own player station.", tags `Player Stations`, `Build Order`, `Upkeep`)
- Modify: `README.md`: `generate-guides` row in the generators table, `pkg/stationplan` row in the packages table, and a usage line `go run ./cmd/generate-guides`

- [ ] **Step 1:** Update the kbnav test expectation to include Guides; run `go test ./internal/kbnav/` → FAIL.
- [ ] **Step 2:** Edit `Items` → PASS.
- [ ] **Step 3:** Edit `kb/index.html` and `README.md`.
- [ ] **Step 4: Commit** `feat(kb): Guides in site nav and home page`

### Task 6: The Player Station guide

**Files:**
- Create: `guides/player-stations.md`
- Create (generated): `kb/guides/index.html`, `kb/guides/player-stations.html`

Content is written from `~/spacemolt/spacemolt/data/stations/cloverfield/findings.md`. Every hand-written claim gets its basis (`measured`, `catalog`, `doc-quoted`, `player-reported`) and date.
- **intro:** what a player station is; why own one (no rent, own storage and services, crossroads trade); the official docs omit what a station ships with, maintenance and power, and this records what we measured founding **Conjunction Junction** (Cloverfield, 2026-10-25).
- **steps:**
  0. *Found the station*: facts: fee 5,000,000 (credits 5000000, measured); requires Station Core in cargo, `manage_bases`, undocked, a lawless system with no station (doc-quoted); ships with 500k faction storage, 0 power, 0 life support (player-reported LT1428); **no personal storage** (`view_storage` → `no_storage`, measured). Notes: `build_base --name "…"`; it gets its own new POI and you're auto-docked (measured); the built-in storage satisfies the "Faction Storage first" prerequisite (measured).
  1. `locker_bay`: notes: stock [[flex_polymer]] **before** it finishes; rent 0 at your own station; +750 corporation_management XP to every member (measured).
  2. `faction_workshop`: ⚠ upkeep consumes [[focused_crystal]] (4 [[energy_crystal]] + 1 [[palladium_ore]] each, catalog recipe) ≈ 346 energy crystals/day.
  3. `atmospheric_recycler`: the costliest upkeep. [[water_ice]] is mineable in Cloverfield; source the others before building.
  4. *Power*: `stellar_collector_grid`, alternatives `fuel_cell_generator`, `bunker_fed_reactor`, `scrap_reactor`. Notes: services list no power draw (catalog) but production does; a star system makes the collector grid the simplest to supply.
  5. *Fuel*: no facility; notes: "Not yet charted: the player-reported order says fuel production comes next. This step will name the facility once we've built it."
- **body (reference):** the `station` command table (copied from findings, with the "✅ tested" markers), "Where the official docs are wrong" (storage contents, `found_station` vs `build_base`, storage capacity units), and "Not yet measured" (defense, measured upkeep).

- [ ] **Step 1:** Write `guides/player-stations.md`.
- [ ] **Step 2:** Run `go run ./cmd/generate-guides`. Expected: exit 0, and the warnings list is empty. Fix any `[[id]]` it reports; check exact item ids in `catalog_items.json`.
- [ ] **Step 3:** Open `kb/guides/player-stations.html` in a browser at desktop and narrow widths. Check the sidebar is sticky on desktop and stacks above the steps on narrow screens, the links resolve, and the life-support total reads 12.
- [ ] **Step 4: Commit** `docs(guides): Building a Player Station, first guide` (stage `guides/` and `kb/guides/` by path).
