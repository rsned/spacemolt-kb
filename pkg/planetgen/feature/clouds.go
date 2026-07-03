// Package feature contains optional surface-overlay generators that
// run on top of the rocky-pipeline base render. clouds.go produces a
// separate cube-map for atmospheric archetypes; civ.go adds
// civilization signs (Phase 9b).
package feature

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// CloudField holds per-pixel cloud density on a cube-map. Density is
// the post-shadow brightness (what gets rendered to RGBA); Alpha is
// the raw pre-shadow cloud fraction (used for compositing the cloud
// PNG over the planet color cube-map).
type CloudField struct {
	Size    int
	Density [cubemap.NumFaces][]float64
	Alpha   [cubemap.NumFaces][]float64
}

// storm is a Rankine-vortex storm center on the unit sphere.
type storm struct {
	center [3]float64
	sense  float64 // ±1 — direction of swirl (reserved; v1 ignores rotation)
}

// GenerateClouds returns the cloud cube-map for the given profile +
// seed. Returns nil when profile.Cloud.Coverage <= 0 (the profile
// disables cloud generation).
func GenerateClouds(profile *types.PlanetProfile, master int64, S int) *CloudField {
	cfg := profile.Cloud
	if cfg.Coverage <= 0 {
		return nil
	}
	if cfg.Freq == 0 {
		cfg.Freq = 4
	}
	if cfg.Octaves == 0 {
		cfg.Octaves = 4
	}

	cf := &CloudField{Size: S}
	for f := range cf.Density {
		cf.Density[f] = make([]float64, S*S)
		cf.Alpha[f] = make([]float64, S*S)
	}

	warpGen := noise.New(seed.Domain(master, "clouds.warp"))
	densGen := noise.New(seed.Domain(master, "clouds.density"))
	storms := generateStorms(cfg)

	sun := normalizeSun(cfg.SunDir)

	// First pass: alpha (raw cloud fraction).
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				lat := math.Asin(dy)

				// 1. Latitude-band envelope: cos(lat) tapers cover toward poles.
				envelope := cfg.Coverage * math.Cos(lat)

				// 2. Domain-warped fBm density.
				wx := warpGen.FractalNoise3D(dx, dy, dz, 2, 2.0, 0.5, cfg.Freq)
				wy := warpGen.FractalNoise3D(dy+1.7, dz+5.3, dx, 2, 2.0, 0.5, cfg.Freq)
				wz := warpGen.FractalNoise3D(dz+5.3, dx+1.7, dy, 2, 2.0, 0.5, cfg.Freq)
				wdx := dx + cfg.WarpAmp*wx
				wdy := dy + cfg.WarpAmp*wy
				wdz := dz + cfg.WarpAmp*wz
				density := densGen.FractalNoise3D(wdx, wdy, wdz, cfg.Octaves, 2.0, 0.5, cfg.Freq)
				// FractalNoise3D output is roughly [-1, 1].

				// 3. Storm contribution.
				stormBoost := stormContribution(storms, dx, dy, dz, cfg)

				// 4. Combine. Mix coverage envelope with density-modulated
				//    detail (multiplicative — detail breaks up the band
				//    rather than adding mean cover), plus storm boosts.
				//    Clamp to [0, 1].
				detail := 0.5 + 0.5*density // [0,1]
				alpha := envelope*(0.4+1.2*detail) + stormBoost
				if alpha < 0 {
					alpha = 0
				}
				if alpha > 1 {
					alpha = 1
				}
				cf.Alpha[face][py*S+px] = alpha
			}
		}
	}

	// Second pass: shadow.
	for face := cubemap.Face(0); face < cubemap.NumFaces; face++ {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				a := cf.Alpha[face][py*S+px]
				grad := approximateAlphaGradient(cf, face, px, py, S)
				shade := dot3(grad, sun) * cfg.ShadowGain
				lit := a * (1 + shade)
				if lit < 0 {
					lit = 0
				}
				if lit > 1 {
					lit = 1
				}
				cf.Density[face][py*S+px] = lit
			}
		}
	}

	return cf
}

// generateStorms places StormCount Fibonacci-spiral seeds within ±60°
// of the equator, with alternating ±1 rotation senses. Deterministic;
// no RNG jitter in v1 (Fibonacci alone gives well-spaced storms).
func generateStorms(cfg types.CloudConfig) []storm {
	if cfg.StormCount <= 0 {
		return nil
	}
	out := make([]storm, cfg.StormCount)
	g := math.Pi * (3 - math.Sqrt(5)) // golden angle
	for i := range out {
		var y float64
		if cfg.StormCount == 1 {
			y = 0
		} else {
			// y in [-sin(60°), +sin(60°)] across the count.
			y = (1 - 2.0*float64(i)/float64(cfg.StormCount-1)) * math.Sin(math.Pi/3)
		}
		r := math.Sqrt(1 - y*y)
		theta := g * float64(i)
		x := math.Cos(theta) * r
		z := math.Sin(theta) * r
		sense := 1.0
		if i%2 == 1 {
			sense = -1.0
		}
		out[i] = storm{center: [3]float64{x, y, z}, sense: sense}
	}
	return out
}

// stormContribution sums smooth radial-falloff contributions from all
// storms at the query direction. Capped at +0.5 so storms can't fully
// dominate the alpha field.
func stormContribution(storms []storm, dx, dy, dz float64, cfg types.CloudConfig) float64 {
	if len(storms) == 0 || cfg.StormRadiusRad <= 0 {
		return 0
	}
	var total float64
	for _, s := range storms {
		cosA := dx*s.center[0] + dy*s.center[1] + dz*s.center[2]
		if cosA > 1 {
			cosA = 1
		} else if cosA < -1 {
			cosA = -1
		}
		ang := math.Acos(cosA)
		if ang >= cfg.StormRadiusRad {
			continue
		}
		// Smooth radial falloff: linear-squared toward the center.
		t := 1 - ang/cfg.StormRadiusRad
		total += 0.5 * t * t
	}
	if total > 0.5 {
		total = 0.5
	}
	return total
}

// approximateAlphaGradient computes a finite-difference gradient of
// the Alpha field projected into 3-space. Uses
// cubemap.FacePixelNeighbors4 so cross-face neighbors are sampled
// symmetrically (no seam discontinuity in the shading).
func approximateAlphaGradient(cf *CloudField, face cubemap.Face, px, py, S int) [3]float64 {
	a := cf.Alpha[face][py*S+px]
	nbrs := cubemap.FacePixelNeighbors4(face, px, py, S)
	cx, cy, cz := cubemap.FacePixelToDir(face, px, py, S)
	var grad [3]float64
	for _, n := range nbrs {
		nv := cf.Alpha[n.Face][n.PY*S+n.PX]
		nx, ny, nz := cubemap.FacePixelToDir(n.Face, n.PX, n.PY, S)
		ddx := nx - cx
		ddy := ny - cy
		ddz := nz - cz
		delta := nv - a
		grad[0] += delta * ddx
		grad[1] += delta * ddy
		grad[2] += delta * ddz
	}
	// Normalize: divide by 4 neighbors and by an approximate pixel arc length.
	s := 1.0 / 4.0 / (math.Pi / float64(S))
	grad[0] *= s
	grad[1] *= s
	grad[2] *= s
	return grad
}

func normalizeSun(s [3]float64) [3]float64 {
	if s[0] == 0 && s[1] == 0 && s[2] == 0 {
		return [3]float64{1, 0.3, 0}
	}
	n := math.Sqrt(s[0]*s[0] + s[1]*s[1] + s[2]*s[2])
	if n < 1e-9 {
		return [3]float64{1, 0.3, 0}
	}
	return [3]float64{s[0] / n, s[1] / n, s[2] / n}
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}
