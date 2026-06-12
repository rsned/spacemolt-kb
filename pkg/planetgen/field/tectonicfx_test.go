package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestClassifyTectonicsPartitionsBoundaries(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	if fx == nil {
		t.Fatal("nil TectonicFXField")
	}
	// Every convergent boundary pixel must be claimed by exactly one of
	// belt / subduction / arc (distance 0 in exactly one field).
	const eps = 1e-9
	for _, bp := range pf.ConvPixels {
		claimed := 0
		for _, f := range []*cubemap.CubeMapF{fx.BeltDist, fx.SubdDist, fx.ArcDist} {
			if f.Faces[bp.Face][bp.Idx] < eps {
				claimed++
			}
		}
		if claimed != 1 {
			t.Fatalf("conv pixel face %d idx %d claimed by %d classes, want 1", bp.Face, bp.Idx, claimed)
		}
	}
	for _, bp := range pf.DivPixels {
		claimed := 0
		for _, f := range []*cubemap.CubeMapF{fx.RidgeDist, fx.RiftDist} {
			if f.Faces[bp.Face][bp.Idx] < eps {
				claimed++
			}
		}
		if claimed != 1 {
			t.Fatalf("div pixel face %d idx %d claimed by %d classes, want 1", bp.Face, bp.Idx, claimed)
		}
	}
}

func TestClassifyTectonicsBeltsTouchContinents(t *testing.T) {
	// Belt source pixels (dist 0) must sit where the continental mask is
	// high on BOTH sides — sample the mask at the pixel itself and assert
	// it is at least moderately continental.
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0 // supercontinent maximizes cont-cont collisions
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	const eps = 1e-9
	checked := 0
	for _, bp := range pf.ConvPixels {
		if fx.BeltDist.Faces[bp.Face][bp.Idx] >= eps {
			continue
		}
		if m := crust.ContinentalMask.Faces[bp.Face][bp.Idx]; m < 0.25 {
			t.Errorf("belt pixel face %d idx %d sits on mask %v < 0.25", bp.Face, bp.Idx, m)
		}
		checked++
	}
	t.Logf("checked %d belt pixels", checked)
}

func defaultFXCfg() types.TectonicFXConfig {
	return types.TectonicFXConfig{
		BeltAmp: 0.30, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.10, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
}

func TestApplyTectonicFXRaisesBelts(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)
	hm := cubemap.NewF(S) // flat zero heightmap isolates the FX deltas
	ApplyTectonicFX(hm, fx, crust, pf, defaultFXCfg(), 42, S)

	var beltSum float64
	var beltN int
	var farSum float64
	var farN int
	for f := range hm.Faces {
		for i, h := range hm.Faces[f] {
			switch {
			case fx.BeltDist.Faces[f][i] < 300:
				beltSum += h
				beltN++
			case fx.BeltDist.Faces[f][i] > 4000 && fx.RiftDist.Faces[f][i] > 4000 &&
				fx.SubdDist.Faces[f][i] > 4000 && fx.ArcDist.Faces[f][i] > 4000 &&
				fx.RidgeDist.Faces[f][i] > 4000:
				farSum += h
				farN++
			}
		}
	}
	if beltN == 0 || farN == 0 {
		t.Skip("seed produced no belt pixels at this size; acceptable for a single fixed seed")
	}
	beltMean := beltSum / float64(beltN)
	farMean := farSum / float64(farN)
	if beltMean <= farMean+0.02 {
		t.Errorf("belt mean Δh %v not above far-field %v + 0.02", beltMean, farMean)
	}
}

func TestApplyTectonicFXAgeSoftens(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	p.Crust.Assembly = 0
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	fx := ClassifyTectonics(pf, crust, 6371)

	maxAbs := func(age float64) float64 {
		c := *crust
		c.TectonicAge = age
		hm := cubemap.NewF(S)
		ApplyTectonicFX(hm, fx, &c, pf, defaultFXCfg(), 42, S)
		mx := 0.0
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if a := math.Abs(h); a > mx {
					mx = a
				}
			}
		}
		return mx
	}
	young := maxAbs(0.0)
	old := maxAbs(1.0)
	if old >= young {
		t.Errorf("old-planet max relief %v not below young %v", old, young)
	}
}
