package feature

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Crater represents a single crater on the planet surface.
type Crater struct {
	Lat, Lon    float64 // Spherical coordinates of center
	Radius      float64 // Angular radius on the sphere (radians)
	Age         float64 // [0,1]: 1 = young/sharp, 0 = old/eroded
	IsSecondary bool
	ParentIdx   int // Index of the primary that spawned this secondary, or -1
}

// GenerateCratersLegacy is the pre-Phase-3 uniform-with-quadratic-bias
// generator, kept for callers that don't want the power-law pipeline.
// Used internally when profile.PowerLawAlpha == 0 to preserve existing
// archetype output.
func GenerateCratersLegacy(rng *rand.Rand, count int, minRadius, maxRadius float64) []Crater {
	craters := make([]Crater, count)
	for i := range count {
		lat := math.Asin(2*rng.Float64() - 1)
		lon := rng.Float64() * 2 * math.Pi
		t := rng.Float64()
		radius := minRadius + (maxRadius-minRadius)*t*t
		craters[i] = Crater{Lat: lat, Lon: lon, Radius: radius, Age: 1.0, ParentIdx: -1}
	}
	sort.Slice(craters, func(i, j int) bool {
		return craters[i].Radius > craters[j].Radius
	})
	return craters
}

// GenerateCraters builds a crater list from a PlanetProfile.
//
// When PowerLawAlpha > 0: power-law size-frequency distribution, optional
// maria-mask density modulation, beta-distributed ages biased by
// SurfaceAge, and (if SecondaryDensity > 0) secondaries clustered around
// each large primary.
//
// When PowerLawAlpha == 0: the legacy uniform-with-quadratic-bias path is
// used and every crater gets Age=1, so the pipeline is byte-identical to
// the pre-Phase-3 behavior.
func GenerateCraters(seed int64, profile *types.PlanetProfile) []Crater {
	if profile.PowerLawAlpha <= 0 {
		rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7))) //nolint:gosec
		return GenerateCratersLegacy(rng, profile.CraterCount,
			profile.CraterMinRadius, profile.CraterMaxRadius)
	}

	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed*31+7))) //nolint:gosec
	mariaGen := noise.New(seed + 113)
	alpha := profile.PowerLawAlpha
	rmin, rmax := profile.CraterMinRadius, profile.CraterMaxRadius

	primaries := make([]Crater, 0, profile.CraterCount)
	for range profile.CraterCount {
		lat := math.Asin(2*rng.Float64() - 1)
		lon := rng.Float64() * 2 * math.Pi
		// Maria mask rejection: in the upper-MariaDensityFactor of the
		// mask range, drop 70 % of candidates. Sampling is in 3D so the
		// mask is seam-continuous on the sphere.
		if profile.MariaDensityFactor > 0 {
			x := math.Cos(lat) * math.Cos(lon)
			y := math.Sin(lat)
			z := math.Cos(lat) * math.Sin(lon)
			m := 0.5 + 0.5*mariaGen.FractalNoise3D(x*1.5, y*1.5, z*1.5, 3, 2.0, 0.5, 1.0)
			if m > 1-profile.MariaDensityFactor && rng.Float64() < 0.7 {
				continue
			}
		}
		// Inverse-CDF sampling for P(R) ∝ R^(-α) on [rmin, rmax].
		u := rng.Float64()
		var r float64
		if math.Abs(alpha-1) < 1e-9 {
			r = rmin * math.Pow(rmax/rmin, u)
		} else {
			one := 1 - alpha
			r = math.Pow(u*math.Pow(rmax, one)+(1-u)*math.Pow(rmin, one), 1/one)
		}
		age := betaSample(rng, profile.SurfaceAge)
		primaries = append(primaries, Crater{
			Lat: lat, Lon: lon, Radius: r,
			Age: age, ParentIdx: -1,
		})
	}

	out := make([]Crater, 0, len(primaries))
	out = append(out, primaries...)
	// Secondaries: each large primary spawns 5 + d·10·u₁ smaller bowls
	// within ~3R. Indices into `primaries` stay stable since we copied
	// before appending.
	if profile.SecondaryDensity > 0 {
		threshold := rmax * 0.5
		for i, c := range primaries {
			if c.Radius < threshold {
				continue
			}
			n := 5 + int(profile.SecondaryDensity*10*rng.Float64())
			cosLat := math.Cos(c.Lat)
			if math.Abs(cosLat) < 1e-3 {
				cosLat = 1e-3
			}
			for range n {
				dLat := (rng.Float64()*2 - 1) * c.Radius * 3
				dLon := (rng.Float64()*2 - 1) * c.Radius * 3 / cosLat
				out = append(out, Crater{
					Lat: c.Lat + dLat, Lon: c.Lon + dLon,
					Radius:      c.Radius * (0.05 + 0.15*rng.Float64()),
					Age:         c.Age * (0.6 + 0.4*rng.Float64()),
					IsSecondary: true, ParentIdx: i,
				})
			}
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Radius > out[j].Radius
	})
	// ParentIdx semantics: after sort, indices into `out` no longer
	// match. ParentIdx is informational only (used by ejecta in T8 via
	// IsSecondary flag, not by the index), so this is fine.
	return out
}

// betaSample returns a value in [0,1] biased toward `bias`. Uses the
// classic Beta(α,β) sampler with α/β chosen so the mode equals `bias`
// and concentration ≈ 4. bias=0.5 → near-uniform; bias=0 → spike near
// 0; bias=1 → spike near 1.
func betaSample(rng *rand.Rand, bias float64) float64 {
	if bias <= 0 {
		bias = 0.001
	}
	if bias >= 1 {
		bias = 0.999
	}
	const conc = 4.0
	a := 1 + conc*bias
	b := 1 + conc*(1-bias)
	// Sample Beta via two Gamma deviates (Marsaglia-Tsang for shape ≥ 1).
	g1 := gammaSample(rng, a)
	g2 := gammaSample(rng, b)
	return g1 / (g1 + g2)
}

// gammaSample returns a Gamma(shape, 1) deviate for shape ≥ 1.
// Marsaglia–Tsang squeeze method; for shape < 1 we boost via the
// (X = U^(1/shape) · Gamma(shape+1, 1)) identity.
func gammaSample(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		g := gammaSample(rng, shape+1)
		return g * math.Pow(rng.Float64(), 1/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9*d)
	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}

// ApplyCraters stamps a list of craters onto a cube-map heightmap.
// Per-crater depth is scaled by Age² so old craters fade into the
// surrounding terrain while young ones cut sharply. Legacy craters
// (Age == 1) are unaffected.
func ApplyCraters(cm *cubemap.CubeMapF, craters []Crater, depth float64) {
	S := cm.Size
	for _, c := range craters {
		cx := math.Cos(c.Lat) * math.Cos(c.Lon)
		cy := math.Sin(c.Lat)
		cz := math.Cos(c.Lat) * math.Sin(c.Lon)
		threshold := math.Cos(c.Radius * 1.5)
		// Per-crater depth attenuated by age² so older craters look softer.
		// Legacy callers set Age=1 so behavior is unchanged.
		ageScale := c.Age * c.Age
		if ageScale <= 0 {
			continue
		}
		effDepth := depth * ageScale
		for face := range cubemap.Face(cubemap.NumFaces) {
			if !faceIntersectsCone(face, cx, cy, cz, threshold) {
				continue
			}
			for py := range S {
				for px := range S {
					dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
					dot := dx*cx + dy*cy + dz*cz
					if dot < threshold {
						continue
					}
					// dot < -1 is unreachable: the early-exit above
					// guarantees dot >= cos(Radius*1.5) > -1.
					if dot > 1 {
						dot = 1
					}
					dist := math.Acos(dot)
					if dist >= c.Radius {
						continue
					}
					t := dist / c.Radius
					var mod float64
					if t < 0.8 {
						mod = -effDepth * (1 - (t/0.8)*(t/0.8))
					} else {
						rimT := (t - 0.8) / 0.2
						mod = effDepth * 0.15 * math.Sin(rimT*math.Pi)
					}
					h := cm.Get(face, px, py) + mod
					if h < 0 {
						h = 0
					} else if h > 1 {
						h = 1
					}
					cm.Set(face, px, py, h)
				}
			}
		}
	}
}

// faceIntersectsCone reports whether ANY pixel of `face` could lie within
// the angular cone defined by axis (cx,cy,cz) (unit-length) and threshold
// = cos(coneHalfAngle). Conservative: false positives (face accepted but
// no pixel inside) are tolerated; false negatives (face rejected when a
// pixel IS inside) are bugs.
//
// Method: a face's most-distant interior point from the +face axis is a
// corner, at angular distance acos(1/sqrt(3)) ≈ 0.9553 rad ≈ 54.7° from
// the face axis. Any pixel of the face is within that distance of the
// face axis. So if angularDistance(faceAxis, coneAxis) > coneHalfAngle +
// faceHalfDiag, no pixel can fall inside the cone.
func faceIntersectsCone(face cubemap.Face, cx, cy, cz, threshold float64) bool {
	centerDot := faceCenterDot(face, cx, cy, cz)
	if centerDot >= threshold {
		return true
	}
	const faceHalfDiag = 0.9553166181245093 // acos(1/sqrt(3))
	coneHalfAngle := math.Acos(threshold)
	return math.Acos(centerDot) <= coneHalfAngle+faceHalfDiag
}

// faceCenterDot returns the dot product of the +face axis with (cx,cy,cz).
func faceCenterDot(face cubemap.Face, cx, cy, cz float64) float64 {
	switch face {
	case cubemap.FacePosX:
		return cx
	case cubemap.FaceNegX:
		return -cx
	case cubemap.FacePosY:
		return cy
	case cubemap.FaceNegY:
		return -cy
	case cubemap.FacePosZ:
		return cz
	case cubemap.FaceNegZ:
		return -cz
	}
	return 0
}
