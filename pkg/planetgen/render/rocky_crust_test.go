package render

import (
	"math"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// The render package cannot import the root planetgen package (import
// cycle: planetgen → render), so the height-relevant terran fields are
// copied inline from pkg/planetgen/profile.go's terran entry. Palettes,
// biome tables, erosion, flow, and craters are omitted — this test only
// measures heights and the derived ocean level.
var (
	crustTestDefaultSpline = planetcolor.Spline{Knots: []planetcolor.SplineKnot{
		{Input: 0, Output: 0},
		{Input: 1, Output: 0.5},
	}}
	crustTestShelfSpline = planetcolor.Spline{Knots: []planetcolor.SplineKnot{
		{Input: 0.0, Output: 0.0},
		{Input: 0.4, Output: 0.05},
		{Input: 0.5, Output: 0.45},
		{Input: 0.7, Output: 0.55},
		{Input: 1.0, Output: 0.6},
	}}
)

// terranHeightProfile returns a fresh terran-like profile covering the
// height pipeline only; each call returns a new value so tests can
// mutate it freely.
func terranHeightProfile() *types.PlanetProfile {
	return &types.PlanetProfile{
		Type: "terran",
		ControlConfig: types.ControlConfig{
			Continentalness: types.ControlField{Amp: 0.97, Freq: 2.02, Octaves: 5, Lacunarity: 2.12, Persistence: 0.51, Spline: crustTestShelfSpline},
			Detail:          types.ControlField{Amp: 0.55, Freq: 2.95, Octaves: 3, Lacunarity: 2.48, Persistence: 0.67, Spline: crustTestDefaultSpline},
			PeaksValleys:    types.ControlField{Amp: 0.96, Freq: 2.09, Octaves: 5, Lacunarity: 2.5, Persistence: 0.72, Spline: crustTestDefaultSpline},
			Temperature:     types.ControlField{Amp: 1.71, Freq: 1.27, Octaves: 4, Lacunarity: 1.89, Persistence: 0.44, Spline: crustTestDefaultSpline},
			Humidity:        types.ControlField{Amp: 1.38, Freq: 5.81, Octaves: 4, Lacunarity: 2.83, Persistence: 0.73, Spline: crustTestDefaultSpline},
		},
		Ridged: types.RidgedConfig{
			Amp: 0.20, Freq: 1.21, Octaves: 6, Lacunarity: 2.09, Gain: 0.53, Offset: 0.98,
			MaskLow: 0.45, MaskHigh: 0.80,
			PlateConvergentScaleKm: 800,
		},
		OceanLevel:           0.50,
		HeightSmoothRadius:   4,
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
	}
}

// crustTerran returns the terran height profile with the Phase 12
// crust stage force-enabled and pinned parameters for assertions.
func crustTerran() *types.PlanetProfile {
	p := terranHeightProfile()
	p.Crust = types.CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        0.5, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: 0.3, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: 0.5, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
	p.TectonicFX = types.TectonicFXConfig{
		BeltAmp: 0.30, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.10, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
	return p
}

// renderHeightmapForTest mirrors RenderRocky's setup (jitter + plates)
// and calls the internal heightmap pipeline directly.
func renderHeightmapForTest(p *types.PlanetProfile, master int64, S int) (*cubemap.CubeMapF, []feature.Crater, float64) {
	jitter := noise.GenerateJitter(p, master, S)
	plates := field.GeneratePlates(p, master, S)
	return generateRockyHeightmapWithJitter(p, master, S, jitter, plates)
}

func TestCrustPathLandFractionMatchesTarget(t *testing.T) {
	const S = 128
	p := crustTerran()
	for _, master := range []int64{1, 42, 31337} {
		hm, _, lvl := renderHeightmapForTest(p, master, S)
		var below, total int
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if h < lvl {
					below++
				}
				total++
			}
		}
		ocean := float64(below) / float64(total)
		if math.Abs(ocean-0.7) > 0.03 {
			t.Errorf("seed %d: ocean fraction %v, want 0.70 ± 0.03", master, ocean)
		}
	}
}

func TestLegacyPathOceanLevelPassthrough(t *testing.T) {
	const S = 64
	p := terranHeightProfile() // Crust zero-value → legacy
	_, _, lvl := renderHeightmapForTest(p, 42, S)
	if lvl != p.OceanLevel {
		t.Errorf("legacy ocean level %v, want profile value %v", lvl, p.OceanLevel)
	}
}
