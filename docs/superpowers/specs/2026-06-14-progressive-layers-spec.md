# Progressive Layer Rendering — Planetary Forge Wizard

**Date:** 2026-06-14
**Scope:** New guided-walkthrough UX for planet generation
**Branch:** phase-13/progressive-layers

## 1. Problem

Current `planet-explorer` presents all controls at once in a dashboard format. Users must understand the entire pipeline before they can meaningfully adjust parameters. The render generates everything at once with no visibility into intermediate states, making it difficult to:
- Understand how each layer contributes to the final result
- Debug which layer causes visual issues
- Showcase planetary formation as an educational narrative
- Efficiently iterate on specific stages

## 2. Goal

Transform the explorer into a **guided walkthrough** where users build a world layer by layer. The first render is a tectonic preview with glowing boundary lines; each subsequent step applies one texture layer, updating all views. The system caches intermediate state so changing an earlier parameter re-renders only up to the current layer.

**Non-goals (this round):**
- No time-stepped simulation preview (still snapshot-based)
- No undo/redo history (just the current layer stack state)
- No per-layer save/load (only full profile export/import)
- No branching "what if" comparisons (single active state)

**Decisions locked:**
1. **No legacy path** — always use crust tectonics, no fallback to noise-threshold continents
2. **Layer 0 = flat 0.5** — Minecraft-style, mid-height is the starting canvas
3. **Dirty flag invalidation** — changing earlier parameters triggers full re-render to current layer
4. **Template defaults** — each planet type has a complete layer param set for "generate all"
5. **Documentation** — corner brief + detailed MANUAL.md separate from UI

## 3. Layer Architecture

### 3.1 Layer Definition

```go
// Layer is a single stage in the planetary formation pipeline.
type Layer interface {
    // ID is a stable identifier (e.g., "crust", "tectonic-fx", "erosion")
    ID() string

    // Name is the human-readable display name
    Name() string

    // Description is the brief (1-2 sentence) explanation shown in UI
    Description() string

    // Category groups related layers for organization
    Category() Category

    // DependsOn returns layer IDs that must run before this one.
    // Empty slice means this layer only depends on layer 0 (flat 0.5).
    DependsOn() []string

    // Params returns the profile field paths this layer touches.
    // Used to extract template defaults and invalidate cache on change.
    // Format: "Crust.MajorPlates", "Ridged.Amp", etc.
    Params() []string

    // Enabled returns whether this layer is active for the current profile.
    // Some layers are conditional (e.g., Civ only when Tier > 0).
    Enabled(profile *types.PlanetProfile) bool

    // Render applies this layer to the heightmap and returns the updated version.
    // Must not modify the input heightmap; return a new CubeMapF if needed.
    // Context provides seed, size, and cached intermediate fields.
    Render(ctx *Context, hm *cubemap.CubeMapF) *cubemap.CubeMapF

    // RenderDebug returns a visualization suitable for the tectonic preview.
    // Only meaningful for tectonic-related layers; others return nil.
    RenderDebug(ctx *Context) *cubemap.CubeMap
}
```

### 3.2 Context Structure

```go
// Context holds shared state across layer renders.
type Context struct {
    MasterSeed int64
    Size       int  // cube face size (1024, 512, etc.)
    RadiusKm   float64

    // Cached intermediate fields (populated as needed)
    Plates      *field.PlateField
    Crust       *field.CrustField
    TectonicFX  *field.TectonicFXField
    Jitter      *noise.JitterField
    Flow        *field.FlowField
    ClimateT    *cubemap.CubeMapF  // temperature field
    ClimateM    *cubemap.CubeMapF  // humidity field
    RainShadow  *biome.RainShadowField

    // Profile snapshot (immutable for a render pass)
    Profile *types.PlanetProfile
}
```

### 3.3 Layer Registry

The `layers` package maintains a global registry:

```go
var registry = make(map[string]Layer)

func Register(l Layer) {
    registry[l.ID()] = l
}

func Get(id string) Layer {
    return registry[id]
}

func All() []Layer {
    // Return in dependency order via topological sort
}
```

Layers self-register in `init()`:

```go
func init() {
    layers.Register(&CrustLayer{})
    layers.Register(&TectonicFXLayer{})
    // ...
}
```

## 4. Layer Stack

### 4.1 Complete Layer List (Rocky Pipeline)

| ID | Name | Category | Depends On | Params | Enabled When |
|----|------|----------|------------|--------|--------------|
| `flat` | Flat Canvas | Base | — | — | Always |
| `plates` | Tectonic Plates | Tectonics | `flat` | `Crust.MajorPlates`, `Crust.MinorPlates`, `Crust.OceanicFraction` | Always |
| `crust` | Continental Crust | Tectonics | `plates` | `Crust.Assembly`, `Crust.TargetLandFraction`, `Crust.PlatformHeight`, `Crust.OceanFloorHeight`, `Crust.ShelfWidthRad`, `Crust.EdgeNoiseAmp`, `Crust.EdgeNoiseFreq`, `Crust.EdgeNoiseOctaves`, `Crust.CratonsMax` | Always |
| `tectonic-fx` | Boundary Effects | Tectonics | `crust` | `TectonicFX.*` (all 13 params), `Crust.TectonicAge` | Always |
| `sealevel` | Sea Level | Oceans | `tectonic-fx` | (derived, no params) | Always |
| `continentalness` | Continental Noise | Control | `crust` | `ControlConfig.Continentalness` | Always |
| `detail` | Detail Noise | Control | `flat` | `ControlConfig.Detail` | Always |
| `peaks-valleys` | Peaks & Valleys | Control | `flat` | `ControlConfig.PeaksValleys` | Always |
| `coastal` | Coastal Roughening | Surface | `sealevel` | `Coastal.Amp`, `Coastal.Threshold`, `Coastal.Freq` | `Coastal.Amp > 0` |
| `erosion` | Hydraulic Erosion | Surface | `sealevel` | `Erosion.*` (all 10 params) | `Erosion.Droplets > 0` |
| `craters` | Impact Craters | Surface | `sealevel` | `CraterCount`, `CraterMinRadius`, `CraterMaxRadius`, `CraterDepth`, `PowerLawAlpha`, `MariaDensityFactor`, `SurfaceAge`, `SecondaryDensity` | `CraterCount > 0` |
| `temperature` | Temperature Field | Climate | `flat` | `ControlConfig.Temperature` | Always |
| `humidity` | Humidity Field | Climate | `flat` | `ControlConfig.Humidity` | Always |
| `biome` | Biome Coloring | Color | `temperature`, `humidity`, `sealevel` | `BiomeTable` | `len(BiomeTable.Cells) > 0` |
| `rivers` | River Carving | Hydrology | `biome`, `erosion` | `Flow.RiverThreshold`, `Flow.*` (other params) | `Flow.RiverThreshold > 0` |
| `rain-shadow` | Rain Shadow | Climate | `peaks-valleys`, `temperature` | `RainShadow.*` (5 params) | `RainShadow.WalkSteps > 0` |
| `clouds` | Cloud Cover | Atmosphere | `flat` | `Cloud.Coverage`, `Cloud.*` (other params) | `Cloud.Coverage > 0` |
| `civ` | Civilization | Life | `rivers`, `rain-shadow`, `biome` | `Civ.*` (all params) | `Civ.Tier > 0` |

### 4.2 Categories (for UI Grouping)

- **Base**: Foundational layers
- **Tectonics**: Plate and crust formation
- **Oceans**: Water and sea level
- **Control**: Noise-based control fields
- **Surface**: Erosion, craters, coastal
- **Climate**: Temperature, humidity, rain shadow
- **Color**: Biome and palette application
- **Hydrology**: Rivers and flow
- **Atmosphere**: Clouds
- **Life**: Civilization and lights

### 4.3 Dependency Graph

```
flat
 ├─ plates
 │   └─ crust
 │       └─ tectonic-fx
 │           └─ sealevel
 │               ├─ coastal
 │               ├─ erosion
 │               └─ craters
 │
 └─ continentalness (depends on crust for edge shaping)
 └─ detail
 └─ peaks-valleys
     └─ rain-shadow
 └─ temperature
 └─ humidity
     └─ biome
         └─ rivers
         └─ civ
             └─ rain-shadow (also contributes to civ)
 └─ clouds
```

## 5. Tectonic Preview

The first render users see is a special debug visualization combining:
- Base heightmap (exaggerated)
- Plate boundary colors (convergent=red, divergent=blue, transform=yellow)
- Glowing intensity based on boundary magnitude

### 5.1 Preview Renderer

```go
// RenderTectonicPreview generates the educational first render.
func RenderTectonicPreview(ctx *Context, exaggeration float64) *cubemap.CubeMap {
    pf := ctx.Plates
    crust := ctx.Crust
    fx := ctx.TectonicFX

    out := cubemap.New(ctx.Size)
    for face := range out.Faces {
        for py := range ctx.Size {
            for px := range ctx.Size {
                i := py*ctx.Size + px

                // Base height with exaggeration
                h := crust.BaseHeight.Faces[face][i] * exaggeration

                // Heightmap coloring
                color := heightmapColor(h)

                // Boundary glow overlay
                glow := computeGlow(pf, fx, face, i)
                color = blend(color, glow)

                out.Set(face, px, py, color)
            }
        }
    }
    return out
}
```

### 5.2 Glow Computation

```go
func computeGlow(pf *PlateField, fx *TectonicFXField, face cubemap.Face, i int) color.RGBA {
    // Sample all boundary distances at this pixel
    dConv := pf.Convergent[face][i]
    dDiv := pf.Divergent[face][i]
    dTrans := pf.Transform[face][i]

    const glowRange = 500.0 // km

    // Convergent glow (red/orange)
    if dConv < glowRange {
        intensity := (glowRange - dConv) / glowRange
        // Mix with magnitude for brighter active boundaries
        mag := fx.BeltMag.Faces[face][i]
        intensity *= (0.5 + 0.5*mag)
        return lerpRGBA(black, redOrange, intensity)
    }

    // Divergent glow (blue/cyan)
    if dDiv < glowRange {
        intensity := (glowRange - dDiv) / glowRange
        mag := fx.RidgeMag.Faces[face][i]
        intensity *= (0.5 + 0.5*mag)
        return lerpRGBA(black, cyan, intensity)
    }

    // Transform glow (yellow/green)
    if dTrans < glowRange {
        intensity := (glowRange - dTrans) / glowRange
        return lerpRGBA(black, yellowGreen, intensity)
    }

    return black
}
```

## 6. Caching and Invalidation

### 6.1 Layer Stack State

```go
type LayerStack struct {
    layers         []Layer      // sorted by dependency
    currentLayer   int          // index of highest rendered layer
    cachedHeightmap *cubemap.CubeMapF  // state at currentLayer
    cachedContext  *Context     // includes all intermediate fields
    dirtyFrom      int          // 0 = clean, N = layer N changed, re-render from N
}
```

### 6.2 Render Algorithm

```go
func (ls *LayerStack) RenderTo(targetLayer int, ctx *Context) *cubemap.CubeMapF {
    // If we need to render from scratch or dirty point
    startFrom := ls.dirtyFrom
    if startFrom > targetLayer {
        startFrom = 0
    }

    // If starting from layer 0, use flat canvas
    hm := cubemap.NewF(ctx.Size)
    if startFrom == 0 {
        for face := range hm.Faces {
            for i := range hm.Faces[face] {
                hm.Faces[face][i] = 0.5  // flat 0.5
            }
        }
    } else if ls.cachedHeightmap != nil {
        hm = ls.cachedHeightmap.Clone()
    }

    // Render each enabled layer up to target
    for i := startFrom; i <= targetLayer; i++ {
        layer := ls.layers[i]
        if layer.Enabled(ctx.Profile) {
            hm = layer.Render(ctx, hm)
        }
    }

    // Update cache
    ls.cachedHeightmap = hm
    ls.currentLayer = targetLayer
    ls.dirtyFrom = len(ls.layers)  // mark clean

    return hm
}
```

### 6.3 Dirty Tracking

```go
func (ls *LayerStack) MarkDirty(paramPath string) {
    // Find which layer touches this param
    for i, layer := range ls.layers {
        for _, p := range layer.Params() {
            if p == paramPath {
                ls.dirtyFrom = min(ls.dirtyFrom, i)
                return
            }
        }
    }
}
```

## 7. Template Defaults

Each planet type has a complete param snapshot:

```go
type LayerTemplate struct {
    PlanetType string
    // LayerID -> param snapshot
    LayerParams map[string]json.RawMessage
}

func (t *LayerTemplate) ApplyTo(profile *types.PlanetProfile) {
    // Unmarshal each layer's params into profile
}
```

Templates are generated once per planet type (off a canonical seed) and embedded as JSON or computed at startup.

## 8. Future Work (Out of Scope)

- Time-stepped simulation preview
- Undo/redo history stack
- Per-layer save/load
- Branching "what if" comparisons
- Real-time collaboration

