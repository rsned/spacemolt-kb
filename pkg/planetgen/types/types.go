package types

import (
	"image/color"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
)

// PlanetProfile defines the visual parameters for a planet type.
type PlanetProfile struct {
	Type             string
	Renderer         string // "rocky" or "gas_giant"
	Palette          []planetcolor.ColorStop
	NoiseOctaves     int
	NoiseLacunarity  float64
	NoisePersistence float64
	NoiseScale       float64
	CraterCount      int
	CraterMinRadius  float64 // Angular radius on sphere (radians)
	CraterMaxRadius  float64
	CraterDepth      float64 // How deep craters cut into heightmap
	HasPolarCaps     bool
	PolarCapSize     float64 // Latitude fraction (0-0.3)
	PolarCapNoise    float64 // Edge roughness (0 = default 0.08)
	OceanLevel       float64 // Below this = ocean (0 = no ocean)
	OceanColor       color.RGBA
	SnowLine         float64     // Elevation above this gets snow (0 = disabled)
	EquatorialPalette []planetcolor.ColorStop // If set, blended in near equator
	PolarPalette      []planetcolor.ColorStop // If set, blended in near poles (before ice caps)

	// Gas giant specific
	BandCount      int
	TurbulenceAmp  float64 // How much bands are distorted
	BandBlendWidth float64 // Fraction of band width to blend (0 = default 0.2)
	StormCount     int
	StormSize      float64 // Angular radius of storm ovals

	// Tier-S Phase 1: multi-noise control fields and per-field height splines.
	// Empty/zero values mean "use the legacy single-FBM path" (backward compat).
	ControlConfig ControlConfig
	Splines       [5]planetcolor.Spline // Continentalness, Erosion, PV, T, H — order matches ControlConfig fields
	Warp          WarpConfig
	BiomeTable    BiomeTable // Whittaker biome lookup table (empty = use legacy palette path)

	// Tier-S Phase 1 Task 26: per-archetype LUT for final color grading
	LUT string
}

// ControlField is a single 3D fBm control field used to drive the
// height and biome pipelines (see master plan §5.2).
type ControlField struct {
	Amp         float64 // amplitude multiplier
	Freq        float64 // base frequency
	Octaves     int     // number of fBm octaves
	Lacunarity  float64 // frequency multiplier per octave (default 2.0)
	Persistence float64 // amplitude multiplier per octave (default 0.5)
}

// ControlConfig holds the five control fields used by the rocky pipeline.
type ControlConfig struct {
	Continentalness ControlField
	Erosion         ControlField
	PeaksValleys    ControlField
	Temperature     ControlField
	Humidity        ControlField
}

// WarpConfig parameterizes a single Quilez domain-warp pass.
// Apply to a unit-sphere direction before sampling the underlying field.
type WarpConfig struct {
	Amp         float64 // displacement magnitude (in unit-sphere units)
	Freq        float64 // input frequency for the warp noise
	Octaves     int     // warp-noise octaves
	Lacunarity  float64
	Persistence float64
}

// BiomeTable is a 2D grid of biome cells indexed by (T, M) ∈ [0,1]².
// The T axis is rows (0 = cold, last = hot); M axis is columns
// (0 = dry, last = wet). Each cell has a 2-stop palette used to
// color a heightmap value via SampleGradientOkLab.
type BiomeTable struct {
	TBuckets int
	MBuckets int
	Cells    [][]BiomeCell // [tBucket][mBucket]
}

// BiomeCell is a 2-stop palette used to color heightmap values
// in a single (T, M) cell. Output is bilinearly OkLab-blended
// across neighboring cells based on the sample's exact (T, M).
type BiomeCell struct {
	Low  ColorRGB // height=0 color
	High ColorRGB // height=1 color
}

// ColorRGB is a JSON-serializable RGB color (alpha is implicit 255).
type ColorRGB struct {
	R, G, B uint8
}
