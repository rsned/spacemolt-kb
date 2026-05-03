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
//
// This is the legacy entry point: it picks Fibonacci-spiral seeds based
// on the masterSeed alone. New callers should prefer
// GenerateContinentsFromSeeds when a plate-derived seed list is
// available (Phase 8 item 10).
func GenerateContinents(masterSeed int64, cfg types.ContinentConfig, S int) *cubemap.CubeMapF {
	return GenerateContinentsFromSeeds(masterSeed, cfg, S, nil)
}

// GenerateContinentsFromSeeds is the seed-list-aware entry point for
// the Voronoi continents pass.
//
// When seeds is nil or empty, behaves identically (byte-equal) to the
// legacy GenerateContinents: cfg.Seeds Fibonacci-spiral centers.
//
// When seeds is non-empty:
//   - The first min(len(seeds), cfg.Seeds) Voronoi cell centers come
//     from the supplied directions (caller is responsible for ordering
//     determinism).
//   - If len(seeds) < cfg.Seeds, the residual cfg.Seeds-len(seeds)
//     centers are filled from the same Fibonacci-spiral sequence used
//     by the legacy code, starting at index len(seeds). The supplied
//     directions are NOT renormalised.
//
// The per-seed-height RNG always draws exactly cfg.Seeds samples in
// the same domain as the legacy path, so the seeds==nil call path is
// byte-equal to the legacy GenerateContinents.
//
// Phase 8 item 10: callers (rocky pipeline) pass continental plate
// centroids as the seed list so continents anchor to plate geometry
// instead of an independent random spiral.
func GenerateContinentsFromSeeds(masterSeed int64, cfg types.ContinentConfig, S int, seeds [][3]float64) *cubemap.CubeMapF {
	out := cubemap.NewF(S)
	if cfg.Seeds <= 0 {
		return out
	}
	rng := rand.New(rand.NewPCG(uint64(seed.Domain(masterSeed, "continents.height")),
		uint64(seed.Domain(masterSeed, "continents.height")^0x5a5a5a5a))) //nolint:gosec
	centers := make([][3]float64, cfg.Seeds)
	heights := make([]float64, cfg.Seeds)
	phi := math.Pi * (3 - math.Sqrt(5))
	nProvided := len(seeds)
	if nProvided > cfg.Seeds {
		nProvided = cfg.Seeds
	}
	for i := range cfg.Seeds {
		if i < nProvided {
			centers[i] = seeds[i]
		} else {
			y := 1 - 2*float64(i)/float64(cfg.Seeds-1)
			if cfg.Seeds == 1 {
				y = 0
			}
			r := math.Sqrt(1 - y*y)
			theta := phi * float64(i)
			centers[i] = [3]float64{math.Cos(theta) * r, y, math.Sin(theta) * r}
		}
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
