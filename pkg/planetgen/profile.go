package planetgen

import (
	"image/color"

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
		CraterCount:      200,
		CraterMinRadius:  0.005,
		CraterMaxRadius:  0.08,
		CraterDepth:      0.25,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.5, Freq: 0.1, Octaves: 1, Lacunarity: 1, Persistence: 0.42, Spline: defaultSpline},
			Erosion:         types.ControlField{Amp: 0.5, Freq: 0.1, Octaves: 1, Lacunarity: 1, Persistence: 0.5, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.72, Freq: 0.66, Octaves: 6, Lacunarity: 2.9, Persistence: 0.68, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.83, Freq: 2.43, Octaves: 6, Lacunarity: 1.95, Persistence: 0.39, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 0.57, Freq: 5.56, Octaves: 4, Lacunarity: 1.89, Persistence: 0.5, Spline: defaultSpline},
		},
		LUT: "scorched",
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
		CraterCount:      80,
		CraterMinRadius:  0.005,
		CraterMaxRadius:  0.06,
		CraterDepth:      0.2,
		HasPolarCaps:     true,
		PolarCapSize:     0.18,
		PolarCapNoise:    0.15,
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 1.36, Freq: 5.42, Octaves: 5, Lacunarity: 2.11, Persistence: 0.6, Spline: defaultSpline},
			Erosion:         types.ControlField{Amp: 1.44, Freq: 1.81, Octaves: 6, Lacunarity: 2.16, Persistence: 0.49, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.56, Freq: 2.84, Octaves: 3, Lacunarity: 2.99, Persistence: 0.33, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.83, Freq: 0.63, Octaves: 5, Lacunarity: 1.53, Persistence: 0.38, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.04, Freq: 3.48, Octaves: 4, Lacunarity: 1.84, Persistence: 0.41, Spline: defaultSpline},
		},
		LUT: "arid",
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
		CraterCount:      10,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.02,
		CraterDepth:      0.08,
		HasPolarCaps:     true,
		PolarCapSize:     0.15,
		OceanLevel:       0.50,
		OceanColor:       rgba(30, 60, 140),
		SnowLine:         0.78,
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
			Continentalness: types.ControlField{Amp: 0.97, Freq: 2.02, Octaves: 5, Lacunarity: 2.12, Persistence: 0.51, Spline: defaultSpline},
			Erosion:         types.ControlField{Amp: 0.55, Freq: 2.95, Octaves: 3, Lacunarity: 2.48, Persistence: 0.67, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.96, Freq: 2.09, Octaves: 5, Lacunarity: 2.5, Persistence: 0.72, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.71, Freq: 1.27, Octaves: 4, Lacunarity: 1.89, Persistence: 0.44, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.38, Freq: 5.81, Octaves: 4, Lacunarity: 2.83, Persistence: 0.73, Spline: defaultSpline},
		},
		Warp: types.WarpConfig{Amp: 0.03, Freq: 2.64, Octaves: 3, Lacunarity: 2.13, Persistence: 0.22},
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
			LUT:              "terran",
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
		CraterCount:      30,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.04,
		CraterDepth:      0.12,
		HasPolarCaps:     true,
		PolarCapSize:     0.25,
			LUT:              "tundra",
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
		CraterCount:      20,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.04,
		CraterDepth:      0.1,
		HasPolarCaps:     true,
		PolarCapSize:     0.35,
			LUT:              "glacial",
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
		CraterCount:      40,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.05,
		CraterDepth:      0.15,
		HasPolarCaps:     true,
		PolarCapSize:     0.3,
			LUT:              "ice_world",
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
		CraterCount:      10,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.02,
		CraterDepth:      0.08,
		HasPolarCaps:     true,
		PolarCapSize:     0.20,
		OceanLevel:       0.53,
		OceanColor:       rgba(30, 60, 140),
		SnowLine:         0.74,
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
			Continentalness: types.ControlField{Amp: 1.92, Freq: 2.26, Octaves: 5, Lacunarity: 2.98, Persistence: 0.51, Spline: defaultSpline},
			Erosion:         types.ControlField{Amp: 1.30, Freq: 0.69, Octaves: 3, Lacunarity: 1.65, Persistence: 0.68, Spline: defaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.51, Freq: 2.75, Octaves: 5, Lacunarity: 2.90, Persistence: 0.74, Spline: defaultSpline},
			Temperature:     types.ControlField{Amp: 1.52, Freq: 4.73, Octaves: 4, Lacunarity: 1.66, Persistence: 0.36, Spline: defaultSpline},
			Humidity:        types.ControlField{Amp: 1.31, Freq: 2.34, Octaves: 2, Lacunarity: 1.55, Persistence: 0.62, Spline: defaultSpline},
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
		LUT: "super_terran",
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
		CraterCount:      15,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.03,
		CraterDepth:      0.1,
			LUT:              "hothouse",
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
		CraterCount:      20,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.04,
		CraterDepth:      0.15,
		OceanLevel:       0.52,
		OceanColor:       rgba(220, 80, 5),
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{
				Amp: 1.13, Freq: 3.82, Octaves: 6, Lacunarity: 2.29, Persistence: 0.56,
				Spline: planetcolor.Spline{Knots: []planetcolor.SplineKnot{
					{Input: 0, Output: 0},
					{Input: 0.4, Output: 0.05},
					{Input: 0.5, Output: 0.45},
					{Input: 0.7, Output: 0.55},
					{Input: 1, Output: 0.6},
				}},
			},
			Erosion: types.ControlField{
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
		LUT: "lava_world",
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
		PolarCapSize:     0.20,
		PolarCapNoise:    0.11,
		OceanLevel:       0.77,
		OceanColor:       rgba(30, 60, 140),
		SnowLine:         0.74,
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
			Continentalness: types.ControlField{Amp: 1.83, Freq: 5.85, Octaves: 4, Lacunarity: 2.59, Persistence: 0.68, Spline: defaultSpline},
			Erosion:         types.ControlField{Amp: 1.29, Freq: 4.59, Octaves: 4, Lacunarity: 1.80, Persistence: 0.73, Spline: defaultSpline},
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
		LUT: "oceanic",
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
			LUT:              "jovian",
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
			LUT:              "ice_giant",
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
