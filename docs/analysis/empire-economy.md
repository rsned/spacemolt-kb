# Empire Economy Analysis

_Generated 2026-05-26 08:46 UTC_

## Executive Summary

- **Circuit Board** is the most directly demanded intermediate (159 recipes consume it); **Flex Polymer** threads through the most BoM trees (547 products need it).
- Of 713 analyzed products, 37 (5.2%) can be solo-built by every empire and 590 (82.7%) cannot be solo-built by any single empire — those force interregional trade.
- **Voidborn** is the most resource-poor empire, missing 26 of 47 galaxy base materials; **Nebula** is missing the fewest (17).
- Composite SSI leader: **Solarian** (25.2). Trailing: **Outerrim** (20.5).

---

## 1. Component Popularity

_185 intermediate components identified (items that are both crafted and consumed in further recipes)._

### Top 20 Intermediates by Direct Recipe Demand

| Rank | Component | Recipes Using | Category |
|------|-----------|---:|----------|
| 1 | Circuit Board | 159 | refined |
| 2 | Superconductor | 84 | refined |
| 3 | Power Cell | 71 | component |
| 4 | Steel Plate | 71 | refined |
| 5 | Titanium Alloy | 62 | refined |
| 6 | Weapon Housing | 56 | component |
| 7 | Durasteel Plate | 50 | refined |
| 8 | Flex Polymer | 46 | refined |
| 9 | Focused Crystal | 46 | refined |
| 10 | Sensor Array | 39 | component |
| 11 | Processing Core | 36 | component |
| 12 | Stabilized Exotic | 25 | refined |
| 13 | Warhead Assembly | 23 | component |
| 14 | Hull Plating | 22 | component |
| 15 | Ceramite Plating | 19 | refined |
| 16 | Shield Emitter | 19 | component |
| 17 | Refined Quantum Matrix | 18 | refined |
| 18 | Copper Wiring | 17 | refined |
| 19 | Lead Sheet | 17 | component |
| 20 | Power Core | 16 | component |

### Top 20 Intermediates by Transitive Product Demand

| Rank | Component | Products Needing | Category |
|------|-----------|---:|----------|
| 1 | Flex Polymer | 547 | refined |
| 2 | Chlorine Compound | 519 | refined |
| 3 | Circuit Board | 513 | refined |
| 4 | Steel Plate | 503 | refined |
| 5 | Copper Wiring | 415 | refined |
| 6 | Titanium Alloy | 405 | refined |
| 7 | Silver Wiring | 338 | refined |
| 8 | Superconductor | 336 | refined |
| 9 | Focused Crystal | 327 | refined |
| 10 | Power Battery | 324 | refined |
| 11 | Power Cell | 296 | component |
| 12 | Purified Krypton | 222 | refined |
| 13 | Ionized Neon | 220 | refined |
| 14 | Neon Signaling Array | 218 | component |
| 15 | Sensor Array | 217 | component |
| 16 | Hull Plating | 215 | component |
| 17 | Processing Core | 184 | component |
| 18 | Durasteel Plate | 167 | refined |
| 19 | Synthetic Diamond | 147 | refined |
| 20 | Shield Emitter | 145 | component |

### Worth Stockpiling (Top 10 in Both Rankings)

| Component | Direct | Transitive |
|-----------|-------:|-----------:|
| Circuit Board | #1 | #3 |
| Superconductor | #2 | #8 |
| Steel Plate | #4 | #4 |
| Titanium Alloy | #5 | #6 |
| Flex Polymer | #8 | #1 |
| Focused Crystal | #9 | #9 |

### Where the Two Rankings Disagree

**Shallow workhorses** (top of direct, lower in transitive — used in many recipes, but downstream of fewer build chains):

- Laser Focus Array — direct #25 / transitive #104
- Ion Emitter — direct #24 / transitive #102
- Plasma Injector — direct #39 / transitive #115
- Hazmat Container — direct #28 / transitive #103
- Tracking System — direct #21 / transitive #96

**Deep bottlenecks** (top of transitive, lower in direct — used in few recipes directly but those recipes feed many downstream builds):

- Thorium Concentrate — transitive #31 / direct #180
- Uranium Concentrate — transitive #38 / direct #183
- Neon Signaling Array — transitive #14 / direct #158
- Reinforced Bulkhead — transitive #41 / direct #171
- Helium-3 — transitive #49 / direct #141

## 2. Empire Self-Sufficiency Distribution

### Headline Distribution

| Empires Able to Solo-Build | # Items | # Ships | Examples |
|----------------------------|--------:|--------:|----------|
| 5 (all empires) | 37 | 0 | Armor Piercing Rounds Box, Bioactive Compound, Bioluminescent Algae, Bioluminescent Culture |
| 4 | 6 | 0 | Hot Cell, Ionized Neon, Lead Ingot, Lead Sheet |
| 3 | 26 | 0 | Aluminum Sheet, Armor Plate, Armor Plate I, Armor Plate III |
| 2 | 6 | 1 | Cargo Superstructure, Deuterium, Gold Wiring, Hazmat Container |
| 1 (single empire only) | 43 | 4 | Biofiber Mesh, Cargo Expander I, Chaff Bundle, Chlorine Compound |
| 0 (no empire alone) | 342 | 248 | Absence, Absolute Entropy, Absolute Zero, Accretion |

### Per-Empire Breakdown

| Empire | Items Solo | Ships Solo | Exclusive Items | Exclusive Ships |
|--------|-----------:|-----------:|----------------:|----------------:|
| Crimson | 70 | 0 | 4 | 0 |
| Nebula | 84 | 4 | 27 | 4 |
| Outerrim | 65 | 1 | 2 | 0 |
| Solarian | 70 | 1 | 2 | 0 |
| Voidborn | 53 | 0 | 8 | 0 |

### Cross-Empire Dependency Matrix

Cell `[row=A][col=B]` is the number of products `A` cannot solo-build but `A` + `B` together can. Higher = `B` is a more valuable trading partner for `A`.

| From \ With | Crimson | Nebula | Outerrim | Solarian | Voidborn |
|---|---:|---:|---:|---:|---:|
| **Crimson** | — | 52 | 8 | 14 | 18 |
| **Nebula** | 34 | — | 33 | 33 | 12 |
| **Outerrim** | 12 | 55 | — | 15 | 45 |
| **Solarian** | 13 | 50 | 10 | — | 52 |
| **Voidborn** | 35 | 47 | 58 | 70 | — |

## 3. Per-Empire Resource Sheets

For each empire, a resource sheet shows galaxy market share per base material (sum of POI richness in that empire ÷ sum across all named-empire POIs), and the top-10 products with the *worst* ease score (the empire's hardest end-products to source comfortably).

### Crimson Empire

**Resource Position**

| Resource | Richness | Galaxy Share | POIs | Verdict |
|----------|---------:|-------------:|-----:|---------|
| Cobalt Ore | 50.0 | 100.0% | 1 | Dominant |
| Darksteel Ore | 40.0 | 100.0% | 4 | Dominant |
| Fury Crystal | 9.0 | 100.0% | 2 | Dominant |
| Titanium Ore | 60.0 | 52.2% | 1 | Dominant |
| Polonium Ore | 5.0 | 50.0% | 1 | Dominant |
| Chlorine Gas | 51.0 | 39.2% | 3 | Sufficient |
| Uranium Ore | 29.0 | 38.7% | 2 | Sufficient |
| Lead Ore | 23.0 | 38.3% | 2 | Sufficient |
| Radium Ore | 8.0 | 34.8% | 1 | Sufficient |
| Palladium Ore | 76.0 | 34.4% | 3 | Sufficient |
| Plasma Gas | 47.0 | 34.1% | 2 | Sufficient |
| Platinum Ore | 134 | 31.8% | 4 | Sufficient |
| Iron Ore | 402 | 28.7% | 8 | Sufficient |
| Neon Gas | 118 | 27.9% | 4 | Sufficient |
| Vanadium Ore | 138 | 26.4% | 4 | Sufficient |
| Argon Gas | 292 | 25.6% | 6 | Sufficient |
| Nitrogen Ice | 219 | 24.0% | 5 | Sufficient |
| Tungsten Ore | 117 | 22.9% | 3 | Sufficient |
| Carbon Ore | 312 | 21.9% | 6 | Sufficient |
| Xenon Gas | 17.0 | 21.0% | 1 | Sufficient |
| Nebula Gas | 28.0 | 18.9% | 2 | Sufficient |
| Hydrogen Gas | 221 | 17.1% | 4 | Sufficient |
| Water Ice | 233 | 17.0% | 4 | Sufficient |
| Iridium Ore | 25.0 | 16.0% | 1 | Sufficient |
| Copper Ore | 156 | 15.1% | 6 | Sufficient |
| Helium Ice | 10.0 | 10.2% | 1 | Scarce |
| Adamantite Ore | — | 0% | 0 | Missing |
| Aluminum Ore | — | 0% | 0 | Missing |
| Antimatter Containment Cell | — | 0% | 0 | Missing |
| Dark Matter Residue | — | 0% | 0 | Missing |
| Deuterium Ice | — | 0% | 0 | Missing |
| Energy Crystal | — | 0% | 0 | Missing |
| Exotic Crystal | — | 0% | 0 | Missing |
| Exotic Matter | — | 0% | 0 | Missing |
| Fluorine Gas | — | 0% | 0 | Missing |
| Gold Ore | — | 0% | 0 | Missing |
| Ion Gas | — | 0% | 0 | Missing |
| Krypton Gas | — | 0% | 0 | Missing |
| Legacy Ore | — | 0% | 0 | Missing |
| Lithium Ore | — | 0% | 0 | Missing |
| Nickel Ore | — | 0% | 0 | Missing |
| Null Matter | — | 0% | 0 | Missing |
| Phase Crystal | — | 0% | 0 | Missing |
| Prismatic Nebulite | — | 0% | 0 | Missing |
| Quantum Fragments | — | 0% | 0 | Missing |
| Silicon Ore | — | 0% | 0 | Missing |
| Silver Ore | — | 0% | 0 | Missing |
| Sol Alloy Ore | — | 0% | 0 | Missing |
| Thorium Ore | — | 0% | 0 | Missing |
| Trade Crystal | — | 0% | 0 | Missing |
| Void Crystal | — | 0% | 0 | Missing |
| Void Essence | — | 0% | 0 | Missing |

**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):

| Product | Kind | Ease | Bottleneck Resource |
|---------|------|-----:|---------------------|
| Helium-3 | item | 10.2% | Helium Ice |
| Copper Wiring | item | 15.1% | Copper Ore |
| Ghost Rounds | item | 15.1% | Copper Ore |
| Hot Cell | item | 15.1% | Copper Ore |
| Weapon Housing | item | 15.1% | Copper Ore |
| Bioactive Compound | item | 17.0% | Water Ice |
| Bioluminescent Culture | item | 17.0% | Water Ice |
| Combat Stim | item | 17.0% | Water Ice |
| Cryogenic Storage | item | 17.0% | Water Ice |
| Endurance Booster | item | 17.0% | Water Ice |

### Nebula Empire

**Resource Position**

| Resource | Richness | Galaxy Share | POIs | Verdict |
|----------|---------:|-------------:|-----:|---------|
| Krypton Gas | 11.0 | 100.0% | 1 | Dominant |
| Prismatic Nebulite | 37.0 | 100.0% | 3 | Dominant |
| Trade Crystal | 110 | 100.0% | 4 | Dominant |
| Xenon Gas | 64.0 | 79.0% | 3 | Dominant |
| Gold Ore | 61.0 | 62.9% | 2 | Dominant |
| Nebula Gas | 79.0 | 53.4% | 4 | Dominant |
| Thorium Ore | 30.0 | 44.8% | 2 | Dominant |
| Silicon Ore | 70.0 | 43.8% | 1 | Dominant |
| Helium Ice | 39.0 | 39.8% | 3 | Sufficient |
| Neon Gas | 166 | 39.2% | 6 | Sufficient |
| Deuterium Ice | 18.0 | 38.3% | 1 | Sufficient |
| Aluminum Ore | 123 | 36.4% | 3 | Sufficient |
| Hydrogen Gas | 460 | 35.6% | 8 | Sufficient |
| Plasma Gas | 44.0 | 31.9% | 2 | Sufficient |
| Argon Gas | 352 | 30.9% | 8 | Sufficient |
| Radium Ore | 7.0 | 30.4% | 1 | Sufficient |
| Nickel Ore | 55.0 | 29.7% | 1 | Sufficient |
| Water Ice | 328 | 23.9% | 5 | Sufficient |
| Nitrogen Ice | 218 | 23.9% | 5 | Sufficient |
| Lead Ore | 13.0 | 21.7% | 1 | Sufficient |
| Chlorine Gas | 28.0 | 21.5% | 1 | Sufficient |
| Copper Ore | 222 | 21.5% | 6 | Sufficient |
| Uranium Ore | 16.0 | 21.3% | 1 | Sufficient |
| Platinum Ore | 90.0 | 21.3% | 3 | Sufficient |
| Carbon Ore | 262 | 18.4% | 5 | Sufficient |
| Iron Ore | 253 | 18.1% | 6 | Sufficient |
| Iridium Ore | 28.0 | 17.9% | 2 | Sufficient |
| Vanadium Ore | 77.0 | 14.8% | 2 | Scarce |
| Palladium Ore | 29.0 | 13.1% | 1 | Scarce |
| Tungsten Ore | 31.0 | 6.1% | 1 | Scarce |
| Adamantite Ore | — | 0% | 0 | Missing |
| Antimatter Containment Cell | — | 0% | 0 | Missing |
| Cobalt Ore | — | 0% | 0 | Missing |
| Dark Matter Residue | — | 0% | 0 | Missing |
| Darksteel Ore | — | 0% | 0 | Missing |
| Energy Crystal | — | 0% | 0 | Missing |
| Exotic Crystal | — | 0% | 0 | Missing |
| Exotic Matter | — | 0% | 0 | Missing |
| Fluorine Gas | — | 0% | 0 | Missing |
| Fury Crystal | — | 0% | 0 | Missing |
| Ion Gas | — | 0% | 0 | Missing |
| Legacy Ore | — | 0% | 0 | Missing |
| Lithium Ore | — | 0% | 0 | Missing |
| Null Matter | — | 0% | 0 | Missing |
| Phase Crystal | — | 0% | 0 | Missing |
| Polonium Ore | — | 0% | 0 | Missing |
| Quantum Fragments | — | 0% | 0 | Missing |
| Silver Ore | — | 0% | 0 | Missing |
| Sol Alloy Ore | — | 0% | 0 | Missing |
| Titanium Ore | — | 0% | 0 | Missing |
| Void Crystal | — | 0% | 0 | Missing |
| Void Essence | — | 0% | 0 | Missing |

**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):

| Product | Kind | Ease | Bottleneck Resource |
|---------|------|-----:|---------------------|
| Armor Piercing Rounds Box | item | 6.1% | Tungsten Ore |
| Durasteel Plate | item | 6.1% | Tungsten Ore |
| Shard | ship | 6.1% | Tungsten Ore |
| Tungsten Rod | item | 6.1% | Tungsten Ore |
| Tungsten Slug Case | item | 6.1% | Tungsten Ore |
| Cargo Container | item | 18.1% | Iron Ore |
| Cargo Expander I | item | 18.1% | Iron Ore |
| Chaff Bundle | item | 18.1% | Iron Ore |
| Cobble | ship | 18.1% | Iron Ore |
| Corrosive Plasma Cell Pack | item | 18.1% | Iron Ore |

### Outerrim Empire

**Resource Position**

| Resource | Richness | Galaxy Share | POIs | Verdict |
|----------|---------:|-------------:|-----:|---------|
| Dark Matter Residue | 9.0 | 100.0% | 2 | Dominant |
| Phase Crystal | 33.0 | 100.0% | 3 | Dominant |
| Quantum Fragments | 25.0 | 100.0% | 2 | Dominant |
| Polonium Ore | 5.0 | 50.0% | 1 | Dominant |
| Lithium Ore | 43.0 | 48.9% | 1 | Dominant |
| Radium Ore | 8.0 | 34.8% | 1 | Sufficient |
| Nickel Ore | 60.0 | 32.4% | 1 | Sufficient |
| Iridium Ore | 49.0 | 31.4% | 2 | Sufficient |
| Titanium Ore | 30.0 | 26.1% | 1 | Sufficient |
| Lead Ore | 14.0 | 23.3% | 1 | Sufficient |
| Uranium Ore | 17.0 | 22.7% | 1 | Sufficient |
| Vanadium Ore | 118 | 22.6% | 3 | Sufficient |
| Tungsten Ore | 113 | 22.1% | 3 | Sufficient |
| Plasma Gas | 28.0 | 20.3% | 1 | Sufficient |
| Copper Ore | 191 | 18.5% | 6 | Sufficient |
| Aluminum Ore | 58.0 | 17.2% | 1 | Sufficient |
| Carbon Ore | 240 | 16.9% | 4 | Sufficient |
| Iron Ore | 194 | 13.8% | 6 | Scarce |
| Water Ice | 168 | 12.3% | 3 | Scarce |
| Argon Gas | 136 | 11.9% | 3 | Scarce |
| Hydrogen Gas | 144 | 11.1% | 2 | Scarce |
| Nitrogen Ice | 95.0 | 10.4% | 2 | Scarce |
| Platinum Ore | 40.0 | 9.5% | 1 | Scarce |
| Adamantite Ore | — | 0% | 0 | Missing |
| Antimatter Containment Cell | — | 0% | 0 | Missing |
| Chlorine Gas | — | 0% | 0 | Missing |
| Cobalt Ore | — | 0% | 0 | Missing |
| Darksteel Ore | — | 0% | 0 | Missing |
| Deuterium Ice | — | 0% | 0 | Missing |
| Energy Crystal | — | 0% | 0 | Missing |
| Exotic Crystal | — | 0% | 0 | Missing |
| Exotic Matter | — | 0% | 0 | Missing |
| Fluorine Gas | — | 0% | 0 | Missing |
| Fury Crystal | — | 0% | 0 | Missing |
| Gold Ore | — | 0% | 0 | Missing |
| Helium Ice | — | 0% | 0 | Missing |
| Ion Gas | — | 0% | 0 | Missing |
| Krypton Gas | — | 0% | 0 | Missing |
| Legacy Ore | — | 0% | 0 | Missing |
| Nebula Gas | — | 0% | 0 | Missing |
| Neon Gas | — | 0% | 0 | Missing |
| Null Matter | — | 0% | 0 | Missing |
| Palladium Ore | — | 0% | 0 | Missing |
| Prismatic Nebulite | — | 0% | 0 | Missing |
| Silicon Ore | — | 0% | 0 | Missing |
| Silver Ore | — | 0% | 0 | Missing |
| Sol Alloy Ore | — | 0% | 0 | Missing |
| Thorium Ore | — | 0% | 0 | Missing |
| Trade Crystal | — | 0% | 0 | Missing |
| Void Crystal | — | 0% | 0 | Missing |
| Void Essence | — | 0% | 0 | Missing |
| Xenon Gas | — | 0% | 0 | Missing |

**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):

| Product | Kind | Ease | Bottleneck Resource |
|---------|------|-----:|---------------------|
| Liquid Nitrogen | item | 10.4% | Nitrogen Ice |
| Dark Matter Cell | item | 11.1% | Hydrogen Gas |
| Fuel Tank | item | 11.1% | Hydrogen Gas |
| Liquid Hydrogen | item | 11.1% | Hydrogen Gas |
| Prayer | ship | 11.1% | Hydrogen Gas |
| Premium Fuel Cell | item | 11.1% | Hydrogen Gas |
| Purified Argon | item | 11.9% | Argon Gas |
| Bioactive Compound | item | 12.3% | Water Ice |
| Bioluminescent Culture | item | 12.3% | Water Ice |
| Cryogenic Storage | item | 12.3% | Water Ice |

### Solarian Empire

**Resource Position**

| Resource | Richness | Galaxy Share | POIs | Verdict |
|----------|---------:|-------------:|-----:|---------|
| Antimatter Containment Cell | 14.0 | 100.0% | 4 | Dominant |
| Ion Gas | 13.0 | 100.0% | 1 | Dominant |
| Legacy Ore | 9.0 | 100.0% | 2 | Dominant |
| Sol Alloy Ore | 39.0 | 100.0% | 4 | Dominant |
| Deuterium Ice | 29.0 | 61.7% | 2 | Dominant |
| Helium Ice | 49.0 | 50.0% | 3 | Dominant |
| Aluminum Ore | 157 | 46.4% | 3 | Dominant |
| Water Ice | 571 | 41.7% | 9 | Dominant |
| Chlorine Gas | 51.0 | 39.2% | 3 | Sufficient |
| Nickel Ore | 70.0 | 37.8% | 1 | Sufficient |
| Gold Ore | 36.0 | 37.1% | 1 | Sufficient |
| Tungsten Ore | 155 | 30.3% | 4 | Sufficient |
| Nitrogen Ice | 276 | 30.3% | 6 | Sufficient |
| Nebula Gas | 41.0 | 27.7% | 2 | Sufficient |
| Iridium Ore | 43.0 | 27.6% | 2 | Sufficient |
| Hydrogen Gas | 348 | 26.9% | 6 | Sufficient |
| Thorium Ore | 18.0 | 26.9% | 1 | Sufficient |
| Argon Gas | 304 | 26.7% | 7 | Sufficient |
| Platinum Ore | 102 | 24.2% | 3 | Sufficient |
| Carbon Ore | 324 | 22.8% | 6 | Sufficient |
| Titanium Ore | 25.0 | 21.7% | 1 | Sufficient |
| Copper Ore | 224 | 21.7% | 7 | Sufficient |
| Iron Ore | 304 | 21.7% | 7 | Sufficient |
| Vanadium Ore | 106 | 20.3% | 3 | Sufficient |
| Palladium Ore | 44.0 | 19.9% | 2 | Sufficient |
| Neon Gas | 77.0 | 18.2% | 3 | Sufficient |
| Plasma Gas | 19.0 | 13.8% | 1 | Scarce |
| Adamantite Ore | — | 0% | 0 | Missing |
| Cobalt Ore | — | 0% | 0 | Missing |
| Dark Matter Residue | — | 0% | 0 | Missing |
| Darksteel Ore | — | 0% | 0 | Missing |
| Energy Crystal | — | 0% | 0 | Missing |
| Exotic Crystal | — | 0% | 0 | Missing |
| Exotic Matter | — | 0% | 0 | Missing |
| Fluorine Gas | — | 0% | 0 | Missing |
| Fury Crystal | — | 0% | 0 | Missing |
| Krypton Gas | — | 0% | 0 | Missing |
| Lead Ore | — | 0% | 0 | Missing |
| Lithium Ore | — | 0% | 0 | Missing |
| Null Matter | — | 0% | 0 | Missing |
| Phase Crystal | — | 0% | 0 | Missing |
| Polonium Ore | — | 0% | 0 | Missing |
| Prismatic Nebulite | — | 0% | 0 | Missing |
| Quantum Fragments | — | 0% | 0 | Missing |
| Radium Ore | — | 0% | 0 | Missing |
| Silicon Ore | — | 0% | 0 | Missing |
| Silver Ore | — | 0% | 0 | Missing |
| Trade Crystal | — | 0% | 0 | Missing |
| Uranium Ore | — | 0% | 0 | Missing |
| Void Crystal | — | 0% | 0 | Missing |
| Void Essence | — | 0% | 0 | Missing |
| Xenon Gas | — | 0% | 0 | Missing |

**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):

| Product | Kind | Ease | Bottleneck Resource |
|---------|------|-----:|---------------------|
| Ghost Rounds | item | 13.8% | Plasma Gas |
| Hot Cell | item | 13.8% | Plasma Gas |
| Military Fuel Cell | item | 13.8% | Plasma Gas |
| Ionized Neon | item | 18.2% | Neon Gas |
| Armor Plate III | item | 20.3% | Vanadium Ore |
| Capital Ship Frame | item | 20.3% | Vanadium Ore |
| Durasteel Plate | item | 20.3% | Vanadium Ore |
| Hull Reinforcement III | item | 20.3% | Vanadium Ore |
| Armor Piercing Rounds Box | item | 21.7% | Iron Ore |
| Armor Plate | item | 21.7% | Iron Ore |

### Voidborn Empire

**Resource Position**

| Resource | Richness | Galaxy Share | POIs | Verdict |
|----------|---------:|-------------:|-----:|---------|
| Energy Crystal | 20.0 | 100.0% | 4 | Dominant |
| Null Matter | 82.0 | 100.0% | 4 | Dominant |
| Void Essence | 9.0 | 100.0% | 2 | Dominant |
| Silicon Ore | 90.0 | 56.2% | 1 | Dominant |
| Lithium Ore | 45.0 | 51.1% | 1 | Dominant |
| Palladium Ore | 72.0 | 32.6% | 3 | Sufficient |
| Thorium Ore | 19.0 | 28.4% | 1 | Sufficient |
| Copper Ore | 239 | 23.2% | 7 | Sufficient |
| Carbon Ore | 285 | 20.0% | 5 | Sufficient |
| Tungsten Ore | 95.0 | 18.6% | 3 | Sufficient |
| Iron Ore | 248 | 17.7% | 7 | Sufficient |
| Uranium Ore | 13.0 | 17.3% | 1 | Sufficient |
| Lead Ore | 10.0 | 16.7% | 1 | Sufficient |
| Vanadium Ore | 83.0 | 15.9% | 2 | Sufficient |
| Neon Gas | 62.0 | 14.7% | 2 | Scarce |
| Platinum Ore | 56.0 | 13.3% | 2 | Scarce |
| Nitrogen Ice | 104 | 11.4% | 2 | Scarce |
| Hydrogen Gas | 119 | 9.2% | 2 | Scarce |
| Iridium Ore | 11.0 | 7.1% | 1 | Scarce |
| Water Ice | 70.0 | 5.1% | 1 | Scarce |
| Argon Gas | 55.0 | 4.8% | 1 | Scarce |
| Adamantite Ore | — | 0% | 0 | Missing |
| Aluminum Ore | — | 0% | 0 | Missing |
| Antimatter Containment Cell | — | 0% | 0 | Missing |
| Chlorine Gas | — | 0% | 0 | Missing |
| Cobalt Ore | — | 0% | 0 | Missing |
| Dark Matter Residue | — | 0% | 0 | Missing |
| Darksteel Ore | — | 0% | 0 | Missing |
| Deuterium Ice | — | 0% | 0 | Missing |
| Exotic Crystal | — | 0% | 0 | Missing |
| Exotic Matter | — | 0% | 0 | Missing |
| Fluorine Gas | — | 0% | 0 | Missing |
| Fury Crystal | — | 0% | 0 | Missing |
| Gold Ore | — | 0% | 0 | Missing |
| Helium Ice | — | 0% | 0 | Missing |
| Ion Gas | — | 0% | 0 | Missing |
| Krypton Gas | — | 0% | 0 | Missing |
| Legacy Ore | — | 0% | 0 | Missing |
| Nebula Gas | — | 0% | 0 | Missing |
| Nickel Ore | — | 0% | 0 | Missing |
| Phase Crystal | — | 0% | 0 | Missing |
| Plasma Gas | — | 0% | 0 | Missing |
| Polonium Ore | — | 0% | 0 | Missing |
| Prismatic Nebulite | — | 0% | 0 | Missing |
| Quantum Fragments | — | 0% | 0 | Missing |
| Radium Ore | — | 0% | 0 | Missing |
| Silver Ore | — | 0% | 0 | Missing |
| Sol Alloy Ore | — | 0% | 0 | Missing |
| Titanium Ore | — | 0% | 0 | Missing |
| Trade Crystal | — | 0% | 0 | Missing |
| Void Crystal | — | 0% | 0 | Missing |
| Xenon Gas | — | 0% | 0 | Missing |

**Top 10 Hardest Solo-Buildable Products** (lowest ease — empire can build but is thin on a key input):

| Product | Kind | Ease | Bottleneck Resource |
|---------|------|-----:|---------------------|
| Purified Argon | item | 4.8% | Argon Gas |
| Bioactive Compound | item | 5.1% | Water Ice |
| Bioluminescent Culture | item | 5.1% | Water Ice |
| Crystalline Lattice | item | 5.1% | Water Ice |
| Endurance Booster | item | 5.1% | Water Ice |
| Focus Stim | item | 5.1% | Water Ice |
| Liquid Oxygen | item | 5.1% | Water Ice |
| Oxygen Gas | item | 5.1% | Water Ice |
| Pirate Moonshine | item | 5.1% | Water Ice |
| Premium Fuel Cell | item | 5.1% | Water Ice |

### Galaxy Headlines

**Empire Monopolies** (one empire holds ≥80% of galaxy richness):

| Resource | Empire | Share |
|----------|--------|------:|
| Krypton Gas | Nebula | 100.0% |
| Fury Crystal | Crimson | 100.0% |
| Darksteel Ore | Crimson | 100.0% |
| Dark Matter Residue | Outerrim | 100.0% |
| Void Essence | Voidborn | 100.0% |
| Null Matter | Voidborn | 100.0% |
| Sol Alloy Ore | Solarian | 100.0% |
| Prismatic Nebulite | Nebula | 100.0% |
| Cobalt Ore | Crimson | 100.0% |
| Trade Crystal | Nebula | 100.0% |
| Ion Gas | Solarian | 100.0% |
| Quantum Fragments | Outerrim | 100.0% |
| Phase Crystal | Outerrim | 100.0% |
| Energy Crystal | Voidborn | 100.0% |
| Antimatter Containment Cell | Solarian | 100.0% |
| Legacy Ore | Solarian | 100.0% |

**Contested Resources** (present in ≥3 empires, no empire holds ≥40%):

| Resource | Empires Present | Largest Holder Share |
|----------|---------------:|---------------------:|
| Carbon Ore | 5 | 22.8% |
| Copper Ore | 5 | 23.2% |
| Vanadium Ore | 5 | 26.4% |
| Iron Ore | 5 | 28.7% |
| Nitrogen Ice | 5 | 30.3% |
| Tungsten Ore | 5 | 30.3% |
| Argon Gas | 5 | 30.9% |
| Iridium Ore | 5 | 31.4% |
| Platinum Ore | 5 | 31.8% |
| Plasma Gas | 4 | 34.1% |
| Palladium Ore | 4 | 34.4% |
| Radium Ore | 3 | 34.8% |
| Hydrogen Gas | 5 | 35.6% |
| Nickel Ore | 3 | 37.8% |
| Lead Ore | 4 | 38.3% |
| Uranium Ore | 4 | 38.7% |
| Chlorine Gas | 3 | 39.2% |
| Neon Gas | 4 | 39.2% |

## 4. Composite Self-Sufficiency Index

SSI is a 0–100 score: half breadth (how much of the catalog the empire can solo-build) and half comfort (how rich the bottleneck inputs are, averaged across those products).

| Rank | Empire | SSI | Breadth | Mean Ease | Items Solo | Ships Solo | Exclusive |
|-----:|--------|----:|--------:|----------:|-----------:|-----------:|----------:|
| 1 | Solarian | 25.2 | 10.0% | 40.5% | 70 | 1 | 2 |
| 2 | Nebula | 24.6 | 12.3% | 36.9% | 84 | 4 | 31 |
| 3 | Crimson | 24.2 | 9.8% | 38.6% | 70 | 0 | 4 |
| 4 | Voidborn | 23.3 | 7.4% | 39.2% | 53 | 0 | 8 |
| 5 | Outerrim | 20.5 | 9.3% | 31.7% | 65 | 1 | 2 |


---

## Appendix: Methodology

- **Products analyzed**: every craftable item (excluding raw ores/materials) plus every ship in the catalog. Items whose BoM bottoms out on something other than a base ore/material (e.g. salvage-only drops) are skipped.
- **Base materials**: items in category `ore` or `material` — the terminals of `pkg/bom`'s recipe-resolution tree.
- **Empire scope**: only the five named empires (crimson, nebula, outerrim, solarian, voidborn). POIs in unaligned systems are excluded from per-empire totals (but counted in the galaxy denominator).
- **Presence**: an empire "has" a resource if at least one POI in its systems lists richness > 0 for it.
- **Galaxy share**: `sum(richness in empire's POIs) / sum(richness in all named-empire POIs)`. Verdict bands: ≥40% Dominant, 15–40% Sufficient, 1–15% Scarce, 0 Missing.
- **Ease score** for empire E building product P: the minimum galaxy share across P's base materials in E — bottleneck thinking.
- **SSI** (Self-Sufficiency Index, 0–100): `0.5 * (fraction of products E can solo-build) * 100 + 0.5 * (mean ease across those products) * 100`.
