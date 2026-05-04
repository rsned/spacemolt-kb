package feature

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Site is a single civilization location on the unit sphere produced by
// PoissonOnSphere. Population is left at zero here; the population
// assignment pass (P9b Task 3) populates it from the global ranking.
type Site struct {
	Dir          [3]float64 // unit vector
	Population   float64    // [0, 1]; assigned in Task 3 (zero here)
	Habitability float64    // hf.Score sampled at Dir
}

// poissonHabFloor is the lower habitability threshold below which a
// pixel is considered uninhabitable. Pixels with hf.Score < poissonHabFloor
// are excluded from candidate acceptance and from the seed search. The
// "0.05" value mirrors the plan's "no civ here, period" cutoff.
const poissonHabFloor = 0.05

// poissonBridsonK is the Bridson-standard candidate count per active
// site. 20 is the value from Bridson's original 2007 paper.
const poissonBridsonK = 20

// poissonMaxCandidates is a hard ceiling on total candidate generations
// across the entire run. Prevents pathological inputs from looping
// indefinitely while leaving plenty of headroom for typical configs
// (which produce a few hundred sites and a few thousand candidates).
const poissonMaxCandidates = 200_000

// PoissonOnSphere returns Poisson-disc-distributed sites on the unit
// sphere with site density modulated by hf.Score, following Bridson's
// fast Poisson-disc-sampling algorithm adapted directly to spherical
// geometry. Returns nil when civ generation is disabled (cfg.Tier <= 0,
// hf == nil, or no pixel meets the habitability floor).
//
// The minimum acceptance distance for a candidate at a given pixel is
//
//	r(p) = SiteMinDistRad + (1 - hab(p)) * (SiteMaxDistRad - SiteMinDistRad)
//
// so highly habitable cells pack tighter than marginal ones. Candidates
// are sampled uniformly in the spherical-cap annulus [r(active), 2 r(active)]
// around each active site (Bridson's k = 20 attempts per active site).
func PoissonOnSphere(hf *HabitabilityField, cfg types.CivConfig, master int64) []Site {
	if cfg.Tier <= 0 || hf == nil {
		return nil
	}
	if cfg.SiteMinDistRad <= 0 || cfg.SiteMaxDistRad < cfg.SiteMinDistRad {
		return nil
	}
	S := hf.Size
	if S <= 0 {
		return nil
	}

	// Find the highest-habitability cell as the initial seed; reject
	// the whole field if every pixel is below the floor.
	var (
		seedDir   [3]float64
		seedHab   = -1.0
		seedFound bool
	)
	for face := range cubemap.Face(cubemap.NumFaces) {
		row := hf.Score[face]
		if row == nil {
			continue
		}
		for i, v := range row {
			if v < poissonHabFloor {
				continue
			}
			if v > seedHab {
				py := i / S
				px := i - py*S
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				n := math.Sqrt(dx*dx + dy*dy + dz*dz)
				if n == 0 {
					continue
				}
				seedDir = [3]float64{dx / n, dy / n, dz / n}
				seedHab = v
				seedFound = true
			}
		}
	}
	if !seedFound {
		return nil
	}

	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "civ.poisson")),
		uint64(seed.Domain(master, "civ.poisson.stream")),
	))

	rRange := cfg.SiteMaxDistRad - cfg.SiteMinDistRad

	// Per-direction radius in radians, looked up by sampling hab at the
	// direction.
	radiusAt := func(d [3]float64) (float64, float64, bool) {
		face, px, py := cubemap.DirToFacePixel(d[0], d[1], d[2], S)
		hab := hf.Score[face][py*S+px]
		if hab < poissonHabFloor {
			return 0, hab, false
		}
		return cfg.SiteMinDistRad + (1-hab)*rRange, hab, true
	}

	if _, _, ok := radiusAt(seedDir); !ok {
		return nil
	}

	results := []Site{{Dir: seedDir, Habitability: seedHab}}
	active := []int{0} // indices into results

	candidates := 0
	for len(active) > 0 && candidates < poissonMaxCandidates {
		// Bridson typically pops the *last* element (Bridson's original
		// description picks uniformly at random; both work, last-pop is
		// faster and produces very similar distributions).
		ai := active[len(active)-1]
		activeSite := results[ai]
		rA, _, okA := radiusAt(activeSite.Dir)
		if !okA {
			active = active[:len(active)-1]
			continue
		}

		t1, t2 := tangentBasis(activeSite.Dir)
		accepted := false
		for k := 0; k < poissonBridsonK; k++ {
			candidates++
			if candidates >= poissonMaxCandidates {
				break
			}
			// Uniform azimuth.
			theta := 2 * math.Pi * rng.Float64()
			// Uniform arc-distance in [rA, 2 rA]. (A fully area-uniform
			// annulus sample on the cap would weight by sin(arc); for
			// the small radii used here the difference is negligible
			// and Bridson's 2D variant uses the same uniform-distance
			// shortcut.)
			arc := rA + rA*rng.Float64()

			cosT := math.Cos(theta)
			sinT := math.Sin(theta)
			tx := t1[0]*cosT + t2[0]*sinT
			ty := t1[1]*cosT + t2[1]*sinT
			tz := t1[2]*cosT + t2[2]*sinT
			cosA := math.Cos(arc)
			sinA := math.Sin(arc)
			cand := [3]float64{
				activeSite.Dir[0]*cosA + tx*sinA,
				activeSite.Dir[1]*cosA + ty*sinA,
				activeSite.Dir[2]*cosA + tz*sinA,
			}
			n := math.Sqrt(cand[0]*cand[0] + cand[1]*cand[1] + cand[2]*cand[2])
			if n < 1e-12 {
				continue
			}
			cand[0] /= n
			cand[1] /= n
			cand[2] /= n

			rC, habC, okC := radiusAt(cand)
			if !okC {
				continue
			}

			// Reject if any existing site is within rC arc-distance.
			tooClose := false
			for _, s := range results {
				cosD := cand[0]*s.Dir[0] + cand[1]*s.Dir[1] + cand[2]*s.Dir[2]
				if cosD > 1 {
					cosD = 1
				} else if cosD < -1 {
					cosD = -1
				}
				if math.Acos(cosD) < rC {
					tooClose = true
					break
				}
			}
			if tooClose {
				continue
			}

			results = append(results, Site{Dir: cand, Habitability: habC})
			active = append(active, len(results)-1)
			accepted = true
			break
		}

		if !accepted {
			active = active[:len(active)-1]
		}
	}

	return results
}

// tangentBasis returns two unit vectors orthogonal to d (assumed unit)
// and to each other. The choice of basis is arbitrary; we Gram-Schmidt
// against the world X axis, falling back to Y when d is colinear with X.
func tangentBasis(d [3]float64) (t1, t2 [3]float64) {
	axis := [3]float64{1, 0, 0}
	if math.Abs(d[0]) > 0.9 {
		axis = [3]float64{0, 1, 0}
	}
	// t1 = normalize(axis - (axis·d) d)
	dot := axis[0]*d[0] + axis[1]*d[1] + axis[2]*d[2]
	t1 = [3]float64{
		axis[0] - dot*d[0],
		axis[1] - dot*d[1],
		axis[2] - dot*d[2],
	}
	n := math.Sqrt(t1[0]*t1[0] + t1[1]*t1[1] + t1[2]*t1[2])
	t1[0] /= n
	t1[1] /= n
	t1[2] /= n
	// t2 = d × t1 (right-handed; already unit since d ⟂ t1 and both unit).
	t2 = [3]float64{
		d[1]*t1[2] - d[2]*t1[1],
		d[2]*t1[0] - d[0]*t1[2],
		d[0]*t1[1] - d[1]*t1[0],
	}
	return t1, t2
}
