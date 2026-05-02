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
// propagating seed positions to neighbors within each cube face.
// Propagation is face-local — distances do not cross face seams.
// See JumpFloodFromMask for the same constraint when seeding from a
// boolean mask.
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

	propagateJFA(seeds, dists, S)
	return dists
}

// propagateJFA runs Jump Flooding propagation steps S/2, S/4, ..., 1
// plus one final pass at step=1. Each pixel takes the smallest
// great-circle angular distance (in [0,1], divided by π) to any seed
// reachable through 8-neighbor jumps WITHIN the same face. Does not
// propagate across face seams.
//
// seeds is mutated in place. dists is mutated in place. Both are
// allocated and partially initialized by the caller (seeds: seed
// pixels marked, others have face=noJFASeed; dists: 0 at seeds,
// 1.0 elsewhere).
func propagateJFA(seeds [][]jfaSeed, dists *cubemap.CubeMapF, S int) {
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
		// copy next back into seeds
		for face := range seeds {
			seeds[face] = next[face]
		}
	}

	for step := S / 2; step >= 1; step /= 2 {
		propagate(step)
	}
	propagate(1)
}

// JumpFloodFromMask runs JFA over a cube map starting from every
// pixel where mask[face][py*S+px] is true. Returns a CubeMapF where
// each pixel holds the great-circle angular distance (normalized to
// [0, 1] by dividing by π) to the nearest seed pixel on the same
// face. Pixels in a face with no seeds get 1.0.
//
// JFA propagation is face-local — cross-face seams are not bridged.
// This is the same constraint as DistanceToCoast.
func JumpFloodFromMask(mask [cubemap.NumFaces][]bool, S int) *cubemap.CubeMapF {
	seeds := make([][]jfaSeed, cubemap.NumFaces)
	dists := cubemap.NewF(S)
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		seeds[face] = make([]jfaSeed, S*S)
		for i := range seeds[face] {
			seeds[face][i].face = noJFASeed
		}
		for py := range S {
			for px := range S {
				idx := py*S + px
				if mask[face][idx] {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					seeds[face][idx] = jfaSeed{
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
	propagateJFA(seeds, dists, S)
	return dists
}
