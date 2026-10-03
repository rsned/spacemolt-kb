package tectonics

import (
	"math"
	"testing"
)

func testParams(t *testing.T, S int) Params {
	t.Helper()
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face = S
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
