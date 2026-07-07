# Facility “Other” bucket — recipe_outputs coverage gap (RESOLVED 2026-07-07)

_Data audit from `catalog_facilities.json` + `crafting.db`. Not a KB page._

**RESOLVED.** The 2026-07-07 crafting-DB refresh (now **666 recipes, 664 with a
`recipe_outputs` row**) closed this gap. The facilities build-cost **Other**
group dropped from **186 production facilities → 3**, and the 183 regrouped by
produced-item category on regen.

**What remains in “Other” (3 facilities, by design):** `recycling_processor_mk_i`,
`recycling_processor_mk_ii`, `recycling_processor_mk_iii` — recyclers with a
**blank `recipe_id`** and no craftable output, so there is nothing to group them
by. Leave them in “Other” (or drop them from the production category upstream).

**Faction facilities** remain distinguishable via the catalog's `empire` /
`pirate_base_only` / `station_or_faction_only` fields — e.g. **The Wire Room**
(`empire: pirates`, `unique: true`), whose recipe now resolves normally, so it
no longer lands in “Other.” Surfacing an `empire`/faction badge on facility
pages is still an open enhancement.

_To re-audit after a future scrape: production facilities whose `recipe_id` is
absent from `recipe_outputs` (see git history of this file for the query)._
