package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
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
