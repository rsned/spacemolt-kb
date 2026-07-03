package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// SmoothHeightmap applies a per-face disc blur of radius r pixels with
// 1/(1+d) falloff weights to heightmap. Edge pixels clamp at face
// boundaries (no cross-face seam handling — small inconsistencies at
// face seams are visually negligible at small r). r=0 returns
// heightmap unchanged.
func SmoothHeightmap(heightmap *cubemap.CubeMapF, r int, S int) *cubemap.CubeMapF {
	if r <= 0 {
		return heightmap
	}
	// Pre-compute kernel weights for offsets in [-r, r] x [-r, r]; weights are
	// normalized so a fully-in-face kernel sums to 1.
	side := 2*r + 1
	weights := make([]float64, side*side)
	sum := 0.0
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			d := math.Sqrt(float64(dx*dx + dy*dy))
			w := 1.0 / (1.0 + d)
			weights[(dy+r)*side+(dx+r)] = w
			sum += w
		}
	}
	for i := range weights {
		weights[i] /= sum
	}
	out := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				acc := 0.0
				wsum := 0.0 // re-normalize per pixel since edge-clipped kernels lose mass
				for dy := -r; dy <= r; dy++ {
					iy := py + dy
					if iy < 0 || iy >= S {
						continue
					}
					for dx := -r; dx <= r; dx++ {
						ix := px + dx
						if ix < 0 || ix >= S {
							continue
						}
						w := weights[(dy+r)*side+(dx+r)]
						acc += heightmap.Get(face, ix, iy) * w
						wsum += w
					}
				}
				if wsum > 0 {
					out.Set(face, px, py, acc/wsum)
				}
			}
		}
	}
	return out
}
