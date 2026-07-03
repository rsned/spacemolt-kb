package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// jfaSeed represents a candidate seed pixel for JFA propagation.
// value is an arbitrary per-seed scalar (e.g. boundary magnitude) that
// rides along with the seed as it propagates so every interior pixel
// ends up knowing both its distance to and the value at its nearest
// seed. JumpFloodFromMask leaves value = 0; JumpFloodFromMaskWithValue
// initializes it from a parallel float field.
type jfaSeed struct {
	face             int8
	px, py           int16
	dirX, dirY, dirZ float32
	value            float32
}

const noJFASeed = int8(-1)

// DistanceToCoast returns a cube-map field where each pixel holds the
// great-circle angular distance to the nearest below-threshold pixel
// in heightmap, divided by π (so values fall in [0, 1]).
//
// The algorithm uses Jump Flooding with step sizes from S/2 down to 1,
// propagating seed positions to neighbors. Propagation is cross-face
// aware: the 8-neighbor lookup uses cubemap.OffsetPixel to project
// across face seams when an offset leaves the current face's bounds,
// and the great-circle metric uses each seed's stored unit direction
// so distances are correct regardless of which face holds the seed.
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
// reachable through 8-neighbor jumps. Cross-face neighbors are
// resolved via cubemap.OffsetPixel — propagation traverses cube
// seams, so a face with no local seeds inherits distances from its
// neighbors instead of holding the 1.0 sentinel.
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
						nf, nx, ny := cubemap.OffsetPixel(face, px, py, off[0], off[1], S)
						cand := seeds[nf][ny*S+nx]
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

	// First descending sweep: step = S/2, S/4, ..., 1.
	for step := S / 2; step >= 1; step /= 2 {
		propagate(step)
	}
	// Second descending sweep ("JFA²"): standard JFA assumes uniform
	// 8-neighbor reachability at every step, but on a cube map an
	// interior pixel of a seedless face can only cross a face seam at
	// the largest-step pass — subsequent in-face sweeps need the
	// seed to travel inward from edge pixels. A second full sweep
	// gives all pixels another chance to refine through the now-
	// populated cross-face neighbor faces.
	for step := S / 2; step >= 1; step /= 2 {
		propagate(step)
	}
	propagate(1)
}

// JumpFloodFromMask runs JFA over a cube map starting from every
// pixel where mask[face][py*S+px] is true. Returns a CubeMapF where
// each pixel holds the great-circle angular distance (normalized to
// [0, 1] by dividing by π) to the nearest seed pixel anywhere on the
// cube. JFA propagation is cross-face aware (see propagateJFA), so
// a face with no local seeds still receives finite distances from
// neighbors. Pixels only retain the 1.0 sentinel when the entire
// cube has no seeds.
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

// JumpFloodFromMaskWithValue is JumpFloodFromMask plus per-seed value
// propagation. value[face][i] is read at every mask pixel and stored
// on its jfaSeed; after propagation, every output pixel's seed holds
// the value of its nearest source. Returns the distance field (same
// semantics as JumpFloodFromMask) and a parallel float field where
// each pixel = value of the nearest seed (0 for any pixel whose face
// remained unreachable, same sentinel convention as the distance
// field's 1.0).
func JumpFloodFromMaskWithValue(
	mask [cubemap.NumFaces][]bool,
	value [cubemap.NumFaces][]float64,
	S int,
) (dist, propagatedValue *cubemap.CubeMapF) {
	seeds := make([][]jfaSeed, cubemap.NumFaces)
	dist = cubemap.NewF(S)
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
						face:  int8(face),
						px:    int16(px),
						py:    int16(py),
						dirX:  float32(dx),
						dirY:  float32(dy),
						dirZ:  float32(dz),
						value: float32(value[face][idx]),
					}
					dist.Set(face, px, py, 0)
				} else {
					dist.Set(face, px, py, 1.0)
				}
			}
		}
	}
	propagateJFA(seeds, dist, S)
	propagatedValue = cubemap.NewF(S)
	for face := range seeds {
		for i := range seeds[face] {
			if seeds[face][i].face != noJFASeed {
				propagatedValue.Faces[face][i] = float64(seeds[face][i].value)
			}
		}
	}
	return dist, propagatedValue
}
