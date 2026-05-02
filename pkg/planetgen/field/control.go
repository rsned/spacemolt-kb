package field

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// controlFieldDomains lists the seed-domain name for each control
// field, in the same order as ControlConfig fields.
var controlFieldDomains = [5]string{
	"control.continentalness",
	"control.erosion",
	"control.peaks-valleys",
	"biome.temperature",
	"biome.humidity",
}

// GenerateControlFields produces five 3D-fBm cube-map fields, one per
// control field in cfg. Each is seeded by master XOR fnv64a(domain),
// so adding a new control field never shifts existing field outputs.
//
// When jitter is non-nil, the Detail field (index 1) samples its fBm
// through the jittered direction returned by jitter.TransformPixel,
// breaking visible repetition across Voronoi cell boundaries. Pass nil
// to use the unmodified pixel direction for all fields.
//
// Output values are normalized to [0, 1] per the noise.Generator
// convention.
func GenerateControlFields(master int64, cfg types.ControlConfig, S int, jitter *noise.JitterField) [5]*cubemap.CubeMapF {
	fieldsCfg := [5]types.ControlField{
		cfg.Continentalness,
		cfg.Detail,
		cfg.PeaksValleys,
		cfg.Temperature,
		cfg.Humidity,
	}
	out := [5]*cubemap.CubeMapF{}
	for i := range out {
		fc := fieldsCfg[i]
		ng := noise.New(seed.Domain(master, controlFieldDomains[i]))
		out[i] = cubemap.NewF(S)
		for face := range cubemap.Face(cubemap.NumFaces) {
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					if i == 1 && jitter != nil {
						dx, dy, dz = jitter.TransformPixel(face, px, py, dx, dy, dz)
					}
					out[i].Set(face, px, py,
						ng.FractalNoise3D(dx, dy, dz, fc.Octaves, fc.Lacunarity, fc.Persistence, fc.Freq)*fc.Amp)
				}
			}
		}
	}
	return out
}
