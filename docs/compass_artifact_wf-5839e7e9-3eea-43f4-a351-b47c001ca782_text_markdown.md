# Procedurally generating planets that don't look procedural

The single biggest leverage points for your Go pipeline are not better noise — they are **independent control fields** (the Minecraft-1.18 multi-noise approach), **perceptual color spaces** for biome blending (OkLab via `go-colorful`), **domain warping** of every height/cloud lookup (Inigo Quilez), and a **province/plate map** that *modulates* later passes so the planet has regions of distinct character instead of uniform fBm. Everything else — craters with ejecta rays, fractal coastlines, gas-giant curl-noise advection, civilization signs, cloud weather bands — composes on top of that scaffold. The current `PlanetProfile` is doing too much in one layer; the upgrade is conceptually a **stack of orthogonal scalar/vector fields** (continentalness, erosion, peaks-and-valleys, temperature, moisture, age, wind, plates) each with its own noise, then biome/feature lookups derived from them. This is how Dwarf Fortress, Minecraft 1.18, Red Blob, Azgaar, and Star Citizen Planet Tech v5 all work, and the lesson is consistent: **few orthogonal fields + good lookup tables ≫ one big noise**.

Below: each of your seven questions answered concretely, then the cross-cutting topics, then a prioritized "biggest bang for the buck" list.

---

## 1. Additional `PlanetProfile` parameters for variety and density

The current profile fuses control of *what* a planet is (type, palette) with *how its noise behaves* (octaves, scale). The richer pattern is to break the struct into **subsystems**, each driven by its own seeded sub-config. The most impactful additions, grouped:

**Cross-cutting infrastructure fields.** Add `Seed int64`, `DomainWarpAmp/Freq/Octaves` (a single Quilez warp roughly doubles the perceived organic-ness of every downstream field; `q = p + amp·vec3(noise(p+a), noise(p+b), noise(p+c))`), `ProvinceCount/Jitter/WarpAmp` (8–40 warped Voronoi seeds whose cell IDs key into per-province feature recipes — this is what kills the "uniform fBm" look), a low-frequency triple `RoughnessAmp/Freq/Type` map that locally modulates amplitude/frequency/operator of the detail layer, and a single scalar **`SurfaceAge ∈ [0,1]`** that drives crater density, ejecta brightness, erosion exponent, and sediment depth all at once. This last parameter alone unifies the Mercury (young, sharp rays) → Callisto (ancient, soft palimpsest) spectrum.

**Crater system enhancements.** Replace `CraterCount/MinRadius/MaxRadius/Depth` with a richer `CraterSystem` struct. The accepted "stylized but believable" shape (Sebastian Lague / Roy-T) blends a parabolic bowl, a rim hump, and a flat floor with Quilez polynomial smoothing:

```
cavity = x² − 1
rim    = max(x − rimWidth, 0)² · rimSteepness
combined = smoothMin(cavity, rim, k)
height   = smoothMax(combined, floorDepth, k)
```

Sample diameters from a **truncated power-law** with slope α ∈ [−2, −3.5] (Hartmann/Neukum production functions; Robbins 2018):
`D = ((u·(Dmax^(1−α) − Dmin^(1−α))) + Dmin^(1−α))^(1/(1−α))`. Spatially modulate density by a low-frequency mask × a `MariaDensityFactor` (lunar maria ≈ 10× less cratered than highlands), giving the Mars dichotomy and lunar maria/highlands contrast for free. Add **age per crater** (beta-distributed, biased by `SurfaceAge`) so amplitude scales as `(1 − age)^ErosionExponent` — this produces the realistic palimpsest mix of crisp + soft craters on the same surface. **Ejecta rays** are albedo features, not topography: angular sinusoid `0.5 + 0.5·cos(N_rays·θ + φ)` raised to a sharpness power, multiplied by radial falloff `exp(−r/L)`, modulated by a streak fBm and `(1 − age)²`. Sabuwala et al. (PRL 2018) show ray count scales with `D / λ_granularity`, so `RayCountPerUnitD` is a meaningful parameter. Add **secondaries** (annulus around primaries with steeper SFD slope ~−4), **catena** (great-circle arcs of monotone-shrinking craters from tidal disruption), and **multi-ring basins** for the largest craters (2–4 concentric rim rings at fractional radii {1.0, 1.6, 2.4}). Finally, branch on diameter for floor type: `Bowl` (small) → `Flat` → `CentralPeak` → `PeakRing` → `MareFlooded` (when local height < maria threshold).

**Ridge / scarp / valley features.** Add a `RidgeSystem` with ridged-multifractal mountain belts (`r(p) = (offset − |noise|)²` with `weight = clamp(prev·gain·r, 0, 1)`), made **anisotropic** by stretching the input vector along the gradient of a "belt direction" field — this is what makes mountains follow plate boundaries instead of forming spaghetti everywhere. **Lobate scarps** are 1D cliffs from a directional iso-contour: `scarp = h·smoothstep(−w, w, f(p) − threshold)`. **Canyon networks** (Valles Marineris, Noctis Labyrinthus) want **Perlin worms** with a downhill bias: each step direction is `α·gradient(h) + (1−α)·randomNoiseDir`, carve a U-profile around the great-circle path. **Mars striations** are altitude-banded albedo with horizontal frequency perturbed by a small noise (Hugo Elias / GPU Gems 3 trick): `t = altitude·freq + distort·noise(p·4); albedoOffset = sin(2π t)·contrast`.

**Dune fields.** A `DuneSystem` keyed on a smooth **wind tangent vector field** `W(p)` (curl-of-fBm projected to tangent plane → divergence-free streamlines, no convergence artifacts). Then **anisotropic noise** sampled in a coordinate frame stretched along `W(p)` produces transverse dunes for free (Goldberg/Zwicker/Durand SIGGRAPH 2008). A `DuneTypeMix` parameter blends transverse / barchan / longitudinal / star morphologies. For the very-small-scale ripples superimposed on dunes, a Gray-Scott reaction-diffusion run once at low resolution produces aligned ripples (Turing patterns naturally orient perpendicular to a forcing gradient — feed wind direction as that gradient). Worth doing offline.

**Lava and volcanic features.** A `VolcanicSystem` with Poisson-disc-placed volcano centers stamping shield+caldera profiles, and **MrLavaLoba-style probabilistic flow paths** (next step weighted by neighbor `slope^β`) emitting M lobes per volcano to build a lava mask raster. Pyroclastic darkening is a Gaussian-falloff albedo modifier; crusted vs molten is Voronoi `F2−F1` cracked-pattern × lava mask. For `lava_world` types, output an extra **emission channel** keyed to `LavaEmissionGain · smoothstep(0.6, 1.0, lavaMask) · noise`.

**Ice and glacial patterns.** A `IceSystem` that mirrors dunes but with very high-frequency anisotropic ridges aligned to a glacial flow gradient (crevasse fields). **Chaos terrain** (Europa) is *domain-warped Voronoi*: warp `p` by a high-amplitude noise, take Voronoi cells, give each cell a random tilt, soften boundaries by `smoothstep(boundaryWidth, 0, F2−F1)`. **Linea** are great-circle arcs displaced by low-freq noise, stamped as triple-band albedo (light center, dark edges); 4 morphological types (bands, double ridges, ridge complexes, undifferentiated — see Lineamapper 2023).

**Regional variation in roughness.** This is the highest-ROI single addition. A warped-Voronoi province map plus three **independent** low-frequency fields (`R_amp, R_freq, R_type`) modulating the detail noise as `detail = R_amp·baseNoise(p·R_freq, mode_from_R_type)`. Combined with a "dichotomy axis" parameter, this reproduces the Mars north-smooth-plains / south-cratered-highlands split with one extra noise. Mix macro and micro layers via `mix(macro, macro+micro·R_amp, continentMask)` rather than summing; sums always look like fuzz.

**Misc.** `AlbedoNoise*` fields for color variation independent of height (Iapetus, Enceladus tiger stripes, Mars dust). `WindStreak*` fields to smear albedo darkening behind craters along the local wind vector — visible on every Mars HiRISE image. `Sediment*` for low-pass-filtered fill of basins (different color in low pixels). `Playa*` for flat polygonal-cracked salt flats. `Frost*` distinct from snow line, biased by slope aspect. `RimAlbedoBoost*` for the ejecta-ring discoloration.

---

## 2. Fractal, natural coastlines for terran worlds

Your current "fractal-noise < OceanLevel" gives uniform fuzz because the threshold band is the same width everywhere, all length scales are coupled (fBm is isotropic and self-similar), and there's no large-scale structure. The fix is a **layered cocktail**, not a better noise.

The **single highest-ROI** technique is **domain warping** (Quilez): replace `f(p)` with `f(p + q(p))` where `q(p)` is itself a 3D vector noise. One iteration with amplitude 0.3–0.8 and frequency 0.5–1.0 turns blobby coastlines into fjord-like fingering with no extra octaves. Two iterations gives Mandelbrot-river-delta organic shapes. For sphere generation, sample three independent fBm fields as the three components of `q`.

Add **ridged multifractal** noise (Musgrave/libnoise: `r = (offset − |noise|)²` with multiplicative weighting), MAX-blended with normal fBm only inside a "mountain mask" — this gives Norway/Chile/Aleutian-style crested coasts. Then layer on **coastal noise enhancement** (mapgen4 recipe): `e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)`. The `(1 − e⁴)` bell peaks at sea level and decays both inland and offshore, so high-frequency detail lives only at the coast — archipelagos and atolls appear at every zoom for free. **Voronoi continental plates** (10–50 seeds via Fibonacci spiral on the sphere, each cell warped by a noise vector before nearest-seed lookup) provide the polygonal continental silhouette that fBm cannot. The combination most cited in practice is: **Voronoi continents → domain-warped fBm elevation → coastal-noise enhancement → ridged multifractal in mountain belts.** This is essentially the Red Blob / mapgen4 / `raguilar011095` cocktail.

For "alien but plausible" feel, change the **continental mask shape** (radial blobs for Spore; high-N hexagonal Voronoi for "shattered" worlds; anisotropic warping for banded continents). Martin O'Leary's central insight is structurally different and worth knowing: **don't start from noise — start from a coastline shape and define elevation as distance-from-coast**, then erode. This is a stronger, more controllable substitute when you want curated-feeling planets.

For **scale-invariance**, set `lacunarity = 2.0` exactly with `amplitude ∝ r^−H` (`H ≈ 0.8–1.0`), and rotate each octave by an irrational angle (Quilez's `m = mat2(0.80, 0.60, −0.60, 0.80)` trick) to prevent lattice alignment that becomes obvious when zooming. A cheap **simulated coastal erosion** — Gaussian blur weighted only where `|e − seaLevel| < ε`, followed by a contrast remap — smooths the kilometer scale while leaving mountain interiors crisp.

---

## 3. Multi-layer heightmap composition

The libnoise "complex planet" example composes ~100 modules; you do not need that depth, but you do need the **operator vocabulary**. Concrete combiners that earn their cost:

`continent_mask · detail_fbm` — base form. Forces detail onto land; ocean floor stays flat. **Frequency-stack three explicit bands**: continental ridged at `f≈0.5–1`, billow rolling hills at `f≈4–8`, fBm dunes at `f≈32–64`, summed with weights (1, 0.3, 0.07). The visual signature of Earth-scale matte paintings is "billowing in the meso, sharp at the macro." Use **`select(c, a, b, falloff)`** (libnoise smooth-edged selector) for biome-specific terrain dispatch on a noise-driven control field: plains↔mountains, dunes↔scree, etc.

The **ridged-mountains-masked-by-continentality** pattern is the most under-used trick: `h = base + smoothstep(0.5, 0.7, distance_to_plate_boundary) · ridged_multifractal`. Without masking, ridged noise produces unrealistic spaghetti-mountains everywhere; with masking, mountain belts align with plate convergence as on Earth. Treat elevation as `h = uplift − erosion` (Cordonnier 2016 stream-power: `∂h/∂t = U − k·A^m·|∇h|^n`) when you have a flow-accumulation field.

**Layered Perlin worms** are the right tool for canyon networks (Valles Marineris, Mariner Valley). A worm is a particle path whose 3D direction is `normalize(noise_x(p), noise_y(p), noise_z(p))`; trace N worms from canyon-source seeds, carve a cone-shaped trench around each path. On a sphere, re-normalize position each step to stay on the surface. Iterating worms-of-worms gives dendritic detail. **Billow** (`2|fBm| − 1`) for plains/dunes, **ridge** for tectonic uplift, plain fBm for transitions. **Warp high-frequency detail by the gradient of a low-frequency flow field** to align ridges directionally — this is what makes No Man's Sky's "uber noise" look directional rather than blobby.

The cheat-sheet of operators worth implementing: `add` (layer detail), `mul` (mask/gate with [0,1] mask), `max` (cliff-form unions), `min` (carve), `pow(a, k)` for redistribution (k>1 sharpens, k<1 flattens; Patel's `1 − (1−x)²` shifts the histogram so most of the planet is ocean), `select` for biome dispatch, `warp` per Quilez, `billow` and `ridge` as `abs`-variants.

---

## 4. Better biome generation and coloring

The fundamental fix: **stop driving biomes from latitude alone**. Drive them from at least two independent noise fields (temperature, moisture) plus modifiers (continentality, rain shadow, river flow, altitude). This is the Whittaker model — used by Red Blob, mapgen2/4, Dwarf Fortress, Minecraft 1.18, and Azgaar.

**Whittaker classification.** Generate `T(p)` and `M(p)` as independent 3D fBm with different seed offsets. Add a latitude term to `T` (`T_base = poleC + (equatorC − poleC)·cos(lat)`) and an elevation lapse rate (~6.5 °C/km). Look up biome from a 2D table; for soft edges, **bilinearly interpolate in (T, M) space between neighboring palette entries** rather than snapping. Add small per-pixel jitter (±3 channel units) sourced from a high-frequency noise. This single step makes deserts and rainforests coexist at the same latitude — the unmistakable signature of "real" biome maps.

**Continentality.** Compute distance-to-ocean via **Jump Flooding Algorithm** (Rong & Tan 2006, ~11 passes for a 2000-wide image with horizontal wrap), then `M' = M·exp(−D/L_cont)` with `L_cont ≈ 3000 km`. JFA is approximate but adequate for moisture. Paradox uses JFA for province borders too — same code reusable.

**Rain shadow / orographic precipitation.** Define a latitude-dependent prevailing wind (trades easterly < 30°, westerlies < 60°, polar easterlies). Sweep moisture parcels east-to-west or west-to-east in each latitude row, picking up moisture over ocean and depositing it on the upwind side of mountains: `if dE > 0: rain = min(carry, dE·gain); carry -= rain`. Lee side: `M = base · 0.15` (Wikipedia notes lee can be 1/10 windward). Azgaar and mapgen4 both implement this; the visual payoff is unmistakable (Atacama-style coastal deserts forming naturally).

**Voronoi biome mosaicing.** On top of Whittaker, scatter ~100–500 biome seeds with Poisson-disc on the sphere; smooth by storing soft weights over the `k=3` nearest seeds (`w_i = exp(−d_i/σ)`). Add domain-warp on the seed lookup so cell edges aren't straight. Result: large irregular zones, Stellaris-flavored.

**Rivers and watersheds.** D8 flow accumulation (Jenson & Domingue 1988): fill depressions (Planchon-Darboux), assign each pixel a downhill neighbor, topologically sort by elevation high→low, accumulate `acc[down] += acc[me] + 1`. Pixels with `acc > τ` are rivers; paint them as bright threads, then add a Gaussian moisture/vegetation boost in a 5–15 px radius around them — the Nile-through-Sahara look that's missing from pure climate models.

**Treelines vary with latitude** as `treelineKm = equatorTreelineKm · cos(lat)^power`. Or use Red Blob's "equivalent elevation" formulation `eq = e + poles + (equator−poles)·sin(πy/H)` for a single-axis biome lookup.

**Blue-noise patches** for ecological transitions: Poisson-disc samples on the sphere, each picks a biome via `(T, M)` but with 20% chance of jittering into a neighboring Whittaker cell. Each sample dilates with a soft-falloff radius. This produces the speckled mosaic you see at biome boundaries (oases in deserts, forest patches in tundra) — the missing texture in latitude-band approaches.

**How specific games handle it.** *Dwarf Fortress* uses six orthogonal scalar fields (elevation, rainfall, temperature, drainage, savagery, volcanism) plus a discrete good/neutral/evil alignment overlay; biomes are intersections (high rainfall + low drainage → swamp). *Minecraft 1.18* (Henrik Kniberg JFokus 2022) uses six independent 3D noises — **continentalness, erosion, weirdness, temperature, humidity, depth** — combined via **splines, not multiplication**, then biomes assigned via 5D nearest-bucket lookup. **This is the model your generator should most closely emulate.** *Civilization* uses prevailing-wind moisture transport. *No Man's Sky* picks a planet **archetype** first (lush/desert/frozen/exotic) with its own 3–5 color palette, then generates within it (Sean Murray GDC 2017; Grant Duncan emphasized rule-based palettes inspired by 70s sci-fi covers, not random hue shifts — Kate Compton's "procedural oatmeal" warning). *Caves of Qud* layers abstractions (region → village concept → architecture → individual building) rather than sampling everything from one noise. *Stellaris* applies a per-archetype color-correction LUT over hand-painted variants.

The **#1 color upgrade for trivial code** is switching biome-color interpolation to **OkLab or HCL via `lucasb-eyer/go-colorful`**. RGB interpolation goes through muddy gray; Lab/OkLab gradients between desert and tundra stay perceptually clean. ~20 lines of code, dramatic improvement.

---

## 5. Procedural urban / civilization signs visible from orbit

For 2000×1000 textures viewed from orbital distance, civilizations should be small (1–10 px) clustered features with two output variants — **dayside** (greys, agriculture, deforestation, roads) and **nightside** (warm pinpoints with bloom). The NASA Black Marble (Suomi NPP VIIRS Day-Night Band) is the canonical reference image: clusters of warm-amber points with falloff around city cores, forming "necklaces" along coastlines and rivers (most of Earth's lights cluster within ~200 km of coast).

**Settlement positions.** Compute a per-pixel **habitability score** weighted across coast bonus (peak ~10–50 km from coast), river bonus (flow accumulation), temperature bell curve (centered at 15 °C), elevation `exp(−elev/h_ref)`, slope `(1 − slope)`, fertility from biome, plus penalties for desert and `|lat| > 70°`. Sample candidates with **Bridson Poisson-disc on the sphere** at minimum spacing `r_min ≈ 30 km` (megacities) plus a denser pass at `r_min ≈ 5 km` for towns; accept each candidate with probability `S(p)^γ` (γ = 2–3). Optionally do a Mitchell's-best-candidate pass (`S(p) · minDistToExisting`).

**Zipfian populations.** Sort candidates by score descending, assign `pop = popMax · rank^(−1)` × jitter. This gives 1 megacity, ~10 large cities (10× smaller), ~100 medium, ~1000 villages — the Black Marble brightness distribution.

**Nightside lights.** Per settlement: `intensity = (pop/popMax)^0.6` (sublinear — cities don't get proportionally brighter), color lerps from sodium-amber `#ffaa55` (small/agrarian) to white `#ffeebb` (modern megacities) to cool `#cce4ff` (industrial/spacefaring). Plant `3 + log₂(pop)` sub-points within radius (sprawl), splat each as a Gaussian of `σ ≈ 1.5 px` at full intensity plus a wide `σ ≈ 6 px` halo at 0.1× for atmospheric scatter. Add power-law decay points around the cluster (suburbs at `1.5R..8R`, `intensity ∝ 1/d²`). Bias sub-cluster placement along the local coastline tangent (sample distance-to-ocean gradient, prefer perpendicular spread). Tonemap finally with `1 − exp(−x·exposure)` so megacities glow without clipping smaller ones.

**Dayside city patches.** Per settlement: `cityRadius ≈ 1 + 3·√(pop/popMax)`. Coverage falls off via `smoothstep(R·1.2, R·0.3, d)`. Modulate biome color toward city color (tan/grey palette `#9a8e7a, #7c7468, #a89c84`) by 0.7 of coverage; desaturate by 0.3 of coverage. Multiply in a high-frequency cell noise (0.5 km scale) so the patch has visible internal structure.

**Agriculture grids.** Mask farmable areas as `(0.3 < M < 0.85) ∧ (slope < 0.05) ∧ (5°C < T < 30°C) ∧ (distToSettlement < 200 km)`. Within Voronoi field cells (2–10 km), pick a random rotation per cell and render a grid via `step(0.5, fract(ux/fieldSize)) ⊕ step(0.5, fract(uy/fieldSize))`. Color-shift toward yellow-green `#c4d878` (wheat) or harvested `#c8a050`. A high-frequency anisotropic noise stretched 8:1 along field rotation gives the impression of plowing rows. Voronoi cell variation between neighboring fields produces the patchwork-quilt look visible at orbital scales (US Midwest, Indo-Gangetic plain, Pampas).

**Roads.** Build a Delaunay triangulation over settlements (use stereographic projection or `fogleman/delaunay`); compute MST weighted by `length·(1 + terrainCost)` with A* paths penalizing mountains/oceans; add ~30% of remaining Delaunay edges with probability proportional to `popA·popB / dist²` (gravity model). Render as 1–2 px lines; major routes 2 px with faint amber tint. For nightside, sprinkle dim lights along major roads at ~10 px intervals — this is what creates the "highway necklaces" between US east-coast cities on Black Marble.

**Cleared land / deforestation.** Radial fade around each settlement with ragged `pow(noise, 0.5)` edge, shifting forest biomes toward grassland by 60% of fade intensity, slight brightness up + saturation down. Add sparse outlying splotches (slash-and-burn) using coarse blue-noise.

**Civilization tier as a single scalar** `civTier ∈ [0,1]` gates everything: stone-age has nothing; agrarian has tiny dim lights and dirt tracks; industrial adds gridlike roads + rectangular fields; modern has full Black Marble brightness; spacefaring adds continental light bridges and sometimes domed circular geometric agriculture. Each feature multiplies by `smoothstep(threshold, 1.0, civTier)`.

For street-level detail Parish & Müller "Procedural Modeling of Cities" (SIGGRAPH 2001) use self-sensitive L-systems, but at planet scale MST + Delaunay + gravity model is sufficient and much simpler. **Don't try to L-system individual streets** at 2 km/pixel.

---

## 6. Cloud layer overlay

A separate alpha-channel image overlaid on the planet, generated as a stack of latitude-banded coverage modulators × domain-warped fBm × storm vortex displacements, then fake-shaded with the sun direction.

**Octave recipes for cloud types.** Cumulus uses 5–7 octaves billow/fBm at lacunarity 2.0, gain 0.5, thresholded at `smoothstep(0.45, 0.65, n)`. Cirrus uses 6–8 octaves *ridged* at gain 0.55–0.6, threshold 0.55–0.7, then *zonally stretched* to make filaments. Stratus is 3–4 octaves fBm low contrast. Storm anvils mix fBm + ridged at high threshold. Per Quilez, "detune" frequencies (2.01, 1.99, 2.03) and rotate per octave to break grid alignment.

**Domain warp.** One Quilez warp pass with `k ≈ 0.3–1.5` is enough; for sphere sampling build a vec3 warp by sampling at three offsets:
```
warp = (n(p+0), n(p+5.2,1.3,9.7), n(p+12.1,8.4,3.5)) · warpStrength
density = fbm3(p + warp, ...)
```

**Latitude-banded coverage.** Build `C(lat) = base + ITCZ + subtropical_high + storm_track + polar`. ITCZ is a Gaussian bump at ~5° N (`σ ≈ 6°`, coverage 0.85–0.95). Subtropical highs at ±30° are negative bumps. Mid-latitude storm tracks at ±50° are broad positive bands rich in cyclonic features. Polar haze is a mild stratus. Then `cloud = smoothstep(1−C(lat), 1−C(lat)+0.1, fbm_warped(p))` — raising coverage lowers the threshold.

**Cyclonic spirals.** Place storms with Poisson-disc on the storm-track latitude bands. Use a **Rankine vortex** (finite core, well-behaved): solid-body inside `R_core`, `Γ/r` outside. Add small inward radial component for spiral inflow. Convert tangential + radial back to a 3D tangent-plane displacement on the sphere; sum contributions of all storms; advect noise lookup `density = fbm(p + dt·sum_v)`. Sign by Coriolis: `+1` (CCW) in N hemisphere for cyclones; `−1` in S. For painterly look, sample noise along an **Archimedean log-spiral** in storm-local polar: `θ' = θ + sign·k·log(r/R)`, then `spiralMask = smoothstep(0.3, 0.7, sin(armCount·θ' − phase))`.

**Coriolis-like banding strength as a parameter.** Anisotropic noise lookup with longitude scaled by `1/zonal_stretch`: Earth uses 1.3–2.0 in subtropics; Jupiter uses 5–20×. This is the same trick used by Barth Paléologue's procedural-gas-giants tutorial: "instead of sampling at the position on the planet, we sample at the position stretched vertically to compress the clouds horizontally."

**Layered cloud types.** `alpha = max(cumulus·0.9, cirrus·0.6, stratus·0.4)`; tint cirrus cooler/bluer. The Frostbite/Horizon: Zero Dawn approach (Schneider 2015, Hillaire SIG2016) uses **Perlin-Worley** (fBm Perlin mixed with inverted Worley) for both fluffy cores and connecting filaments, plus a small 256×128 **weather map** controlling coverage/type/height per location. This is the most directly applicable blueprint.

**Fake volumetric self-shadowing.** Compute the cloud-density gradient via finite difference; project sun direction onto local tangent plane; `shade = clamp(dot(grad, −sunTangent), 0, 1)`. Optional cheap cone-trace: march N steps in image-space along sun direction, accumulate density for `extinction = exp(−k·sumDensity)`. Add a silver-lining factor `pow(saturate(dot(gradDir, sunTangent)), 8)`. This approximates Beer-Lambert (Frostbite §4) on a 2D map.

**Procedural noise vs simulated atmosphere.** For your offline pipeline, the **best hybrid** is Stam stable-fluids on a 64×128 wrapped grid (~10s of sim) producing only a *velocity field*, then advect a procedural noise texture by it. Stam's semi-Lagrangian scheme (trace each cell backwards by `−dt·v`, sample bilinearly) is ~100 lines of C. Add a Coriolis term `f = 2Ω sin(lat)` rotating velocity. This gives you natural-looking cyclone formation that pure noise cannot — cf. `gaseous-giganticus`, which does exactly this for Jupiter-class planets.

---

## 7. Better gas giant bands and storms

Your current implementation handles bands + storms but lacks the fluid-dynamics character of real Jupiter visualizations. Two high-impact upgrades: replace turbulence-only distortion with **divergence-free curl-noise advection**, and use **iterated semi-Lagrangian advection** of color rather than direct noise lookup.

**Curl noise (Bridson 2007).** Take a noise potential `ψ(x)`, define velocity as `v = (∂ψ/∂y, −∂ψ/∂x)` in 2D — exactly divergence-free, so advected color/density is conserved (no patches grow or shrink). Eddies, ribbons, and shears appear naturally. On the sphere use a single scalar potential and finite-difference along local east/north tangent unit vectors with `ε ≈ 0.001`. Stefan Gustavson's "Tiling and Noise Gradients" gives the analytical noise-gradient formula, much faster than central differences.

**Turbulent advection.** For each pixel: backward-trace `p_traced = p − dt·(zonal_jet(lat) + curlNoise(p_traced)·amp)` for `N = 4..16` iterations, then sample `bandColorRamp(lat) · detailNoise(p_traced)`. More iterations → longer streaks → more Kelvin-Helmholtz wisps at band boundaries. **This is exactly what `gaseous-giganticus` does** (Stephen Cameron's open C source) and is the single most important change you can make to your gas-giant renderer. Critical params from its source: `band_speed_factor`, `noise_scale ≈ 3.0`, `velocity_factor ≈ 1200` advection steps, `bands` 6–11.

**Worley `F2−F1` for in-band cells.** Add 0.15× weight inside zones for granulation; stretch the lookup zonally (longitude × 1.0, latitude × 4.0) so cells are zonally elongated like real Jovian eddies.

**Storm spawning at specific latitudes.** Don't scatter uniformly. Define a `StormBands []StormBand` array with explicit latitudes, sigmas, counts, sizes, and signs reflecting Jupiter morphology — anticyclonic GRS at −22° (one), Oval BA at −40° (one), brown-barge cyclones at +18° (six small), white anticyclonic ovals at ±45–60°. Sample positions with Poisson-disc on each latitude band. The GRS is anticyclonic (high pressure, opposite Earth cyclones) — your sign field needs to support that.

**Storm shape via potential flow + log-spiral.** Around each storm, use uniform-flow + point-vortex stream function `ψ(r,θ) = U·r·sin(θ) + (Γ/2π)·ln(r)` so the ambient band flow naturally bends around the storm rather than slicing through it. For the storm body itself, use a log-spiral mask in storm-local polar: `θ' = θ + sign·(1/tan α)·log(r/R)` with pitch angle α ≈ 75° for tight spirals; modulate radius by fBm to break symmetry.

**Multi-scale flow** stacks three velocity fields: zonal jets `v_jet(lat)` (latitude-only), medium curl `curlNoise(p·1.5, oct=2)·0.3` (eddies), small curl `curlNoise(p·8, oct=4)·0.05` (turbulence), plus discrete vortex fields from each storm.

**Kelvin-Helmholtz ribbons** at band boundaries via parametric form: `phase = lon·waveNum + bandShearPhase; roll_y = sin(phase)·0.5 + boundary_y; mask = exp(−((lat−roll_y)/σ)²)`. Add a 3× higher-frequency wiggle to break perfect periodicity. The trick is to draw the ribbon as the *boundary between* color regions, not as a third color.

**Color modulation within bands.** John Whigham's blog uses a **Cassini-derived 1D Jupiter color slice** as the band ramp — an excellent and cheap approach you should adopt directly. Real palette layers: zones (cool, ammonia ice) `#f5e8c8`, belts (warmer, NH₃SH) `#b4784b`, storm cores `#c35a3c`, polar haze `#a0aab4`. Modulate by `noise·0.15` for chromophore variation; multiply by haze tint near poles via `mix(color, hazeColor, smoothstep(0.7, 1.0, |sin(lat)|²)·0.3)`.

**Von Kármán vortex streets** stylized: place a chain of alternating-sign Lamb-Oseen Gaussian vortices along a downstream lon range with `b/a ≈ 0.281` spacing ratio (Kármán's classical stability result). Useful for wakes downstream of GRS-like obstacles, polar lee waves, and band-boundary detail.

**Cassini-derived color ramps + curl-noise advection** is the closest open recipe to NASA's Juno-style visualizations. Space Engine and Outerra both use stretched simplex octaves with curl-noise advection over zonal bands — same technique.

---

## Cross-cutting — well-known generator playbooks

**Spore** (Hecker et al.): runtime procedural texture atlases that flood-fill triangles by similar 3D normal-axis groups, project flat, and compose biomes as overlapping spherical region "stamps" in 3D — terraforming becomes volume composition. The Stellaris look is similar in spirit: per-archetype palette + LUT. **No Man's Sky** (Murray, McKendrick GDC 2017): voxel octrees on cubes projected onto spheres, dual-contouring polygonization, **archetype + variation** with 64-bit deterministic seeds, modular generator stack composing larger-than-sum-of-parts content, triplanar texture splatting. They publicly stated the superformula was "difficult to control" and didn't ship in NMS. **Star Citizen Planet Tech v5** (Sean Tracy): quadtree LOD on cube-sphere; terrain assignment driven by **temperature, humidity, geology, soil type, soil depth, nutrients** + derived sunlight exposure and slope aspect; flora competes for placement; rocks placed from offline erosion sim; per-pixel edge blending plus parallax occlusion mapping. **Outerra** (Brano Kemen): quadrilateralized spherical cube + wavelet-compressed tiles, slope-modulated noise as cheap erosion proxy, Voronoi-cell tile-coordinate jitter to break detail-texture repetition. **Space Engine** (Romanyuk): Musgrave-style ridged multifractal heightfield, palette presets per planet class, three layered detail textures handling sub-pixel scale, Voronoi tiling-break trick. **Minecraft 1.18** (Henrik Kniberg JFokus 2022): five orthogonal climate noises (continentalness, erosion, weirdness, temperature, humidity) combined via **splines, not multiplication**, biomes assigned via nearest-bucket in the 5D space, separate 3D Perlin layers for caves. **Dwarf Fortress** (Toady One): six base fields seeded coarsely + filled fractally via midpoint displacement, pole gradient on temperature, vegetation derived from {elev, rainfall, temp}, biome rejection sampling, then erosion + rivers + orographic correction. **Caves of Qud** (Bucklew & Grinblat GDC 2019): Wave Function Collapse on tile grids; layered abstractions (region → village → architecture → contents). **Stellaris**: per-archetype texture variant with climate-group LUT applied as hue/sat grade.

The unifying lesson: **pick a small number of orthogonal scalar fields, drive everything from them, lookup tables and splines beat noise multiplication.**

---

## Cross-cutting — sphere-native texturing

**Equirectangular** is fine as an *output* format but suffers from severe pole compression in storage. **Cube-sphere** (quadrilateralized spherical cube, Catlike Coding's `sphere = (x·√(1 − y²/2 − z²/2 + y²z²/3), …)`) eliminates pole singularity, is GPU-cubemap-native, and has only ~20% area variance corner-to-center; used by Outerra, Star Citizen, Acko. Discontinuities at the 6 cube edges need blending. **Icosphere subdivision** has the best uniformity (~20% better than cube-sphere) and no edges; ideal for stylized per-vertex-color rendering (Stellaris/Spore aesthetic). **HEALPix** gives perfect equal-area pixels with isolatitude rings — essential for cosmology, overkill for games. **Goldberg polyhedra** (icosahedral hex-tiled spheres with 12 pentagon defects) are the RimWorld choice, distinctive when you want visible tile borders. **Direct 3D sampling on the unit sphere** (your current approach) is fundamentally seamless; the only "loss" is wasted samples near poles in equirect output, a storage concern not a generation concern.

The **practical recommendation** is: **generate on cube-sphere faces, bake to equirectangular** for output. This gives uniform sample density and pole-free generation while keeping the simple output format. The bake is straightforward — for each equirect pixel compute the direction vector, pick the cube face by largest absolute component, compute UV on that face, sample bilinearly. If your inputs are already 3D-noise-on-sphere (as yours are), the cube-sphere intermediate adds little; it matters most when your input layers are 2D images you want to combine.

---

## Cross-cutting — erosion on a sphere

**Particle-based hydraulic erosion (Beyer / Lague)** is the workhorse: spawn N droplets at random points on the surface; each has (position, velocity, water, sediment); at each step interpolate height + gradient, update direction with inertia, move one cell, compute capacity `c = max(−Δh, minSlope)·|v|·water·k`, deposit if `sediment > c` or moving uphill else erode along an erosion-radius brush, evaporate water. Self-reinforcing channels emerge naturally; sediment fills receivers; ridges sharpen. ~70k–200k particles needed on a 2000×1000 grid; trivially parallelizable with goroutines. Reference impl: `henrikglass/erodr` (~300 lines, easy to port to Go); Go option already available as `setanarut/rainfall`.

**Grid-based hydraulic (Mei et al., Št'ava)** is more correct for dendritic networks but has 10+ params and instability is common — Frozen Fractal's blog documents the frustration. Skip unless particle erosion proves insufficient.

**Stylized "fake" erosion** is what your project actually wants for the Stellaris/Spore look: Gaussian blur radius 2–3 px + contrast remap (`h = (h − mean)·1.4 + mean`); flow-accumulation map as a darkening albedo texture; anisotropic blur stretched along `∇h`; Sobel-derivative shading baked into albedo; ridge-sharpening unsharp mask; **thermal/talus-angle erosion** where any slope > 30–45° moves half the excess to the lower neighbor (~10 lines, 30–100 iterations, gives clean scree slopes).

**Sphere-aware variants.** Cleanest is **direct on icosphere mesh** (`OptimisticPeach/sphere_terrain` in Rust is the reference; each vertex has 5–6 neighbors). For your equirect pipeline the simplest retrofit is to weight horizontal neighbor contributions by `1/cos(lat)` to compensate for pole stretching. For particle erosion, use angular movement (project velocity to the tangent plane each step, slerp positions); no special-casing of the φ = ±π seam needed because positions live in 3D.

---

## Cross-cutting — plate tectonics

The standard pipeline (Andy Gainey's "Experilous", Red Blob, LeatherBee, Frozen Fractal, Tectonics.js, weigert/SimpleTectonics):

Place 10–50 seed points on the sphere (Fibonacci spiral); plate assignment via **random flood-fill**, *not* BFS (BFS gives smooth circular boundaries; random fill gives the jagged ones you want). Each plate gets a rotation axis (3D unit vector through center) and angular speed; local velocity at point `p` is `v = ω × p`. Each plate gets oceanic vs continental flag (oceanic = lower base, higher density, younger plates float higher). For each pixel on a plate boundary, classify by relative velocity dot boundary normal: **convergent** (mountain belt cont-cont, volcanic arc + trench cont-oce, island arc oce-oce), **divergent** (mid-ocean ridge / continental rift), **transform** (fault line, only roughness). Compute three distance fields (one per boundary type). Augment heightmap as `h = plate_base + boundary_uplift(distance, type) + noise_detail`.

**Critical pitfall**: any nonzero relative velocity triggers full mountain growth without thresholds; introduce `Δdistance·n < −0.75` threshold (Red Blob's documented bug fix). For your Go pipeline, port LeatherBee's Voronoi+vector approach (~500 lines) or implement the simplified Gainey version directly. **`Flokey82/genworldvoronoi`** is the closest existing Go implementation — a direct port of Amit Patel's `redblobgames/1843-planet-generation` with plate tectonics, wind/precipitation, rivers, civilizations, cities — even if you don't use it as a dependency, it's a goldmine. The C++ **`Mindwerks/plate-tectonics`** (Lauri Viitanen's PlaTec) is the only mature alternative; its 2D output requires reprojection to sphere.

---

## Cross-cutting — Go libraries that earn their keep

**`lucasb-eyer/go-colorful`** (Tier S) — RGB/HSL/Lab/Luv/HCL/OkLab/OkLCh, color distance metrics, `BlendOkLab`/`BlendHcl`, palette generators. **The single highest-ROI library import for biome work**: switching biome interpolation from RGB to OkLab kills muddy gradients with ~20 LOC.

**`fogleman/delaunay`** — port of Mapbox's Delaunator, 1M points in 1.27s. Use for: Voronoi continents (circumcenters of triangles), MST road networks (Kruskal over Delaunay edges), city Voronoi.

**`fogleman/poissondisc`** — Bridson Poisson-disc; correct, simple. Use for biome seeds, settlement positions, crater placements, storm placements.

**`disintegration/imaging`** + **`anthonynsimon/bild`** — image processing; bild has blend modes and flood-fill that imaging lacks.

**`Flokey82/genworldvoronoi`** — direct Go port of Red Blob's planet generator with plates, climate, rivers, civilizations. Inspect for technique reference even if not used as dep.

**`setanarut/rainfall`** — only mature Go particle-erosion lib; SebLague-style droplet algorithm.

**`kelindar/noise`** — fast simplex (~16ns per 2D sample, zero allocations) with built-in fBm; faster than opensimplex-go for hot loops.

**Gaps to fill yourself** (≤100 lines each): curl-noise (gradient of your existing 3D noise), Jump Flooding Algorithm for distance transforms, L-systems for road backbones, thermal erosion (talus angle), HEALPix port if needed.

**`fogleman/gg`** for drawing labels/borders/rivers on top of the equirect; **`fogleman/fauxgl`** if you want to render an icosphere-with-vertex-colors and flatten to equirect for a literally-low-poly Spore-style output mode.

---

## Prioritized bang-for-buck enhancement list

Ranked by visual impact ÷ implementation effort, given your current stack and the Stellaris/Spore aesthetic target. Each item is independently shippable.

**Tier S — massive visual change, hours-to-days of work each.**

1. **Switch biome color interpolation to OkLab via `go-colorful`** (~20 LOC). Single largest aesthetic win available. Eliminates muddy gray transitions between biome colors.
2. **Adopt Minecraft-1.18 multi-noise: `continentalness`, `erosion`, `peaks-and-valleys`, `temperature`, `humidity`** — five independent 3D noises on the sphere, combined via **monotone cubic splines, not multiplication** (~150 LOC including spline class). This single change moves you from "noise-texture planet" to "designed-feeling planet."
3. **Domain warping on every height/cloud/biome lookup** (~30 LOC). One Quilez warp pass with `q = p + amp·vec3(noise(p+a), noise(p+b), noise(p+c))` doubles perceived organic-ness of every downstream field.
4. **Whittaker biome lookup driven by independent T and M noise fields with bilinear color interpolation between palette cells** (~100 LOC). Fixes the latitude-banding patchiness immediately.
5. **Per-archetype color-correction LUT pass** (Stellaris technique, ~30 LOC). Each planet type gets a small 3D color cube applied as final grade — instant Stellaris-look unification.

**Tier A — high visual change, days of work.**

6. **Ridged multifractal layer for mountains, masked by a continentality / plate-distance field** (~80 LOC for ridged multifractal + the mask). Prevents spaghetti-mountains; aligns ranges with continental structure.
7. **Province / roughness modulation map** (warped Voronoi 8–40 cells + three low-frequency control fields modulating amplitude/frequency/operator). Produces Mars-style regional variety without per-region authoring.
8. **JFA distance-to-coast field** (~80 LOC). Reusable for: continentality moisture, atmospheric haze near coasts, glowing province borders, river seeding, urban-cluster bias.
9. **Particle-based hydraulic erosion via `setanarut/rainfall`** (~10 LOC integration, several minutes runtime). Adds the realistic alluvial-fan / vein detail that distinguishes plausible planets from pure-noise ones.
10. **Coastal-noise enhancement (`(1 − e⁴)·high_fBm` near sea level)** plus optional Voronoi continental masks (~100 LOC). Archipelagos and fjords appear at every zoom for free.
11. **Rebuild crater system: power-law SFD + density mask + age + ejecta rays as albedo + multi-floor-type branching** (~250 LOC for the full upgrade). Most recognizable feature on barren bodies.
12. **Curl-noise turbulent advection for gas giants** (replacing/augmenting current turbulence) — N=4–16 backward-tracing semi-Lagrangian iterations through zonal jets + curl noise (~100 LOC). The single most important gas-giant change; matches `gaseous-giganticus` quality.
13. **Cassini-derived 1D Jupiter color ramp + storm latitude bands array** for gas giants (~50 LOC, plus Whigham's color slice as a static asset).

**Tier B — distinctive aesthetic upgrades, days-to-week.**

14. **Voronoi tectonic plates with simple velocity vectors and three boundary distance fields** (~300 LOC simplified Gainey). Mountain belts naturally on convergent boundaries.
15. **D8 flow accumulation → river threading + green-corridor moisture boost** (~150 LOC). Nile-through-desert effect.
16. **Wind-driven rain shadow across mountains** (~80 LOC sweep per latitude band). Atacama-style coastal deserts emerge naturally.
17. **Cloud-layer overlay**: latitude-banded coverage × domain-warped fBm × Rankine-vortex storms + fake self-shadowing (~250 LOC). Earth-from-orbit realism on terran/super-terran.
18. **Civilization signs**: habitability-scored Poisson-disc settlements + Zipfian populations + nightside Black Marble splats + dayside agriculture grids + Delaunay/MST roads (~400 LOC for the full daytime+nightside pair). High wow-factor per LOC for terran/super-terran.
19. **Generate on cube-sphere, bake to equirect** (~80 LOC). Removes pole pinch in noise sample density; reusable for downstream 3D model export.
20. **Voronoi cell-coordinate jitter to break detail-texture repetition** (Outerra/Space Engine trick, ~40 LOC). Subtle but every AAA planet engine does this.

**Tier C — polish, hours each.**

21. **Wind streaks behind craters (Mars dark-tail pattern)**, **frost lines distinct from snow lines**, **playas/salt flats with polygonal cracks**, **albedo blotch noise independent of height**, **rim discoloration boundary layer**, **sediment fill in lows**.
22. **Anisotropic dune fields aligned to a curl-noise wind tangent field**.
23. **Chaos terrain + linea + crevasses for ice worlds**.
24. **Lava-lobe probabilistic flow paths + emission channel for lava worlds**.
25. **Reaction-diffusion ripples on dunes (offline once, tiled)**.

**Things to skip** for your project: HEALPix (overkill); full PDE-based plate tectonics like Mindwerks (~80% of visual benefit comes from the simplified Voronoi-plate version at ~10% of effort); Wave Function Collapse (wrong tool for continuous-noise textures); L-systems for individual streets at 2 km/pixel (use MST/Delaunay instead); superformula (Hello Games shipped without it for a reason).

If you have time for only **five** changes, do: (1) OkLab color blending, (2) multi-noise control fields with splines, (3) domain warping, (4) Whittaker T+M biome lookup, (5) per-archetype LUT. Together they cost ~400 LOC and transform the output from "procedural" to "designed."