# Phase 13: Progressive Layers — Design Complete

**Branch:** `phase-13/progressive-layers`
**Status:** Design & PoC complete, awaiting Phase 12 merge for implementation
**Date:** 2026-06-14

---

## What We've Built

### 1. Specification Document
📄 `docs/superpowers/specs/2026-06-14-progressive-layers-spec.md`

Complete specification including:
- Problem statement and goals
- Layer architecture and interface design
- Complete 18-layer dependency graph for rocky pipeline
- Caching and dirty tracking strategy
- Template system design

### 2. Layer Registry Implementation
📄 `pkg/planetgen/layers/registry.go`

- `Layer` interface with all required methods
- `Context` struct for shared state
- Global registry with dependency ordering via topological sort
- Category-based filtering

### 3. Layer Stack Implementation
📄 `pkg/planetgen/layers/stack.go`

- Caching with checkpoint resume
- Dirty tracking via `MarkDirty(paramPath)`
- Thread-safe operations
- Efficient re-render from dirty point

### 4. WASM Interface Design
📄 `docs/superpowers/plans/2026-06-14-wasm-interface.md`

Complete export signatures for:
- State management (`wizardInit`, `wizardSetTargetLayer`)
- Layer metadata (`wizardLayerName`, `wizardLayerDesc`, etc.)
- Parameter access (`wizardSetParam`, `wizardGetParam`)
- Rendering (`wizardRenderTo`, `wizardRenderPreview`)
- Template system (`wizardApplyTemplate`, `wizardTemplateList`)
- JS binding sketch

### 5. Tectonic Preview PoC
📄 `pkg/planetgen/render/tectonic_preview.go`

- `RenderTectonicPreview()` — educational first render
- Heightmap coloring with continental/oceanic distinction
- Glowing boundary lines (red=conv, blue=div, yellow=transform)
- Intensity modulated by boundary magnitude
- Debug visualization for boundary verification

### 6. Example Layer Implementations
📄 `pkg/planetgen/layers/example_layer.go`

Shows how to implement 5 representative layers:
- `FlatLayer` — base canvas
- `CrustLayer` — continental crust from cratons
- `TectonicFXLayer` — boundary effects
- `SeaLevelLayer` — derived quantile
- `ErosionLayer` — conditional surface layer

---

## The Layer System

### Complete Rocky Pipeline (18 layers)

```
Layer 0: flat (Flat Canvas)
  ↓
Layer 1: plates (Tectonic Plates)
  ↓
Layer 2: crust (Continental Crust)
  ↓
Layer 3: tectonic-fx (Boundary Effects)
  ↓
Layer 4: sealevel (Sea Level)
  ↓
Layer 5: continentalness (Continental Noise)
  ↓
Layer 6: detail (Detail Noise)
  ↓
Layer 7: peaks-valleys (Peaks & Valleys)
  ↓
Layer 8: coastal (Coastal Roughening)
  ↓
Layer 9: erosion (Hydraulic Erosion)
  ↓
Layer 10: craters (Impact Craters)
  ↓
Layer 11: temperature (Temperature Field)
  ↓
Layer 12: humidity (Humidity Field)
  ↓
Layer 13: biome (Biome Coloring)
  ↓
Layer 14: rivers (River Carving)
  ↓
Layer 15: rain-shadow (Rain Shadow)
  ↓
Layer 16: clouds (Cloud Cover)
  ↓
Layer 17: civ (Civilization)
```

### Categories

- **Base** (1): flat
- **Tectonics** (3): plates, crust, tectonic-fx
- **Oceans** (1): sealevel
- **Control** (3): continentalness, detail, peaks-valleys
- **Surface** (3): coastal, erosion, craters
- **Climate** (3): temperature, humidity, rain-shadow
- **Color** (1): biome
- **Hydrology** (1): rivers
- **Atmosphere** (1): clouds
- **Life** (1): civ

---

## Wizard Flow

### User Journey

```
1. "What kind of planet do you want to make?"
   ↓ Select: terran, super_terran, oceanic, arid, tundra, glacial, ...

2. "How much water and land?"
   ↓ Land% slider (drives TargetLandFraction)
   ↓ Generate plates + cratons

3. TECTONIC PREVIEW
   ┌─────────────────────────────────────┐
   │  [Sphere] [Equirect] [Cube Map]     │
   │  Glowing boundary lines              │
   │  Exaggeration slider: ━━●━━━ 2.5×   │
   │  [Generate New Seed] [Next >]       │
   └─────────────────────────────────────┘

4. LAYER WALKTHROUGH
   Each step:
   - Layer name and description in corner
   - Toggle ON/OFF
   - Adjust parameters
   - Render button
   - [← Back] [Regenerate] [Next →]
```

### Key Features

- **Incremental rendering**: Only render up to current layer
- **Smart caching**: Resume from checkpoint, not from scratch
- **Dirty tracking**: Change earlier param → re-render to current layer
- **Template defaults**: Each planet type has complete layer params
- **Jump ahead**: Skip to any layer directly

---

## Next Steps

### Immediate (Blocking on Phase 12)

1. ✅ **Phase 12: Complete crust integration** — Merge tectonic continents work
2. ⏳ **Refactor render/rocky.go** — Use LayerStack instead of monolithic function
3. ⏳ **Implement concrete layers** — Create `layer_*.go` for each layer type
4. ⏳ **Wasm exports** — Add wizard functions to `cmd/planet-explorer/wasm/main.go`
5. ⏳ **JS UI** — Implement guided walkthrough in `app.js`

### After Integration

6. **Generate templates** — Extract default params per planet type
7. **Test caching** — Verify dirty tracking and resume work correctly
8. **Performance profile** — Ensure incremental rendering is faster
9. **Documentation** — Write MANUAL.md with full layer explanations

---

## Files Created

```
docs/superpowers/specs/2026-06-14-progressive-layers-spec.md
docs/superpowers/plans/2026-06-14-wasm-interface.md
docs/superpowers/plans/2026-06-14-progressive-layers-summary.md
docs/superpowers/plans/2026-06-14-progressive-layers-status.md (this file)
pkg/planetgen/layers/registry.go
pkg/planetgen/layers/stack.go
pkg/planetgen/layers/example_layer.go
pkg/planetgen/render/tectonic_preview.go
```

---

## Design Decisions Locked

1. **No legacy path** — Always use crust tectonics
2. **Layer 0 = flat 0.5** — Minecraft-style mid-height canvas
3. **Dirty flag invalidation** — Earlier param change → re-render to current layer
4. **Template defaults** — Per-planet-type layer param sets
5. **Corner brief + manual** — UI shows brief docs, separate MANUAL.md for details

---

## Open Questions Resolved

| Question | Answer |
|----------|--------|
| Legacy path? | No — always crust |
| Layer 0 state? | Flat at 0.5 |
| Param change behavior? | Full re-render to current layer |
| Template system? | Yes, per planet type |
| Documentation approach? | Corner brief + separate manual |

---

## Estimated Implementation Effort

| Task | Estimate |
|------|----------|
| Phase 12 integration | In progress |
| Render refactor | 2-3 days |
| Layer implementations | 5-7 days |
| WASM exports | 1-2 days |
| JS UI work | 3-5 days |
| Testing & polish | 2-3 days |
| **Total** | **~2-3 weeks** |
