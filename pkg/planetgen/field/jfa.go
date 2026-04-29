package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// jfaSeed represents a candidate ocean pixel for JFA propagation.
type jfaSeed struct {
	face       int8
	px, py     int16
	dirX, dirY, dirZ float32
}

const noJFASeed = int8(-1)

// DistanceToCoast returns a cube-map field where each pixel holds the
// great-circle angular distance to the nearest below-threshold pixel
// in heightmap, divided by π (so values fall in [0, 1]).
//
// The algorithm uses Jump Flooding with step sizes from S/2 down to 1,
// propagating seed positions to neighbors. Cross-face connectivity is
// automatic via cubemap.CubeMapF.Sample at seams.
func DistanceToCoast(heightmap *cubemap.CubeMapF, threshold float64, S int) *cubemap.CubeMapF {
	seeds := make([][]jfaSeed, cubemap.NumFaces)
	dists := cubemap.NewF(S)

	// Initialize: mark ocean pixels as seeds, all others as max distance.
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		seeds[face] = make([]jfaSeed, S*S)
		for i := range seeds[face] {
			seeds[face][i].face = noJFASeed
		}
		for py := range S {
			for px := range S {
				if heightmap.Get(face, px, py) < threshold {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					seeds[face][py*S+px] = jfaSeed{
						face: int8(face),
						px:   int16(px),
						py:   int16(py),
						dirX: float32(dx),
						dirY: float32(dy),
						dirZ: float32(dz),
					}
					dists.Set(face, px, py, 0)
				} else {
					dists.Set(face, px, py, 1.0)
				}
			}
		}
	}

	// Propagate seeds at decreasing step sizes.
	propagate := func(step int) {
		next := make([][]jfaSeed, cubemap.NumFaces)
		for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
			next[face] = make([]jfaSeed, S*S)
			copy(next[face], seeds[face])

			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					best := next[face][py*S+px]
					bestDist := dists.Get(face, px, py)

					// Check 8 neighbors at step offset.
					for _, off := range [8][2]int{
						{-step, -step}, {0, -step}, {step, -step},
						{-step, 0}, {step, 0},
						{-step, step}, {0, step}, {step, step},
					} {
						nx, ny := px+off[0], py+off[1]
						if nx < 0 || nx >= S || ny < 0 || ny >= S {
							continue
						}
						cand := seeds[face][ny*S+nx]
						if cand.face == noJFASeed {
							continue
						}

						// Great-circle distance: arccos of dot product, divided by π.
						cosA := dx*float64(cand.dirX) + dy*float64(cand.dirY) + dz*float64(cand.dirZ)
						if cosA > 1 {
							cosA = 1
						}
						if cosA < -1 {
							cosA = -1
						}
						ang := math.Acos(cosA) / math.Pi
						if ang < bestDist {
							best = cand
							bestDist = ang
						}
					}

					next[face][py*S+px] = best
					dists.Set(face, px, py, bestDist)
				}
			}
		}
		seeds = next
	}

	// Run JFA: step sizes S/2, S/4, ..., 2, 1, plus one final pass at 1.
	for step := S / 2; step >= 1; step /= 2 {
		propagate(step)
	}
	propagate(1)

	return dists
}
