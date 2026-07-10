package patch

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// applyTectonicFX (layer 1) evaluates the production FXDelta formula
// per patch pixel against the cropped Dist/Mag fields. All FX params
// and TectonicAge behave exactly as on the sphere; only the field
// resolution (upsampled from S_tect) differs.
func applyTectonicFX(ctx *Context, st *State) *State {
	f := ctx.Fields
	w := f.Window
	cfg := ctx.Profile.TectonicFX
	age := ctx.Sphere.Crust.TectonicAge
	g := field.NewFXGens(ctx.Master)

	ns := *st
	ns.Height = st.Height.Clone()
	for iy := range w.Size {
		for ix := range w.Size {
			i := iy*w.Size + ix
			dx, dy, dz := w.Dir(ix, iy)
			s := field.FXSample{
				BeltDist: f.BeltDist.Data[i], BeltMag: f.BeltMag.Data[i],
				SubdDist: f.SubdDist.Data[i], SubdMag: f.SubdMag.Data[i],
				ArcDist: f.ArcDist.Data[i], ArcMag: f.ArcMag.Data[i],
				RidgeDist: f.RidgeDist.Data[i], RidgeMag: f.RidgeMag.Data[i],
				RiftDist: f.RiftDist.Data[i], RiftMag: f.RiftMag.Data[i],
				TransformDist:   f.Transform.Data[i],
				ContinentalMask: f.ContinentalMask.Data[i],
			}
			ns.Height.Data[i] += field.FXDelta(dx, dy, dz, s, cfg, age, g)
		}
	}
	return &ns
}
