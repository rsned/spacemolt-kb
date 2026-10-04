package tectonics

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func stepState(t *testing.T, S int, thN, thS float64, poleN, poleS [3]float64, degN, degS float64) *State {
	t.Helper()
	th, labels, plates := twoHemispheres(S, thN, thS)
	p := testParams(t, S)
	p.RelaxIters = 0
	ms := []Motion{{Pole: unit(poleN), DegPerStep: degN}, {Pole: unit(poleS), DegPerStep: degS}}
	return NewState(p, th, labels, plates, ms)
}

func countFeat(s *State, ft uint8) int {
	n := 0
	for _, v := range s.FeatType.Cells {
		if v == ft {
			n++
		}
	}
	return n
}

func TestStepSinglePlateRotatesWithoutGaps(t *testing.T) {
	S := 16
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i, d := range Dirs(S) {
		th.Cells[i] = 0.5 + 0.3*d[0]
	}
	p := testParams(t, S)
	s := NewState(p, th, labels, PlateStats(th, labels, 1), []Motion{{Pole: [3]float64{0, 0, 1}, DegPerStep: 3}})
	s.Step(newRNG(1, "t"))
	if n := countFeat(s, FeatDivergent); n != 0 {
		t.Errorf("%d divergent pixels on a one-plate world", n)
	}
	// thickness pattern rotated: the +x high moved toward +y
	best, bi := -1.0, 0
	for i, v := range s.Th.Cells {
		if v > best {
			best, bi = v, i
		}
	}
	if d := Dirs(S)[bi]; d[1] <= 0 {
		t.Errorf("thickest pixel at %v, expected to have rotated toward +y", d)
	}
}

func TestStepDivergentGapFills(t *testing.T) {
	// The north plate rotates about +x (its rim at +y lifts north, its rim at
	// -y sinks south); the south plate rotates the opposite way. So along +y
	// the plates separate (ridge) and along -y they converge.
	s := stepState(t, 24, 0.8, 0.8, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	s.Step(newRNG(1, "t"))
	div, conv := 0, 0
	for i, ft := range s.FeatType.Cells {
		d := Dirs(24)[i]
		switch ft {
		case FeatDivergent:
			div++
			if d[1] < 0 {
				t.Fatalf("divergent pixel on the converging side at %v", d)
			}
			if v := s.Th.Cells[i]; math.Abs(v-s.P.RidgeThickness) > s.P.RidgeJitter+1e-9 {
				t.Fatalf("ridge thickness %g", v)
			}
			if s.Age.Cells[i] != 0 {
				t.Fatalf("new crust age %d", s.Age.Cells[i])
			}
		case FeatConvergent:
			conv++
			if d[1] > 0 {
				t.Fatalf("convergent pixel on the diverging side at %v", d)
			}
		}
	}
	if div == 0 || conv == 0 {
		t.Fatalf("div %d conv %d", div, conv)
	}
}

func TestStepCollisionRaisesThickness(t *testing.T) {
	s := stepState(t, 24, 0.8, 0.7, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	before := s.Th.Clone()
	s.Step(newRNG(1, "t"))
	raised := 0
	for i, ft := range s.FeatType.Cells {
		if ft == FeatConvergent && s.Th.Cells[i] > before.Cells[i]+s.P.CollisionUplift/2 {
			raised++
			if s.FeatAge.Cells[i] != 0 {
				t.Fatalf("belt age %d, want 0", s.FeatAge.Cells[i])
			}
		}
	}
	if raised == 0 {
		t.Error("no collision pixel got thicker")
	}
}

func TestStepSubductionTrenchesThinSide(t *testing.T) {
	s := stepState(t, 24, 0.8, 0.2, [3]float64{1, 0, 0}, [3]float64{-1, 0, 0}, 4, 4)
	before := s.Th.Clone()
	s.Step(newRNG(1, "t"))
	trench, lost := 0, 0
	for i, ft := range s.FeatType.Cells {
		if ft == FeatConvergent && s.Labels.Cells[i] != 0 {
			lost++ // the thin south plate must never win a convergent pixel
		}
	}
	for i := range s.Th.Cells {
		if s.Labels.Cells[i] == 1 && s.Th.Cells[i] < before.Cells[i]-1e-9 {
			trench++
		}
	}
	if lost != 0 || trench == 0 {
		t.Errorf("thin plate won %d pixels, trench pixels %d", lost, trench)
	}
}

func TestStepTransformFault(t *testing.T) {
	// Both plates rotate about z in opposite senses: motion along the
	// equatorial boundary is purely tangential.
	s := stepState(t, 24, 0.8, 0.8, [3]float64{0, 0, 1}, [3]float64{0, 0, -1}, 4, 4)
	s.Step(newRNG(1, "t"))
	if tr := countFeat(s, FeatTransform); tr == 0 {
		t.Error("no transform pixels")
	}
	if cv := countFeat(s, FeatConvergent); cv > countFeat(s, FeatTransform)/4 {
		t.Errorf("too many convergent pixels (%d) for pure shear", cv)
	}
}

func TestStepAreaConservedAndDeterministic(t *testing.T) {
	a := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	b := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	a.Plates[0].Major, b.Plates[0].Major = true, true
	countMajor := func(s *State) int {
		n := 0
		for _, pl := range s.Plates {
			if pl.Major {
				n++
			}
		}
		return n
	}
	majorsBefore := countMajor(a)
	for range 5 {
		a.Step(newRNG(9, "t"))
		b.Step(newRNG(9, "t"))
	}
	for i := range a.Th.Cells {
		if a.Th.Cells[i] != b.Th.Cells[i] || a.Labels.Cells[i] != b.Labels.Cells[i] || a.FeatType.Cells[i] != b.FeatType.Cells[i] {
			t.Fatalf("pixel %d differs", i)
		}
	}
	total := 0.0
	for _, pl := range a.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
	if got := countMajor(a); got != majorsBefore {
		t.Errorf("%d major plates after stepping, want %d", got, majorsBefore)
	}
	if fb := a.FeatureByte(0); fb>>6 != uint8(a.FeatType.Cells[0]) {
		t.Errorf("feature byte %08b", fb)
	}
}

func TestStepTransformScarsOnce(t *testing.T) {
	// Pure shear along a persistent seam: the same pixels stay on the fault
	// every step, but must only be scarred when the fault first forms.
	s := stepState(t, 24, 0.8, 0.8, [3]float64{0, 0, 1}, [3]float64{0, 0, -1}, 4, 4)
	for range 5 {
		s.Step(newRNG(1, "t"))
	}
	for i, v := range s.Th.Cells {
		if v < 0.8-s.P.FaultScar-1e-9 {
			t.Fatalf("pixel %d thickness %g: scarred more than once", i, v)
		}
	}
	if countFeat(s, FeatTransform) == 0 {
		t.Error("no transform pixels after 5 steps")
	}
}

func TestStepSlowPlateStillDrifts(t *testing.T) {
	// A plate moving 0.3 rim-pixels per step must still drift: the pending
	// rotation accumulates until it reaches a whole pixel, then hops.
	S := 32
	pixelDeg := 90.0 / float64(S)
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i, d := range Dirs(S) {
		th.Cells[i] = 0.5 + 0.3*d[0]
	}
	p := testParams(t, S)
	p.RelaxIters = 0
	s := NewState(p, th, labels, PlateStats(th, labels, 1), []Motion{{Pole: [3]float64{0, 0, 1}, DegPerStep: 0.3 * pixelDeg}})
	rng := newRNG(1, "t")
	for range 20 {
		s.Step(rng)
	}
	if n := countFeat(s, FeatDivergent); n != 0 {
		t.Errorf("%d divergent pixels on a one-plate world", n)
	}
	best, bi := -1.0, 0
	for i, v := range s.Th.Cells {
		if v > best {
			best, bi = v, i
		}
	}
	d := Dirs(S)[bi]
	got := math.Atan2(d[1], d[0]) * 180 / math.Pi
	t.Logf("thickest pixel rotated %.3f deg (rigid expectation %.3f)", got, 20*0.3*pixelDeg)
	if want := 0.5 * 20 * 0.3 * pixelDeg; got < want {
		t.Errorf("thickest pixel rotated %.3f deg about z, want >= %.3f", got, want)
	}
}

func TestStepBeltMovesWithPlate(t *testing.T) {
	// A carried feature advects with its plate: one pixel-angle of rotation
	// about z moves a convergent mark to the forward-rotated pixel, aged by one.
	S := 32
	pixelDeg := 90.0 / float64(S)
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i := range th.Cells {
		th.Cells[i] = 0.8
	}
	p := testParams(t, S)
	p.RelaxIters = 0
	s := NewState(p, th, labels, PlateStats(th, labels, 1), []Motion{{Pole: [3]float64{0, 0, 1}, DegPerStep: pixelDeg}})
	dirs := Dirs(S)
	f, px, py := cubemap.DirToFacePixel(1, 0.01, 0.01, S) // just off +x on the equator
	i0 := s.Th.Index(f, px, py)
	s.FeatType.Cells[i0], s.FeatAge.Cells[i0] = FeatConvergent, 5
	want := rotate(dirs[i0], [3]float64{0, 0, 1}, pixelDeg)
	f, px, py = cubemap.DirToFacePixel(want[0], want[1], want[2], S)
	i1 := s.Th.Index(f, px, py)
	if i1 == i0 {
		t.Fatal("test setup: rotation did not leave the pixel")
	}
	s.Step(newRNG(1, "t"))
	if s.FeatType.Cells[i1] != FeatConvergent || s.FeatAge.Cells[i1] != 6 {
		t.Errorf("destination pixel feature %d age %d, want convergent age 6", s.FeatType.Cells[i1], s.FeatAge.Cells[i1])
	}
	if s.FeatType.Cells[i0] == FeatConvergent && s.FeatAge.Cells[i0] == 6 {
		t.Error("original pixel still carries the belt")
	}
}

func TestRetireCarriesBornAndLastRift(t *testing.T) {
	s := stepState(t, 16, 0.8, 0.3, [3]float64{1, 0, 0}, [3]float64{0, 1, 0}, 3, 2)
	s.Plates[1].Born, s.Plates[1].LastRift = 4, 7
	s.Step(newRNG(1, "t"))
	if s.Plates[1].Born != 4 || s.Plates[1].LastRift != 7 {
		t.Errorf("plate 1 after step: born %d lastRift %d, want 4 7", s.Plates[1].Born, s.Plates[1].LastRift)
	}
	rows := s.Snapshot(1).Plates
	if rows[1].Born != 4 {
		t.Errorf("PlateRow.Born = %d, want 4", rows[1].Born)
	}
}
