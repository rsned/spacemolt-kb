# Empire Economy — Insights & Takeaways

_Hand-written insights from the first run of `cmd/analyze-empire-economy`._
_Based on the report generated 2026-05-26 (713 products analyzed, 5 empires)._
_Source report: [empire-economy.md](empire-economy.md)._

---

## 1. Trade is the design, not the bug

**Zero ships in the entire galaxy can be solo-built by all five empires.** Not one. Shipbuilding is *structurally* trade-dependent — a war that severs trade routes halts ship construction everywhere simultaneously. Only 5.2% of products are universally solo-buildable, and those are the trivial ones (bioactives, common ingot refining).

## 2. Half of base materials are politically controlled

16 resources are held ≥80% by a single empire vs. 18 "contested" (no empire holds ≥40%). The split is along material type:

- **Contested**: the boring common ores — iron, copper, carbon, vanadium, hydrogen. Open market.
- **Monopolized**: the *named* exotic materials — Trade Crystal (Nebula 100%), Darksteel (Crimson 100%), Phase Crystal (Outerrim 100%), Sol Alloy (Solarian 100%), Void Essence (Voidborn 100%).

Empire identity is literally encoded into the resource map.

## 3. Each empire has a thematic specialty cluster

| Empire | Theme | Signature monopolies |
|---|---|---|
| Solarian | Imperial core / water-sun | Sol Alloy Ore, Legacy Ore, Ion Gas, Antimatter Containment Cell; dominant in Water Ice (41.7%), Helium Ice (50%), Deuterium Ice (61.7%) |
| Nebula | Gases / mercantile | Krypton, Trade Crystal, Prismatic Nebulite; near-monopoly Xenon (79%) and Nebula Gas (53%) |
| Outerrim | Quantum / exotic | Phase Crystal, Quantum Fragments, Dark Matter Residue |
| Voidborn | Void / dark | Null Matter, Void Essence, Energy Crystal |
| Crimson | Martial alloys | Darksteel, Cobalt, Fury Crystal |

Solarian has the **most monopolies** (4) — it controls the "imperial core" tier resources.

## 4. Voidborn is the most diplomatically interesting empire

It's the *most resource-poor* (missing 26 of 47 used base materials — more than anyone) but holds three strategic monopolies. **High leverage, high need.** Its dependency row is brutal: Voidborn→Solarian unlocks 70 products, Voidborn→Outerrim 58. It can't survive without trade — but it has bargaining chips no one else can offer.

## 5. Nebula has breadth, Solarian has comfort, the SSI gap is tiny

- **Nebula** leads in solo-buildable products (84 items + 4 ships = 88) and in exclusivity (31 exclusive products).
- **Solarian** builds fewer things (71) but more *comfortably* — mean ease 38.7% vs Nebula 35.7%.
- The whole SSI spread across all five empires is just **20.5 → 25.2** — a 4.7-point window.

That's a **deliberately balanced game**. No empire is grossly disadvantaged on the composite measure.

## 6. Outerrim is last on SSI despite three monopolies

Holding Quantum Fragments and Phase Crystal doesn't make you self-sufficient — it makes you an **exporter**. Strong specialty resources translate to leverage, not autonomy. Outerrim's economic role is "trade middleman" rather than "self-sustaining power."

Worth watching whether players who go Outerrim feel poor (need to trade constantly) or powerful (everyone needs them).

## 7. The dependency matrix is asymmetric in load-bearing ways

- **Nebula is everyone's favorite partner** (52, 55, 50, 47 product-unlocks for the other empires) — broadest base coverage.
- **Solarian ↔ Voidborn is the strongest two-way axis** (52/70 unlocks).
- **Outerrim ↔ Crimson barely help each other** (12 and 8 unlocks) — their resource sets either overlap or are mutually irrelevant. Predicts they'd be diplomatically inert toward each other.
- **Crimson → Nebula** unlocks 52 products; **Nebula → Crimson** unlocks 34. Nebula gives Crimson more than vice versa.

## 8. Flex Polymer and Chlorine Compound are the skeleton of the economy

- **Flex Polymer** appears in 547 of 713 product BoM trees (**77%**).
- **Chlorine Compound** appears in 519 (**73%**).

These two single intermediates are downstream-required by three-quarters of *everything craftable*. A facility outage on either would freeze most of the catalog. Both are intermediate components, not raw materials — so the choke point is at the *refining* stage, not at mining.

## 9. The nuclear chain is a hidden bottleneck cluster

These all sit "deep" in the BoM tree — few recipes use them directly, but those recipes feed many downstream builds:

| Component | Transitive rank | Direct rank |
|---|---:|---:|
| Thorium Concentrate | #31 | #180 |
| Uranium Concentrate | #38 | #183 |
| Thorium Fuel Rod | #16 | low |
| Uranium Hexafluoride | #20 | low |

A reactor-focused playstyle would care intensely about these even though casual recipe-browsing wouldn't surface them. Likely a high-leverage stockpile target.

## 10. Shallow workhorses cluster at the weapon-system layer

The "high direct, low transitive" items are all weapon/ship-system intermediates: Laser Focus Array, Ion Emitter, Plasma Injector, Tracking System, Hazmat Container. These sit at the "final assembly" layer — used in many specific weapons but not further refined into anything else. A factory specializing in any one of these could supply many distinct end-products with no further dependencies.

---

## Open questions / threads to chase

- **Per-system view** (instead of per-empire). Which individual systems are the most economically defensible? Likely a handful of capitals dominate.
- **"What if I had system X" simulator**. Would adding one specific system to an empire flip its SSI ranking? Would identify the highest-value contested systems.
- **Trade-route weighting**. Currently assumes any empire-internal POI is equally accessible. Distance/fuel cost would surface "you technically have it but the only POI is 14 jumps deep."
- **Time-series tracking**. Re-run weekly and chart how SSI shifts as exploration progresses. Voidborn's score should rise sharply once players discover more of its space.
- **Cross-reference with player activity**. Do players actually settle in empires that match their playstyle, or do the SSI numbers not match perceived comfort?

---

## 11. The other end of the skeleton: nearly-useless base materials

Counterpoint to the "Flex Polymer / Chlorine Compound carry the economy" finding. From the persisted `bill_of_materials` table (canonical recipe selection across all 2,322 items + ships + facilities), demand for raw bases spans **three orders of magnitude**:

| Base Material | Products Needing It | Notes |
|---|---:|---|
| Iron Ore | 2,197 | Everything |
| Titanium Ore | 2,136 | Everything |
| Silicon Ore | 2,105 | Everything |
| Fluorine Gas | 2,103 | Everything |
| Copper Ore | 1,089 | Half the catalog |
| ... | ... | ... |
| Silver Ore | **3** | Cloaking Device I, Electrum Ingot, Silver Wiring |
| Adamantite Ore | **4** | Adamantite Bar, Darksteel Armor, Mass Driver, Nanite Hull Coating |
| Darksteel Ore | **5** | Five Crimson-themed combat items |
| Nebula Gas | **5** | Combat stims + exotic dusts only |
| Chlorine Gas | **6** | Six chemical-warfare items |

The top base material is **730× more demanded** than the bottom one. Three categories of "low-demand" emerge:

### Genuinely under-used

- **Silver Ore** (3 consumers): Cloaking Device I, Electrum Ingot, and Silver Wiring. Apparently Silver Wiring is reachable via alternative recipes that don't go through silver ore — silver_ore itself is a near-vestigial material.
- **Adamantite Ore** (4): Narrow martial-gear pipeline. Only matters if you're crafting Mass Drivers or Darksteel Armor.
- **Nebula Gas** (5): Feeds *only* combat stims, berserker compounds, and trace exotics — all themselves low-volume. A genuine niche.
- **Chlorine Gas** (6): Feeds six items including the wide-impact Chlorine Compound. Limited consumer count but Chlorine Compound's downstream reach means this is *narrow-but-load-bearing* — a single chokepoint.

### Faction-flagship niche (looks underused but isn't)

These have low item-count but feed many faction-flagship **ships**:

| Base | Items | Ships | Empire flavor |
|---|---:|---:|---|
| Legacy Ore | 3 | 8 | Solarian capital ships |
| Antimatter Containment Cell | 7 | 7 | Voidborn weapons + ships |
| Prismatic Nebulite | 3 | 25 | Nebula faction line |
| Dark Matter Residue | 6 | 27 | Outerrim quantum tech |
| Void Essence | 8 | 26 | Voidborn |
| Phase Crystal | 4 | 35 | Outerrim |
| Trade Crystal | 5 | 37 | Nebula |
| Sol Alloy Ore | 10 | 41 | Solarian |

These aren't useless — they're **monopoly leverage materials**. Their entire purpose is faction identity: own this resource, gate the faction's flagship ships. Compare with the Galaxy Monopolies table — these match exactly. The monopolies *are* the faction-flagship pillars.

### Single-purpose pillar

- **Plasma Gas** (24 consumers): all 24 are plasma-themed (cannons, cells, torpedoes, mines, repeaters). A complete vertical specialty pillar — if you're not running a plasma loadout, it's invisible to you; if you are, it's central.

### What this means

- **Silver Ore is the closest thing to a vestigial resource** — three consumers, none of which are themselves heavily downstream. If a player spent their whole career never touching silver ore, they'd barely notice.
- **Nebula Gas is the deepest dead-end** — its consumers (stims, exotic dust) don't feed further chains. Mining it is almost purely for the combat-consumable economy.
- The "useless"-looking faction materials are actually **the most strategically loaded** — they have the smallest consumer set but each consumer is a high-value capital ship. This is monopoly-by-design.
- **Caveat**: the BoM table reflects the *canonical* recipe selected by `pkg/bom`'s SelectRecipe. Items with alternative recipes (notably Silver Wiring) may flow through these "underused" bases when the canonical path is unavailable. So "demand=3" should read as "the default crafting path almost never touches this" rather than "literally unusable."
