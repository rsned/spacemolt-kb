package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

func TestClimateMatchesProductionFormula(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(9)
	if err != nil {
		t.Fatal(err)
	}
	if st.T == nil || st.M == nil || st.RainMult == nil {
		t.Fatal("climate layer must publish T, M, RainMult")
	}
	w := ctx.Fields.Window
	ix, iy := 11, 23
	dx, dy, dz := w.Dir(ix, iy)
	cc := ctx.Profile.ControlConfig
	tg := noise.New(seed.Domain(ctx.Master, "biome.temperature"))
	tn := tg.FractalNoise3D(dx, dy, dz, cc.Temperature.Octaves, cc.Temperature.Lacunarity, cc.Temperature.Persistence, cc.Temperature.Freq) * cc.Temperature.Amp
	lat := math.Asin(dy)
	latBias := 0.5 + 0.5*math.Cos(lat)*0.6
	want := tn*0.7 + latBias*0.3
	if want < 0 {
		want = 0
	} else if want > 1 {
		want = 1
	}
	if got := st.T.At(ix, iy); math.Abs(got-want) > 1e-12 {
		t.Fatalf("T mismatch: got %v want %v", got, want)
	}
	for _, v := range st.RainMult.Data {
		if !(v > 0 && v <= 2) {
			t.Fatalf("rain multiplier out of (0,2]: %v", v)
		}
	}
}

func TestBiomeColorProducesImage(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(10)
	if err != nil {
		t.Fatal(err)
	}
	if st.Img == nil || st.Img.Bounds().Dx() != ctx.Fields.Window.Size {
		t.Fatal("biome layer must produce a window-sized image")
	}
	// Non-degenerate: at least two distinct colors.
	first := st.Img.Pix[0:4:4]
	for i := 4; i < len(st.Img.Pix); i += 4 {
		if st.Img.Pix[i] != first[0] || st.Img.Pix[i+1] != first[1] || st.Img.Pix[i+2] != first[2] {
			return
		}
	}
	t.Fatal("biome image is a single flat color")
}
