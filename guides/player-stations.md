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
      Build this second, so the faction can craft at the station straight away. Crafting by
      hand is slow next to real production lines, but it means build materials and upkeep
      items can be made on site until those lines come online.

      ⚠ Upkeep consumes one [[focused_crystal]] a cycle, and each is made from 4
      [[energy_crystal]] + 1 [[palladium_ore]], so **4 energy crystals every cycle**. Decide
      where those come from before building it.
  - title: Refuel service
    facility: refueling_dock
    notes: |
      Third, because everyone needs fuel: your own miners and haulers working out of the
      station, and every pilot passing through. Faction members draw from the shared tank for
      free; everyone else pays your station price (set with `station set_refuel_price`). Until
      fuel production comes online, keep the tank filled by tankers depositing fuel from their
      own tanks (storage deposit, `item_id=fuel`). Its upkeep is
      [[fuel_cell]] and [[liquid_hydrogen]], so plan both supplies before it finishes.

      Until a refuel service exists, ships docked at your station can't refuel at all; the
      server's message (`no_fuel_source … insufficient credits for station refueling`) is
      misleading, because credits are not the problem (*measured*).
  - title: Fuel storage
    facility: fuel_tank_expansion
    notes: |
      ⚠ **Don't build a [[faction_fuel_bunker]] here.** At your own station the server refuses
      it: *"a private Faction Fuel Bunker doesn't make sense on your own station — its fuel
      would just sit unused. Build a Fuel Tank Expansion … instead"* (*measured* 2026-10-26).

      Each expansion adds 50,000 to the station's own shared tank (the bare station tank holds
      200), and you can build several. Fill it from fuel-production facilities or by depositing
      a docked ship's fuel (storage deposit, `item_id=fuel`). With `station set_auto_buy_fuel`
      on, visiting pilots can sell fuel into the tank too, paid from the treasury.
      Whether the expansions feed the refueling dock's 5,000-unit tank is *not yet measured*.
  - title: Market
    facility: trade_concourse
    notes: |
      The rest of a full station is its **services**. A mature station lists eleven:
      `crafting`, `marine_training`, `market`, `medical`, `missions`, `personnel`, `refuel`,
      `repair`, `salvage_yard`, `shipyard` and `storage` (*player-reported*). Each has a level-1
      facility a player station can build.

      The market comes right after fuel because it starts the station earning: crossroads
      traffic trades here, and you set the listing fee they pay with
      `station set_market_fee`. The catalog lists its upkeep as one [[trade_authenticator]] a
      cycle, but players have found the authenticators go **per trade**, possibly on top of
      the per-cycle charge, so a busy market eats them much faster than any other upkeep
      (*player-reported*). Line up a steady source before you build, sized to your trade
      volume rather than to the cycle.
  - title: Life support
    facility: atmospheric_recycler
    break_before: |
      **Next: production support.** With storage, crafting, fuel and a market in place, we
      switch from basic services to power and life support, the two things production
      facilities need, so production lines can start moving to the station.
    notes: |
      The heaviest upkeep of the early build: five different inputs every cycle.
      [[water_ice]] is usually mineable nearby (Cloverfield has an ice field); plan a supply of
      the other four before you build. Other players build it third (*player-reported*); we
      build it after the market, together with power, because the two of them are what
      production facilities need: this is where more advanced crafting and processing can
      move to the station.

      There is no other level-1 life-support facility a player station can build. The
      recycler upgrades to [[climate_control_array]] (life support 20), [[biosphere_ring]] (35)
      and [[full_habitat_dome]] (60).

      **Upgrade, don't stack.** When you need more life support, upgrade toward the
      [[full_habitat_dome]] rather than building more recyclers. One station that stacked
      recyclers spends roughly twice the consumables of a few domes for the same life support
      (*player-reported*). The upgrades cost more credits up front, but upkeep never stops, so
      the domes come out ahead over time.
  - title: Power
    facility: stellar_collector_grid
    alternatives: [fuel_cell_generator, bunker_fed_reactor, scrap_reactor, dark_matter_reactor]
    notes: |
      Service facilities list **no power draw** in the catalog, but production facilities do,
      so power, with life support, is what unlocks production lines at the station.
      [[capacitor_bank]], [[buffer_array]] and [[cryo_capacitor_vault]] add battery storage
      on top of any of these. In a system with a star the collector grid is
      the simplest to keep fed: 2 [[circuit_board]] per cycle and nothing else. The Bunker-Fed
      Reactor burns [[fuel]] instead: 55 per cycle.

      **Size power for your defenses, not your production.** Production facilities draw
      little unless every one of them runs at once, and players report never getting near the
      power limit that way. In combat, production stops and the defenses get all the power.
      So build only enough to run your defense facilities at peak (*player-reported*). One
      experienced builder would rebuild all their power as Bunker-Fed Reactors if
      infrastructure could be dismantled; whether it can is *not yet known*.

      We built the grid at Conjunction Junction on 2026-10-26; the build reply again gave
      750 corporation_management XP to every member and showed rent 0. Once it finished,
      `get_base` gained a `power` block (absent until then) showing `supply: 20`, matching the
      catalog, and the grid counted toward station health alongside the Locker Bay
      (`satisfied_count 2 / total_service_infra 2`) (*measured*).
  - title: Shipyard
    facility: outfitting_bay
    break_before: |
      **From here on the order is flexible.** With storage, crafting, fuel, a market and
      production support in place, build the remaining services as you need them. Our
      suggestion is shipyard, repair and salvage yard first, then missions, medical, personnel
      and marine training.
    notes: |
      The most expensive level-1 service to build: thousands of [[steel_plate]] plus
      [[circuit_board]], and the longest build time. Upkeep includes [[titanium_alloy]].
      It upgrades to [[fabrication_yard]], [[assembly_complex]] and [[grand_shipworks]].
  - title: Repair
    facility: repair_slip
    notes: |
      Outside pilots pay your `station set_repair_price` per hull point. Upkeep is
      [[repair_kit]] and [[steel_plate]].
  - title: Salvage yard
    facility: reclamation_dock
    notes: |
      Breaks down what pilots drag in from debris fields. Upkeep includes a [[fuel_cell]],
      like the refueling dock.
  - title: Missions
    facility: contract_terminal
    notes: |
      A job board for the faction or anyone passing through. Cheap to build and to keep:
      [[circuit_board]] and [[copper_wiring]].
  - title: Medical
    facility: first_aid_ward
    notes: |
      It **does** have upkeep, listed in a different place from the usual upkeep: a service
      pool fed by [[medical_supplies]] (catalog `service_pool_supply_item`). Running out
      hurts the station's operational status, and the pool may refill on a different cycle
      from the regular consumables (*player-reported*). The catalog gives the pool 32 units,
      refilling 8 per cycle at 5 units per supply. Our reading is about 1.6 medical_supplies a
      cycle, *not yet measured*. The build also needs [[medical_supplies]]. It upgrades to
      [[trauma_clinic]], [[surgical_center]] and [[regenerative_medical_complex]], each with a
      four-times-larger pool.
  - title: Personnel
    facility: crew_registry
    notes: |
      Upkeep is a service pool fed by [[protein_rations]], listed apart from the usual upkeep;
      running out hurts operational status (*player-reported*). The catalog pool is 32 units,
      refilling 8 per cycle at 8 units per ration: about 1 protein_rations a cycle by our
      reading, *not yet measured*. It upgrades to [[personnel_bureau]],
      [[sector_labor_exchange]] and [[fleet_personnel_command]].
  - title: Marine training
    facility: security_training_post
    notes: |
      Upkeep is a service pool fed by [[protein_rations]], listed apart from the usual upkeep;
      running out hurts operational status (*player-reported*). The catalog pool is 8 units,
      refilling 2 per cycle at 2 units per ration: about 1 protein_rations a cycle by our
      reading, *not yet measured*. The upgrades use rations faster (2 units each).
      It upgrades to [[marine_training_barracks]], [[fleet_marine_academy]] and
      [[expeditionary_warfare_command]].
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
| Is there enough power? | `get_base` or `facility list` | `power.supply` vs `power.current_draw`, `battery_stored` / `battery_capacity`, `efficiency`. `get_base` shows the `power` block only once a power source exists |
| What is a build still waiting on? | `facility list` | `construction.pending[].status` (`waiting_for_materials`) and `materials[].quantity_missing` |
| What does it cost the treasury? | `facility list` | `labor_per_cycle` per facility; `faction_rent` (0 at your own station) |

**What the server doesn't tell you:** for a *service* facility (the Locker Bay, the Faction
Workshop) there is no "starved" list, only `maintenance_satisfied: false`. To know what to haul,
compare the facility's `maintenance_per_cycle` (the upkeep tables in the steps above) against
`view_faction_storage` at the station.

**Two clocks (*player-reported*, being measured):** facility **rent** bills every **100 ticks**
(about 17 minutes), but facilities **consume their upkeep** on a separate maintenance cycle of
about **3,000 ticks** (about 8.3 hours). Until we have measured that cycle, this page gives
upkeep **per cycle** only; per-day figures will come back once the length is confirmed. Life support may run on its own 100-tick cycle:
Grand Exchange reports `life_support.maintenance_cycle_ticks: 100`.

**What happens when it runs dry (*measured*):** the Locker Bay went `maintenance_satisfied:
false` with `maintenance_level` 0.4, the station dropped from `thriving` to `critical`
(*"Critical infrastructure failure. Emergency lighting only."*), and `get_base` listed **no
services at all**, so personal storage stopped working.

## Station administration

Run these while docked at your own station. `info` is open to any member; every other
action needs the **ManageBases** permission. Outposts support only `info`, `set_name` and
`set_description`. Rows marked ✅ have been run live.

| Action | Params | v1 client example | Raw payload | What it does |
|---|---|---|---|---|
| `info` ✅ 2026-10-25 | — | `station action=info` | `{"action":"info"}` | Current config: name, description, public_access, allow_outsider_facilities, auto_buy_fuel, contraband_mode/items, service_access (once services exist). |
| `set_name` | `name` | `station action=set_name name="Conjunction Junction"` | `{"action":"set_name","name":"Conjunction Junction"}` | Rename the station. |
| `set_description` ✅ 2026-10-25 | `description` (≤500 chars) | `station action=set_description description="CRFT home. Crossroads of Haven and the Rim."` | `{"action":"set_description","description":"…"}` | Station blurb shown on dock. |
| `set_public` ✅ reply seen 2026-10-25 | `public` true/false | `station action=set_public public=true` | `{"action":"set_public","public":true}` | When false, only the owning faction, allowed factions and allowed players may dock. |
| `set_build_policy` | `allow_outsiders` true/false | `station action=set_build_policy allow_outsiders=true` | `{"action":"set_build_policy","allow_outsiders":true}` | Let non-members build their own facilities. Tenants pay rent each cycle to our treasury. |
| `set_service_access` | `service` = market\|refuel\|repair\|shipyard\|crafting\|salvage_yard\|missions, `access` = public\|allies\|faction | `station action=set_service_access service=refuel access=public` | `{"action":"set_service_access","service":"refuel","access":"public"}` | Gate one service to the faction, allies, or everyone. |
| `set_market_fee` | `fee_percent` 0–10 | `station action=set_market_fee fee_percent=3` | `{"action":"set_market_fee","fee_percent":3}` | Listing fee outside traders pay; goes to our treasury. |
| `set_refuel_price` | `price` (per fuel unit) | `station action=set_refuel_price price=12` | `{"action":"set_refuel_price","price":12}` | What outside pilots pay per unit of fuel; to treasury. |
| `set_repair_price` | `price` (per hull point) | `station action=set_repair_price price=4` | `{"action":"set_repair_price","price":4}` | What outside pilots pay per hull point repaired; to treasury. |
| `set_auto_buy_fuel` | `auto_buy_fuel` true/false | `station action=set_auto_buy_fuel auto_buy_fuel=true` | `{"action":"set_auto_buy_fuel","auto_buy_fuel":true}` | Off by default. When on, any docked pilot can sell fuel from their tank into our shared tank at live scarcity prices, paid from the treasury (capped to what it can afford). |
| `set_contraband` | `contraband_items` (array of item ids) | `station action=set_contraband contraband_items=[pirate_moonshine,void_spice]` | `{"action":"set_contraband","contraband_items":["pirate_moonshine"]}` | Replaces this station market's contraband (banned) list. `[]` allows **all** items. Smuggling skill bypass still applies; empire customs unchanged. |
| `reset_contraband` | — | `station action=reset_contraband` | `{"action":"reset_contraband"}` | Back to the default contraband rules. |
| `allow_player` | `player` (id or username) | `station action=allow_player player=marketbot_home` | `{"action":"allow_player","player":"marketbot_home"}` | Let a non-member dock while the station is private. |
| `remove_player` | `player` | `station action=remove_player player=marketbot_home` | `{"action":"remove_player","player":"marketbot_home"}` | Take a player off the allow list. |
| `ban` | `player` | `station action=ban player=<username>` | `{"action":"ban","player":"<username>"}` | Blocks docking immediately and drops them from the allow list. |
| `unban` | `player` | `station action=unban player=<username>` | `{"action":"unban","player":"<username>"}` | Lift a ban. |
| `allow_faction` | `faction` (id) | `station action=allow_faction faction=6686e24354619eadffc738b607425457` | `{"action":"allow_faction","faction":"6686e…"}` | Let a whole faction dock while private (example id: Explorers Guild, a CRFT ally). |
| `remove_faction` | `faction` (id) | `station action=remove_faction faction=6686e24354619eadffc738b607425457` | `{"action":"remove_faction","faction":"6686e…"}` | Remove that faction's access. |

> A missing value is **not refused**: the server reads it as false or 0. `set_public` with no
> value makes a station private (*measured*).

## What you can build here

The facility catalog marks every facility a player station can build with
**`player_station_buildable: true`**: 63 of the catalog's 2,907 facilities (catalog read
2026-10-26). Plan from that flag, not from the official guides. Besides the steps above, it covers:

- **Power:** [[stellar_collector_grid]], [[fuel_cell_generator]], [[bunker_fed_reactor]],
  [[scrap_reactor]], [[dark_matter_reactor]], plus [[capacitor_bank]] and [[buffer_array]] storage.
- **Services for visitors:** [[refueling_dock]], [[trade_concourse]], [[repair_slip]],
  [[outfitting_bay]], [[contract_terminal]], [[reclamation_dock]], [[fabrication_yard]].
- **Defense:** [[point_defense_battery]], [[station_defense_turret]], [[shield_projector]],
  [[reinforced_plating]], up to [[siege_lance]] and [[shield_bastion]].
- **Food and leisure venues:** [[dockside_diner]], [[frontier_cantina]], [[rec_lounge]] and others.

The `faction_*` facilities you rent space for at NPC stations (fuel bunkers and similar) are
**not** on the list.

## Where the official docs are wrong

- **What a station comes with.** The factions guide says market, refuel, repair, shipyard,
  crafting, power and life support are included. A fresh station has faction storage only,
  with 0 power, 0 life support and no services (*measured / player-reported*).
- **Founding command.** base-builder says `facility action=found_station`; what works is
  `build_base` (*measured*).
- **Storage capacity units.** base-builder lists faction storage in total items (500 → 50,000);
  the factions guide lists per item type (100k → 500k). The station's 500k is per item type
  (*player-reported*).
- **Fuel storage.** The [[faction_fuel_bunker]] is refused at your own station; the server
  points you to the [[fuel_tank_expansion]] instead (*measured*).

## Not yet measured

- Real upkeep consumption per cycle (catalog figures are shown above until we have it).
- How fast a `critical` station recovers once its upkeep is stocked.
- Defense: guns, ammo and shields.
- Whether infrastructure facilities (power, life support) can be dismantled or replaced.
- Exactly how [[trade_authenticator]] upkeep scales with trades.
- How service pools ([[medical_supplies]], [[protein_rations]]) draw down, and on what cycle.
