package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
)

// TestPatchSphereConsistency pins the "patch is a true crop" property
// (spec §6.2): with sTect == sProd, window pixel dirs are exactly face
// pixel centers, bilinear Sample degenerates to exact reads, and
// layers 0-1 must match a sphere-side base+FX computation bit-for-bit.
func TestPatchSphereConsistency(t *testing.T) {
	prof := terranProfile(t)
	const S = 64
	master := int64(777)
	sd, err := ComputeSphere(prof, master, S)
	if err != nil {
		t.Fatal(err)
	}
	w := Window{Face: cubemap.FacePosX, X0: 16, Y0: 16, Size: 32, SProd: S}
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStack(&Context{Sphere: sd, Fields: f, Profile: prof, Master: master})
	st, err := s.RenderTo(1)
	if err != nil {
		t.Fatal(err)
	}

	// Sphere side: BaseHeight + ApplyTectonicFX at the same S.
	hm := sd.Crust.BaseHeight.Clone()
	field.ApplyTectonicFX(hm, sd.FX, sd.Crust, sd.Plates, prof.TectonicFX, master, S)

	worst := 0.0
	for iy := range w.Size {
		for ix := range w.Size {
			want := hm.Get(w.Face, w.X0+ix, w.Y0+iy)
			got := st.Height.At(ix, iy)
			if d := math.Abs(got - want); d > worst {
				worst = d
			}
		}
	}
	if worst > 1e-9 {
		t.Fatalf("patch layers 0-1 diverge from sphere crop: worst |Δ| = %g", worst)
	}
}
