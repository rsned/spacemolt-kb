package planetgen

import (
	"image/color"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// PlanetProfile is the exported alias for types.PlanetProfile.
// It's kept here for backward compatibility with callers that reference it as planetgen.PlanetProfile.
type PlanetProfile = types.PlanetProfile














// mustParseLUT parses a .cube file and panics on error.

// Parsed LUT instances

func rgba(r, g, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// defaultSpline is the trivial linear ramp used to gate the new
// control-field pipeline on without yet shaping height non-linearly.
// Tier-S Task 15 will replace per-archetype.
var defaultSpline = planetcolor.Spline{Knots: []planetcolor.SplineKnot{
	{Input: 0, Output: 0},
	{Input: 1, Output: 0.5},
}}

// continentalShelfSpline maps Continentalness fbm into a height curve with a
// sharp coastal cliff at input 0.4–0.5 (output jumps 0.05 → 0.45) and a
// gentle continental rise above that. Used by most rocky archetypes from
// Phase 3 T10 onward to produce coherent landmass shapes instead of the
// linear default.
var continentalShelfSpline = planetcolor.Spline{Knots: []planetcolor.SplineKnot{
	{Input: 0.0, Output: 0.0},
	{Input: 0.4, Output: 0.05},
	{Input: 0.5, Output: 0.45},
	{Input: 0.7, Output: 0.55},
	{Input: 1.0, Output: 0.6},
}}

// Profiles contains the visual parameters for all planet types.
var Profiles = map[string]*types.PlanetProfile{
	"scorched": {
		Type:     "scorched",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(40, 40, 40)},
			{0.2, rgba(80, 75, 70)},
			{0.4, rgba(120, 115, 105)},
			{0.6, rgba(150, 145, 135)},
			{0.8, rgba(180, 175, 165)},
			{1.0, rgba(220, 215, 210)},
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:        350,
		CraterMinRadius:    0.014,
		CraterMaxRadius:    0.048,
		CraterDepth:        0.24,
		PowerLawAlpha:      4.48,
		MariaDensityFactor: 0.08,
		SurfaceAge:         0.69,
		SecondaryDensity:   0.16,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.5, Freq: 0.1, Octaves: 1, Lacunarity: 1, Persistence: 0.42, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 0.5, Freq: 0.1, Octaves: 1, Lacunarity: 1, Persistence: 0.5, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.72, Freq: 0.66, Octaves: 6, Lacunarity: 2.9, Persistence: 0.68, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.83, Freq: 2.43, Octaves: 6, Lacunarity: 1.95, Persistence: 0.39, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 0.57, Freq: 5.56, Octaves: 4, Lacunarity: 1.89, Persistence: 0.5, Spline: defaultSpline},
		},
		LUT: "scorched",
		// Phase 7 Tier B
		PlateCount:           4,
		OceanicPlateFraction: 0.2,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
	},
	"arid": {
		Type:     "arid",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(100, 60, 30)},
			{0.2, rgba(140, 80, 45)},
			{0.4, rgba(180, 120, 70)},
			{0.6, rgba(200, 150, 100)},
			{0.8, rgba(210, 175, 130)},
			{1.0, rgba(230, 210, 180)},
		},
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:        179,
		CraterMinRadius:    0.017,
		CraterMaxRadius:    0.032,
		CraterDepth:        0.22,
		PowerLawAlpha:      2.13,
		MariaDensityFactor: 0.16,
		SurfaceAge:         0.57,
		SecondaryDensity:   0.03,
		HasPolarCaps:       true,
		PolarCapSize:       0.18,
		PolarCapNoise:      0.15,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.36, Freq: 5.42, Octaves: 5, Lacunarity: 2.11, Persistence: 0.6, Spline: defaultSpline},
			Detail:         types.ControlField{Amp: 1.44, Freq: 1.81, Octaves: 6, Lacunarity: 2.16, Persistence: 0.49, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.56, Freq: 2.84, Octaves: 3, Lacunarity: 2.99, Persistence: 0.33, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.83, Freq: 0.63, Octaves: 5, Lacunarity: 1.53, Persistence: 0.38, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.04, Freq: 3.48, Octaves: 4, Lacunarity: 1.84, Persistence: 0.41, Spline: defaultSpline},
		},
		Erosion: types.ErosionConfig{
			Droplets:        100000,
			Inertia:         0.05,
			Capacity:        6,
			ErosionRate:     0.3,
			Deposition:      0.5,
			Evaporation:     0.01,
			MinSlope:        0.01,
			MaxStepsPerDrop: 50,
			Gravity:         4,
		},
		HeightSmoothRadius: 2,
		LUT: "arid",
		// Phase 7 Tier B
		PlateCount:           6,
		OceanicPlateFraction: 0.3,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 8 item 15: very sparse, dry valleys
		Flow: types.FlowConfig{RiverThreshold: 1500, RiverDepth: 0.005},
		// Phase 8 item 16
		RainShadow: types.RainShadowConfig{
			WalkSteps: 12, StepArcRad: 0.087, MountainCutoff: 0.55,
			WindRainBoost: 0.2, LeeFactor: 0.05,
		},
	},
	"terran": {
		Type:     "terran",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(12, 50, 10)},     // Deep dark forest
			{0.1, rgba(10, 42, 8)},      // Dense jungle
			{0.2, rgba(18, 55, 15)},     // Dark forest
			{0.3, rgba(30, 75, 25)},     // Forest
			{0.4, rgba(50, 100, 40)},    // Forest green
			{0.5, rgba(75, 115, 55)},    // Woodland
			{0.6, rgba(110, 125, 70)},   // Light grassland
			{0.68, rgba(135, 120, 78)},  // Brown foothills
			{0.75, rgba(150, 140, 118)}, // Rocky highlands
			{0.82, rgba(170, 165, 152)}, // Gray mountain
			{0.9, rgba(190, 188, 182)},  // Bare rock
			{1.0, rgba(220, 220, 218)},  // Mountain peak
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.1,
		NoisePersistence: 0.62,
		NoiseScale:       3.0,
		CraterCount:        3,
		CraterMinRadius:    0.011,
		CraterMaxRadius:    0.068,
		CraterDepth:        0.11,
		PowerLawAlpha:      1.75,
		MariaDensityFactor: 0.05,
		SurfaceAge:         0.48,
		SecondaryDensity:   0.5,
		HasPolarCaps:       true,
		PolarCapSize:       0.15,
		OceanLevel:         0.50,
		OceanColor:         rgba(30, 60, 140),
		SnowLine:           0.78,
		EquatorialPalette: []planetcolor.ColorStop{
			{0.0, rgba(190, 170, 120)},  // Sandy lowland
			{0.15, rgba(200, 180, 130)}, // Light sand
			{0.3, rgba(185, 165, 110)},  // Desert tan
			{0.45, rgba(170, 150, 95)},  // Dry savanna
			{0.6, rgba(145, 125, 75)},   // Arid scrub
			{0.7, rgba(135, 118, 78)},   // Dry hills
			{0.8, rgba(155, 145, 125)},  // Rocky
			{0.9, rgba(175, 170, 158)},  // Gray peaks
			{1.0, rgba(200, 200, 195)},  // Bare rock
		},
		PolarPalette: []planetcolor.ColorStop{
			{0.0, rgba(65, 95, 58)},     // Cold scrub
			{0.2, rgba(85, 105, 75)},    // Boreal green
			{0.4, rgba(105, 115, 88)},   // Taiga
			{0.6, rgba(135, 135, 115)},  // Tundra brown
			{0.7, rgba(158, 152, 142)},  // Rocky tundra
			{0.8, rgba(178, 174, 168)},  // Frost rock
			{0.9, rgba(200, 198, 195)},  // Icy peaks
			{1.0, rgba(225, 225, 222)},  // Snow
		},
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.97, Freq: 2.02, Octaves: 5, Lacunarity: 2.12, Persistence: 0.51, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 0.55, Freq: 2.95, Octaves: 3, Lacunarity: 2.48, Persistence: 0.67, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.96, Freq: 2.09, Octaves: 5, Lacunarity: 2.5, Persistence: 0.72, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.71, Freq: 1.27, Octaves: 4, Lacunarity: 1.89, Persistence: 0.44, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.38, Freq: 5.81, Octaves: 4, Lacunarity: 2.83, Persistence: 0.73, Spline: defaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.20, Freq: 1.21, Octaves: 6, Lacunarity: 2.09, Gain: 0.53, Offset: 0.98,
			MaskLow: 0.45, MaskHigh: 0.80,
			PlateConvergentScaleKm: 800,
		},
		BiomeTable: types.BiomeTable{
			TBuckets: 4,
			MBuckets: 4,
			Cells: [][]types.BiomeCell{
				{ // T=0 cold
					{Low: types.ColorRGB{200, 210, 220}, High: types.ColorRGB{240, 245, 250}}, // dry tundra → snow
					{Low: types.ColorRGB{180, 190, 195}, High: types.ColorRGB{230, 235, 240}},
					{Low: types.ColorRGB{160, 175, 180}, High: types.ColorRGB{220, 225, 235}},
					{Low: types.ColorRGB{120, 150, 170}, High: types.ColorRGB{200, 220, 235}}, // wet ice/glacier
				},
				{ // T=1 cool
					{Low: types.ColorRGB{160, 150, 130}, High: types.ColorRGB{200, 195, 180}},
					{Low: types.ColorRGB{110, 130, 95}, High: types.ColorRGB{170, 180, 165}},
					{Low: types.ColorRGB{75, 105, 70}, High: types.ColorRGB{150, 165, 145}},
					{Low: types.ColorRGB{55, 90, 60}, High: types.ColorRGB{130, 150, 135}},
				},
				{ // T=2 warm
					{Low: types.ColorRGB{195, 180, 145}, High: types.ColorRGB{220, 215, 190}}, // dry savanna
					{Low: types.ColorRGB{145, 155, 80}, High: types.ColorRGB{195, 195, 165}},
					{Low: types.ColorRGB{75, 130, 50}, High: types.ColorRGB{160, 175, 130}}, // grassland
					{Low: types.ColorRGB{30, 90, 25}, High: types.ColorRGB{110, 140, 90}}, // forest
				},
				{ // T=3 hot
					{Low: types.ColorRGB{225, 195, 130}, High: types.ColorRGB{240, 225, 180}}, // dry desert
					{Low: types.ColorRGB{200, 180, 105}, High: types.ColorRGB{220, 210, 155}},
					{Low: types.ColorRGB{145, 170, 80}, High: types.ColorRGB{195, 200, 140}},
					{Low: types.ColorRGB{20, 80, 35}, High: types.ColorRGB{90, 130, 80}}, // tropical jungle
				},
			},
		},
		Erosion: types.ErosionConfig{
			Droplets:        250000,
			Inertia:         0.12,
			Capacity:        4,
			ErosionRate:     0.3,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.1,
			MaxStepsPerDrop: 250,
			Gravity:         4,
			BrushFalloff:    7,
		},
		HeightSmoothRadius: 4,
		LUT: "terran",
		// Phase 7 Tier B
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 8 item 15
		Flow: types.FlowConfig{RiverThreshold: 200, RiverDepth: 0.02},
		// Phase 8 item 16
		RainShadow: types.RainShadowConfig{
			WalkSteps: 12, StepArcRad: 0.087, MountainCutoff: 0.65,
			WindRainBoost: 0.4, LeeFactor: 0.15,
		},
		// Phase 9a item 17
		Cloud: types.CloudConfig{
			Coverage: 0.45, BandLatRad: 0.26, Freq: 4, Octaves: 4, WarpAmp: 0.4,
			StormCount: 5, StormRadiusRad: 0.20,
			SunDir: [3]float64{1, 0.3, 0}, ShadowGain: 0.5,
		},
		// Phase 9b item 18
		Civ: types.CivConfig{
			Tier: 0.5, SiteMinDistRad: 0.0314, SiteMaxDistRad: 0.1047,
			MaxPopulation: 1.0, NightLightHue: 0.12, AgricultureRatio: 0.4,
		},
	},
	"tundra": {
		Type:     "tundra",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(90, 100, 80)},
			{0.25, rgba(120, 130, 100)},
			{0.5, rgba(150, 155, 130)},
			{0.7, rgba(180, 180, 165)},
			{0.85, rgba(200, 200, 190)},
			{1.0, rgba(230, 230, 225)},
		},
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:        92,
		CraterMinRadius:    0.008,
		CraterMaxRadius:    0.094,
		CraterDepth:        0.15,
		PowerLawAlpha:      2.40,
		MariaDensityFactor: 0.24,
		SurfaceAge:         0.30,
		SecondaryDensity:   0.05,
		HasPolarCaps:       true,
		PolarCapSize:       0.25,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.50, Freq: 2.20, Octaves: 6, Lacunarity: 2.02, Persistence: 0.44, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 1.12, Freq: 4.07, Octaves: 4, Lacunarity: 1.78, Persistence: 0.66, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 1.82, Freq: 3.99, Octaves: 2, Lacunarity: 1.52, Persistence: 0.48, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.37, Freq: 4.62, Octaves: 2, Lacunarity: 2.88, Persistence: 0.71, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.55, Freq: 3.50, Octaves: 2, Lacunarity: 1.67, Persistence: 0.70, Spline: defaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.28, Freq: 1.35, Octaves: 6, Lacunarity: 2.02, Gain: 0.70, Offset: 0.89,
			MaskLow: 0.40, MaskHigh: 0.68,
			PlateConvergentScaleKm: 700,
		},
		Erosion: types.ErosionConfig{
			Droplets:        250000,
			Inertia:         0.12,
			Capacity:        4,
			ErosionRate:     0.3,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.1,
			MaxStepsPerDrop: 250,
			Gravity:         4,
			BrushFalloff:    7,
		},
		HeightSmoothRadius: 4,
		LUT: "tundra",
		// Phase 7 Tier B
		PlateCount:           8,
		OceanicPlateFraction: 0.5,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 8 item 15: sparser drainage in tundra
		Flow: types.FlowConfig{RiverThreshold: 300, RiverDepth: 0.015},
		// Phase 8 item 16
		RainShadow: types.RainShadowConfig{
			WalkSteps: 8, StepArcRad: 0.087, MountainCutoff: 0.7,
			WindRainBoost: 0.3, LeeFactor: 0.2,
		},
	},
	"glacial": {
		Type:     "glacial",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(160, 180, 200)},
			{0.3, rgba(180, 200, 220)},
			{0.5, rgba(200, 215, 230)},
			{0.7, rgba(215, 225, 235)},
			{0.85, rgba(230, 235, 240)},
			{1.0, rgba(245, 248, 250)},
		},
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       3.0,
		CraterCount:        173,
		CraterMinRadius:    0.014,
		CraterMaxRadius:    0.061,
		CraterDepth:        0.25,
		PowerLawAlpha:      2.06,
		MariaDensityFactor: 0.09,
		SurfaceAge:         0.50,
		SecondaryDensity:   0.08,
		HasPolarCaps:       true,
		PolarCapSize:       0.35,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.84, Freq: 4.56, Octaves: 2, Lacunarity: 2.45, Persistence: 0.75, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 1.94, Freq: 1.14, Octaves: 3, Lacunarity: 2.39, Persistence: 0.68, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.76, Freq: 1.05, Octaves: 6, Lacunarity: 3.00, Persistence: 0.59, Spline: defaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.22, Freq: 4.33, Octaves: 4, Lacunarity: 2.32, Gain: 0.75, Offset: 1.04,
			MaskLow: 0.50, MaskHigh: 0.79,
			PlateConvergentScaleKm: 600,
		},
		Erosion: types.ErosionConfig{
			Droplets:        50000,
			Inertia:         0.05,
			Capacity:        4,
			ErosionRate:     0.25,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.01,
			MaxStepsPerDrop: 50,
			Gravity:         4,
		},
		HeightSmoothRadius: 2,
		LUT: "glacial",
		// Phase 7 Tier B
		PlateCount:           6,
		OceanicPlateFraction: 0.4,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 8 item 15
		Flow: types.FlowConfig{RiverThreshold: 400, RiverDepth: 0.018},
		// Phase 8 item 16
		RainShadow: types.RainShadowConfig{
			WalkSteps: 6, StepArcRad: 0.087, MountainCutoff: 0.7,
			WindRainBoost: 0.25, LeeFactor: 0.25,
		},
	},
	"ice_world": {
		Type:     "ice_world",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(140, 170, 200)},
			{0.2, rgba(160, 190, 215)},
			{0.4, rgba(175, 205, 225)},
			{0.6, rgba(190, 215, 235)},
			{0.8, rgba(210, 230, 240)},
			{1.0, rgba(240, 245, 250)},
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.2,
		NoisePersistence: 0.45,
		NoiseScale:       3.5,
		CraterCount:        82,
		CraterMinRadius:    0.009,
		CraterMaxRadius:    0.037,
		CraterDepth:        0.24,
		PowerLawAlpha:      1.61,
		MariaDensityFactor: 0.16,
		SurfaceAge:         0.66,
		SecondaryDensity:   0.25,
		HasPolarCaps:       true,
		PolarCapSize:       0.3,
		OceanLevel:         0.45,
		OceanColor:         rgba(117, 174, 234),
		SnowLine:           0.50,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.73, Freq: 1.53, Octaves: 2, Lacunarity: 1.56, Persistence: 0.64, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 1.67, Freq: 4.43, Octaves: 4, Lacunarity: 2.28, Persistence: 0.63, Spline: continentalShelfSpline},
		},
		Warp: types.WarpConfig{
			Amp: 0.26, Freq: 1.10, Octaves: 2, Lacunarity: 2.69, Persistence: 0.28,
		},
		Ridged: types.RidgedConfig{
			Amp: 0.14, Freq: 4.30, Octaves: 3, Lacunarity: 2.24, Gain: 0.51, Offset: 1.17,
			MaskLow: 0.48, MaskHigh: 0.65,
			PlateConvergentScaleKm: 0, // ice_world has no plates → legacy Continentalness mask
		},
		ShadingStrength:     0.5,
		ShadingExaggeration: 0.1,
		Erosion: types.ErosionConfig{
			Droplets:        50000,
			Inertia:         0.05,
			Capacity:        4,
			ErosionRate:     0.25,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.01,
			MaxStepsPerDrop: 50,
			Gravity:         4,
		},
		HeightSmoothRadius: 2,
		LUT: "ice_world",
		// Phase 7 Tier B
		JitterEnabled:   true,
		JitterCellCount: 120,
		JitterRotMax:    math.Pi / 4,
		JitterOffsetMax: 0.1,
	},
	"super_terran": {
		Type:     "super_terran",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(12, 50, 10)},
			{0.1, rgba(10, 42, 8)},
			{0.2, rgba(18, 55, 15)},
			{0.3, rgba(30, 75, 25)},
			{0.4, rgba(50, 100, 40)},
			{0.5, rgba(75, 115, 55)},
			{0.6, rgba(110, 125, 70)},
			{0.68, rgba(135, 120, 78)},
			{0.75, rgba(150, 140, 118)},
			{0.82, rgba(170, 165, 152)},
			{0.9, rgba(190, 188, 182)},
			{1.0, rgba(220, 220, 218)},
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.1,
		NoisePersistence: 0.62,
		NoiseScale:       3.0,
		CraterCount:        157,
		CraterMinRadius:    0.007,
		CraterMaxRadius:    0.015,
		CraterDepth:        0.12,
		PowerLawAlpha:      1.80,
		MariaDensityFactor: 0.10,
		SurfaceAge:         0.59,
		SecondaryDensity:   0.34,
		HasPolarCaps:       true,
		PolarCapSize:       0.25,
		OceanLevel:         0.53,
		OceanColor:         rgba(30, 60, 140),
		SnowLine:           0.74,
		EquatorialPalette: []planetcolor.ColorStop{
			{0.0, rgba(190, 170, 120)},
			{0.15, rgba(200, 180, 130)},
			{0.3, rgba(185, 165, 110)},
			{0.45, rgba(170, 150, 95)},
			{0.6, rgba(145, 125, 75)},
			{0.7, rgba(135, 118, 78)},
			{0.8, rgba(155, 145, 125)},
			{0.9, rgba(175, 170, 158)},
			{1.0, rgba(200, 200, 195)},
		},
		PolarPalette: []planetcolor.ColorStop{
			{0.0, rgba(65, 95, 58)},
			{0.2, rgba(85, 105, 75)},
			{0.4, rgba(105, 115, 88)},
			{0.6, rgba(135, 135, 115)},
			{0.7, rgba(158, 152, 142)},
			{0.8, rgba(178, 174, 168)},
			{0.9, rgba(200, 198, 195)},
			{1.0, rgba(225, 225, 222)},
		},
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.20, Freq: 2.64, Octaves: 4, Lacunarity: 2.75, Persistence: 0.30, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 1.30, Freq: 0.69, Octaves: 3, Lacunarity: 1.65, Persistence: 0.68, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.51, Freq: 2.75, Octaves: 5, Lacunarity: 2.90, Persistence: 0.74, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.52, Freq: 4.73, Octaves: 4, Lacunarity: 1.66, Persistence: 0.36, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.31, Freq: 2.34, Octaves: 2, Lacunarity: 1.55, Persistence: 0.62, Spline: defaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.23, Freq: 1.69, Octaves: 5, Lacunarity: 1.89, Gain: 0.41, Offset: 1.15,
			MaskLow: 0.34, MaskHigh: 0.74,
			PlateConvergentScaleKm: 800,
		},
		ShadingStrength: 0.05,
		BiomeTable: types.BiomeTable{
			TBuckets: 4,
			MBuckets: 4,
			Cells: [][]types.BiomeCell{
				{
					{Low: types.ColorRGB{200, 210, 220}, High: types.ColorRGB{240, 245, 250}},
					{Low: types.ColorRGB{180, 190, 195}, High: types.ColorRGB{230, 235, 240}},
					{Low: types.ColorRGB{160, 175, 180}, High: types.ColorRGB{220, 225, 235}},
					{Low: types.ColorRGB{120, 150, 170}, High: types.ColorRGB{200, 220, 235}},
				},
				{
					{Low: types.ColorRGB{160, 150, 130}, High: types.ColorRGB{200, 195, 180}},
					{Low: types.ColorRGB{110, 130, 95}, High: types.ColorRGB{170, 180, 165}},
					{Low: types.ColorRGB{75, 105, 70}, High: types.ColorRGB{150, 165, 145}},
					{Low: types.ColorRGB{55, 90, 60}, High: types.ColorRGB{130, 150, 135}},
				},
				{
					{Low: types.ColorRGB{195, 180, 145}, High: types.ColorRGB{220, 215, 190}},
					{Low: types.ColorRGB{145, 155, 80}, High: types.ColorRGB{195, 195, 165}},
					{Low: types.ColorRGB{75, 130, 50}, High: types.ColorRGB{160, 175, 130}},
					{Low: types.ColorRGB{30, 90, 25}, High: types.ColorRGB{110, 140, 90}},
				},
				{
					{Low: types.ColorRGB{225, 195, 130}, High: types.ColorRGB{240, 225, 180}},
					{Low: types.ColorRGB{200, 180, 105}, High: types.ColorRGB{220, 210, 155}},
					{Low: types.ColorRGB{145, 170, 80}, High: types.ColorRGB{195, 200, 140}},
					{Low: types.ColorRGB{20, 80, 35}, High: types.ColorRGB{90, 130, 80}},
				},
			},
		},
		Erosion: types.ErosionConfig{
			Droplets:        250000,
			Inertia:         0.12,
			Capacity:        4,
			ErosionRate:     0.3,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.1,
			MaxStepsPerDrop: 250,
			Gravity:         4,
			BrushFalloff:    7,
		},
		HeightSmoothRadius: 4,
		LUT: "super_terran",
		// Phase 7 Tier B
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 8 item 15
		Flow: types.FlowConfig{RiverThreshold: 250, RiverDepth: 0.025},
		// Phase 8 item 16
		RainShadow: types.RainShadowConfig{
			WalkSteps: 12, StepArcRad: 0.087, MountainCutoff: 0.65,
			WindRainBoost: 0.4, LeeFactor: 0.15,
		},
		// Phase 9a item 17
		Cloud: types.CloudConfig{
			Coverage: 0.50, BandLatRad: 0.26, Freq: 4, Octaves: 4, WarpAmp: 0.4,
			StormCount: 6, StormRadiusRad: 0.18,
			SunDir: [3]float64{1, 0.3, 0}, ShadowGain: 0.5,
		},
		// Phase 9b item 18
		Civ: types.CivConfig{
			Tier: 0.3, SiteMinDistRad: 0.0314, SiteMaxDistRad: 0.1047,
			MaxPopulation: 0.8, NightLightHue: 0.12, AgricultureRatio: 0.4,
		},
	},
	"hothouse": {
		Type:     "hothouse",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(120, 130, 60)},
			{0.2, rgba(140, 150, 70)},
			{0.4, rgba(100, 120, 50)},
			{0.6, rgba(160, 170, 90)},
			{0.8, rgba(190, 195, 140)},
			{1.0, rgba(220, 220, 190)},
		},
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.0,
		CraterCount:        138,
		CraterMinRadius:    0.020,
		CraterMaxRadius:    0.044,
		CraterDepth:        0.28,
		PowerLawAlpha:      2.15,
		MariaDensityFactor: 0.42,
		SurfaceAge:         0.52,
		SecondaryDensity:   0.09,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.24, Freq: 2.86, Octaves: 5, Lacunarity: 2.31, Persistence: 0.66, Spline: defaultSpline},
			Detail:         types.ControlField{Amp: 0.74, Freq: 1.87, Octaves: 6, Lacunarity: 2.24, Persistence: 0.69, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 1.45, Freq: 4.94, Octaves: 6, Lacunarity: 2.51, Persistence: 0.70, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 0.64, Freq: 3.47, Octaves: 4, Lacunarity: 1.97, Persistence: 0.38, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 0.81, Freq: 0.99, Octaves: 4, Lacunarity: 2.33, Persistence: 0.78, Spline: defaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.29, Freq: 2.21, Octaves: 3, Lacunarity: 2.35, Gain: 0.65, Offset: 1.02,
			MaskLow: 0.47, MaskHigh: 0.74,
			PlateConvergentScaleKm: 0, // hothouse has no plates → legacy Continentalness mask
		},
		LUT: "hothouse",
		// Phase 7 Tier B
		JitterEnabled:   true,
		JitterCellCount: 120,
		JitterRotMax:    math.Pi / 4,
		JitterOffsetMax: 0.1,
		// Phase 9a item 17
		Cloud: types.CloudConfig{
			Coverage: 0.85, BandLatRad: 0.40, Freq: 3, Octaves: 3, WarpAmp: 0.5,
			StormCount: 4, StormRadiusRad: 0.30,
			SunDir: [3]float64{1, 0.3, 0}, ShadowGain: 0.4,
		},
	},
	"lava_world": {
		Type:     "lava_world",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(20, 15, 10)},     // Deep basalt
			{0.2, rgba(35, 25, 18)},     // Dark rock
			{0.4, rgba(50, 35, 22)},     // Brown rock
			{0.6, rgba(65, 45, 28)},     // Warm rock
			{0.8, rgba(55, 38, 25)},     // Dark crust
			{1.0, rgba(40, 28, 18)},     // Dark peaks
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.2,
		NoisePersistence: 0.55,
		NoiseScale:       3.0,
		CraterCount:        128,
		CraterMinRadius:    0.011,
		CraterMaxRadius:    0.090,
		CraterDepth:        0.12,
		PowerLawAlpha:      1.67,
		MariaDensityFactor: 0.06,
		SurfaceAge:         0.43,
		SecondaryDensity:   0.30,
		OceanLevel:         0.52,
		OceanColor:         rgba(220, 80, 5),
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{
				Amp: 1.13, Freq: 3.82, Octaves: 6, Lacunarity: 2.29, Persistence: 0.56,
				Spline: continentalShelfSpline,
			},
			Detail: types.ControlField{
				Amp: 0.89, Freq: 2.53, Octaves: 6, Lacunarity: 1.92, Persistence: 0.34,
				Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{
					{Input: 0, Output: 0}, {Input: 1, Output: 0.6},
				}},
			},
			PeaksValleys: types.ControlField{
				Amp: 0.75, Freq: 5.18, Octaves: 6, Lacunarity: 2.28, Persistence: 0.69,
				Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{
					{Input: 0, Output: 0}, {Input: 1, Output: 0.6},
				}},
			},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.14, Freq: 2.22, Octaves: 3, Lacunarity: 2.34, Gain: 0.74, Offset: 1.08,
			MaskLow: 0.44, MaskHigh: 0.65,
			PlateConvergentScaleKm: 400,
		},
		LUT: "lava_world",
		// Phase 7 Tier B
		PlateCount:           4,
		OceanicPlateFraction: 0.2,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
	},
	"oceanic": {
		Type:     "oceanic",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(12, 50, 10)},
			{0.1, rgba(10, 42, 8)},
			{0.2, rgba(18, 55, 15)},
			{0.3, rgba(30, 75, 25)},
			{0.4, rgba(50, 100, 40)},
			{0.5, rgba(75, 115, 55)},
			{0.6, rgba(110, 125, 70)},
			{0.68, rgba(135, 120, 78)},
			{0.75, rgba(150, 140, 118)},
			{0.82, rgba(170, 165, 152)},
			{0.9, rgba(190, 188, 182)},
			{1.0, rgba(220, 220, 218)},
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.1,
		NoisePersistence: 0.62,
		NoiseScale:       3.0,
		CraterCount:      10,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.02,
		CraterDepth:      0.08,
		HasPolarCaps:     true,
		PolarCapSize:     0.24,
		PolarCapNoise:    0.12,
		OceanLevel:       0.77,
		OceanColor:       rgba(30, 60, 140),
		SnowLine:         0.82,
		EquatorialPalette: []planetcolor.ColorStop{
			{0.0, rgba(190, 170, 120)},
			{0.15, rgba(200, 180, 130)},
			{0.3, rgba(185, 165, 110)},
			{0.45, rgba(170, 150, 95)},
			{0.6, rgba(145, 125, 75)},
			{0.7, rgba(135, 118, 78)},
			{0.8, rgba(155, 145, 125)},
			{0.9, rgba(175, 170, 158)},
			{1.0, rgba(200, 200, 195)},
		},
		PolarPalette: []planetcolor.ColorStop{
			{0.0, rgba(65, 95, 58)},
			{0.2, rgba(85, 105, 75)},
			{0.4, rgba(105, 115, 88)},
			{0.6, rgba(135, 135, 115)},
			{0.7, rgba(158, 152, 142)},
			{0.8, rgba(178, 174, 168)},
			{0.9, rgba(200, 198, 195)},
			{1.0, rgba(225, 225, 222)},
		},
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.83, Freq: 5.85, Octaves: 4, Lacunarity: 2.59, Persistence: 0.68, Spline: continentalShelfSpline},
			Detail:         types.ControlField{Amp: 1.29, Freq: 4.59, Octaves: 4, Lacunarity: 1.80, Persistence: 0.73, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 1.00, Freq: 4.22, Octaves: 3, Lacunarity: 2.97, Persistence: 0.67, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.88, Freq: 2.37, Octaves: 5, Lacunarity: 1.54, Persistence: 0.32, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.57, Freq: 0.84, Octaves: 4, Lacunarity: 2.23, Persistence: 0.35, Spline: defaultSpline},
		},
		BiomeTable: types.BiomeTable{
			TBuckets: 4,
			MBuckets: 4,
			Cells: [][]types.BiomeCell{
				{
					{Low: types.ColorRGB{200, 210, 220}, High: types.ColorRGB{240, 245, 250}},
					{Low: types.ColorRGB{180, 190, 195}, High: types.ColorRGB{230, 235, 240}},
					{Low: types.ColorRGB{160, 175, 180}, High: types.ColorRGB{220, 225, 235}},
					{Low: types.ColorRGB{120, 150, 170}, High: types.ColorRGB{200, 220, 235}},
				},
				{
					{Low: types.ColorRGB{160, 150, 130}, High: types.ColorRGB{200, 195, 180}},
					{Low: types.ColorRGB{110, 130, 95}, High: types.ColorRGB{170, 180, 165}},
					{Low: types.ColorRGB{75, 105, 70}, High: types.ColorRGB{150, 165, 145}},
					{Low: types.ColorRGB{55, 90, 60}, High: types.ColorRGB{130, 150, 135}},
				},
				{
					{Low: types.ColorRGB{195, 180, 145}, High: types.ColorRGB{220, 215, 190}},
					{Low: types.ColorRGB{145, 155, 80}, High: types.ColorRGB{195, 195, 165}},
					{Low: types.ColorRGB{75, 130, 50}, High: types.ColorRGB{160, 175, 130}},
					{Low: types.ColorRGB{30, 90, 25}, High: types.ColorRGB{110, 140, 90}},
				},
				{
					{Low: types.ColorRGB{225, 195, 130}, High: types.ColorRGB{240, 225, 180}},
					{Low: types.ColorRGB{200, 180, 105}, High: types.ColorRGB{220, 210, 155}},
					{Low: types.ColorRGB{145, 170, 80}, High: types.ColorRGB{195, 200, 140}},
					{Low: types.ColorRGB{20, 80, 35}, High: types.ColorRGB{90, 130, 80}},
				},
			},
		},
		Erosion: types.ErosionConfig{
			Droplets:        250000,
			Inertia:         0.12,
			Capacity:        4,
			ErosionRate:     0.3,
			Deposition:      0.3,
			Evaporation:     0.01,
			MinSlope:        0.1,
			MaxStepsPerDrop: 250,
			Gravity:         4,
			BrushFalloff:    7,
		},
		HeightSmoothRadius: 4,
		LUT: "oceanic",
		// Phase 7 Tier B
		PlateCount:           12,
		OceanicPlateFraction: 0.9,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
		// Phase 9a item 17
		Cloud: types.CloudConfig{
			Coverage: 0.65, BandLatRad: 0.30, Freq: 4, Octaves: 4, WarpAmp: 0.4,
			StormCount: 8, StormRadiusRad: 0.22,
			SunDir: [3]float64{1, 0.3, 0}, ShadowGain: 0.5,
		},
	},

	// Gas giants
	"jovian": {
		Type:     "jovian",
		Renderer: "gas_giant",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(180, 140, 100)},  // Tan
			{0.15, rgba(210, 180, 140)}, // Light tan
			{0.3, rgba(200, 160, 120)},  // Brown
			{0.45, rgba(220, 200, 180)}, // Cream/white
			{0.6, rgba(190, 140, 90)},   // Orange-brown
			{0.75, rgba(160, 120, 80)},  // Dark brown
			{0.9, rgba(210, 190, 160)},  // Light zone
			{1.0, rgba(180, 150, 110)},  // Tan
		},
		NoiseOctaves:     6,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.0,
		BandCount:        26,
		TurbulenceAmp:    0.012,
		StormCount:       3,
		StormSize:        0.25,
		Warp: types.WarpConfig{
			Amp: 0.34, Freq: 0.53, Octaves: 3, Lacunarity: 1.69, Persistence: 0.24,
		},
		Curl: types.CurlConfig{
			Amp: 0.10, Iterations: 6, DT: 0.08, Freq: 1.83, JetAmp: 0.17,
		},
		StormBands: []types.StormBand{
			{Lat: -0.34, HalfWidth: 0.12, Color: types.ColorRGB{200, 80, 80}, Strength: 0.5},
			{Lat: 0.50, HalfWidth: 0.15, Color: types.ColorRGB{200, 80, 80}, Strength: 0.5},
			{Lat: 0.26, HalfWidth: 0.04, Color: types.ColorRGB{200, 196, 81}, Strength: 0.5},
		},
		LUT: "jovian",
		// Phase 7 Tier B: no plates or jitter for gas giants
		JitterEnabled: false,
	},
	"ice_giant": {
		Type:     "ice_giant",
		Renderer: "gas_giant",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(100, 150, 185)},  // Darker blue belt
			{0.2, rgba(115, 160, 190)},  // Medium blue
			{0.4, rgba(125, 170, 200)},  // Blue
			{0.6, rgba(140, 180, 205)},  // Slightly lighter
			{0.8, rgba(150, 190, 210)},  // Light teal
			{1.0, rgba(165, 200, 218)},  // Lightest zone
		},
		NoiseOctaves:     5,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.45,
		NoiseScale:       1.5,
		BandCount:        22,
		TurbulenceAmp:    0.008,
		BandBlendWidth:   0.45,
		StormCount:       0,
		StormSize:        0.05,
		Curl: types.CurlConfig{
			Amp: 0.03, Iterations: 14, DT: 0.13, Freq: 4.08, JetAmp: 0.41,
		},
		LUT: "ice_giant",
		// Phase 7 Tier B: no plates or jitter for gas giants
		JitterEnabled: false,
	},

	// Unknown/unclassified planets — neutral gray rocky appearance
	"unknown": {
		Type:     "unknown",
		Renderer: "rocky",
		Palette: []planetcolor.ColorStop{
			{0.0, rgba(90, 90, 95)},
			{0.2, rgba(110, 110, 115)},
			{0.4, rgba(130, 130, 132)},
			{0.6, rgba(150, 148, 145)},
			{0.8, rgba(170, 168, 165)},
			{1.0, rgba(195, 192, 190)},
		},
		NoiseOctaves:     7,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.5,
		NoiseScale:       2.5,
		CraterCount:      25,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.04,
		CraterDepth:      0.12,
			LUT:              "unknown",
		// Phase 7 Tier B: no plates or jitter for unknown
		JitterEnabled: false,
	},
}

// GetProfile returns the planet profile for the given type.
// Returns the "unknown" profile for unrecognized types.
func GetProfile(planetType string) *types.PlanetProfile {
	if p, ok := Profiles[planetType]; ok {
		return p
	}
	return Profiles["unknown"]
}
