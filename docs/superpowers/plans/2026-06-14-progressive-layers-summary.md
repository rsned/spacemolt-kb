# Progressive Layers Design Summary

**Date:** 2026-06-14
**Branch:** phase-13/progressive-layers
**Status:** Design complete, awaiting Phase 12 merge for implementation

## What We've Designed

### 1. Spec Document
`docs/superpowers/specs/2026-06-14-progressive-layers-spec.md`

Defines:
- Problem: Dashboard UX is overwhelming, no intermediate visibility
- Goal: Guided walkthrough with layer-by-layer rendering
- Layer architecture with dependency graph
- Complete layer list (18 layers for rocky pipeline)
- Tectonic preview visualization approach
- Caching and dirty tracking strategy

**Key decisions locked:**
- No legacy path (always crust)
- Layer 0 = flat 0.5
- Dirty flag invalidation
- Template defaults per planet type
- Corner brief + separate manual documentation

### 2. Layer Registry
`pkg/planetgen/layers/registry.go`

Defines the `Layer` interface:

```go
type Layer interface {
    ID() string
    Name() string
    Description() string
    Category() Category
    DependsOn() []string
    Params() []string
    Enabled(profile *types.PlanetProfile) bool
    Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF
    RenderDebug(ctx *Context) *cubemap.CubeMap
}
```

- Global registry with `Register()` for self-registration
- `All()` returns layers in dependency order via topological sort
- `ByCategory()` filters by category
- Context struct holds cached intermediate fields

### 3. Layer Stack
`pkg/planetgen/layers/stack.go`

Implements caching and dirty tracking:

```go
type LayerStack struct {
    layers         []Layer
    currentLayer   int
    cachedHeightmap *cubemap.CubeMapF
    cachedContext  *Context
    dirtyFrom      int
}
```

- `RenderTo(targetLayer, ctx)` renders up to target using cache
- `MarkDirty(paramPath)` invalidates from the layer that touches the param
- Thread-safe with mutex
- `FindLayer(id)` for index lookup

### 4. WASM Interface
`docs/superpowers/plans/2026-06-14-wasm-interface.md`

New exports for the wizard UX:

**State management:**
- `wizardInit(type, seed) → layerCount`
- `wizardSetTargetLayer(index)`
- `wizardRenderPreview(exaggeration)`

**Layer metadata:**
- `wizardLayerID/Name/Desc/Category(index)`
- `wizardLayerEnabled(index)`
- `wizardLayerDependencies(index) → JSON`

**Parameter access:**
- `wizardLayerParams(index) → JSON`
- `wizardSetParam(path, valueJSON)`
- `wizardGetParam(path) → JSON`

**Rendering:**
- `wizardRenderTo(layerIndex)`
- `wizardGetCubeFace(face) → RGBA data`
- `wizardGetEquirect() → RGBA data`

**Templates:**
- `wizardTemplateList() → JSON array`
- `wizardApplyTemplate(type)`
- `wizardExportProfile() / wizardImportProfile()`

Includes JS binding sketch and usage examples.

### 5. Tectonic Preview PoC
`pkg/planetgen/render/tectonic_preview.go`

Educational first-render visualization:

- Heightmap coloring with continental vs oceanic distinction
- Glowing boundary lines:
  - Convergent: red/orange
  - Divergent: blue/cyan
  - Transform: yellow/green
- Glow intensity modulated by boundary magnitude
- `RenderTectonicPreview()` for main preview
- `RenderTectonicPreviewDebug()` for boundary verification

## Complete Layer List (Rocky Pipeline)

| ID | Category | Dependencies | Params |
|----|----------|--------------|--------|
| `flat` | Base | — | — |
| `plates` | Tectonics | `flat` | Crust.{MajorPlates,MinorPlates,OceanicFraction} |
| `crust` | Tectonics | `plates` | Crust.{Assembly,TargetLandFraction,PlatformHeight,...} |
| `tectonic-fx` | Tectonics | `crust` | TectonicFX.*, Crust.TectonicAge |
| `sealevel` | Oceans | `tectonic-fx` | (derived) |
| `continentalness` | Control | `crust` | ControlConfig.Continentalness |
| `detail` | Control | `flat` | ControlConfig.Detail |
| `peaks-valleys` | Control | `flat` | ControlConfig.PeaksValleys |
| `coastal` | Surface | `sealevel` | Coastal.* |
| `erosion` | Surface | `sealevel` | Erosion.* |
| `craters` | Surface | `sealevel` | CraterCount,PowerLawAlpha,SurfaceAge,... |
| `temperature` | Climate | `flat` | ControlConfig.Temperature |
| `humidity` | Climate | `flat` | ControlConfig.Humidity |
| `biome` | Color | `temperature,humidity,sealevel` | BiomeTable |
| `rivers` | Hydrology | `biome,erosion` | Flow.* |
| `rain-shadow` | Climate | `peaks-valleys,temperature` | RainShadow.* |
| `clouds` | Atmosphere | `flat` | Cloud.* |
| `civ` | Life | `rivers,rain-shadow,biome` | Civ.* |

## Next Steps

1. **Complete Phase 12** — Merge tectonic continents work so crust/field packages exist
2. **Implement concrete layers** — Create `layer_*.go` files for each layer type
3. **Integrate with render/rocky.go** — Refactor to use LayerStack instead of monolithic function
4. **Update WASM** — Add wizard exports and update JS bindings
5. **Build UI** — Implement guided walkthrough in app.js
6. **Test templates** — Generate default param sets per planet type

## Files Created

```
docs/superpowers/specs/2026-06-14-progressive-layers-spec.md
docs/superpowers/plans/2026-06-14-wasm-interface.md
docs/superpowers/plans/2026-06-14-progressive-layers-summary.md (this file)
pkg/planetgen/layers/registry.go
pkg/planetgen/layers/stack.go
pkg/planetgen/render/tectonic_preview.go
```

## Known Issues

- Import paths reference Phase 12 packages (`field.CrustField`, etc.) that don't exist in this worktree yet
- Will resolve once Phase 12 branch merges to main
- No code is actually broken — these are design documents and PoC stubs

