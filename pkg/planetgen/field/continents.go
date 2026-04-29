package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateContinents returns a cube-map field where each pixel holds
// the base height of its nearest Fibonacci-spiral continent seed, with
// per-seed heights drawn uniformly from [HeightLo, HeightHi]. Seed
// positions are warped by a low-frequency fbm if cfg.WarpAmp > 0.
func GenerateContinents(masterSeed int64, cfg types.ContinentConfig, S int) *cubemap.CubeMapF {
	out := cubemap.NewF(S)
	if cfg.Seeds <= 0 {
		return out
	}
	rng := rand.New(rand.NewPCG(uint64(seed.Domain(masterSeed, "continents.height")),
		uint64(seed.Domain(masterSeed, "continents.height")^0x5a5a5a5a))) //nolint:gosec
	centers := make([][3]float64, cfg.Seeds)
	heights := make([]float64, cfg.Seeds)
	phi := math.Pi * (3 - math.Sqrt(5))
	for i := range cfg.Seeds {
		y := 1 - 2*float64(i)/float64(cfg.Seeds-1)
		if cfg.Seeds == 1 {
			y = 0
		}
		r := math.Sqrt(1 - y*y)
		theta := phi * float64(i)
		centers[i] = [3]float64{math.Cos(theta) * r, y, math.Sin(theta) * r}
		heights[i] = cfg.HeightLo + rng.Float64()*(cfg.HeightHi-cfg.HeightLo)
	}
	var warp *noise.Generator
	if cfg.WarpAmp > 0 {
		warp = noise.New(seed.Domain(masterSeed, "continents.warp"))
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				if warp != nil {
					dx += cfg.WarpAmp * (warp.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
					dy += cfg.WarpAmp * (warp.FractalNoise3D(dy, dz, dx, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
					dz += cfg.WarpAmp * (warp.FractalNoise3D(dz, dx, dy, 3, 2.0, 0.5, cfg.WarpFreq) - 0.5)
					n := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if n > 0 {
						dx, dy, dz = dx/n, dy/n, dz/n
					}
				}
				bestIdx, bestDot := 0, -2.0
				for i, c := range centers {
					d := dx*c[0] + dy*c[1] + dz*c[2]
					if d > bestDot {
						bestDot = d
						bestIdx = i
					}
				}
				out.Set(face, px, py, heights[bestIdx])
			}
		}
	}
	return out
}
