package patch

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Fields is the per-pixel patch-extraction contract (spec §4.2): the
// sphere fields downstream layers consume, bilinearly upsampled from
// S_tect at each patch pixel's true direction. PlateID is
// nearest-neighbor (categorical, debug overlay only).
type Fields struct {
	Window Window

	BaseHeight, ContinentalMask *Grid

	BeltDist, BeltMag   *Grid
	SubdDist, SubdMag   *Grid
	ArcDist, ArcMag     *Grid
	RidgeDist, RidgeMag *Grid
	RiftDist, RiftMag   *Grid
	Transform           *Grid

	PlateID []int16 // row-major Size×Size
}

// wrapF wraps raw per-face float slices (PlateField SDF layout) in a
// CubeMapF so we can reuse its bilinear direction Sample.
func wrapF(size int, faces [cubemap.NumFaces][]float64) *cubemap.CubeMapF {
	return &cubemap.CubeMapF{Size: size, Faces: faces}
}

func cropF(src *cubemap.CubeMapF, w Window) *Grid {
	g := NewGrid(w.Size)
	for iy := range w.Size {
		for ix := range w.Size {
			x, y, z := w.Dir(ix, iy)
			g.Set(ix, iy, src.Sample(x, y, z))
		}
	}
	return g
}

func ExtractFields(sd *SphereData, w Window) (*Fields, error) {
	if err := w.Valid(); err != nil {
		return nil, err
	}
	f := &Fields{Window: w}
	f.BaseHeight = cropF(sd.Crust.BaseHeight, w)
	f.ContinentalMask = cropF(sd.Crust.ContinentalMask, w)
	f.BeltDist = cropF(sd.FX.BeltDist, w)
	f.BeltMag = cropF(sd.FX.BeltMag, w)
	f.SubdDist = cropF(sd.FX.SubdDist, w)
	f.SubdMag = cropF(sd.FX.SubdMag, w)
	f.ArcDist = cropF(sd.FX.ArcDist, w)
	f.ArcMag = cropF(sd.FX.ArcMag, w)
	f.RidgeDist = cropF(sd.FX.RidgeDist, w)
	f.RidgeMag = cropF(sd.FX.RidgeMag, w)
	f.RiftDist = cropF(sd.FX.RiftDist, w)
	f.RiftMag = cropF(sd.FX.RiftMag, w)
	f.Transform = cropF(wrapF(sd.STect, sd.Plates.Transform), w)

	f.PlateID = make([]int16, w.Size*w.Size)
	for iy := range w.Size {
		for ix := range w.Size {
			x, y, z := w.Dir(ix, iy)
			face, px, py := cubemap.DirToFacePixel(x, y, z, sd.STect)
			f.PlateID[iy*w.Size+ix] = sd.Plates.PlateID[face][py*sd.STect+px]
		}
	}
	return f, nil
}
