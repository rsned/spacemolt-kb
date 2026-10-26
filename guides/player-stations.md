---
title: Building a Player Station
maintenance_cycle_ticks: 3000
maintenance_cycle_basis: "player-reported 2026-10-26, not yet measured (rent bills every 100 ticks; upkeep is a separate, longer cycle)"
summary: From Station Core to a running home station, with every facility's build cost and upkeep, measured at Conjunction Junction (Cloverfield).
intro: |
  A **player station** is a base your faction owns outright, founded in lawless space from a
  [[station_core]]. Owning one means no rent at NPC stations, your own storage and services,
  and a market other pilots can use if your station sits on a crossroads.

  The official guides leave the important parts out: what a new station actually comes with,
  what each facility consumes to keep running, and in what order to build. This guide records
  what we measured while founding **Conjunction Junction** in Cloverfield on 2026-10-25.
  Build costs and upkeep below come from the game catalog. Our own observations are labelled
  *measured*, other players' reports *player-reported*, and the official docs *doc-quoted*.

  > **The one rule:** a facility whose upkeep isn't in faction storage stops working, and one
  > unsatisfied facility takes the whole station from `thriving` to `critical` (*measured*).
  > Stock every input **before** the build finishes.
steps:
  - title: Found the station
    facts:
      - {label: Founding fee, value: "5,000,000 cr", credits: 5000000, basis: "measured 2026-10-25 (treasury first, then your wallet)"}
      - {label: Requires, value: "Station Core in cargo · manage_bases · undocked · lawless system with no station", basis: "doc-quoted"}
      - {label: Faction storage, value: "500k per item type, included", basis: "player-reported; deposits work on a bare station (measured)"}
      - {label: Power / life support, value: "0 / 0", basis: "player-reported"}
      - {label: Personal storage, value: "none (view_storage → no_storage)", basis: "measured"}
      - {label: Stock defense, value: "hull 60,000 · shield 12,000 · armor 15", basis: "measured (get_base)"}
      - {label: Fuel tank, value: "0 / 200", basis: "measured (get_base)"}
    notes: |
      `build_base --name "Your Station"` (the factions guide's command; base-builder's
      `found_station` is not what we used). The station anchors at **its own new point of
      interest** next to where you are, and your ship is docked there automatically (*measured*).

      The station's built-in faction storage satisfies the "Faction Storage is required before
      any other faction facility" rule: `faction_build` works straight away (*measured*).
  - title: Personal storage
    facility: locker_bay
    notes: |
      Adds personal storage for anyone docked. At your own station rent is **0**, and the build
      gave **750 corporation_management XP to every faction member** (*measured*).

      ⚠ We built it with no [[flex_polymer]] in faction storage, and the station went
      `critical` with zero services as soon as it finished (*measured*). Deposit a few days of
      [[flex_polymer]] and [[steel_plate]] first.
  - title: Crafting
    facility: faction_workshop
    notes: |
      ⚠ Upkeep consumes [[focused_crystal]], each made from 4 [[energy_crystal]] + 1
      [[palladium_ore]]: about 3 a day at a ~3,000-tick maintenance cycle, so roughly **12 energy
      crystals a day**. Decide where those come from before building it.
  - title: Life support
    facility: atmospheric_recycler
    notes: |
      The heaviest upkeep of the early build: five different inputs every cycle.
      [[water_ice]] is usually mineable nearby (Cloverfield has an ice field); plan a supply of
      the other four before you build. Reported as the third build (*player-reported*).
  - title: Power
    facility: stellar_collector_grid
    alternatives: [fuel_cell_generator, bunker_fed_reactor, scrap_reactor]
    notes: |
      Service facilities list **no power draw** in the catalog, but production facilities do,
      so power is what unlocks production lines. In a system with a star the collector grid is
      the simplest to keep fed: 2 [[circuit_board]] per cycle and nothing else. The Bunker-Fed
      Reactor burns [[fuel]] instead: 55 per cycle, about 158 a day.
  - title: Fuel production
    notes: |
      *Not yet charted.* Other players report fuel production comes next once storage,
      crafting and life support are up. This step will name the facility once we have built it.
---
## Reading your station

The official guides don't say how to tell what a station needs. These are the places the
server does tell you, all *measured* at Conjunction Junction and Grand Exchange in October 2026.

| Question | Command | Field to read |
|---|---|---|
| Is the station healthy overall? | `get_base`, or the reply when you dock | `condition` (`thriving` … `critical`), `satisfied_count` / `total_service_infra`, `satisfaction_pct` |
| Which facility is starving? | `facility list` | `maintenance_satisfied` (true/false) and `maintenance_level` (0–1) per facility; the description switches to its *degraded* text |
| What does a facility consume? | `facility types --facility_type <id>` | `maintenance_per_cycle` (also shown on some facilities in `facility list`) |
| What is life support short of right now? | `facility list` or `get_base` | `life_support.starved` and `life_support.remediation` (only present when short), against `supply`, `demand`, `maintenance` |
| Is there enough power? | `facility list` | `power.supply` vs `power.current_draw`, `battery_stored` / `battery_capacity`, `efficiency` |
| What is a build still waiting on? | `facility list` | `construction.pending[].status` (`waiting_for_materials`) and `materials[].quantity_missing` |
| What does it cost the treasury? | `facility list` | `labor_per_cycle` per facility; `faction_rent` (0 at your own station) |

**What the server doesn't tell you:** for a *service* facility (the Locker Bay, the Faction
Workshop) there is no "starved" list, only `maintenance_satisfied: false`. To know what to haul,
compare the facility's `maintenance_per_cycle` (the upkeep tables in the steps above) against
`view_faction_storage` at the station.

**Two clocks (*player-reported*, being measured):** facility **rent** bills every **100 ticks**
(about 17 minutes), but facilities **consume their upkeep** on a separate maintenance cycle of
about **3,000 ticks** (about 8.3 hours, roughly 2.9 times a day). The per-day upkeep figures on
this page use the 3,000-tick cycle. Life support may run on its own 100-tick cycle:
Grand Exchange reports `life_support.maintenance_cycle_ticks: 100`.

**What happens when it runs dry (*measured*):** the Locker Bay went `maintenance_satisfied:
false` with `maintenance_level` 0.4, the station dropped from `thriving` to `critical`
(*"Critical infrastructure failure. Emergency lighting only."*), and `get_base` listed **no
services at all**, so personal storage stopped working.

## Station administration

Run these while docked at your own station. `info` is open to any member; every other
action needs the **ManageBases** permission. Outposts support only `info`, `set_name` and
`set_description`. Rows marked ✅ have been run live.

| Action | Params | play_as example | Raw payload | What it does |
|---|---|---|---|---|
| `info` ✅ 2026-10-25 | — | `station info` | `{"action":"info"}` | Current config: name, description, public_access, allow_outsider_facilities, auto_buy_fuel, contraband_mode/items, service_access (once services exist). |
| `set_name` | `name` | `station set_name --name "Conjunction Junction"` | `{"action":"set_name","name":"Conjunction Junction"}` | Rename the station. |
| `set_description` ✅ 2026-10-25 | `description` (≤500 chars) | `station set_description --description "CRFT home. Crossroads of Haven and the Rim."` | `{"action":"set_description","description":"…"}` | Station blurb shown on dock. |
| `set_public` ✅ reply seen 2026-10-25 | `public` true/false | `station set_public true` | `{"action":"set_public","public":true}` | When false, only the owning faction, allowed factions and allowed players may dock. |
| `set_build_policy` | `allow_outsiders` true/false | `station set_build_policy --allow_outsiders true` | `{"action":"set_build_policy","allow_outsiders":true}` | Let non-members build their own facilities. Tenants pay rent each cycle to our treasury. |
| `set_service_access` | `service` = market\|refuel\|repair\|shipyard\|crafting\|salvage_yard\|missions, `access` = public\|allies\|faction | `station set_service_access --service refuel --access public` | `{"action":"set_service_access","service":"refuel","access":"public"}` | Gate one service to the faction, allies, or everyone. |
| `set_market_fee` | `fee_percent` 0–10 | `station set_market_fee --fee_percent 3` | `{"action":"set_market_fee","fee_percent":3}` | Listing fee outside traders pay; goes to our treasury. |
| `set_refuel_price` | `price` (per fuel unit) | `station set_refuel_price --price 12` | `{"action":"set_refuel_price","price":12}` | What outside pilots pay per unit of fuel; to treasury. |
| `set_repair_price` | `price` (per hull point) | `station set_repair_price --price 4` | `{"action":"set_repair_price","price":4}` | What outside pilots pay per hull point repaired; to treasury. |
| `set_auto_buy_fuel` | `auto_buy_fuel` true/false | `station set_auto_buy_fuel --auto_buy_fuel true` | `{"action":"set_auto_buy_fuel","auto_buy_fuel":true}` | Off by default. When on, any docked pilot can sell fuel from their tank into our shared tank at live scarcity prices, paid from the treasury (capped to what it can afford). |
| `set_contraband` | `contraband_items` (array of item ids) | `station set_contraband --contraband_items pirate_moonshine,void_spice` | `{"action":"set_contraband","contraband_items":["pirate_moonshine"]}` | Replaces this station market's contraband (banned) list. `[]` allows **all** items. Smuggling skill bypass still applies; empire customs unchanged. |
| `reset_contraband` | — | `station reset_contraband` | `{"action":"reset_contraband"}` | Back to the default contraband rules. |
| `allow_player` | `player` (id or username) | `station allow_player --player marketbot_home` | `{"action":"allow_player","player":"marketbot_home"}` | Let a non-member dock while the station is private. |
| `remove_player` | `player` | `station remove_player --player marketbot_home` | `{"action":"remove_player","player":"marketbot_home"}` | Take a player off the allow list. |
| `ban` | `player` | `station ban --player <username>` | `{"action":"ban","player":"<username>"}` | Blocks docking immediately and drops them from the allow list. |
| `unban` | `player` | `station unban --player <username>` | `{"action":"unban","player":"<username>"}` | Lift a ban. |
| `allow_faction` | `faction` (id) | `station allow_faction --faction 6686e24354619eadffc738b607425457` | `{"action":"allow_faction","faction":"6686e…"}` | Let a whole faction dock while private (example id: Explorers Guild, a CRFT ally). |
| `remove_faction` | `faction` (id) | `station remove_faction --faction 6686e24354619eadffc738b607425457` | `{"action":"remove_faction","faction":"6686e…"}` | Remove that faction's access. |

> A missing value is **not refused**: the server reads it as false or 0. `set_public` with no
> value makes a station private (*measured*).

## Where the official docs are wrong

- **What a station comes with.** The factions guide says market, refuel, repair, shipyard,
  crafting, power and life support are included. A fresh station has faction storage only,
  with 0 power, 0 life support and no services (*measured / player-reported*).
- **Founding command.** base-builder says `facility action=found_station`; what works is
  `build_base` (*measured*).
- **Storage capacity units.** base-builder lists faction storage in total items (500 → 50,000);
  the factions guide lists per item type (100k → 500k). The station's 500k is per item type
  (*player-reported*).

## Not yet measured

- Real upkeep consumption per cycle (catalog figures are shown above until we have it).
- How fast a `critical` station recovers once its upkeep is stocked.
- Defense: guns, ammo and shields.
