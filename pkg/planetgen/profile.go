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
			{0.0, rgba(30, 80, 30)},     // Deep lowland green
			{0.3, rgba(60, 120, 50)},     // Forest green
			{0.5, rgba(100, 140, 70)},    // Light green
			{0.65, rgba(140, 130, 90)},   // Brown highlands
			{0.8, rgba(160, 150, 130)},   // Rocky peaks
			{1.0, rgba(230, 230, 230)},   // Snow peaks
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
			{0.0, rgba(20, 70, 25)},
			{0.3, rgba(40, 100, 40)},
			{0.5, rgba(70, 120, 55)},
			{0.65, rgba(110, 110, 70)},
			{0.8, rgba(140, 130, 110)},
			{1.0, rgba(210, 210, 210)},
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
}

// GetProfile returns the planet profile for the given type, or nil if unknown.
func GetProfile(planetType string) *PlanetProfile {
	return Profiles[planetType]
}
