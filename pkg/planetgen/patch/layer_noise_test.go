package patch

import (
	"math"
	"testing"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

func TestControlNoiseMatchesProductionFormula(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st1, _ := s.RenderTo(1)
	st2, err := s.RenderTo(2)
	if err != nil {
		t.Fatal(err)
	}
	// Recompute pixel (7,9) by hand with the production domains.
	w := ctx.Fields.Window
	ix, iy := 7, 9
	dx, dy, dz := w.Dir(ix, iy)
	cc := ctx.Profile.ControlConfig

	jx, jy, jz := dx, dy, dz
	if ctx.Sphere.Jitter != nil {
		jx, jy, jz = ctx.Sphere.Jitter.Transform(dx, dy, dz)
	}
	det := noise.New(seed.Domain(ctx.Master, "control.erosion"))
	dv := det.FractalNoise3D(jx, jy, jz, cc.Detail.Octaves, cc.Detail.Lacunarity, cc.Detail.Persistence, cc.Detail.Freq) * cc.Detail.Amp
	pv := noise.New(seed.Domain(ctx.Master, "control.peaks-valleys"))
	pvv := pv.FractalNoise3D(dx, dy, dz, cc.PeaksValleys.Octaves, cc.PeaksValleys.Lacunarity, cc.PeaksValleys.Persistence, cc.PeaksValleys.Freq) * cc.PeaksValleys.Amp

	want := st1.Height.At(ix, iy) +
		planetcolor.EvalSpline(cc.Detail.Spline, dv) +
		planetcolor.EvalSpline(cc.PeaksValleys.Spline, pvv)
	if got := st2.Height.At(ix, iy); math.Abs(got-want) > 1e-12 {
		t.Fatalf("control-noise pixel mismatch: got %v want %v", got, want)
	}
}

func TestNormalizeUsesSphereAffine(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st3, _ := s.RenderTo(3)
	st4, err := s.RenderTo(4)
	if err != nil {
		t.Fatal(err)
	}
	sd := ctx.Sphere
	want := (st3.Height.At(5, 5) - sd.HMin) / (sd.HMax - sd.HMin)
	if got := st4.Height.At(5, 5); math.Abs(got-want) > 1e-12 {
		t.Fatalf("normalize must use sphere HMin/HMax: got %v want %v", got, want)
	}
}

func TestHeightSmoothIsNoopAtZeroRadius(t *testing.T) {
	ctx := testContext(t)
	prof := *ctx.Profile
	prof.HeightSmoothRadius = 0
	ctx.Profile = &prof
	s := NewStack(ctx)
	st2, _ := s.RenderTo(2)
	st3, _ := s.RenderTo(3)
	for i := range st3.Height.Data {
		if st3.Height.Data[i] != st2.Height.Data[i] {
			t.Fatal("smooth with radius 0 must be identity")
		}
	}
}
