package patch

import (
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// patchControlDomains matches field/control.go's controlFieldDomains
// for the two crust-path height contributors ("control.erosion" is
// Detail's historical domain name — do not "fix" it, it would reseed
// every planet).
var patchControlDomains = [2]struct {
	domain string
	pick   func(cc types.ControlConfig) types.ControlField
	jitter bool
}{
	{"control.erosion", func(cc types.ControlConfig) types.ControlField { return cc.Detail }, true},
	{"control.peaks-valleys", func(cc types.ControlConfig) types.ControlField { return cc.PeaksValleys }, false},
}

// applyControlNoise (layer 2): Detail + PeaksValleys fBm splines at
// true patch directions. Continentalness is skipped on the crust path
// (rocky.go L417-419).
func applyControlNoise(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	cc := ctx.Profile.ControlConfig
	ns := *st
	ns.Height = st.Height.Clone()
	for _, cd := range patchControlDomains {
		fc := cd.pick(cc)
		if fc.Amp == 0 || fc.Octaves <= 0 {
			continue
		}
		ng := noise.New(seed.Domain(ctx.Master, cd.domain))
		for iy := range w.Size {
			for ix := range w.Size {
				dx, dy, dz := w.Dir(ix, iy)
				if cd.jitter && ctx.Sphere.Jitter != nil {
					dx, dy, dz = ctx.Sphere.Jitter.Transform(dx, dy, dz)
				}
				v := ng.FractalNoise3D(dx, dy, dz, fc.Octaves, fc.Lacunarity, fc.Persistence, fc.Freq) * fc.Amp
				ns.Height.Data[iy*w.Size+ix] += planetcolor.EvalSpline(fc.Spline, v)
			}
		}
	}
	return &ns
}

// applyHeightSmooth (layer 3): flat port of field.SmoothHeightmap — a
// full (2r+1)x(2r+1) square kernel (NOT a disc: the source has no
// distance cutoff) with 1/(1+d) falloff weights, edge-clamped
// (out-of-bounds neighbors are skipped) and per-pixel renormalized.
func applyHeightSmooth(ctx *Context, st *State) *State {
	r := ctx.Profile.HeightSmoothRadius
	if r <= 0 {
		return st
	}
	size := st.Height.Size
	ns := *st
	ns.Height = NewGrid(size)
	for iy := range size {
		for ix := range size {
			var sum, wsum float64
			for oy := -r; oy <= r; oy++ {
				ny := iy + oy
				if ny < 0 || ny >= size {
					continue
				}
				for ox := -r; ox <= r; ox++ {
					nx := ix + ox
					if nx < 0 || nx >= size {
						continue
					}
					d := math.Sqrt(float64(ox*ox + oy*oy))
					wgt := 1.0 / (1.0 + d)
					sum += st.Height.At(nx, ny) * wgt
					wsum += wgt
				}
			}
			if wsum > 0 {
				ns.Height.Set(ix, iy, sum/wsum)
			}
		}
	}
	return &ns
}

// applyNormalize (layer 4): the sphere-derived affine — a patch-local
// min/max would disagree with the production render (spec §4.2). No
// clamp: out-of-range patch values are legal and harmless.
func applyNormalize(ctx *Context, st *State) *State {
	sd := ctx.Sphere
	if sd.HMax <= sd.HMin {
		return st
	}
	inv := 1 / (sd.HMax - sd.HMin)
	ns := *st
	ns.Height = NewGrid(st.Height.Size)
	for i, v := range st.Height.Data {
		ns.Height.Data[i] = (v - sd.HMin) * inv
	}
	return &ns
}
