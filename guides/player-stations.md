---
title: Building a Player Station
maintenance_cycle_ticks: 3000
maintenance_cycle_basis: "measured ~3,000 at Conjunction Junction 2026-10-27 from three facilities' storage drops; approximate (rent bills every 100 ticks)"
summary: From Station Core to a running home station, with every facility's build cost and upkeep, measured at Conjunction Junction (Cloverfield).
intro: |
  A **player station** is a base your faction owns outright, founded in lawless space from a
  [[station_core]]. Owning one means no rent at NPC stations, your own storage and services,
  and a market other pilots can use if your station sits on a crossroads.

  The official guides leave the important parts out: what a new station actually comes with,
  what each facility consumes to keep running, and in what order to build. This guide records
  what we measured while founding **Conjunction Junction** in Cloverfield on 2026-10-25, and
  while running it since (last updated 2026-11-08: 24 faction facilities, condition
  `thriving`, 8 of 8 counted facilities satisfied).
  Build costs and upkeep below come from the game catalog. Our own observations are labelled
  *measured*, other players' reports *player-reported*, and the official docs *doc-quoted*.

  > **The one rule:** stock every facility's upkeep in faction storage **before** the build
  > finishes. A facility short of upkeep doesn't stop outright: it drops into a reduced
  > operating mode (for example running at half speed) and its description switches to its
  > *degraded* text. The station's condition steps down gradually with the share of
  > facilities left unsatisfied (*measured*: one unpaid dock out of four took us from
  > `thriving` to `operational`). On a young station with only one or two facilities, that
  > one shortfall is most of the station, which is how ours hit `critical` (*measured*).
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

      **What we learned building it first:** we built it with no [[flex_polymer]] in faction
      storage, and `get_base` read `critical` with zero services (`satisfied_count 0 /
      total_service_infra 1`). That wasn't the station collapsing: the Locker Bay was the
      only facility counted, so one unpaid facility was 100% of the station. Once its upkeep
      arrived it read `thriving`, 1 of 1 (*measured* 2026-10-26). With more facilities, the
      same shortfall costs a step of condition, not the whole station. Deposit a few days of
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
  - title: Life Support
    facility: atmospheric_recycler
    break_before: |
      **Next: production support.** With storage and crafting in place, build life support
      and power, the two things production facilities need, so production lines (including
      the fuel combustor that fills the refuel tank) can start moving to the station.
    notes: |
      The heaviest upkeep of the early build: five different inputs every cycle.
      [[water_ice]] is usually mineable nearby (Cloverfield has an ice field); plan a supply of
      the other four before you build. Other players build it third (*player-reported*); so do
      we, together with power, because the two of them are what production facilities need:
      this is where more advanced crafting and processing can move to the station. (We
      actually built the refueling dock first, and its empty tank had to wait for both.)

      **Stock all five before it finishes, and keep them stocked.** If one input runs out, the
      plant goes `starved` and the station drops to `operational`. `get_base` then shows a
      `life_support.starved` list naming **every** upkeep input, not just the missing one, so
      check faction storage for the real gap. Ours was oxygen. After a restock, the plant only
      recovers at its next maintenance check: ours went from `starved` to `thriving` within about
      60 ticks, taking just the 1 oxygen it still owed (*measured* 2026-10-27).

      Without it, production is refused outright: *"no_life_support: This station has no life
      support yet. Build a life support facility (faction_build) before adding facilities that
      draw life support."* (*measured* 2026-10-27, building an [[h2_fuel_combustor]]).

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
      infrastructure could be dismantled. It can: see [Moving a facility](#moving-a-facility).

      We built the grid at Conjunction Junction on 2026-10-26; the build reply again gave
      750 corporation_management XP to every member and showed rent 0. Once it finished,
      `get_base` gained a `power` block (absent until then) showing `supply: 20`, matching the
      catalog, and the grid counted toward station health alongside the Locker Bay
      (`satisfied_count 2 / total_service_infra 2`) (*measured*).
  - title: Refuel service
    facility: refueling_dock
    notes: |
      Right after life support and power, because everyone needs fuel: your own miners and
      haulers working out of the station, and every pilot passing through. Waiting for those
      two first pays off here: the dock's tank starts empty, and the way to fill it, an
      [[h2_fuel_combustor]], is a production facility that needs both. The dock brings its **own 5,000-unit tank**:
      `get_base` `max_fuel` went from 200 to 5,000 when ours was built, with no tank expansion
      (*measured* 2026-10-26). Faction members draw from the shared tank for free; everyone
      else pays your station price (set with `station set_refuel_price`).

      ⚠ **Stock its upkeep before it finishes:** [[fuel_cell]] and [[liquid_hydrogen]] every
      cycle. Ours finished with no fuel cells in faction storage, and `get_base` showed
      `satisfied_count 3 / total_service_infra 4`, condition `operational` ("signs of deferred
      maintenance") and **no `refuel` in `services`**: an unpaid dock is built but sells
      nothing. After we deposited 1,000 fuel cells the station was back to `thriving`, 4/4,
      with `refuel` listed, within 69 ticks (*measured*).

      ⚠ **Then fill the tank.** The dock starts empty, and a refuel answers *"station_fuel_empty:
      This station's fuel reserves are depleted"* (*measured*). The way to fill it is an
      [[h2_fuel_combustor]]: crafting `manufacture_fuel_basic` there turns 10
      [[liquid_hydrogen]] into 100 fuel per run (about 1 s a run, 8 cr labor), and the output
      goes straight into the station's tank, not into faction storage, whatever the craft
      reply says. Each completed run adds its 100 fuel as it finishes, so the tank climbs
      steadily while the job runs. Faction storage and the treasury cover the job when the crafter's own
      can't (*measured* 2026-10-27). Until the combustor is up,
      a CRFT member can top the tank up from their own ship (`faction_deposit_items`,
      `item_id=fuel`).

      Until a refuel service exists, ships docked at your station can't refuel at all; the
      server's message (`no_fuel_source … insufficient credits for station refueling`) is
      misleading, because credits are not the problem (*measured*).
  - title: Market
    facility: trade_concourse
    notes: |
      The rest of a full station is its **services**. A mature station lists eleven:
      `crafting`, `marine_training`, `market`, `medical`, `missions`, `personnel`, `refuel`,
      `repair`, `salvage_yard`, `shipyard` and `storage` (*player-reported*). Each has a level-1
      facility a player station can build.

      The market comes after power, life support and fuel, because it starts the station
      earning: crossroads
      traffic trades here, and you set the listing fee they pay with
      `station set_market_fee`. The catalog lists its upkeep as one [[trade_authenticator]] a
      cycle, but players have found the authenticators go **per trade**, possibly on top of
      the per-cycle charge, so a busy market eats them much faster than any other upkeep
      (*player-reported*). Line up a steady source before you build, sized to your trade
      volume rather than to the cycle.
  - title: Shipyard
    facility: outfitting_bay
    break_before: |
      **From here on the order is flexible.** With storage, crafting, production support,
      fuel and a market in place, build the remaining services as you need them. Our
      suggestion is shipyard, repair and salvage yard first, then missions, medical, personnel
      and marine training, and more fuel tank once the dock's 5,000 runs short.
    notes: |
      The most expensive level-1 service to build: thousands of [[steel_plate]] plus
      [[circuit_board]], and the longest build time. Upkeep includes [[titanium_alloy]].
      It upgrades to [[fabrication_yard]], [[assembly_complex]] and [[grand_shipworks]].

      We built the Outfitting Bay on 2026-11-01 and upgraded it to a [[fabrication_yard]] the
      next day (14,000 [[steel_plate]] for the upgrade). An upgrade is its own action:
      `faction_build fabrication_yard` is refused with *"this is an upgrade — use 'upgrade'
      action instead"*, and `upgrade` needs both `facility_id` and `facility_type`
      (*measured*). The level-2 yard **builds two hulls at once**; further commissions queue as
      `PENDING` (*measured* 2026-11-08).

      **Building ships here.** Empire designs are exclusive: a station can build another
      empire's class only with a **ship license**, bought once per class by the faction
      (`buy_ship_license`; T1 1M, T2 5M, T3 10M, T4 20M, T5 50M). We licensed the outerrim
      Congregation (1,900 cargo) for 1M. A **faction-funded commission** then draws every
      material from faction storage, the treasury pays the labor plus a 10% royalty on the
      materials' value to the design's empire (about 650–820 cr per Congregation), and the
      finished hull is delivered to whoever placed the order (*measured*). Anyone can
      commission a class; supply the materials, because at the cash price the yard has to
      buy them itself, and a material that isn't on the market stalls the build for good
      (*player-reported*).
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
  - title: Fuel storage
    facility: fuel_tank_expansion
    notes: |
      Late in the build: the refueling dock's own 5,000-unit tank is enough to get a station
      started. Add expansions when that runs short. Each expansion adds 50,000 to the
      station's shared tank, and you can build several. Fill it from fuel-production
      facilities or by depositing a docked ship's fuel (storage deposit, `item_id=fuel`). With
      `station set_auto_buy_fuel` on, visiting pilots can sell fuel into the tank too, paid
      from the treasury.

      Fuel production needs tank capacity, and the refueling dock's tank counts: you do not
      need an expansion first. ⚠ **Server bug (developer-confirmed, being fixed):** for up to
      about 17 minutes after the dock finishes, the station does not count its tank yet.
      During that window, building a fuel producer or depositing fuel wrongly answers
      *"no_fuel_bunker: This station needs fuel tank capacity … Use 'faction_build' with
      facility_type='fuel_tank_expansion' first."* We hit exactly that with an
      [[h2_fuel_combustor]] (*measured* 2026-10-26). Wait and retry; don't build an expansion
      because of it. Fuel production also draws life support and power.

      ⚠ **Don't build a [[faction_fuel_bunker]] here.** At your own station the server refuses
      it: *"a private Faction Fuel Bunker doesn't make sense on your own station — its fuel
      would just sit unused. Build a Fuel Tank Expansion … instead"* (*measured* 2026-10-26).
  - title: Ship garage
    facility: faction_ship_garage
    break_before: |
      **Faction facilities.** These serve the faction rather than visitors: a shared ship
      pool, more members, and named storage buckets.
    notes: |
      A shared pool of up to 20 hulls at the station. Put a ship in with
      `faction_deposit_items` and its ship ID (the reply's action is `faction_garage_store`);
      any member docked there takes it with `switch_ship`, which **transfers ownership** to
      them and parks their old ship at the station (*measured*). This is how we hand freshly
      built Congregations to haulers. For a pilot outside the faction, the owner can gift the
      parked hull instead: `send_gift <username> <ship_id>` while docked where it is
      (`gift_ship`, *measured* 2026-11-08).
  - title: Recruitment
    facility: hiring_board
    notes: |
      Without a recruitment facility a faction is capped at **20 members**: invites still go
      out, but accepting one answers *"faction_full: This faction has reached its maximum of 20
      members"*. The Hiring Board raises the cap to 50 (the upgrades: 100, 200 and more).
      Members already above the cap keep their seats; we had 38 (*measured* 2026-11-08). We
      built ours from a dismantled package, with no other materials (see
      [Moving a facility](#moving-a-facility)).
  - title: Storage buckets
    facility: storage_extension
    notes: |
      Adds a named bucket to faction storage (ours holds up to 100,000 of each item, up to
      1,000 packages). We use it to keep a repair reserve apart from working stock, since
      builds and upgrades can take their materials from a named bucket.
  - title: Point defense
    facility: point_defense_battery
    break_before: |
      **Defense.** No one had attacked us; we built defenses before anyone did. A wrecked
      station has to be repaired before it works again, at about 30% of each facility's build
      cost in credits and in that facility's materials, which must be at the station
      (*player-reported*).
    notes: |
      The cheapest gun, and it upgrades twice for reach ([[station_defense_turret]], then
      [[heavy_defense_battery]]). After ours finished, `get_base` showed `weapon_dps` 30
      (*measured* 2026-11-06). ⚠ **Every gun needs ammunition** in faction storage: this one
      fires [[ferrous_slug_case]] (hand recipe: 3 [[iron_ore]] + 2 [[steel_plate]] → 5). Its
      degraded text describes empty magazines, so a gun built without ammo likely does
      nothing in a fight.
  - title: Armor plating
    facility: reinforced_plating
    notes: |
      Armor cuts each hit to the hull by its amount, and the plating repairs itself after
      combat. No power draw. Ours raised `get_base` **hull from 60,000 to 85,000 and armor
      from 15 to 60** (*measured* 2026-11-08). Upkeep and repairs use [[armor_plate]].
  - title: Fire control
    facility: fire_control_center
    notes: |
      Improves the guns' accuracy (*player-reported*); `get_base` shows no new number for
      it. Upkeep is [[circuit_board]].
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

**Only some facilities count toward condition.** With 24 faction facilities built,
`get_base` reads `total_service_infra 8` (*measured* 2026-11-08): the services, power, life
support, fuel storage and the defense facilities count; production lines, the garage, the
office and storage buckets don't. A starving production line slows itself down without
moving the station's condition.

**What the server doesn't tell you:** for a *service* facility (the Locker Bay, the Faction
Workshop) there is no "starved" list, only `maintenance_satisfied: false`. To know what to haul,
compare the facility's `maintenance_per_cycle` (the upkeep tables in the steps above) against
`view_faction_storage` at the station.

**Two clocks:** facility **rent** bills every **100 ticks** (about 17 minutes;
*player-reported*), but facilities **consume their upkeep** on a separate maintenance cycle
of about **3,000 ticks** (about 8.3 hours). We measured that at Conjunction Junction: the
Locker Bay, Faction Workshop and Refueling Dock each used one cycle's upkeep roughly every
3,000 ticks, each on its own phase, counted from its build (*measured* 2026-10-27). A 100-tick
cycle would have used about 30 times as much. Don't be misled by `get_base`: the
`life_support` block reports `maintenance_cycle_ticks: 100` (*measured*), which does not match
what the other facilities consume. Whether life support itself burns its upkeep every 100
ticks is *being measured* from our recycler's water ice. Until the cycle is pinned down
precisely, this page gives upkeep **per cycle** only.

**What happens when it runs dry:** the effect is graduated, not a switch.
- **The facility** drops into a reduced operating mode rather than stopping: it keeps running,
  but slower (for example at half speed). Its description switches from the *satisfied* text
  to the *degraded* text, and `maintenance_level` falls below 1 (*player-reported*). For example, the catalog text
  for the Climate Control Array reads *"Thermal regulators hold steady.
  Water purification runs ahead of consumption, building a healthy reserve."* when satisfied,
  and *"Temperature swings between sections and water purification lags behind demand."*
  when degraded (*doc-quoted*).
- **The station's `condition`** steps down through its levels with the share of facilities
  unsatisfied. One unpaid Refueling Dock out of four gave `satisfied_count 3 /
  total_service_infra 4` and condition `operational` (*measured*).
- **On a very young station** that share is large. Early on, the Locker Bay went
  `maintenance_satisfied: false` with `maintenance_level` 0.4, the station dropped from
  `thriving` to `critical` (*"Critical infrastructure failure. Emergency lighting only."*),
  and `get_base` listed **no services at all**, so personal storage stopped working
  (*measured*).

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

## Moving a facility

`facility action=faction_dismantle` takes a facility apart so you can rebuild it somewhere
else. We first ran it on 2026-10-27, on an [[iron_refinery]] that CRFT rented at Grand
Exchange Station (*measured*):

- **The facility goes offline at once** and vanishes when the dismantle finishes. Any repair
  in progress is cancelled, and repair costs already spent are not refunded.
- **It takes a few minutes**: 231 ticks for the refinery.
- **All the build materials come back.** The refinery returned 250 [[control_node]] and
  2,510 [[copper_piping]], exactly its catalog recipe, packed into 28 labeled packages.
- **Packages land in the faction storage at that station.** Haul them like any cargo, unpack
  them at the destination, and rebuild from the recovered materials.
- Items left in faction storage can still be viewed and withdrawn after the storage facility
  itself is dismantled.
- Rent pauses while a facility is dismantling (from the rent note).

**Rebuilding from packages (*measured*).** You don't have to unpack: pass the package IDs
straight to the build, `facility faction_build <type> --package_ids <id>[,<id>…]`. Each
package must hold exactly what the build still needs of an item, or the build is refused up
front. We rebuilt the iron refinery at Conjunction Junction this way on 2026-10-28, and on
2026-11-08 built a Hiring Board from its single package with no other materials. Each package
takes **100 cargo**, and dismantling costs one [[cargo_container]] per package
(*doc-quoted*). Moving four facilities' packages (Fiber Draw Tower 41, Polymer Extruder 40,
Intel Terminal 3, Hiring Board 1) from Grand Exchange took three Congregation trips of up
to 19 packages each.

The reply does not say whether the credit part of the build cost (89,000 for the refinery)
comes back. Expect to pay it again when you rebuild. Rented `faction_*` facilities at NPC
stations cannot be rebuilt at your own station. Check that the facility is on the
[buildable list](#what-you-can-build-here) before dismantling it to move it.

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
- **"Build a fuel tank expansion first" right after the dock finishes.** For up to about
  17 minutes after a [[refueling_dock]] completes, fuel producers and fuel deposits are refused
  with `no_fuel_bunker`, naming the [[fuel_tank_expansion]]. It is a server timing bug
  (developer-confirmed); the dock's own tank is enough, so retry later (*measured*).
- **`fuel_depot` is not yours to build.** It is the empire-station fuel facility; at your own
  station the refuel facility is the [[refueling_dock]] (developer-confirmed).

## Not yet measured

- Real upkeep consumption per cycle (catalog figures are shown above until we have it).
- How fast a `critical` station recovers once its upkeep is stocked.
- Defense in a real fight: how much ammunition a fight uses, and what fire control changes.
- Shields, batteries and extra power ([[shield_projector]], [[battery_bank]],
  [[bunker_fed_reactor]]): not built yet; all three need [[power_cell]] in quantity.
- Whether dismantling refunds any credits.
- Exactly how [[trade_authenticator]] upkeep scales with trades.
- How service pools ([[medical_supplies]], [[protein_rations]]) draw down, and on what cycle.
