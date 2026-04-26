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
}
