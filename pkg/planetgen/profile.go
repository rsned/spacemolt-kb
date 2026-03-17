package planetgen

import "image/color"

// PlanetProfile defines the visual parameters for a planet type.
type PlanetProfile struct {
	Type             string
	Renderer         string // "rocky" or "gas_giant"
	Palette          []ColorStop
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
	EquatorialPalette []ColorStop // If set, blended in near equator
	PolarPalette      []ColorStop // If set, blended in near poles (before ice caps)

	// Gas giant specific
	BandCount      int
	TurbulenceAmp  float64 // How much bands are distorted
	BandBlendWidth float64 // Fraction of band width to blend (0 = default 0.2)
	StormCount     int
	StormSize      float64 // Angular radius of storm ovals
}

func rgba(r, g, b uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: 255}
}

// Profiles contains the visual parameters for all planet types.
var Profiles = map[string]*PlanetProfile{
	"scorched": {
		Type:     "scorched",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"arid": {
		Type:     "arid",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"terran": {
		Type:     "terran",
		Renderer: "rocky",
		Palette: []ColorStop{
			{0.0, rgba(40, 100, 40)},    // Lowland green
			{0.2, rgba(30, 85, 30)},     // Dark forest
			{0.4, rgba(55, 110, 45)},    // Forest green
			{0.6, rgba(100, 130, 70)},   // Light woodland
			{0.7, rgba(130, 120, 80)},   // Brown foothills
			{0.8, rgba(150, 140, 120)},  // Rocky highlands
			{0.9, rgba(170, 165, 155)},  // Gray peaks
			{1.0, rgba(200, 200, 195)},  // Bare rock
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.55,
		NoiseScale:       2.5,
		CraterCount:      10,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.02,
		CraterDepth:      0.08,
		HasPolarCaps:     true,
		PolarCapSize:     0.15,
		OceanLevel:       0.55,
		OceanColor:       rgba(30, 60, 140),
		SnowLine:         0.85,
		EquatorialPalette: []ColorStop{
			{0.0, rgba(160, 140, 90)},   // Sandy lowland
			{0.2, rgba(180, 155, 100)},  // Desert sand
			{0.4, rgba(170, 145, 85)},   // Dry savanna
			{0.6, rgba(140, 120, 70)},   // Arid scrub
			{0.7, rgba(130, 115, 75)},   // Dry hills
			{0.8, rgba(150, 140, 120)},  // Rocky
			{0.9, rgba(170, 165, 155)},  // Gray peaks
			{1.0, rgba(200, 200, 195)},  // Bare rock
		},
		PolarPalette: []ColorStop{
			{0.0, rgba(70, 100, 65)},    // Cold scrub
			{0.2, rgba(90, 110, 80)},    // Boreal green
			{0.4, rgba(110, 120, 90)},   // Taiga
			{0.6, rgba(140, 140, 120)},  // Tundra brown
			{0.7, rgba(160, 155, 145)},  // Rocky tundra
			{0.8, rgba(180, 175, 170)},  // Frost rock
			{0.9, rgba(200, 198, 195)},  // Icy peaks
			{1.0, rgba(225, 225, 222)},  // Snow
		},
	},
	"tundra": {
		Type:     "tundra",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"glacial": {
		Type:     "glacial",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"ice_world": {
		Type:     "ice_world",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"super_terran": {
		Type:     "super_terran",
		Renderer: "rocky",
		Palette: []ColorStop{
			{0.0, rgba(25, 80, 30)},     // Deep green
			{0.2, rgba(20, 70, 25)},     // Dark forest
			{0.4, rgba(45, 100, 40)},    // Forest
			{0.6, rgba(80, 115, 55)},    // Light woodland
			{0.7, rgba(110, 110, 70)},   // Brown foothills
			{0.8, rgba(140, 130, 110)},  // Rocky
			{0.9, rgba(170, 165, 155)},  // Gray peaks
			{1.0, rgba(200, 200, 195)},  // Bare rock
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.55,
		NoiseScale:       2.5,
		CraterCount:      10,
		CraterMinRadius:  0.003,
		CraterMaxRadius:  0.02,
		CraterDepth:      0.08,
		HasPolarCaps:     true,
		PolarCapSize:     0.1,
		OceanLevel:       0.50,
		OceanColor:       rgba(25, 50, 120),
		SnowLine:         0.88,
		EquatorialPalette: []ColorStop{
			{0.0, rgba(140, 125, 75)},
			{0.2, rgba(160, 140, 85)},
			{0.4, rgba(145, 130, 70)},
			{0.6, rgba(120, 110, 65)},
			{0.7, rgba(115, 105, 70)},
			{0.8, rgba(140, 130, 110)},
			{0.9, rgba(170, 165, 155)},
			{1.0, rgba(200, 200, 195)},
		},
		PolarPalette: []ColorStop{
			{0.0, rgba(60, 90, 55)},
			{0.2, rgba(75, 100, 70)},
			{0.4, rgba(100, 115, 85)},
			{0.6, rgba(130, 130, 110)},
			{0.7, rgba(155, 150, 140)},
			{0.8, rgba(175, 172, 168)},
			{0.9, rgba(195, 195, 192)},
			{1.0, rgba(220, 220, 218)},
		},
	},
	"hothouse": {
		Type:     "hothouse",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
	"lava_world": {
		Type:     "lava_world",
		Renderer: "rocky",
		Palette: []ColorStop{
			{0.0, rgba(20, 15, 10)},     // Dark basalt
			{0.3, rgba(40, 30, 20)},     // Dark rock
			{0.45, rgba(60, 40, 25)},    // Brown rock
			{0.55, rgba(180, 60, 10)},   // Orange lava glow
			{0.65, rgba(255, 100, 0)},   // Bright lava
			{0.75, rgba(200, 50, 5)},    // Cooling lava
			{0.85, rgba(50, 35, 25)},    // Cooled crust
			{1.0, rgba(30, 20, 15)},     // Dark peaks
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.2,
		NoisePersistence: 0.55,
		NoiseScale:       3.0,
		CraterCount:      20,
		CraterMinRadius:  0.004,
		CraterMaxRadius:  0.04,
		CraterDepth:      0.15,
	},
	"oceanic": {
		Type:     "oceanic",
		Renderer: "rocky",
		Palette: []ColorStop{
			{0.0, rgba(80, 150, 80)},    // Low islands
			{0.3, rgba(100, 160, 90)},   // Green land
			{0.5, rgba(140, 155, 100)},  // Sandy highlands
			{0.7, rgba(170, 170, 130)},  // Sandy peaks
			{1.0, rgba(210, 210, 190)},  // Mountain peaks
		},
		NoiseOctaves:     8,
		NoiseLacunarity:  2.0,
		NoisePersistence: 0.55,
		NoiseScale:       2.5,
		CraterCount:      0,
		HasPolarCaps:     true,
		PolarCapSize:     0.08,
		OceanLevel:       0.65,
		OceanColor:       rgba(15, 45, 120),
	},

	// Gas giants
	"jovian": {
		Type:     "jovian",
		Renderer: "gas_giant",
		Palette: []ColorStop{
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
	},
	"ice_giant": {
		Type:     "ice_giant",
		Renderer: "gas_giant",
		Palette: []ColorStop{
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
	},

	// Unknown/unclassified planets — neutral gray rocky appearance
	"unknown": {
		Type:     "unknown",
		Renderer: "rocky",
		Palette: []ColorStop{
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
	},
}

// GetProfile returns the planet profile for the given type.
// Returns the "unknown" profile for unrecognized types.
func GetProfile(planetType string) *PlanetProfile {
	if p, ok := Profiles[planetType]; ok {
		return p
	}
	return Profiles["unknown"]
}
