package field

import (
	"math"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// planchonDarbouxMaxIters caps the relaxation loop in case of a
// pathological heightmap (the standard sphere-closed case typically
// converges in 4-8 sweeps at S=512). Exceeding this is treated as a
// soft failure: we return whatever we have so the caller still gets
// a usable filled map.
const planchonDarbouxMaxIters = 32

// neighbor8Deltas mirrors the order in cubemap.FacePixelNeighbors8.
// Used by computeD8Pointers when scanning candidate neighbors so the
// stored int8 index can be looked up later via FacePixelNeighbors8
// to recover the downhill neighbor's address.
var neighbor8Deltas = [8][2]int{
	{1, 0}, {-1, 0}, {0, 1}, {0, -1},
	{1, 1}, {-1, 1}, {1, -1}, {-1, -1},
}

// FlowField holds D8 pointers + upstream flow accumulation + river mask
// derived from a heightmap by GenerateFlow. Faces are indexed by
// cubemap.Face; per-face slices are flat row-major Size*Size buffers.
type FlowField struct {
	Size int
	// D8 stores -1 for sinks (no strictly-lower 8-neighbor exists) and
	// otherwise an index in [0,8) that matches the order of
	// cubemap.FacePixelNeighbors8 — i.e. for downhill neighbor address
	// nbr := cubemap.FacePixelNeighbors8(face, px, py, S)[ff.D8[face][i]].
	D8 [cubemap.NumFaces][]int8
	// Accum is the upstream "rain count" per pixel: each pixel starts
	// with 1.0 and adds its accum to its D8-downhill neighbor.
	Accum [cubemap.NumFaces][]float64
	// Rivers is the river mask: Accum[face][i] >= cfg.RiverThreshold.
	Rivers [cubemap.NumFaces][]bool
}

// GenerateFlow runs Planchon-Darboux fill, D8 pointers, accumulation,
// and the river mask on the given heightmap. Returns nil when
// cfg.RiverThreshold <= 0 (rivers disabled).
//
// The fill is computed on a clone of heightmap; the input is not
// modified. Callers wishing to carve channels into the heightmap
// should call CarveRivers afterwards.
func GenerateFlow(heightmap *cubemap.CubeMapF, cfg types.FlowConfig) *FlowField {
	if cfg.RiverThreshold <= 0 {
		return nil
	}
	S := heightmap.Size
	filled := planchonDarbouxFill(heightmap)
	ff := &FlowField{Size: S}
	for f := range ff.D8 {
		ff.D8[f] = make([]int8, S*S)
		ff.Accum[f] = make([]float64, S*S)
		ff.Rivers[f] = make([]bool, S*S)
	}
	computeD8Pointers(filled, ff)
	computeAccumulation(filled, ff)
	for f := range ff.Rivers {
		for i, a := range ff.Accum[f] {
			ff.Rivers[f][i] = a >= cfg.RiverThreshold
		}
	}
	return ff
}

// CarveRivers writes river channels into heightmap by subtracting
// cfg.RiverDepth where ff.Rivers is set. Operates in place. ff may
// be nil — in that case the call is a no-op (matching the
// "FlowConfig.RiverThreshold == 0 → no rivers" gate).
//
// Idempotency: calling CarveRivers twice will subtract the depth
// twice. Callers that need to re-carve should regenerate the
// heightmap first.
func CarveRivers(heightmap *cubemap.CubeMapF, ff *FlowField, cfg types.FlowConfig) {
	if ff == nil {
		return
	}
	for f := range heightmap.Faces {
		mask := ff.Rivers[f]
		for i := range heightmap.Faces[f] {
			if mask[i] {
				heightmap.Faces[f][i] -= cfg.RiverDepth
			}
		}
	}
}

// planchonDarbouxFill returns a "filled" heightmap where local minima
// are raised to the lowest-saddle elevation. The classic Planchon-
// Darboux algorithm (Planchon & Darboux 2001, "A fast, simple and
// versatile algorithm to fill the depressions of digital elevation
// models") initializes filled = +∞ on the interior and = original on
// the open boundary, then sweeps
//
//	filled[p] = max(original[p], min over 8-neighbors of filled[n])
//
// until no change.
//
// On a closed sphere there is no open boundary, so we use the global
// minimum of the original heightmap as a single "drained" seed pixel
// (initialized to the original elevation there) and set every other
// pixel to MaxOriginal+1. The same relaxation rule then propagates
// the drainage outward: each pixel descends to the lowest level it
// can reach without crossing a saddle below its own elevation. This
// is the standard closed-manifold adaptation; pits are filled to
// their saddle elevation, the global minimum remains a sink (which
// computeD8Pointers marks as -1).
//
// Returns a fresh CubeMapF; the input is not mutated. Caps iterations
// at planchonDarbouxMaxIters to bound the worst case.
func planchonDarbouxFill(heightmap *cubemap.CubeMapF) *cubemap.CubeMapF {
	S := heightmap.Size

	// Find the global minimum to serve as the drained seed.
	var seedFace cubemap.Face
	seedPx, seedPy := 0, 0
	minH := math.Inf(1)
	maxH := math.Inf(-1)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				v := heightmap.Get(face, px, py)
				if v < minH {
					minH = v
					seedFace, seedPx, seedPy = face, px, py
				}
				if v > maxH {
					maxH = v
				}
			}
		}
	}

	// Initialize filled to a value strictly above the maximum original
	// elevation everywhere, then set the seed pixel to its original.
	hi := maxH + 1.0
	filled := cubemap.NewF(S)
	for face := range filled.Faces {
		for i := range filled.Faces[face] {
			filled.Faces[face][i] = hi
		}
	}
	filled.Set(seedFace, seedPx, seedPy, minH)

	// Relaxation sweeps: each iteration walks all pixels in both
	// forward and reverse raster order, lowering each to
	// max(original, min over 8-neighbors of filled). Bidirectional
	// scanning is the standard Planchon-Darboux speedup (PD §3.2):
	// the forward sweep propagates drainage along (+x, +y) gradients
	// in one pass and the reverse sweep along (-x, -y), so the
	// drainage front advances at O(1/iter) instead of crawling at
	// O(1/scan-line). Empirically converges in ≤ 8 iterations on
	// terran-class heightmaps at S = 512.
	for iter := 0; iter < planchonDarbouxMaxIters; iter++ {
		changed := relaxForward(heightmap, filled, S)
		if relaxReverse(heightmap, filled, S) {
			changed = true
		}
		if !changed {
			break
		}
	}
	return filled
}

// relaxForward / relaxReverse are the two halves of one Planchon-
// Darboux iteration: forward raster sweep over all faces and the
// reverse of it. Each returns true when any pixel was lowered.
func relaxForward(orig, filled *cubemap.CubeMapF, S int) bool {
	changed := false
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				if relaxOne(orig, filled, face, px, py, S) {
					changed = true
				}
			}
		}
	}
	return changed
}

func relaxReverse(orig, filled *cubemap.CubeMapF, S int) bool {
	changed := false
	for face := cubemap.Face(cubemap.NumFaces) - 1; face >= 0; face-- {
		for py := S - 1; py >= 0; py-- {
			for px := S - 1; px >= 0; px-- {
				if relaxOne(orig, filled, face, px, py, S) {
					changed = true
				}
			}
		}
	}
	return changed
}

// relaxOne applies the Planchon-Darboux relaxation rule at a single
// pixel. Returns true when the filled value was lowered.
func relaxOne(orig, filled *cubemap.CubeMapF, face cubemap.Face, px, py, S int) bool {
	o := orig.Get(face, px, py)
	cur := filled.Get(face, px, py)
	if cur == o {
		return false
	}
	nbrs := cubemap.FacePixelNeighbors8(face, px, py, S)
	minN := math.Inf(1)
	for _, n := range nbrs {
		v := filled.Get(n.Face, n.PX, n.PY)
		if v < minN {
			minN = v
		}
	}
	target := o
	if minN > target {
		target = minN
	}
	if target < cur {
		filled.Set(face, px, py, target)
		return true
	}
	return false
}

// computeD8Pointers writes ff.D8: the index of the steepest-descent
// 8-neighbor for each pixel; -1 if no neighbor is strictly lower than
// the pixel itself (a sink — the global minimum or a flat region).
//
// "Steepest descent" is measured by elevation drop only; the diagonal
// vs. cardinal pixel-distance is ignored because the cube-map seam
// already distorts pixel distance and the threshold-based river mask
// is robust to small biases in pointer choice.
func computeD8Pointers(filled *cubemap.CubeMapF, ff *FlowField) {
	S := filled.Size
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				h := filled.Get(face, px, py)
				nbrs := cubemap.FacePixelNeighbors8(face, px, py, S)
				best := int8(-1)
				bestH := h
				for i, n := range nbrs {
					nh := filled.Get(n.Face, n.PX, n.PY)
					if nh < bestH {
						bestH = nh
						best = int8(i)
					}
				}
				ff.D8[face][py*S+px] = best
			}
		}
	}
}

// flowSortRecord is one entry in the elevation-sorted pixel list used
// to drive the accumulation pass. Sort key: elevation descending, then
// (face, py, px) ascending for deterministic tie-breaks.
type flowSortRecord struct {
	face cubemap.Face
	px   int
	py   int
	elev float64
}

// computeAccumulation writes ff.Accum: each pixel starts with 1.0
// (its own "rain") and forwards its accumulation to its D8-downhill
// neighbor. Pixels are processed in descending elevation order so an
// upstream pixel's accumulation is finalized before its downstream
// neighbor is processed.
//
// Tie-break: pixels with equal elevation are ordered by (face, py,
// px) so two runs with the same heightmap produce byte-identical
// Accum buffers.
func computeAccumulation(filled *cubemap.CubeMapF, ff *FlowField) {
	S := filled.Size
	total := cubemap.NumFaces * S * S
	records := make([]flowSortRecord, 0, total)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				records = append(records, flowSortRecord{
					face: face,
					px:   px,
					py:   py,
					elev: filled.Get(face, px, py),
				})
				ff.Accum[face][py*S+px] = 1.0
			}
		}
	}
	sort.Slice(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if a.elev != b.elev {
			return a.elev > b.elev
		}
		if a.face != b.face {
			return a.face < b.face
		}
		if a.py != b.py {
			return a.py < b.py
		}
		return a.px < b.px
	})
	for _, r := range records {
		idx := ff.D8[r.face][r.py*S+r.px]
		if idx < 0 {
			continue // sink
		}
		dx, dy := neighbor8Deltas[idx][0], neighbor8Deltas[idx][1]
		nf, nx, ny := cubemap.OffsetPixel(r.face, r.px, r.py, dx, dy, S)
		acc := ff.Accum[r.face][r.py*S+r.px]
		ff.Accum[nf][ny*S+nx] += acc
	}
}
