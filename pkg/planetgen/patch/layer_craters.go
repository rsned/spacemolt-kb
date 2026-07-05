package patch

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
)

func cratersEnabled(ctx *Context) bool { return ctx.Profile.CraterCount > 0 }

// applyCraters (layer 7): the production global crater list
// (feature.GenerateCraters — the SAME deterministic list production
// stamps on the full sphere), stamped only where it intersects the
// window. Bowl/rim profile is transcribed verbatim from
// feature.ApplyCraters (crater.go:187) so a patch render and the
// corresponding crop of a full production render agree pixel-for-pixel.
func applyCraters(ctx *Context, st *State) *State {
	all := feature.GenerateCraters(ctx.Master, ctx.Profile)
	w := ctx.Fields.Window
	ns := *st
	ns.Height = st.Height.Clone()
	depth := ctx.Profile.CraterDepth
	var kept []feature.Crater
	for _, cr := range all {
		cx := math.Cos(cr.Lat) * math.Cos(cr.Lon)
		cy := math.Sin(cr.Lat)
		cz := math.Cos(cr.Lat) * math.Sin(cr.Lon)
		// Cheap window cull: compare against the window center dir.
		mx, my, mz := w.Dir(w.Size/2, w.Size/2)
		halfDiag := float64(w.Size) / float64(w.SProd) * (math.Pi / 2) // window half-diagonal upper bound, radians
		if math.Acos(clampDot(cx*mx+cy*my+cz*mz)) > cr.Radius+halfDiag {
			continue
		}
		kept = append(kept, cr)
		for iy := range w.Size {
			for ix := range w.Size {
				dx, dy, dz := w.Dir(ix, iy)
				ang := math.Acos(clampDot(dx*cx + dy*cy + dz*cz))
				if ang >= cr.Radius {
					continue
				}
				idx := iy*w.Size + ix
				h := ns.Height.Data[idx] + craterDeltaLikeApplyCraters(ang, cr, depth)
				// Mirror feature.ApplyCraters' per-pixel-per-crater clamp
				// to [0,1] so cumulative overlapping craters behave
				// identically to the production stamping order.
				if h < 0 {
					h = 0
				} else if h > 1 {
					h = 1
				}
				ns.Height.Data[idx] = h
			}
		}
	}
	ns.Craters = kept
	return &ns
}

// craterDeltaLikeApplyCraters reproduces the exact per-pixel bowl/rim
// height delta computed by feature.ApplyCraters (crater.go:187):
//
//   - effDepth = depth * Age^2 (per-crater depth attenuated by age² so
//     older craters look softer; legacy craters have Age == 1).
//   - t = ang / cr.Radius (fractional distance from center, in [0,1)
//     since the caller has already excluded ang >= cr.Radius).
//   - t < 0.8: parabolic bowl, deepest (-effDepth) at the center,
//     tapering to 0 at t == 0.8.
//   - t >= 0.8: raised rim, a half-sine bump peaking at
//     effDepth*0.15 midway through the rim band and returning to 0 at
//     the crater edge.
func craterDeltaLikeApplyCraters(ang float64, cr feature.Crater, depth float64) float64 {
	ageScale := cr.Age * cr.Age
	effDepth := depth * ageScale
	t := ang / cr.Radius
	if t < 0.8 {
		return -effDepth * (1 - (t/0.8)*(t/0.8))
	}
	rimT := (t - 0.8) / 0.2
	return effDepth * 0.15 * math.Sin(rimT*math.Pi)
}

func clampDot(d float64) float64 { return math.Max(-1, math.Min(1, d)) }
