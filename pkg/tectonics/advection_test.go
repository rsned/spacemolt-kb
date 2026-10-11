package tectonics

import (
	"math"
	"testing"
)

// jaggedHemispheres labels the sphere north/south of a wobbly equator so
// the boundary is a staircase, like watershed plates.
func jaggedHemispheres(S int, th0 float64) (*Grid[float64], *Grid[int32], []Plate) {
	th, labels := NewGrid[float64](S), NewGrid[int32](S)
	for i, d := range Dirs(S) {
		th.Cells[i] = th0
		wobble := 0.12 * math.Sin(7*math.Atan2(d[1], d[0]))
		if d[2] >= wobble {
			labels.Cells[i] = 0
		} else {
			labels.Cells[i] = 1
		}
	}
	return th, labels, PlateStats(th, labels, 2)
}

func freshCrust(s *State) int {
	n := 0
	for i, a := range s.Age.Cells {
		if a == 0 && s.FeatType.Cells[i] == FeatDivergent {
			n++
		}
	}
	return n
}

func TestShearAlongJaggedBoundaryOpensNoCrust(t *testing.T) {
	S := 32
	p := testParams(t, S)
	p.RelaxIters, p.FaultScar = 0, 0 // no scarring, so thickness must stay exactly 0.6
	pixelDeg := 90.0 / float64(S)
	th, labels, plates := jaggedHemispheres(S, 0.6)
	// both plates spin about z at different rates: pure shear along the seam
	s := NewState(p, th, labels, plates, []Motion{{Pole: [3]float64{0, 0, 1}, DegPerStep: pixelDeg}, {Pole: [3]float64{0, 0, 1}, DegPerStep: 0}})
	rng := newRNG(1, "x")
	for step := range 5 {
		s.Step(rng)
		if n := freshCrust(s); n != 0 {
			t.Fatalf("step %d: %d pixels of fresh ridge crust along a pure shear seam", step, n)
		}
		for i, v := range s.Th.Cells {
			if math.Abs(v-0.6) > 1e-9 {
				t.Fatalf("step %d: pixel %d thickness %.4f changed under shear", step, i, v)
			}
		}
	}
}

func TestDivergenceStillOpensRidgeCrust(t *testing.T) {
	S := 32
	p := testParams(t, S)
	p.RelaxIters = 0
	pixelDeg := 90.0 / float64(S)
	th, labels, plates := jaggedHemispheres(S, 0.6)
	// north spins +x, south spins -x: they part on the +y side and collide on the -y side
	s := NewState(p, th, labels, plates, []Motion{{Pole: [3]float64{1, 0, 0}, DegPerStep: pixelDeg}, {Pole: [3]float64{1, 0, 0}, DegPerStep: -pixelDeg}})
	s.Step(newRNG(1, "x"))
	plus, minus := 0, 0
	for i, a := range s.Age.Cells {
		if a == 0 && s.FeatType.Cells[i] == FeatDivergent {
			if Dirs(S)[i][1] > 0 {
				plus++
			} else {
				minus++
			}
		}
	}
	if plus == 0 {
		t.Fatal("no ridge crust on the diverging side")
	}
	if minus > plus/10 {
		t.Fatalf("ridge crust on the converging side: %d vs %d", minus, plus)
	}
}

func TestOceanThickeningCappedAtBand(t *testing.T) {
	S := 8
	p := testParams(t, S)
	p.RelaxIters = 0
	th, labels := NewGrid[float64](S), NewGrid[int32](S)
	for i := range th.Cells {
		th.Cells[i] = p.RidgeThickness
	}
	s := NewState(p, th, labels, PlateStats(th, labels, 1), []Motion{{Pole: [3]float64{0, 0, 1}}})
	rng := newRNG(1, "x")
	for range 2000 {
		s.Step(rng)
	}
	for i, v := range s.Th.Cells {
		if v > p.OceanBandMax+1e-12 {
			t.Fatalf("pixel %d: ocean thickened to %.4f above OceanBandMax %g", i, v, p.OceanBandMax)
		}
	}
}

func TestRelaxWithinKeepsCoastStep(t *testing.T) {
	S := 8
	th := NewGrid[float64](S)
	for i, d := range Dirs(S) {
		if d[2] >= 0 {
			th.Cells[i] = 0.55
		} else {
			th.Cells[i] = 0.15
		}
	}
	before := th.Clone()
	if n := RelaxWithin(th, 0.05, 3, 0.5); n != 0 {
		t.Fatalf("RelaxWithin adjusted %d pairs across the coast", n)
	}
	for i := range th.Cells {
		if th.Cells[i] != before.Cells[i] {
			t.Fatalf("pixel %d changed", i)
		}
	}
	if Relax(th, 0.05, 1) == 0 {
		t.Fatal("plain Relax should still see the step (sanity)")
	}
}

func TestContinentsSurviveAdvection(t *testing.T) {
	p, err := DefaultParams("terran")
	if err != nil {
		t.Fatal(err)
	}
	p.Face, p.KeyframeEvery = 64, 100
	p.ErosionRate, p.RelaxIters, p.TrenchDepth, p.FaultScar, p.RiftChancePerMyr = 0, 0, 0, 0, 0
	start := -1.0
	if err := Run(p, 2026, func(f Frame) error {
		cont := 0
		for _, v := range f.Th.Cells {
			if v >= p.ContinentalThreshold {
				cont++
			}
		}
		share := float64(cont) / float64(f.Th.Len())
		if start < 0 {
			start = share
		} else if share < 0.6*start {
			t.Fatalf("step %d: continental share %.3f fell below 60%% of its start %.3f", f.Step, share, start)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSmallAdjacentPlatesDivergeAcrossSeam guards against the gap classifier's
// ridge test collapsing when the two plates' centroids are close together: the
// brief's formula compares dot(rel, nrm) against 0.3*|rel| using an
// un-normalised nrm, so for small, closely-spaced plates |nrm| itself can be
// well under 0.3 and no relative motion, however strongly opening, can ever
// pass the test. Two small caps side by side on an otherwise single
// background plate, pulling apart along their shared seam, must still open
// ridge crust there.
func TestSmallAdjacentPlatesDivergeAcrossSeam(t *testing.T) {
	S := 32
	p := testParams(t, S)
	p.RelaxIters, p.FaultScar = 0, 0
	pixelDeg := 90.0 / float64(S)
	radius := 3 * pixelDeg // ~8.4 degrees: a small plate, well under the bug's ~17 degree cutoff
	lonDeg := func(sign float64) [3]float64 {
		lon := sign * radius * math.Pi / 180
		return [3]float64{math.Cos(lon), math.Sin(lon), 0}
	}
	cA, cB := lonDeg(-1), lonDeg(1) // caps touch at longitude 0
	cosRadius := math.Cos(radius * math.Pi / 180)
	th, labels := NewGrid[float64](S), NewGrid[int32](S)
	for i, d := range Dirs(S) {
		th.Cells[i] = 0.6
		switch {
		case dot(d, cA) >= cosRadius:
			labels.Cells[i] = 0
		case dot(d, cB) >= cosRadius:
			labels.Cells[i] = 1
		default:
			labels.Cells[i] = 2
		}
	}
	plates := PlateStats(th, labels, 3)
	// cap A drifts west, cap B drifts east: they pull apart across their
	// shared seam at longitude 0; the background plate stays put.
	s := NewState(p, th, labels, plates, []Motion{
		{Pole: [3]float64{0, 0, 1}, DegPerStep: -pixelDeg},
		{Pole: [3]float64{0, 0, 1}, DegPerStep: pixelDeg},
		{Pole: [3]float64{0, 0, 1}, DegPerStep: 0},
	})
	s.Step(newRNG(1, "x"))
	if n := freshCrust(s); n == 0 {
		t.Fatal("no ridge crust where two small adjacent plates diverge across their shared seam")
	}
}
