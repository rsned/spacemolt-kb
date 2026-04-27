package noise

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Warper applies a single Quilez domain-warp pass to 3D points.
type Warper struct {
	cfg          types.WarpConfig
	xGen, yGen, zGen *Generator
}

// NewWarper builds a Warper seeded by master via the warp.{x,y,z}
// named domains. Same master always produces the same warp.
func NewWarper(master int64, cfg types.WarpConfig) *Warper {
	return &Warper{
		cfg:  cfg,
		xGen: New(seed.Domain(master, "warp.x")),
		yGen: New(seed.Domain(master, "warp.y")),
		zGen: New(seed.Domain(master, "warp.z")),
	}
}

// Warp returns p + amp · vec3(fbm_x(p), fbm_y(p), fbm_z(p)). The
// returned point is generally NOT unit-length; callers that require
// unit-sphere directions must re-normalize.
func (w *Warper) Warp(x, y, z float64) (float64, float64, float64) {
	if w.cfg.Amp == 0 {
		return x, y, z
	}
	dx := w.xGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	dy := w.yGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	dz := w.zGen.FractalNoise3D(x, y, z, w.cfg.Octaves, w.cfg.Lacunarity, w.cfg.Persistence, w.cfg.Freq)
	return x + w.cfg.Amp*(2*dx-1), y + w.cfg.Amp*(2*dy-1), z + w.cfg.Amp*(2*dz-1)
}
