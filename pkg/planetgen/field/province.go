package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	pgseed "github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// GenerateProvinces returns three cube-map fields capturing per-region
// roughness modulation:
//
//	id    integer cell membership encoded as float in [0, Count)
//	ramp  per-cell amp modulator in [1-Jitter, 1+Jitter]
//	rfreq per-cell freq modulator in [1-Jitter, 1+Jitter]
//
// Cells are seeded from a Fibonacci spiral on the unit sphere; a per-pixel
// fBm warp can shift the cell-lookup direction so cell boundaries curve.
//
// Returns (nil, nil, nil) when cfg.Count <= 0.
func GenerateProvinces(masterSeed int64, cfg types.ProvinceConfig, S int) (id, ramp, rfreq *cubemap.CubeMapF) {
	if cfg.Count <= 0 {
		return nil, nil, nil
	}
	centers := fibonacciSphere(cfg.Count)
	jitterAmp := make([]float64, cfg.Count)
	jitterFreq := make([]float64, cfg.Count)
	jitterRng := rand.New(rand.NewPCG(uint64(pgseed.Domain(masterSeed, "province.jitter")), 0xDEADBEEF))
	for i := range cfg.Count {
		jitterAmp[i] = 1.0 + (jitterRng.Float64()*2-1)*cfg.Jitter
		jitterFreq[i] = 1.0 + (jitterRng.Float64()*2-1)*cfg.Jitter
	}

	id = cubemap.NewF(S)
	ramp = cubemap.NewF(S)
	rfreq = cubemap.NewF(S)

	var warpGen *noise.Generator
	if cfg.WarpAmp > 0 {
		warpGen = noise.New(pgseed.Domain(masterSeed, "province.warp"))
	}

	for face := range cubemap.Face(cubemap.NumFaces) {
		for y := range S {
			for x := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, x, y, S)
				if warpGen != nil {
					wx := warpGen.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 1.5)*2 - 1
					wy := warpGen.FractalNoise3D(dx+11, dy+7, dz+13, 3, 2.0, 0.5, 1.5)*2 - 1
					wz := warpGen.FractalNoise3D(dx-9, dy-3, dz-5, 3, 2.0, 0.5, 1.5)*2 - 1
					dx += wx * cfg.WarpAmp
					dy += wy * cfg.WarpAmp
					dz += wz * cfg.WarpAmp
					n := math.Sqrt(dx*dx + dy*dy + dz*dz)
					if n > 0 {
						dx, dy, dz = dx/n, dy/n, dz/n
					}
				}
				bestI, bestD := 0, -2.0
				for i, c := range centers {
					d := dx*c[0] + dy*c[1] + dz*c[2]
					if d > bestD {
						bestD = d
						bestI = i
					}
				}
				id.Set(face, x, y, float64(bestI))
				ramp.Set(face, x, y, jitterAmp[bestI])
				rfreq.Set(face, x, y, jitterFreq[bestI])
			}
		}
	}
	return id, ramp, rfreq
}

// fibonacciSphere returns n points roughly evenly distributed on the unit sphere.
func fibonacciSphere(n int) [][3]float64 {
	out := make([][3]float64, n)
	if n <= 0 {
		return out
	}
	if n == 1 {
		out[0] = [3]float64{0, 1, 0}
		return out
	}
	phi := math.Pi * (math.Sqrt(5) - 1)
	for i := range n {
		y := 1 - float64(2*i)/float64(n-1)
		r := math.Sqrt(1 - y*y)
		theta := float64(i) * phi
		out[i] = [3]float64{math.Cos(theta) * r, y, math.Sin(theta) * r}
	}
	return out
}
