package tectonics

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func testParams(t *testing.T, S int) Params {
	t.Helper()
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face = S
	p.ErosionRate = 0 // step tests assert pre-erosion behaviour; erosion has its own tests
	return p
}

func maxNeighborDelta(th *Grid[float64]) float64 {
	nb := Neighbors4(th.S)
	worst := 0.0
	for i := range th.Len() {
		for _, j := range nb[i] {
			worst = math.Max(worst, math.Abs(th.Cells[i]-th.Cells[j]))
		}
	}
	return worst
}

func TestThicknessBoundedSmoothAndVaried(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 42)
	lo, hi, sum := 1.0, 0.0, 0.0
	for _, v := range th.Cells {
		lo, hi, sum = math.Min(lo, v), math.Max(hi, v), sum+v
	}
	if lo < 0 || hi > 1 || hi-lo < 0.5 {
		t.Errorf("range [%g,%g]", lo, hi)
	}
	mean := sum / float64(th.Len())
	variance := 0.0
	for _, v := range th.Cells {
		variance += (v - mean) * (v - mean)
	}
	if variance/float64(th.Len()) < 0.01 {
		t.Errorf("field is nearly flat, variance %g", variance/float64(th.Len()))
	}
	if d := maxNeighborDelta(th); d > p.MaxNeighborDelta+1e-9 {
		t.Errorf("neighbour delta %g exceeds %g", d, p.MaxNeighborDelta)
	}
}

func TestThicknessDeterministic(t *testing.T) {
	p := testParams(t, 16)
	a, b := GenerateThickness(p, 7), GenerateThickness(p, 7)
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Fatalf("pixel %d differs", i)
		}
	}
	c := GenerateThickness(p, 8)
	same := 0
	for i := range a.Cells {
		if a.Cells[i] == c.Cells[i] {
			same++
		}
	}
	if same == len(a.Cells) {
		t.Error("different seeds gave identical fields")
	}
}

func TestRelaxEnforcesBoundAndConverges(t *testing.T) {
	th := NewGrid[float64](8)
	th.Cells[0] = 1 // a single spike
	for Relax(th, 0.05, 1) > 0 {
	}
	if d := maxNeighborDelta(th); d > 0.05+1e-9 {
		t.Errorf("delta %g after relax", d)
	}
	for _, v := range th.Cells {
		if v < 0 || v > 1 {
			t.Fatalf("value %g escaped [0,1]", v)
		}
	}
}

func landShareByArea(th *Grid[float64], threshold float64) float64 {
	w := AreaWeights(th.S)
	land := 0.0
	for i, v := range th.Cells {
		if v >= threshold {
			land += w[i]
		}
	}
	return land / (4 * math.Pi)
}

func TestRemapLandShare(t *testing.T) {
	p := testParams(t, 32)
	th := NewGrid[float64](32)
	rng := newRNG(7, "test.remap")
	for i := range th.Cells {
		th.Cells[i] = rng.Float64()
	}
	normalize(th)
	remapBands(th, p)
	if got := landShareByArea(th, p.ContinentalThreshold); math.Abs(got-p.LandShareT0) > 0.01 {
		t.Fatalf("land share %.3f, want %.2f ± 0.01", got, p.LandShareT0)
	}
	for i, v := range th.Cells {
		if v < p.ContinentalThreshold && v > p.OceanBandMax+1e-12 {
			t.Fatalf("pixel %d: ocean thickness %.4f above OceanBandMax", i, v)
		}
		if v >= p.ContinentalThreshold && v < p.LandBandMin-1e-12 {
			t.Fatalf("pixel %d: continent thickness %.4f below LandBandMin", i, v)
		}
		if v < p.RidgeThickness-1e-12 || v > 1+1e-12 {
			t.Fatalf("pixel %d: thickness %.4f outside [RidgeThickness, 1]", i, v)
		}
	}
	for range 200 {
		if Relax(th, p.MaxNeighborDelta, 1) == 0 {
			break
		}
	}
	if d := maxNeighborDelta(th); d > p.MaxNeighborDelta+1e-9 {
		t.Fatalf("gradient cap violated after relax: %.4f", d)
	}
}

func TestGenerateThicknessStartsWithContinents(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 2026)
	got := landShareByArea(th, p.ContinentalThreshold)
	if math.Abs(got-p.LandShareT0) > 0.05 {
		t.Fatalf("land share after remap+relax %.3f, want %.2f ± 0.05", got, p.LandShareT0)
	}

	elev := Elevation(th, InitialAges(th, p), p)
	writeVis(t, "thickness_remap_continents", renderIso(th, cubemap.FacePosZ, p.ContinentalThreshold, 60),
		renderIso(elev, cubemap.FacePosZ, 0, 6))
}
