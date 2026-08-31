# Station repair-cost exercise (2026-08-30)

Grand Exchange Station post-wreck repair bill: 30% of build materials for
the base level + every upgrade level of each damaged facility, walked via
`upgrades_from` chains in `data/snapshots/latest/catalog_facilities.json`.

- `damaged.json` — the 53 damaged facilities (from the game's repairs list)
- `levels.json` — instance_id → level (from station_facilities)
- `compute.py` — chain walk + 30% + market pricing (needs crafting.db)
- `result.json` — full per-facility breakdown, uncapped (~1.63M units)
- `result_capped.json` — with the NPC-station cap of 2000 per item per
  facility (75,129 units total)
