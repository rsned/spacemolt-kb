package tectonics

import "testing"

func TestBasinsCoverEveryPixel(t *testing.T) {
	p := testParams(t, 24)
	th := GenerateThickness(p, 3)
	labels, n := basins(th)
	if n < 2 {
		t.Fatalf("only %d basins", n)
	}
	for i, l := range labels.Cells {
		if l < 0 || int(l) >= n {
			t.Fatalf("pixel %d label %d of %d", i, l, n)
		}
	}
}

func TestBuildPlatesCountsAreasAndThinBoundaries(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 11)
	labels, plates := BuildPlates(th, p, 11)
	if n := len(plates); n < p.MajorMin+p.MinorMin || n > p.MajorMax+p.MinorMax {
		t.Fatalf("%d plates, want %d..%d", n, p.MajorMin+p.MinorMin, p.MajorMax+p.MinorMax)
	}
	majors, total := 0, 0.0
	for i, pl := range plates {
		if pl.ID != int32(i) {
			t.Errorf("plate %d has id %d", i, pl.ID)
		}
		if pl.Major {
			majors++
		}
		if pl.Area < p.MinPlateArea {
			t.Errorf("plate %d area %g below minimum", i, pl.Area)
		}
		total += pl.Area
	}
	if majors < p.MajorMin || majors > p.MajorMax {
		t.Errorf("%d majors", majors)
	}
	if total < 0.999 || total > 1.001 {
		t.Errorf("areas sum to %g", total)
	}
	if plates[0].Area < plates[len(plates)-1].Area {
		t.Error("plates not sorted largest first")
	}
	if plates[0].Area >= 0.5 {
		t.Errorf("largest plate covers %g of the sphere, want < 0.5", plates[0].Area)
	}
	// the dominant-plate phase lifts the largest plate to a seeded share of
	// DominantMin..DominantMax (Earth keeps one plate near 0.3)
	if plates[0].Area < p.DominantMin-0.02 {
		t.Errorf("largest plate %g below the dominant share floor %g", plates[0].Area, p.DominantMin)
	}
	hasMinor := false
	for _, pl := range plates {
		if !pl.Major {
			hasMinor = true
			break
		}
	}
	if !hasMinor {
		t.Error("no minor plates")
	}
	// boundaries lie on thin crust: mean thickness of boundary pixels is
	// below the mean of the whole field.
	nb := Neighbors4(th.S)
	bsum, bn, sum := 0.0, 0, 0.0
	for i := range th.Len() {
		sum += th.Cells[i]
		for _, j := range nb[i] {
			if labels.Cells[j] != labels.Cells[i] {
				bsum += th.Cells[i]
				bn++
				break
			}
		}
	}
	if bn == 0 || bsum/float64(bn) >= sum/float64(th.Len()) {
		t.Errorf("boundary mean %g not below field mean %g", bsum/float64(bn), sum/float64(th.Len()))
	}
}

func TestBuildPlatesSeamBlind(t *testing.T) {
	// Boundary density on face-edge pixels must not be wildly higher than
	// in face interiors; a seam bug would show as plates cut along cube edges.
	p := testParams(t, 32)
	th := GenerateThickness(p, 5)
	labels, _ := BuildPlates(th, p, 5)
	nb := Neighbors4(th.S)
	var edgeB, edgeN, inB, inN int
	for i := range th.Len() {
		_, px, py := th.Addr(i)
		edge := px == 0 || py == 0 || px == th.S-1 || py == th.S-1
		isB := false
		for _, j := range nb[i] {
			if labels.Cells[j] != labels.Cells[i] {
				isB = true
			}
		}
		if edge {
			edgeN++
			if isB {
				edgeB++
			}
		} else {
			inN++
			if isB {
				inB++
			}
		}
	}
	edgeRate := float64(edgeB) / float64(edgeN)
	inRate := float64(inB) / float64(inN)
	if edgeRate > 3*inRate+0.05 {
		t.Errorf("boundary rate on seams %.3f vs interior %.3f", edgeRate, inRate)
	}
}
