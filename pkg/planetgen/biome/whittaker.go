package biome

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateClimateFields produces temperature (T) and moisture (M) cube
// maps from a planet's profile. Phase 1 uses:
//
//	T(p) = temperatureField(p) * 0.7 + latBias(p) * 0.3
//	M(p) = humidityField(p)
//
// Both are normalized to [0, 1].
func GenerateClimateFields(seed int64, profile *types.PlanetProfile, S int) (T, M *cubemap.CubeMapF) {
	fields := field.GenerateControlFields(seed, profile.ControlConfig, S)
	tNoise := fields[3]
	mNoise := fields[4]

	T = cubemap.NewF(S)
	M = cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				_, dy, _ := cubemap.FacePixelToDir(face, px, py, S)
				lat := math.Asin(dy)
				latBias := 0.5 + 0.5*math.Cos(lat)*0.6 // [0.2, 0.8] at poles vs equator
				t := tNoise.Get(face, px, py)*0.7 + latBias*0.3
				if t < 0 {
					t = 0
				} else if t > 1 {
					t = 1
				}
				T.Set(face, px, py, t)
				M.Set(face, px, py, mNoise.Get(face, px, py))
			}
		}
	}
	return T, M
}
