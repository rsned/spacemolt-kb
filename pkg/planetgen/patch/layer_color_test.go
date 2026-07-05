package patch

import "testing"

func TestWaterlinesPaintsOcean(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(11)
	if err != nil {
		t.Fatal(err)
	}
	sea := ctx.Sphere.SeaLevel
	oc := ctx.Profile.OceanColor
	size := st.Height.Size
	checked := 0
	for iy := 0; iy < size; iy += 7 {
		for ix := 0; ix < size; ix += 7 {
			if st.Height.At(ix, iy) < sea*0.5 { // deep ocean only
				o := st.Img.PixOffset(ix, iy)
				// Deep ocean pixels must be recognizably ocean-hued:
				// the dominant channel of OceanColor stays dominant.
				if oc.B > oc.R && st.Img.Pix[o+2] <= st.Img.Pix[o] {
					t.Fatalf("deep ocean pixel (%d,%d) not ocean-colored", ix, iy)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Skip("window has no deep ocean at this seed")
	}
}

func TestSeaLevelViewOverride(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	base, _ := s.RenderTo(11)
	ctx.SeaLevelView = 0.95 // drown almost everything
	s.MarkDirty("seaLevelView")
	flooded, err := s.RenderTo(11)
	if err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := 0; i < len(base.Img.Pix); i += 4 {
		if base.Img.Pix[i] != flooded.Img.Pix[i] || base.Img.Pix[i+1] != flooded.Img.Pix[i+1] {
			diff++
		}
	}
	if diff < len(base.Img.Pix)/4/10 {
		t.Fatalf("raising sea level to 0.95 changed only %d pixels", diff)
	}
}
