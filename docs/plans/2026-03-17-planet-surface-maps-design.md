# Planet Surface Map Generator — Design

## Goal

Generate procedural 2000x1000 mercator projection surface maps for every planet
in the game database. Each of the 12 planet types gets a distinct visual style
inspired by real solar system bodies. Images are deterministic — the same planet
always produces the same map.

## Decisions

- **Language:** Go (fits existing KB build pipeline)
- **Seed:** FNV-32 hash of the planet name — deterministic and automatic
- **Renderers:** Two pipelines — rocky (10 types) and gas giant (2 types)
- **Noise:** Simplex noise via external Go library (opensimplex-go or similar)
- **Output:** PNG files in `kb/images/planets/`
- **Surface features:** Noise + procedural craters. Additional features (volcanic
  hotspots, ice cracks, cloud layers) deferred to a future task.

## Architecture

```
cmd/generate-planet-maps/
    main.go              # CLI — loads DB, iterates planets, writes PNGs

pkg/planetgen/
    planetgen.go         # Public API: Generate(planetType, seed string) *image.RGBA
    profile.go           # PlanetProfile definitions for all 12 types
    rocky.go             # RockyRenderer — heightmap, craters, colorize
    gasgiant.go          # GasGiantRenderer — bands, turbulence, storms
    noise.go             # Noise helpers — sphere sampling, octave layering
    crater.go            # Crater generation and heightmap stamping
    color.go             # Palette interpolation utilities
```

### PlanetProfile

Each planet type is defined by a profile struct:

```go
type PlanetProfile struct {
    Type             string
    Renderer         string          // "rocky" or "gas_giant"
    Palette          []ColorStop     // elevation/band color stops
    NoiseOctaves     int             // complexity (6-8 typical)
    NoiseLacunarity  float64
    NoisePersistence float64
    CraterDensity    float64         // 0.0 (oceanic) to 1.0 (scorched)
    CraterSizeRange  [2]float64      // min/max radius as fraction of image
    HasPolarCaps     bool
    PolarCapSize     float64         // 0.0-0.3 latitude fraction
    OceanLevel       float64         // below this elevation = ocean color
}
```

## Rocky Planet Renderer

Ten types: Scorched, Arid, Terran, Tundra, Glacial, Ice World, Super Terran,
Hothouse, Lava World, Oceanic.

### Pipeline

1. **Base heightmap:** Sample 6-8 octaves of simplex noise per pixel. Map pixel
   coordinates to spherical coordinates (longitude = `x/width * 2pi`,
   latitude = `pi/2 - y/height * pi`), then sample 3D noise at the
   corresponding point on a unit sphere. This avoids seam artifacts and gives
   natural wrapping.

2. **Crater stamping:** Generate N craters (based on `CraterDensity`) at random
   positions on the sphere. Each crater modifies the heightmap — bowl-shaped
   depression with a slight rim. Placed in decreasing size order so small ones
   overlap large ones naturally.

3. **Colorization:** Map heightmap value [0.0-1.0] to the type's color palette
   via linear interpolation between stops.

4. **Polar caps:** If enabled, blend white/ice color near poles based on
   latitude, with a noisy edge.

5. **Ocean level:** For Terran/Oceanic, any heightmap below `OceanLevel` gets
   ocean-blue coloring with depth-based shading.

## Gas Giant Renderer

Two types: Jovian, Ice Giant.

### Pipeline

1. **Band structure:** Define 8-12 horizontal bands at varying latitudes, each
   with a base color. Band widths vary via noise modulation.

2. **Turbulence distortion:** Offset Y coordinate per pixel by sampling simplex
   noise. Warps straight bands into wavy flowing shapes. Distortion varies by
   latitude.

3. **Fine detail:** Second pass of higher-frequency noise as color modulation on
   top of bands, giving swirling cloudy texture within each band.

4. **Storm spots:** For Jovian, stamp 1-3 oval storm features at random
   latitudes. Elliptical regions where color shifts and flow lines curve around.
   Ice Giants get fewer/no storms.

Same spherical coordinate mapping as rocky to avoid seams.

## Color Palettes

| Type | Base Colors | Craters | Polar Caps | Ocean | Notes |
|------|------------|---------|------------|-------|-------|
| Scorched | Grays, dark charcoal, white highlights | ~200 | No | No | Mercury-like |
| Arid | Rust orange, tan, dark brown | ~80 | Small white | No | Mars-like |
| Terran | Blue ocean, green/brown land, white | ~10 | Medium | 0.45 | Earth-like |
| Tundra | Muted green-gray, brown, pale white | ~30 | Large | No | Cold steppe |
| Glacial | White, pale blue, blue-gray | ~20 | Very large | No | Ice-covered |
| Ice World | Pale blue, white, cyan | ~40 | Blends in | No | Europa-like |
| Super Terran | Deep green, blue ocean, brown | ~10 | Small | 0.40 | Bigger Earth |
| Hothouse | Yellow-green, dark green, hazy white | ~15 | No | No | Venus-like |
| Lava World | Black basalt, bright orange/red veins | ~20 | No | No | Glowing fissures |
| Oceanic | Deep blue, teal, white caps | 0 | Small | 0.85 | Mostly water |
| Jovian | Orange, brown, white, tan bands | N/A | N/A | N/A | Storms, turbulence |
| Ice Giant | Teal, blue, cream, gray bands | N/A | N/A | N/A | Smooth flow |

## CLI Usage

```bash
# Generate all planet maps
go run ./cmd/generate-planet-maps

# Generate for a specific type (testing)
go run ./cmd/generate-planet-maps -type scorched -seed "test" -out test.png
```

## Output

PNGs written to `kb/images/planets/{system_name}_{planet_name}.png`.

## Performance

At 2000x1000 with 8 octaves, each image takes ~0.5-1s. With ~600 planets total,
full generation takes ~5-10 minutes. Parallelizable with goroutines.

## Dependencies

- One external: opensimplex noise library for Go
- Stdlib: `image`, `image/png`, `math`, `hash/fnv`

## Future Work

- Planet detail pages with image, radius, mass, gravity, atmosphere data
- Agent-generated survey notes and descriptions
- Additional surface features: volcanic hotspots, ice cracks, cloud layers
- Integration into KB generator as a single build step
