# Facility Build-Cost Pages — Design

**Date:** 2026-07-06
**Status:** Approved (pending spec review)

## Summary

Add a facility build-cost section to the KB that estimates, for **every facility**,
the market cost of **constructing** it, shown two ways (a fully-flattened **BoM**
view and a direct-components **Recipe** view) and priced two ways (a market-average
estimate and a galaxy-wide cheapest depth-walked estimate).

Pages mirror the existing `items` and `recipes` KB sections: a landing index of
category-group cards, then one page per group with a horizontal cross-group TOC,
listing each facility's two cost tables inline.

This reuses the market/order-book loading already built for the item/ship build-cost
matrix (`cmd/generate-build-costs`), extending that binary rather than adding a new one.

## Background & data model

A facility has two distinct "recipes" that must not be conflated:

1. **Production recipe** (`recipe_id`, e.g. `build_piercing_railgun_i`) — what the
   facility *makes* when operating (output: `piercing_railgun_i`, a weapon). Its
   inputs are what the facility *consumes* to produce output. **Used only for
   grouping** (by the produced item's market category). Not a construction cost.

2. **Construction bill** (`build_materials`, e.g. `steel_plate ×6400`,
   `weapon_housing ×2600`, …) — the direct components needed to *build the facility
   itself*. This is the cost we care about. No recipe object outputs the facility id
   itself; `build_materials` **is** the construction bill.

The two **cost views** are both about constructing the facility:

- **BoM (ore) view** — `build_materials` fully flattened to base materials (ores /
  raw materials). Already precomputed in the crafting DB `bill_of_materials` table
  with `target_type='facility'` (2,440 facilities present).
- **Recipe (components) view** — the facility's direct `build_materials` as listed
  (some entries are intermediates like `steel_plate`, `weapon_housing`), one level
  above the flattened BoM.

A facility builds as **one unit**, so each table shows a single **build-cost total**
(no per-unit / units-per-run line).

## Pricing

Each component in each view is priced two ways:

- **MKT-AVG** — the component's sell-side (ask) volume-weighted average price from
  `market_ohlcv` (`side='sell'`), the same `loadSellVWAP` reference the matrix uses.
  Line cost = `qty × VWAP[component]`. A component with no VWAP renders `—` and is
  excluded from the total, which is then marked as covering only *k/N* components.
- **Galaxy (cheapest, depth-walked)** — pool every station's resting **sell** ladder
  for the component into one combined ascending ladder, then walk it depth-first to
  cover the required `qty` (`buildcost.Walk` / `PriceRequirements` semantics). This is
  what you'd actually pay to source the inputs galaxy-wide. When pooled depth cannot
  cover the required quantity the view is **infeasible**; show covered/total component
  coverage (`N/M`) rather than a misleading total, matching the existing matrix pages.

The outlier price cap (`price-cap-mult`, VWAP-referenced) that the matrix already
applies when loading books is reused, so sentinel "not for sale" sell orders don't
distort the galaxy walk.

## Grouping & navigation

Structure mirrors `kb/items` and `kb/recipes` (index → per-group page):

- `kb/build-costs/facilities/index.html` — landing. One card per group (name +
  facility count) plus a legend explaining the two views and two price columns.
- `kb/build-costs/facilities/<group>/index.html` — one page per group. A **horizontal
  TOC** across all groups sits at the top (active group highlighted, others linked to
  their sibling pages). Facilities are listed alphabetically within the group; each is
  an anchored `<section id="<facility_id>">` whose heading links to the facility's
  existing `kb/facilities/<category>/<id>.html` detail page and notes what it produces.
  The section body holds the **BoM (ore)** table and the **Recipe (components)** table.

Groups:

- **Production facilities** → grouped by the **market category of the produced item**
  (`recipe_id` → `recipe_outputs` → item category): `weapon`, `ammo`, `component`,
  `refined`, `utility`, `defense`, `consumable`, `contraband`, `mining`, `drone`,
  `material`, `ore`. Production facilities whose `recipe_id` has no resolvable output,
  or whose output has no market category (~210 total), fall into an **`other`** group.
- **Non-production facilities** → grouped by **facility category**: `service`,
  `infrastructure`, `faction`, `personal`.

Production group names reuse the item market-category vocabulary (same names as the
`kb/items` subdirectories); non-production group names are the facility categories.
These vocabularies are disjoint, so group directory names do not collide.

Approximate group sizes (latest snapshot): component 558, refined 468, weapon 260,
utility 252, other ~210, ammo 184, defense 172, consumable 149, service 68,
infrastructure 65, contraband 40, faction 43, mining 36, drone 20, material/ore 16,
personal 13. ~17 groups total.

## Component tables

Each facility section renders two tables with identical columns:

```
Facility Name  (→ detail page)   Level N   produces: <item>

BoM (ore)
  COMPONENT        QTY   MKT-AVG(unit)   MKT-AVG(total)   GALAXY(unit)   GALAXY(total)
  copper_ore         2         25.38           50.76          24.10          48.20
  titanium_ore       8       3579.67       28,637.36        3421.00      27,368.00
  ---- build cost                          28,762.90                     27,508.00

Recipe (components)
  COMPONENT        QTY   MKT-AVG(unit)   MKT-AVG(total)   GALAXY(unit)   GALAXY(total)
  steel_plate     6400          …               …             …              …
  ---- build cost                              …                            …
```

- A `—` in a MKT-AVG cell means the component has no sell VWAP; a `—` / `N/M` in the
  GALAXY column means pooled depth could not cover the required quantity.
- Component names link to their `kb/items/<category>/<id>.html` page when categorized.
- Totals: MKT-AVG total is the sum over priced components (annotated `k/N priced` when
  incomplete); GALAXY total is shown only when feasible, otherwise the coverage `N/M`.

## Implementation

Extend `cmd/generate-build-costs` (do not add a new binary):

- New `facilities.go` — loads the facility catalog (`loadFacilities`-equivalent from
  the latest `catalog_facilities.json`), loads facility BoM rows from
  `bill_of_materials` where `target_type='facility'`, resolves each production
  facility's produced-item category via `recipe_id` → `recipe_outputs` → `items`,
  assigns groups, and builds the per-facility BoM + Recipe cost views using the
  already-loaded `books`, `sellVWAP`, and a new pooled **galaxy book**.
- New helper `galaxyBook(books)` — merges every station's sell ladder per item into
  one ascending combined ladder, returning a `*buildcost.Book` the galaxy walk prices
  against. (BestBuy is irrelevant here.)
- New templates under `cmd/generate-build-costs/templates/` for the facilities landing
  and per-group pages, styled consistently with the existing matrix/detail pages.
- A `-facilities-out` flag (default `kb/build-costs/facilities`, empty disables) gates
  generation, wired into `main.go` after the existing matrix/detail/station-cover
  steps, reusing the already-loaded DBs and books. Logs a one-line summary
  (groups, facilities, feasible-galaxy count).

Reused as-is: `loadBooks`, `loadSellVWAP`, `loadStations` (not needed unless we want
station context — galaxy pooling makes stations unnecessary here), `commaInt`,
`buildcost.PriceRequirements` / `Walk`, the outlier cap.

## Testing

- `galaxyBook` merge+sort: multiple stations' ladders for one item pool into a single
  ascending ladder; walk cost matches hand-computed depth-fill; infeasible when pooled
  depth < required qty.
- Grouping: a production facility with a resolvable weapon output lands in `weapon`; one
  with no recipe output / uncategorized output lands in `other`; each non-production
  category maps to its own group.
- BoM vs Recipe views draw from `bill_of_materials` (flattened) and `build_materials`
  (direct) respectively for the same facility.
- MKT-AVG total annotation reflects `k/N` priced components; a component missing VWAP
  renders `—` and is excluded from the total.
- Render: a group page emits one anchored section per facility, a cross-group TOC with
  the active group marked, and facility-name → detail-page links.

## Out of scope

- Per-station facility cost columns / a facility × station matrix (galaxy-pooled only).
- Hop-radius tabs (the item matrix's +1/+2/+3 pooling); galaxy-wide is the single reach.
- Maintenance-per-cycle costs, labor cost, rent — construction inputs only.
- Buy-side (bid) valuation of inputs.
