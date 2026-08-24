// Tectonic plate seed types, Fibonacci-spiral seeding, random
// flood-fill across cube faces, boundary classification, and three
// boundary distance fields (convergent / divergent / transform).

package field

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Plate captures a tectonic plate's identity and motion.
//
// Seed is the unit-vector direction of the Fibonacci-spiral seed used
// to start the plate's flood-fill region. RotAxis + AngSpeed encode an
// instantaneous angular-velocity vector ω = AngSpeed · RotAxis used
// only to classify boundaries via relative velocity at boundary pixels.
// No animation is implied.
//
// IsOceanic is drawn at construction time as a Bernoulli sample on the
// archetype's OceanicPlateFraction; it does not depend on the
// heightmap.
type Plate struct {
	ID        int
	Seed      [3]float64
	RotAxis   [3]float64
	AngSpeed  float64
	IsOceanic bool
	// Weight is the flood-fill growth bias (Phase 12 two-tier seeding):
	// frontier candidates from a plate are pushed Weight times, so a
	// weight-4 major plate expands ~4× faster than a weight-1 minor.
	// Legacy single-tier seeding sets Weight = 1 on every plate.
	Weight float64
}

// BoundaryPixel is one boundary source pixel retained for crust-aware
// classification (Phase 12): its location, smoothed tangent normal
// pointing toward the foreign plate, and clamped velocity magnitude.
type BoundaryPixel struct {
	Face cubemap.Face
	Idx  int // py*S + px
	N    [3]float64
	Mag  float64
}

// PlateField is the per-pixel plate output for a planet at a given
// face size. PlateID[face][py*S+px] holds the plate id (-1 for unset
// during construction; never persisted).
//
// Convergent / Divergent / Transform are signed-distance fields in km
// from each pixel to the nearest boundary of the corresponding type.
// Pixels in a face with no boundary of the corresponding type get
// math.Pi * RadiusKm (the geodesic half-circumference, ~20015 km for
// Earth-like radius), not math.MaxFloat64.
type PlateField struct {
	Size       int
	Plates     []Plate
	PlateID    [cubemap.NumFaces][]int16
	Convergent [cubemap.NumFaces][]float64
	Divergent  [cubemap.NumFaces][]float64
	Transform  [cubemap.NumFaces][]float64
	// ConvergentMag / DivergentMag hold the per-pixel magnitude of the
	// nearest convergent/divergent boundary, propagated outward by the
	// JFA pass. Magnitude is (|vRel·n| − T) / (1 − T) clamped to [0,1]
	// at the source pixel: 0 = pixel sits exactly at the convergence
	// threshold, 1 = maximum collision/spread. Downstream stages
	// (Ridged amplitude, Basin depth) multiply by this so violent
	// boundaries produce taller mountain belts and deeper trenches
	// while gentle boundaries just produce subtle topography.
	ConvergentMag [cubemap.NumFaces][]float64
	DivergentMag  [cubemap.NumFaces][]float64
	// ConvPixels / DivPixels are the sparse boundary source pixels of
	// each kind, with smoothed normals — consumed by ClassifyTectonics.
	ConvPixels []BoundaryPixel
	DivPixels  []BoundaryPixel
}

// GeneratePlates produces a PlateField for a planet at face size S.
// Returns nil when no plates are seeded (legacy: profile.PlateCount ==
// 0; crust path: profile.Crust.MajorPlates == 0).
//
// When profile.Crust.MajorPlates > 0 the crust path runs: two-tier
// seeding (major + minor filler plates) with growth-weighted flood
// fill, ignoring profile.PlateCount. Otherwise the legacy single-tier
// path runs off profile.PlateCount.
//
// Pipeline:
//  1. seedPlates: N spiral seeds + per-plate motion + oceanic flag.
//  2. floodFillPlates: assign every pixel a plate id by random-walk frontier.
//  3. computeSDFs: classify boundaries + run three JFA passes for
//     convergent / divergent / transform SDFs in km.
//
// All RNG draws happen inside the named-domain seeds defined in
// pkg/planetgen/field/plates.go so adding new sub-steps in later
// phases never shifts existing field values.
func GeneratePlates(profile *types.PlanetProfile, master int64, S int) *PlateField {
	var plates []Plate
	if profile.Crust.MajorPlates > 0 {
		if profile.Crust.MajorPlates+profile.Crust.MinorPlates > math.MaxInt16 {
			panic("planetgen/field: MajorPlates+MinorPlates exceeds int16 max (32767)")
		}
		plates = seedPlatesTwoTier(profile, master)
	} else {
		if profile.PlateCount > math.MaxInt16 {
			panic("planetgen/field: PlateCount exceeds int16 max (32767)")
		}
		plates = seedPlates(profile, master)
	}
	if len(plates) == 0 {
		return nil
	}
	pf := &PlateField{Size: S, Plates: plates}
	for f := range pf.PlateID {
		pf.PlateID[f] = make([]int16, S*S)
		for i := range pf.PlateID[f] {
			pf.PlateID[f][i] = -1
		}
	}
	floodFillPlates(pf, master, S)
	computeSDFs(pf, profile)
	return pf
}

// floodFillPlates assigns every pixel a plate id via random-walk
// frontier expansion starting from each plate's spiral seed pixel.
//
// Termination is bounded: every pop either assigns a previously-
// unassigned pixel or skips an already-assigned duplicate; pushes only
// happen for unassigned pixels at push time, so the frontier drains.
func floodFillPlates(pf *PlateField, master int64, S int) {
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.fill.random")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.fill.random.stream")), //nolint:gosec
	))

	// Mark each plate's seed pixel.
	for i := range pf.Plates {
		d := pf.Plates[i].Seed
		f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
		pf.PlateID[f][py*S+px] = int16(i)
	}

	// Frontier: list of (unassigned-pixel, claiming-plate-id) candidates.
	type frontierItem struct {
		Addr cubemap.PixelAddr
		ID   int16
	}
	frontier := make([]frontierItem, 0, S*S)

	pushNeighbors := func(face cubemap.Face, px, py int, id int16) {
		repeat := max(int(pf.Plates[id].Weight+0.5), 1)
		nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
		for _, n := range nbrs {
			if pf.PlateID[n.Face][n.PY*S+n.PX] == -1 {
				for range repeat {
					frontier = append(frontier, frontierItem{Addr: n, ID: id})
				}
			}
		}
	}

	// Seed the frontier from each plate's seed pixel.
	for i := range pf.Plates {
		d := pf.Plates[i].Seed
		f, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
		pushNeighbors(f, px, py, int16(i))
	}

	for len(frontier) > 0 {
		// Pick a random index, swap-pop.
		idx := rng.IntN(len(frontier))
		item := frontier[idx]
		frontier[idx] = frontier[len(frontier)-1]
		frontier = frontier[:len(frontier)-1]

		if pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] != -1 {
			continue // already filled by a different chain
		}
		pf.PlateID[item.Addr.Face][item.Addr.PY*S+item.Addr.PX] = item.ID
		pushNeighbors(item.Addr.Face, item.Addr.PX, item.Addr.PY, item.ID)
	}

	// Seam-pin pass: cross-face neighbor lookup is symmetric (verified
	// by cubemap.TestFacePixelNeighbors4Symmetric), but the random-pop
	// frontier can still assign a matched-pair pixel and its cross-face
	// twin different plate ids when both pixels' winning frontier entries
	// were pushed from different chains. This pass enforces categorical
	// equality at every seam pair by a deterministic lexicographic
	// tiebreak. It is idempotent: applying the rule twice yields the
	// same result, so walking each pair from both sides is safe.
	//
	// Bound: at most 12·S seam pixels exist out of 6·S² total, so the
	// reassignment touches ≤ 0.4%·(1/S) of pixels — far below the noise
	// floor of plate-area statistics, and the per-archetype plate-count
	// invariant is preserved because we never overwrite the only pixel
	// of a plate (each plate's seed pixel is interior-of-its-own-plate
	// at S=64 with PlateCount ≤ 16).
	for f := cubemap.Face(0); f < cubemap.NumFaces; f++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				if px > 0 && px < S-1 && py > 0 && py < S-1 {
					continue
				}
				nbrs := cubemap.FacePixelNeighbors4(f, px, py, S)
				for _, n := range nbrs {
					if n.Face == f {
						continue
					}
					hereID := pf.PlateID[f][py*S+px]
					thereID := pf.PlateID[n.Face][n.PY*S+n.PX]
					if hereID == thereID {
						continue
					}
					// Lexicographic tiebreak: lower (face, py, px) wins.
					if n.Face < f || (n.Face == f && (n.PY < py || (n.PY == py && n.PX < px))) {
						pf.PlateID[f][py*S+px] = thereID
					} else {
						pf.PlateID[n.Face][n.PY*S+n.PX] = hereID
					}
				}
			}
		}
	}
}

// boundaryKind classifies a plate-boundary pixel by its relative-
// velocity dot-product against the boundary normal. boundaryNone
// indicates an interior pixel (no neighbor with a different plate id).
type boundaryKind int8

const (
	boundaryNone boundaryKind = iota
	boundaryConvergent
	boundaryDivergent
	boundaryTransform
)

// classifyBoundary returns the plate-boundary type at a point given
// the relative velocity vRel between the two plates at that point and
// the boundary normal n (unit vector from "this" plate's pixel toward
// the differing-plate neighbor pixel, in tangent space). T is the
// signed-velocity threshold (profile.PlateConvergentT, default 0.75).
func classifyBoundary(vRel, n [3]float64, T float64) boundaryKind {
	proj := vRel[0]*n[0] + vRel[1]*n[1] + vRel[2]*n[2]
	// vRel = vA - vB; n points from A's pixel toward B's pixel. If
	// vRel·n > 0, plate A is moving into B's territory → colliding
	// (convergent). If vRel·n < 0, A is moving away from B → spreading
	// (divergent).
	switch {
	case proj > +T:
		return boundaryConvergent
	case proj < -T:
		return boundaryDivergent
	default:
		return boundaryTransform
	}
}

// rawBoundary holds the unsmoothed per-pixel boundary data computed by
// computeRawBoundaries. foreign is the dominant foreign plate id at
// this pixel (the plate id seen most often among the 4-neighbors that
// differ from here, ties broken by lowest id), or -1 for interior
// pixels. (nx,ny,nz) is the tangent-projected unit normal pointing
// from here toward the foreign-side neighbor pixels averaged into a
// single direction. This per-pixel n is noisy along the jagged
// flood-fill front; extractBoundaries averages across a 5×5 stencil
// to recover a physically-meaningful boundary direction.
type rawBoundary struct {
	foreign    int16
	nx, ny, nz float64
}

func cross(a, b [3]float64) [3]float64 {
	return [3]float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func scale(a [3]float64, s float64) [3]float64 {
	return [3]float64{a[0] * s, a[1] * s, a[2] * s}
}

// boundarySmoothStencilRadius sets the half-width of the smoothing
// window. A 2-pixel radius gives a 5×5 stencil; on a 256-face cube
// that's roughly 5° of arc, large enough to suppress pixel-grid
// noise in n but small enough to preserve true along-boundary
// transitions between convergent/divergent regimes.
const boundarySmoothStencilRadius = 2

// computeRawBoundaries returns per-pixel boundary data: the dominant
// foreign plate id at every boundary pixel plus the per-pixel
// tangent-projected unit normal pointing from this plate toward that
// foreign plate. Interior pixels (no 4-neighbor on a different plate)
// have foreign = -1. The returned normals are noisy along the jagged
// flood-fill front and are smoothed by extractBoundaries before being
// used for classification.
func computeRawBoundaries(pf *PlateField) [cubemap.NumFaces][]rawBoundary {
	S := pf.Size
	var rb [cubemap.NumFaces][]rawBoundary
	for f := range rb {
		rb[f] = make([]rawBoundary, S*S)
		for i := range rb[f] {
			rb[f][i].foreign = -1
		}
	}
	for face := range cubemap.NumFaces {
		for py := range S {
			for px := range S {
				here := pf.PlateID[face][py*S+px]
				nbrs := cubemap.FacePixelNeighbors4(cubemap.Face(face), px, py, S)
				// Count foreign-plate neighbors and pick the dominant
				// one — most-common wins, lowest id breaks ties for
				// determinism across runs.
				var counts map[int16]int
				for _, nb := range nbrs {
					there := pf.PlateID[nb.Face][nb.PY*S+nb.PX]
					if there == here {
						continue
					}
					if counts == nil {
						counts = make(map[int16]int, 4)
					}
					counts[there]++
				}
				if counts == nil {
					continue
				}
				bestID := int16(math.MaxInt16)
				bestCount := 0
				for id, c := range counts {
					if c > bestCount || (c == bestCount && id < bestID) {
						bestCount = c
						bestID = id
					}
				}
				// Sum tangent-projected normals from every neighbor
				// that belongs to the dominant foreign plate.
				dx, dy, dz := cubemap.FacePixelToDir(cubemap.Face(face), px, py, S)
				p := [3]float64{dx, dy, dz}
				var nx, ny, nz float64
				for _, nb := range nbrs {
					there := pf.PlateID[nb.Face][nb.PY*S+nb.PX]
					if there != bestID {
						continue
					}
					ndx, ndy, ndz := cubemap.FacePixelToDir(nb.Face, nb.PX, nb.PY, S)
					rnx := ndx - dx
					rny := ndy - dy
					rnz := ndz - dz
					proj := rnx*p[0] + rny*p[1] + rnz*p[2]
					nx += rnx - proj*p[0]
					ny += rny - proj*p[1]
					nz += rnz - proj*p[2]
				}
				mag := math.Sqrt(nx*nx + ny*ny + nz*nz)
				if mag < 1e-9 {
					continue
				}
				rb[face][py*S+px] = rawBoundary{
					foreign: bestID,
					nx:      nx / mag,
					ny:      ny / mag,
					nz:      nz / mag,
				}
			}
		}
	}
	return rb
}

// magnitudeFromProj normalizes |proj| to a magnitude in [0,1] by
// subtracting the threshold and rescaling. proj is the signed scalar
// vRel·n; absProj is |proj|. At absProj == T the result is 0 (the
// pixel sits exactly on the convergent/divergent threshold); at
// absProj == 1 it saturates to 1. Used by extractBoundaries to fill
// the per-pixel ConvergentMag / DivergentMag fields before JFA.
func magnitudeFromProj(absProj, T float64) float64 {
	denom := 1 - T
	if denom <= 0 {
		if absProj > 0 {
			return 1
		}
		return 0
	}
	m := (absProj - T) / denom
	if m < 0 {
		return 0
	}
	if m > 1 {
		return 1
	}
	return m
}

// extractBoundaries classifies every pixel of pf using a smoothed
// boundary normal and returns binary masks plus per-pixel magnitudes
// at convergent/divergent pixels. The smoothing — a 5×5 average of
// raw per-pixel n vectors that share the same plate-pair — kills the
// pixel-grid flicker that previously caused the same physical
// boundary spot to register as both convergent AND divergent on
// adjacent pixels. The returned conv/div/trans masks are mutually
// exclusive at every pixel (asserted by TestExtractBoundariesMutuallyExclusive).
//
// convMag[face][i] and divMag[face][i] hold (|projection| − T) / (1 − T)
// clamped to [0,1] for boundary pixels, 0 elsewhere. Phase B's JFA
// propagates these magnitudes alongside coordinates so every interior
// pixel knows the strength of its nearest collision/spread.
//
// Precondition: pf.PlateID must be fully assigned — no pixel may
// hold the -1 sentinel. (GeneratePlates guarantees this after
// floodFillPlates returns.) Calling on a partially-assigned field
// will panic via out-of-bounds index into pf.Plates.
func extractBoundaries(pf *PlateField, T float64) (
	conv, div, trans [cubemap.NumFaces][]bool,
	convMag, divMag [cubemap.NumFaces][]float64,
	convPix, divPix []BoundaryPixel,
) {
	S := pf.Size
	for f := range conv {
		conv[f] = make([]bool, S*S)
		div[f] = make([]bool, S*S)
		trans[f] = make([]bool, S*S)
		convMag[f] = make([]float64, S*S)
		divMag[f] = make([]float64, S*S)
	}
	rb := computeRawBoundaries(pf)
	r := boundarySmoothStencilRadius
	for face := range cubemap.NumFaces {
		for py := range S {
			for px := range S {
				rbHere := rb[face][py*S+px]
				if rbHere.foreign < 0 {
					continue
				}
				here := pf.PlateID[face][py*S+px]
				foreign := rbHere.foreign
				// Smooth n across the on-face 5×5 stencil. Match same
				// (this,foreign) pair; sum the opposite-side pixels
				// with negated n since they describe the same physical
				// boundary from the other side.
				var nx, ny, nz float64
				for dy := -r; dy <= r; dy++ {
					qy := py + dy
					if qy < 0 || qy >= S {
						continue
					}
					for dx := -r; dx <= r; dx++ {
						qx := px + dx
						if qx < 0 || qx >= S {
							continue
						}
						rbN := rb[face][qy*S+qx]
						if rbN.foreign < 0 {
							continue
						}
						qHere := pf.PlateID[face][qy*S+qx]
						switch {
						case qHere == here && rbN.foreign == foreign:
							nx += rbN.nx
							ny += rbN.ny
							nz += rbN.nz
						case qHere == foreign && rbN.foreign == here:
							nx -= rbN.nx
							ny -= rbN.ny
							nz -= rbN.nz
						}
					}
				}
				// Re-tangent-project at p and renormalize.
				dx, dy, dz := cubemap.FacePixelToDir(cubemap.Face(face), px, py, S)
				p := [3]float64{dx, dy, dz}
				projN := nx*p[0] + ny*p[1] + nz*p[2]
				nx -= projN * p[0]
				ny -= projN * p[1]
				nz -= projN * p[2]
				nmag := math.Sqrt(nx*nx + ny*ny + nz*nz)
				if nmag < 1e-9 {
					// Smoothing collapsed n to zero; fall back to the
					// per-pixel direction (rare — only at extremely
					// short boundaries with self-cancelling normals).
					nx, ny, nz = rbHere.nx, rbHere.ny, rbHere.nz
				} else {
					nx /= nmag
					ny /= nmag
					nz /= nmag
				}
				a := pf.Plates[here]
				b := pf.Plates[foreign]
				va := cross(scale(a.RotAxis, a.AngSpeed), p)
				vb := cross(scale(b.RotAxis, b.AngSpeed), p)
				vRelX := va[0] - vb[0]
				vRelY := va[1] - vb[1]
				vRelZ := va[2] - vb[2]
				proj := vRelX*nx + vRelY*ny + vRelZ*nz
				idx := py*S + px
				switch {
				case proj > +T:
					conv[face][idx] = true
					convMag[face][idx] = magnitudeFromProj(proj, T)
					convPix = append(convPix, BoundaryPixel{
						Face: cubemap.Face(face), Idx: idx,
						N: [3]float64{nx, ny, nz}, Mag: convMag[face][idx],
					})
				case proj < -T:
					div[face][idx] = true
					divMag[face][idx] = magnitudeFromProj(-proj, T)
					divPix = append(divPix, BoundaryPixel{
						Face: cubemap.Face(face), Idx: idx,
						N: [3]float64{nx, ny, nz}, Mag: divMag[face][idx],
					})
				default:
					trans[face][idx] = true
				}
			}
		}
	}
	return
}

// seedPlates returns N plates with Fibonacci-spiral unit-vector seeds,
// random rotation axes, [0,1] angular speeds, and Bernoulli-sampled
// oceanic flags. Deterministic for fixed (profile.PlateCount,
// OceanicPlateFraction, master).
func seedPlates(profile *types.PlanetProfile, master int64) []Plate {
	n := profile.PlateCount
	if n <= 0 {
		return nil
	}
	plates := make([]Plate, n)

	// Fibonacci spiral on the unit sphere with a small per-seed jitter
	// so different planet seeds produce visibly distinct plate layouts.
	rngSeed := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.seeds")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.seeds.stream")), //nolint:gosec
	))
	rngMotion := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.motion")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.motion.stream")), //nolint:gosec
	))
	rngOceanic := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.oceanic")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.oceanic.stream")), //nolint:gosec
	))

	goldenAngle := math.Pi * (3.0 - math.Sqrt(5)) // π·(3 − √5)
	for i := range n {
		// Fibonacci spiral point on the unit sphere.
		y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
		radius := math.Sqrt(1.0 - y*y)
		theta := goldenAngle * float64(i)
		// Jitter the spiral seed direction by ~3° to break visual symmetry
		// between planets that share PlateCount.
		jitter := (rngSeed.Float64() - 0.5) * (math.Pi / 30)
		x := math.Cos(theta+jitter) * radius
		z := math.Sin(theta+jitter) * radius

		plates[i].ID = i
		plates[i].Weight = 1
		plates[i].Seed = [3]float64{x, y, z}

		// Random axis on unit sphere via Marsaglia's method (rejection sampling
		// in the unit disk → project onto sphere).
		for {
			a := 2*rngMotion.Float64() - 1
			b := 2*rngMotion.Float64() - 1
			s := a*a + b*b
			if s >= 1 {
				continue
			}
			f := 2 * math.Sqrt(1-s)
			plates[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
			break
		}
		plates[i].AngSpeed = rngMotion.Float64()
		plates[i].IsOceanic = rngOceanic.Float64() < profile.OceanicPlateFraction
	}

	// Sum-to-zero angular momentum: subtract the mean ω from each plate
	// so the lithosphere has no net spin relative to the mantle. Without
	// this, neighboring plates sampled independently can share similar
	// motion, producing "conga-line" boundaries (everywhere transform).
	// Earth-like dynamics require opposing pairs of plates, which
	// emerges naturally once the global mean is removed. Plates with
	// motion very close to the mean end up with tiny residual speed —
	// rare in practice for n ≥ 6.
	balanceMomentum(plates)
	return plates
}

// balanceMomentum subtracts the mean angular-velocity vector from every
// plate so the lithosphere has no net spin (see seedPlates for why).
func balanceMomentum(plates []Plate) {
	n := len(plates)
	var mx, my, mz float64
	for i := range plates {
		mx += plates[i].RotAxis[0] * plates[i].AngSpeed
		my += plates[i].RotAxis[1] * plates[i].AngSpeed
		mz += plates[i].RotAxis[2] * plates[i].AngSpeed
	}
	mx /= float64(n)
	my /= float64(n)
	mz /= float64(n)
	for i := range plates {
		wx := plates[i].RotAxis[0]*plates[i].AngSpeed - mx
		wy := plates[i].RotAxis[1]*plates[i].AngSpeed - my
		wz := plates[i].RotAxis[2]*plates[i].AngSpeed - mz
		mag := math.Sqrt(wx*wx + wy*wy + wz*wz)
		if mag < 1e-12 {
			plates[i].AngSpeed = 0
			continue
		}
		plates[i].RotAxis = [3]float64{wx / mag, wy / mag, wz / mag}
		plates[i].AngSpeed = mag
	}
}

// seedPlatesTwoTier (Phase 12) seeds MajorPlates spiral seeds with
// growth weight MajorGrowthBias plus MinorPlates gap-filler seeds with
// weight 1, placed by farthest-point sampling over a 64-candidate
// Fibonacci pool. Motion and momentum balancing match seedPlates.
// Oceanic flags use Crust.OceanicFraction ("carries no craton").
func seedPlatesTwoTier(profile *types.PlanetProfile, master int64) []Plate {
	// poolN is the size of the Fibonacci candidate pool minors are
	// farthest-point-picked from.
	const poolN = 64
	nMaj := profile.Crust.MajorPlates
	nMin := profile.Crust.MinorPlates
	if nMaj <= 0 {
		return nil
	}
	if nMin > poolN {
		nMin = poolN // farthest-point pool size; more minors than candidates is meaningless
	}
	bias := profile.Crust.MajorGrowthBias
	if bias <= 0 {
		bias = 4
	}
	total := nMaj + nMin
	plates := make([]Plate, 0, total)

	rngSeed := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.seeds")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.seeds.stream")), //nolint:gosec
	))
	rngMotion := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.motion")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.motion.stream")), //nolint:gosec
	))
	rngOceanic := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "plates.oceanic")),        //nolint:gosec
		uint64(seed.Domain(master, "plates.oceanic.stream")), //nolint:gosec
	))

	goldenAngle := math.Pi * (3.0 - math.Sqrt(5))
	spiral := func(i, n int) [3]float64 {
		y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
		radius := math.Sqrt(1.0 - y*y)
		theta := goldenAngle * float64(i)
		jitter := (rngSeed.Float64() - 0.5) * (math.Pi / 30)
		return [3]float64{math.Cos(theta+jitter) * radius, y, math.Sin(theta+jitter) * radius}
	}

	// Majors: spiral over nMaj.
	for i := range nMaj {
		plates = append(plates, Plate{ID: i, Seed: spiral(i, nMaj), Weight: bias})
	}

	// Minors: farthest-point picks from a 64-candidate spiral pool.
	pool := make([][3]float64, poolN)
	for i := range poolN {
		pool[i] = spiral(i, poolN)
	}
	for m := range nMin {
		// True farthest-point pick: minimize the dot to the nearest existing seed.
		bestIdx := 0
		bestClosest := 2.0 // dot to the nearest existing seed; smaller = farther away
		for ci, c := range pool {
			closest := -2.0
			for _, p := range plates {
				if d := dot3(p.Seed, c); d > closest {
					closest = d
				}
			}
			if closest < bestClosest {
				bestClosest = closest
				bestIdx = ci
			}
		}
		plates = append(plates, Plate{ID: nMaj + m, Seed: pool[bestIdx], Weight: 1})
		// Remove the chosen candidate so the next minor picks elsewhere.
		pool[bestIdx] = pool[len(pool)-1]
		pool = pool[:len(pool)-1]
	}

	// Motion + oceanic flags, in plate order (matches seedPlates idiom).
	for i := range plates {
		for {
			a := 2*rngMotion.Float64() - 1
			b := 2*rngMotion.Float64() - 1
			s := a*a + b*b
			if s >= 1 {
				continue
			}
			f := 2 * math.Sqrt(1-s)
			plates[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
			break
		}
		plates[i].AngSpeed = rngMotion.Float64()
		plates[i].IsOceanic = rngOceanic.Float64() < profile.Crust.OceanicFraction
	}
	balanceMomentum(plates)
	return plates
}

// computeSDFs runs three JFA passes over the boundary-type masks and
// scales the angular-distance output to kilometers using the planet
// RadiusKm. RadiusKm defaults to 6371 km (Earth-like) when zero.
//
// JFA output is angular distance / π in [0, 1]; multiplying by π · R
// gives geodesic km in [0, π·R].
func computeSDFs(pf *PlateField, profile *types.PlanetProfile) {
	conv, div, trans, convMag, divMag, convPix, divPix := extractBoundaries(pf, profile.PlateConvergentT)
	radius := profile.RadiusKm
	if radius == 0 {
		radius = 6371
	}
	factor := math.Pi * radius
	scaleKm := func(f *cubemap.CubeMapF) [cubemap.NumFaces][]float64 {
		var out [cubemap.NumFaces][]float64
		for i := range f.Faces {
			out[i] = make([]float64, len(f.Faces[i]))
			for j, v := range f.Faces[i] {
				out[i][j] = v * factor
			}
		}
		return out
	}
	copyOut := func(f *cubemap.CubeMapF) [cubemap.NumFaces][]float64 {
		var out [cubemap.NumFaces][]float64
		for i := range f.Faces {
			out[i] = make([]float64, len(f.Faces[i]))
			copy(out[i], f.Faces[i])
		}
		return out
	}
	convDist, convMagOut := JumpFloodFromMaskWithValue(conv, convMag, pf.Size)
	divDist, divMagOut := JumpFloodFromMaskWithValue(div, divMag, pf.Size)
	pf.Convergent = scaleKm(convDist)
	pf.Divergent = scaleKm(divDist)
	pf.Transform = scaleKm(JumpFloodFromMask(trans, pf.Size))
	pf.ConvergentMag = copyOut(convMagOut)
	pf.DivergentMag = copyOut(divMagOut)
	// Populate sparse pixel lists after the dense mag arrays are built so
	// BoundaryPixel.Mag matches the float32-rounded value stored in the
	// dense field (JFA internally stores magnitudes as float32, causing a
	// tiny precision shift). Reading back from the dense array guarantees
	// the invariant pf.ConvergentMag[bp.Face][bp.Idx] == bp.Mag.
	// convPix/divPix are extractBoundaries' sole returned references, so
	// mutating them in place is safe.
	pf.ConvPixels = convPix
	for i := range pf.ConvPixels {
		bp := &pf.ConvPixels[i]
		bp.Mag = pf.ConvergentMag[bp.Face][bp.Idx]
	}
	pf.DivPixels = divPix
	for i := range pf.DivPixels {
		bp := &pf.DivPixels[i]
		bp.Mag = pf.DivergentMag[bp.Face][bp.Idx]
	}
}
