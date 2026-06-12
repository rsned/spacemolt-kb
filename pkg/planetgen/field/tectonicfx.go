// Crust-aware boundary effects (Phase 12): classify each plate-boundary
// pixel by the continental crust on its two sides, then JFA-propagate
// per-class distance + magnitude fields consumed by ApplyTectonicFX.
package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// TectonicFXField holds five (distance-km, magnitude) field pairs, one
// per crust-aware boundary class. Distances follow the PlateField
// convention: geodesic km, half-circumference where a face has no
// source pixel of that class.
type TectonicFXField struct {
	Size      int
	BeltDist  *cubemap.CubeMapF // convergent cont-cont
	BeltMag   *cubemap.CubeMapF
	SubdDist  *cubemap.CubeMapF // convergent ocean-cont (trench + cordillera)
	SubdMag   *cubemap.CubeMapF
	ArcDist   *cubemap.CubeMapF // convergent oce-oce (trench + island arc)
	ArcMag    *cubemap.CubeMapF
	RidgeDist *cubemap.CubeMapF // divergent in ocean (mid-ocean ridge)
	RidgeMag  *cubemap.CubeMapF
	RiftDist  *cubemap.CubeMapF // divergent under/between cratons
	RiftMag   *cubemap.CubeMapF
}

// crustPairSampleOffset is the sampling offset (in direction-space
// units, ~4 pixels at the working resolution) used to look at the
// crust on each side of a boundary pixel.
func crustPairSampleOffset(S int) float64 { return 4.0 / float64(S) }

// ClassifyTectonics splits the convergent boundary pixels into belt /
// subduction / arc and the divergent ones into rift / ridge by
// sampling the continental mask a few pixels to each side along the
// smoothed boundary normal, then JFA-propagates each class.
func ClassifyTectonics(pf *PlateField, crust *CrustField, radiusKm float64) *TectonicFXField {
	if pf == nil || crust == nil {
		return nil
	}
	S := pf.Size
	if radiusKm == 0 {
		radiusKm = 6371
	}
	delta := crustPairSampleOffset(S)

	var beltM, subdM, arcM, ridgeM, riftM [cubemap.NumFaces][]bool
	var beltV, subdV, arcV, ridgeV, riftV [cubemap.NumFaces][]float64
	for f := range beltM {
		beltM[f] = make([]bool, S*S)
		subdM[f] = make([]bool, S*S)
		arcM[f] = make([]bool, S*S)
		ridgeM[f] = make([]bool, S*S)
		riftM[f] = make([]bool, S*S)
		beltV[f] = make([]float64, S*S)
		subdV[f] = make([]float64, S*S)
		arcV[f] = make([]float64, S*S)
		ridgeV[f] = make([]float64, S*S)
		riftV[f] = make([]float64, S*S)
	}

	sideMasks := func(bp BoundaryPixel) (here, there float64) {
		px := bp.Idx % S
		py := bp.Idx / S
		dx, dy, dz := cubemap.FacePixelToDir(bp.Face, px, py, S)
		here = crust.ContinentalMask.Sample(dx-bp.N[0]*delta, dy-bp.N[1]*delta, dz-bp.N[2]*delta)
		there = crust.ContinentalMask.Sample(dx+bp.N[0]*delta, dy+bp.N[1]*delta, dz+bp.N[2]*delta)
		return here, there
	}

	const contThresh = 0.5
	for _, bp := range pf.ConvPixels {
		a, b := sideMasks(bp)
		switch {
		case a > contThresh && b > contThresh:
			beltM[bp.Face][bp.Idx] = true
			beltV[bp.Face][bp.Idx] = bp.Mag
		case a > contThresh || b > contThresh:
			subdM[bp.Face][bp.Idx] = true
			subdV[bp.Face][bp.Idx] = bp.Mag
		default:
			arcM[bp.Face][bp.Idx] = true
			arcV[bp.Face][bp.Idx] = bp.Mag
		}
	}
	const riftThresh = 0.35
	for _, bp := range pf.DivPixels {
		a, b := sideMasks(bp)
		if math.Max(a, b) > riftThresh {
			riftM[bp.Face][bp.Idx] = true
			riftV[bp.Face][bp.Idx] = bp.Mag
		} else {
			ridgeM[bp.Face][bp.Idx] = true
			ridgeV[bp.Face][bp.Idx] = bp.Mag
		}
	}

	factor := math.Pi * radiusKm
	scaleKm := func(f *cubemap.CubeMapF) *cubemap.CubeMapF {
		for i := range f.Faces {
			for j := range f.Faces[i] {
				f.Faces[i][j] *= factor
			}
		}
		return f
	}
	fx := &TectonicFXField{Size: S}
	var d, m *cubemap.CubeMapF
	d, m = JumpFloodFromMaskWithValue(beltM, beltV, S)
	fx.BeltDist, fx.BeltMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(subdM, subdV, S)
	fx.SubdDist, fx.SubdMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(arcM, arcV, S)
	fx.ArcDist, fx.ArcMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(ridgeM, ridgeV, S)
	fx.RidgeDist, fx.RidgeMag = scaleKm(d), m
	d, m = JumpFloodFromMaskWithValue(riftM, riftV, S)
	fx.RiftDist, fx.RiftMag = scaleKm(d), m
	return fx
}
