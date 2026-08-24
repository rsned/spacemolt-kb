// Crust-aware boundary effects (Phase 12): classify each plate-boundary
// pixel by the continental crust on its two sides, then JFA-propagate
// per-class distance + magnitude fields consumed by ApplyTectonicFX.

package field

import (
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
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

	// contThresh 0.5 is the standard land/ocean cutoff of the continental
	// mask: a side counts as continental only when it is more land than
	// not, so belts (both sides continental) require genuine craton on
	// each flank rather than incidental shelf.
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
	// riftThresh 0.35 is deliberately below contThresh: continental rifts
	// form wherever divergence tears even partial craton coverage (the
	// thinned, not-yet-fully-continental margin of a splitting landmass),
	// so we route a divergent pixel to "rift" on weaker continental
	// evidence than a belt requires; everything else is a mid-ocean ridge.
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

// FXSample is the per-pixel bundle of tectonic fields FXDelta reads.
// On the cube path the values come straight from the field rasters; on
// the Patch Lab path they are bilinear crops upsampled at patch dirs.
type FXSample struct {
	BeltDist, BeltMag   float64
	SubdDist, SubdMag   float64
	ArcDist, ArcMag     float64
	RidgeDist, RidgeMag float64
	RiftDist, RiftMag   float64
	TransformDist       float64
	ContinentalMask     float64
}

// FXGens bundles the two seeded generators the FX pass consumes.
type FXGens struct {
	Act, Belt *noise.Generator
}

// NewFXGens seeds the two generators FXDelta consumes, domain-separated
// off master exactly as ApplyTectonicFX did inline.
func NewFXGens(master int64) *FXGens {
	return &FXGens{
		Act:  noise.New(seed.Domain(master, "tectonicfx.activity")),
		Belt: noise.New(seed.Domain(master, "tectonicfx.belt")),
	}
}

// FXDelta returns the tectonic-FX height delta at one pixel. It is the
// exact per-pixel body of ApplyTectonicFX, extracted so the Patch Lab
// flat path evaluates the identical formula.
func FXDelta(dx, dy, dz float64, s FXSample, cfg types.TectonicFXConfig, age float64, g *FXGens) float64 {
	hs := 1.0 - 0.55*age // height scale: 1.0 young → 0.45 old
	ws := 1.0 + 0.8*age  // width scale:  1.0 young → 1.8 old
	actFreq := cfg.ActivityFreq
	if actFreq <= 0 {
		actFreq = 1.5
	}
	beltOct := cfg.BeltOctaves
	if beltOct <= 0 {
		beltOct = 5
	}
	// Gaussian envelope, cut at 3 sigma to skip most pixels cheaply.
	env := func(distKm, widthKm float64) float64 {
		if widthKm <= 0 || distKm > 3*widthKm {
			return 0
		}
		x := distKm / widthKm
		return math.Exp(-x * x)
	}
	// Lazy activity: the fBm is a pure function of the pixel direction,
	// and only the belt and cordillera branches consume it — those fire
	// on a small fraction of pixels, so computing it up front for every
	// pixel wastes the bulk of the pass.
	activity := -1.0
	getActivity := func() float64 {
		if activity < 0 {
			activity = 0.5 + 0.5*g.Act.FractalNoise3D(dx, dy, dz, 2, 2.0, 0.5, actFreq)
		}
		return activity
	}
	contHere := s.ContinentalMask
	var dh float64

	// 1. Collision belt (cont-cont): ridged relief inside a
	// wide envelope straddling the suture.
	if cfg.BeltAmp > 0 {
		if e := env(s.BeltDist, cfg.BeltWidthKm*ws); e > 0 {
			r := g.Belt.RidgedFractal3D(dx*cfg.BeltFreq, dy*cfg.BeltFreq, dz*cfg.BeltFreq, beltOct, 2.0, 0.5, 1.0)
			mag := 0.4 + 0.6*s.BeltMag
			dh += cfg.BeltAmp * hs * e * mag * getActivity() * r
		}
	}

	// 2+3. Subduction (ocean-cont): cordillera on the
	// continent side, trench on the ocean side.
	if cfg.CordAmp > 0 || cfg.TrenchDepth > 0 {
		d := s.SubdDist
		mag := s.SubdMag
		if contHere > 0.5 {
			if e := env(d, cfg.CordWidthKm*ws); e > 0 {
				r := g.Belt.RidgedFractal3D(dx*cfg.BeltFreq*1.4, dy*cfg.BeltFreq*1.4, dz*cfg.BeltFreq*1.4, beltOct, 2.0, 0.5, 1.0)
				dh += cfg.CordAmp * hs * e * (0.4 + 0.6*mag) * getActivity() * r
			}
		} else {
			if e := env(d, cfg.TrenchWidthKm); e > 0 {
				dh -= cfg.TrenchDepth * e * (0.4 + 0.6*mag)
			}
		}
	}

	// 4. Island arc (oce-oce): trench-adjacent dotted islands
	// gated by a mid-frequency noise so the arc is a chain,
	// not a wall.
	if cfg.ArcAmp > 0 && contHere < 0.5 {
		if e := env(s.ArcDist, cfg.ArcWidthKm); e > 0 {
			islands := smoothstep(0.55, 0.72, g.Act.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 8.0))
			mag := 0.4 + 0.6*s.ArcMag
			dh += cfg.ArcAmp * hs * e * mag * islands
		}
	}

	// 5a. Mid-ocean ridge: gentle bathymetric rise.
	if cfg.RidgeAmp > 0 && contHere < 0.5 {
		if e := env(s.RidgeDist, cfg.RidgeWidthKm); e > 0 {
			dh += cfg.RidgeAmp * e * (0.4 + 0.6*s.RidgeMag)
		}
	}

	// 5b. Continental rift: floor depression + shoulder uplift.
	if cfg.RiftDepth > 0 {
		d := s.RiftDist
		w := cfg.RiftWidthKm * ws
		mag := 0.4 + 0.6*s.RiftMag
		if e := env(d, w); e > 0 {
			// Age deepens rifts (mature rift → Red Sea), unlike
			// belts which age erodes: scale by (0.5 + 0.5·age).
			dh -= cfg.RiftDepth * (0.5 + 0.5*age) * e * mag
		}
		if cfg.RiftShoulder > 0 {
			sd := d - 1.6*w
			if e := env(math.Abs(sd), w*0.7); e > 0 {
				dh += cfg.RiftDepth * cfg.RiftShoulder * e * mag
			}
		}
	}

	// 6. Transform faults: small-scale roughness near the
	// existing transform SDF.
	if cfg.TransformAmp > 0 {
		if e := env(s.TransformDist, cfg.TransformWidthKm); e > 0 {
			n := g.Act.FractalNoise3D(dx, dy, dz, 3, 2.0, 0.5, 12.0) - 0.5
			dh += cfg.TransformAmp * e * n
		}
	}

	return dh
}

// ApplyTectonicFX adds the six crust-aware boundary effects to the
// heightmap in place. TectonicAge (from crust) scales amplitude down
// and width up so old planets read wide-and-rounded (Appalachians)
// while young ones read tall-and-sharp (Himalayas). A low-frequency
// activity noise varies energy between boundaries so they don't all
// look equally violent. Deterministic; no internal state survives.
func ApplyTectonicFX(hm *cubemap.CubeMapF, fx *TectonicFXField, crust *CrustField,
	pf *PlateField, cfg types.TectonicFXConfig, master int64, S int) {
	if hm == nil || fx == nil || crust == nil || pf == nil {
		return
	}
	g := NewFXGens(master)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range S {
			for px := range S {
				i := py*S + px
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				s := FXSample{
					BeltDist: fx.BeltDist.Faces[face][i], BeltMag: fx.BeltMag.Faces[face][i],
					SubdDist: fx.SubdDist.Faces[face][i], SubdMag: fx.SubdMag.Faces[face][i],
					ArcDist: fx.ArcDist.Faces[face][i], ArcMag: fx.ArcMag.Faces[face][i],
					RidgeDist: fx.RidgeDist.Faces[face][i], RidgeMag: fx.RidgeMag.Faces[face][i],
					RiftDist: fx.RiftDist.Faces[face][i], RiftMag: fx.RiftMag.Faces[face][i],
					TransformDist:   pf.Transform[face][i],
					ContinentalMask: crust.ContinentalMask.Faces[face][i],
				}
				if dh := FXDelta(dx, dy, dz, s, cfg, crust.TectonicAge, g); dh != 0 {
					hm.Faces[face][i] += dh
				}
			}
		}
	}
}
