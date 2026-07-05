package patch

import "testing"

func testContext(t *testing.T) *Context {
	t.Helper()
	sd := testSphere(t)
	w := Pick(sd, 64, 128, 1)[0].Window
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	return &Context{Sphere: sd, Fields: f, Profile: sd.Profile, Master: sd.Master}
}

func TestLayerBaseCopiesBaseHeight(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Height.At(3, 3) != ctx.Fields.BaseHeight.At(3, 3) {
		t.Fatal("layer 0 must initialize Height from the BaseHeight crop")
	}
}

func TestLayerFXChangesHeightNearBoundaries(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st0, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	st1, err := s.RenderTo(1)
	if err != nil {
		t.Fatal(err)
	}
	diff := 0
	for i := range st1.Height.Data {
		if st1.Height.Data[i] != st0.Height.Data[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("tectonic FX changed nothing — the picked window should contain active boundaries")
	}
	// Immutability: layer 1 must not have mutated layer 0's cache.
	st0b, _ := s.RenderTo(0)
	if st0b != st0 {
		t.Fatal("cache identity broken")
	}
}
