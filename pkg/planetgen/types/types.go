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

	// Phase 4 Task 3: coastal noise enhancement using JFA distance-to-coast.
	// Amp=0 disables (default).
	Coastal CoastalConfig

	// Phase 4 Task 5: Voronoi continents with per-continent base height.
	// Seeds=0 disables (default).
	Continents ContinentConfig

	// Phase 4 Task 7: Curl-of-fbm tangent-field advection for gas giants.
	// Amp=0 disables (default).
	Curl CurlConfig

	// Phase 4 Task 8: Hand-authored storm band overlays (Great Red Spot, polar
	// storms). Applied after the Jupiter-ramp base palette. Nil slice = no bands.
	StormBands []StormBand

	// Phase 5: particle hydraulic erosion. Droplets=0 disables (default).
	Erosion ErosionConfig

	// HeightSmoothRadius is the radius (in pixels) of a per-face disc blur
	// applied to the heightmap after control-field summation. 0 disables.
	// 2-3 typical for rocky archetypes; smooths fbm popcorn so erosion can
	// form coherent channels.
	HeightSmoothRadius int
}

// StormBand places a hand-authored oval storm feature at a specific latitude.
// Applied on top of the JupiterRamp base palette for gas giants.
type StormBand struct {
	Lat       float64  // latitude in radians (-π/2 … π/2)
	HalfWidth float64  // angular half-width in radians
	Color     ColorRGB // band tint
	Strength  float64  // mix amount [0,1]
}

// ErosionConfig parameterizes the sphere-walk particle hydraulic erosion pass.
// Droplets=0 disables the pass entirely (no-op).
type ErosionConfig struct {
	// Droplets is the canonical droplet count at face=1024. The renderer
	// scales this by face area; Erode itself runs cfg.Droplets verbatim.
	Droplets int
	// Inertia in [0,1]; how much velocity is preserved between steps. 0.05 default.
	Inertia float64
	// Capacity is the sediment capacity multiplier; ~4 default.
	Capacity float64
	// ErosionRate is the fraction of "missing" capacity carved per step; 0.3 default.
	ErosionRate float64
	// Deposition is the fraction of "excess" sediment dropped per step; 0.3 default.
	Deposition float64
	// Evaporation is the water loss per step; 0.01 default.
	Evaporation float64
	// MinSlope is the floor on slope used for capacity to avoid 0; 0.01 default.
	MinSlope float64
	// MaxStepsPerDrop is the hard cap on steps per droplet; 50 default.
	MaxStepsPerDrop int
	// Gravity is the arbitrary gravitational constant; 4.0 default.
	Gravity float64
	// StepLen is the per-step displacement on the unit sphere; 0 = auto = 1/(2*S).
	StepLen float64
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

// CoastalConfig parameterizes localized roughening near coastlines via
// distance-to-coast modulation. Amp=0 disables (default). The formula is
// e_coast = e + α·(1 − e⁴)·(n4 + n5/2 + n6/4)·falloff, where falloff
// smoothly tapers from 1 at the coast to 0 at the threshold distance.
type CoastalConfig struct {
	Amp       float64 // master strength (0 = disabled; typical 0.05–0.2)
	Threshold float64 // distance-to-coast cutoff in [0,1]; effect dies off above this
	Freq      float64 // base frequency for the n4 fbm
}

// ContinentConfig parameterizes Voronoi continents seeded by Fibonacci-spiral
// points on the unit sphere. Each continent has a per-seed base height drawn
// uniformly from [HeightLo, HeightHi]. Seed positions are optionally warped
// by a low-frequency fBm if WarpAmp > 0. Seeds=0 disables (default).
type ContinentConfig struct {
	Seeds    int     // 0 disables; 10–50 typical
	WarpAmp  float64 // displacement amplitude on seed positions
	WarpFreq float64 // displacement fbm frequency
	HeightLo float64 // per-continent base height lower bound (default 0.3)
	HeightHi float64 // per-continent base height upper bound (default 0.7)
}

// CurlConfig parameterizes curl-of-fbm tangent-field semi-Lagrangian advection
// for gas giants. The backward-trace integrates position against a combination
// of zonal-jet and curl-noise displacement over 4–16 iterations.
// Amp=0 and JetAmp=0 disable (default).
type CurlConfig struct {
	Amp        float64 // displacement strength per iteration (0 disables)
	Iterations int     // 4–16 typical
	DT         float64 // step size; 0.05–0.2 typical
	Freq       float64 // curl-noise base frequency
	JetAmp     float64 // zonal-jet contribution per latitude band (0 disables)
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
