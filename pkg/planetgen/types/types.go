package types

import (
	"encoding/json"
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

	// Phase 3 Tier-A: power-law size-frequency distribution. P(R) ∝ R^(-α).
	// 0 = legacy uniform-with-quadratic-bias path (also disables age scaling
	// so existing archetypes render unchanged).
	PowerLawAlpha float64
	// Probability that a candidate crater is rejected in the high-fBm region
	// of a maria mask (low-freq noise). 0 disables; 1 zeros out 70 % of
	// craters in the brightest half of the mask.
	MariaDensityFactor float64
	// Bias for per-crater age in [0,1]. 1 → mostly old (age near 1); 0 →
	// mostly young (age near 0). Only used when PowerLawAlpha > 0.
	SurfaceAge float64
	// Secondary craters per large primary, scaled. 0 disables. Up to 5 + 10·d
	// secondaries are spawned within ~3R of each crater whose radius
	// exceeds half of CraterMaxRadius.
	SecondaryDensity  float64
	HasPolarCaps      bool
	PolarCapSize      float64 // Latitude fraction (0-0.3)
	PolarCapNoise     float64 // Edge roughness (0 = default 0.08)
	OceanLevel        float64 // Below this = ocean (0 = no ocean)
	OceanColor        color.RGBA
	SnowLine          float64                 // Elevation above this gets snow (0 = disabled)
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
	Warp          WarpConfig
	BiomeTable    BiomeTable // Whittaker biome lookup table (empty = use legacy palette path)

	// Tier-A Phase 3: ridged-multifractal mountain belts. Amp=0 disables.
	Ridged RidgedConfig

	// Tier-A Phase 3: Voronoi province modulation. Count=0 disables.
	Provinces ProvinceConfig

	// Phase 3 polish: slope-based Lambertian shading. ShadingStrength=0
	// disables (output unchanged). Exaggeration scales the heightmap
	// gradient so subtle features still produce visible shading.
	ShadingStrength     float64 // 0 = no shading; 1 = full diffuse modulation
	ShadingExaggeration float64 // gradient multiplier; 0 = use 8.0 default

	// Tier-S Phase 1 Task 26: per-archetype LUT for final color grading
	LUT string
}

// RidgedConfig parameterizes a ridged-multifractal mountain pass.
// Applied after the control-field heightmap, masked by the
// Continentalness spline output so ridges only form on land.
// Amp=0 disables the pass entirely.
type RidgedConfig struct {
	Amp        float64 // overall mountain contribution (0 = disabled)
	Freq       float64 // base spatial frequency
	Octaves    int     // ridged-fbm octaves (typical 4-6)
	Lacunarity float64 // freq multiplier per octave (default 2.0)
	Gain       float64 // per-octave weight gain (default 0.5)
	Offset     float64 // ridge sharpness (default 1.0; >1 sharper)
	MaskLow    float64 // Continentalness output ≤ this = no ridges
	MaskHigh   float64 // Continentalness output ≥ this = full ridges
}

// ControlField is a single 3D fBm control field used to drive the
// height and biome pipelines (see master plan §5.2). The optional
// Spline maps the field's [0, Amp] output to a height contribution;
// an empty Spline (no knots) means this field does not contribute to
// the heightmap directly (e.g., Temperature/Humidity feed the biome
// table instead).
type ControlField struct {
	Amp         float64 // amplitude multiplier
	Freq        float64 // base frequency
	Octaves     int     // number of fBm octaves
	Lacunarity  float64 // frequency multiplier per octave (default 2.0)
	Persistence float64 // amplitude multiplier per octave (default 0.5)
	Spline      planetcolor.Spline
}

// ProvinceConfig parameterizes per-region roughness modulation: Voronoi
// cells over the sphere, each with its own amp/freq scalar applied to the
// underlying control fields. Gives each archetype regional variety so the
// whole planet doesn't read as one uniform texture.
type ProvinceConfig struct {
	Count   int     // number of Voronoi cells (8-40 typical; 0 = disabled)
	Jitter  float64 // per-cell scalar jitter strength (0 = uniform; 0.5 = high variety)
	WarpAmp float64 // sphere-warp displacement before nearest-cell lookup; 0 = clean Voronoi
}

// ControlConfig holds the five control fields used by the rocky pipeline.
type ControlConfig struct {
	Continentalness ControlField
	Detail          ControlField // formerly "Erosion"; high-frequency detail-noise layer
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

// UnmarshalJSON accepts both the current "Detail" key and the legacy
// "Erosion" key so explorer JSON dumps from before the Phase-4 rename
// keep loading. When both are present the current key wins.
func (c *ControlConfig) UnmarshalJSON(data []byte) error {
	type raw ControlConfig
	aux := struct {
		*raw
		Erosion *ControlField `json:",omitempty"`
	}{raw: (*raw)(c)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	// If Erosion was provided but Detail wasn't, copy Erosion to Detail
	if aux.Erosion != nil && c.Detail.Amp == 0 && c.Detail.Freq == 0 {
		c.Detail = *aux.Erosion
	}
	return nil
}
