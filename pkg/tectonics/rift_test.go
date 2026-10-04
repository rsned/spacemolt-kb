package tectonics

import (
	"math"
	"testing"
)

// riftWorld is two hemispheres (plate 0 = z>=0, plate 1 = z<0) with a thin
// trough along the x=0 great circle so plate 0's cheapest interior path runs
// from the equator over the pole to the far equator.
func riftWorld(t *testing.T, S int) *State {
	t.Helper()
	th, labels, plates := twoHemispheres(S, 0.8, 0.8)
	for i, d := range Dirs(S) {
		if math.Abs(d[0]) < 0.08 {
			th.Cells[i] = 0.1
		}
	}
	p := testParams(t, S)
	p.RelaxIters = 0
	ms := InitMotion(th, labels, plates, p, 3)
	return NewState(p, th, labels, plates, ms)
}

// boundary pixel of plate 0 nearest the +y direction (on the equator, x≈0)
func startNearPlusY(s *State) int32 {
	dirs := Dirs(s.Th.S)
	best, bestDot := int32(-1), -2.0
	for _, i := range s.boundaryPixels(0) {
		if d := dirs[i][1]; d > bestDot {
			best, bestDot = i, d
		}
	}
	return best
}

func TestRiftSplitsAlongThinTrough(t *testing.T) {
	s := riftWorld(t, 32)
	rng := newRNG(9, "rift")
	if !s.riftPlate(0, startNearPlusY(s), rng) {
		t.Fatal("rift cancelled")
	}
	if len(s.Plates) != 3 || len(s.Motions) != 3 {
		t.Fatalf("plates %d motions %d, want 3 and 3", len(s.Plates), len(s.Motions))
	}
	dirs := Dirs(32)
	var pos, neg, parent, child int
	ridge := 0
	for i, l := range s.Labels.Cells {
		switch l {
		case 0:
			parent++
			if dirs[i][0] > 0 {
				pos++
			}
		case 2:
			child++
			if dirs[i][0] < 0 {
				neg++
			}
		}
		if s.FeatType.Cells[i] == FeatDivergent {
			ridge++
			if s.Th.Cells[i] > s.P.RidgeThickness+s.P.RidgeJitter+1e-9 || s.Th.Cells[i] < s.P.RidgeThickness-s.P.RidgeJitter-1e-9 || s.Age.Cells[i] != 0 || s.FeatAge.Cells[i] != 0 {
				t.Fatalf("ridge pixel %d: th %g age %d featAge %d", i, s.Th.Cells[i], s.Age.Cells[i], s.FeatAge.Cells[i])
			}
		}
	}
	if parent == 0 || child == 0 || ridge == 0 {
		t.Fatalf("parent %d child %d ridge %d", parent, child, ridge)
	}
	// the halves sit on opposite signs of x; either side may be the parent
	sameSide := float64(pos)/float64(parent) >= 0.95 && float64(neg)/float64(child) >= 0.95
	otherSide := float64(parent-pos)/float64(parent) >= 0.95 && float64(child-neg)/float64(child) >= 0.95
	if !sameSide && !otherSide {
		t.Errorf("halves not split by x=0: parent +x %d/%d, child -x %d/%d", pos, parent, neg, child)
	}
	c := s.Plates[2]
	if c.Born != s.StepNo || c.LastRift != s.StepNo || c.Major || c.Retired || c.ID != 2 {
		t.Errorf("child plate %+v", c)
	}
	if s.Plates[0].LastRift != s.StepNo {
		t.Errorf("parent LastRift %d, want %d", s.Plates[0].LastRift, s.StepNo)
	}
	away := add(c.Centroid, scale(s.Plates[0].Centroid, -1))
	if dot(velocityAt(s.Motions[2], c.Centroid), away) <= 0 {
		t.Error("child does not move away from the parent")
	}
	if dot(velocityAt(s.Motions[0], s.Plates[0].Centroid), away) >= 0 {
		t.Error("parent does not move away from the child")
	}
	total := 0.0
	for _, pl := range s.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
}

func TestRiftEligibilityAndCancel(t *testing.T) {
	s := riftWorld(t, 24)
	s.P.RiftChancePerMyr = 1
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.6, rest: 0}) {
		t.Error("plate below the share threshold rifted")
	}
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 1000}) {
		t.Error("plate inside its rest period rifted")
	}
	s.P.RiftMinChildShare = 0.45 // impossible for a 0.5-share parent
	before, th, ft, age := s.Labels.Clone(), s.Th.Clone(), s.FeatType.Clone(), s.Age.Clone()
	if s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 0}) {
		t.Error("rift with an undersized child was not cancelled")
	}
	for i := range before.Cells {
		if before.Cells[i] != s.Labels.Cells[i] {
			t.Fatal("cancelled rift changed labels")
		}
		if th.Cells[i] != s.Th.Cells[i] || ft.Cells[i] != s.FeatType.Cells[i] || age.Cells[i] != s.Age.Cells[i] {
			t.Fatalf("cancelled rift changed pixel %d: th %g→%g feat %d→%d age %d→%d", i,
				th.Cells[i], s.Th.Cells[i], ft.Cells[i], s.FeatType.Cells[i], age.Cells[i], s.Age.Cells[i])
		}
	}
	if len(s.Plates) != 2 || s.Plates[0].LastRift != 0 {
		t.Errorf("cancelled rift changed tables: %d plates, LastRift %d", len(s.Plates), s.Plates[0].LastRift)
	}
	s.P.RiftMinChildShare = 0.03
	if !s.Rift(newRNG(1, "r"), riftParams{share: 0.4, rest: 0}) {
		t.Error("eligible plate did not rift")
	}
	if len(s.Plates) != 3 {
		t.Errorf("%d plates after rift", len(s.Plates))
	}
	s.P.RiftMaxPlates = 3
	if s.Rift(newRNG(2, "r"), riftParams{share: 0.1, rest: 0}) {
		t.Error("rifted past RiftMaxPlates")
	}
}

func TestRiftKeepsTablesParallelAndDeterministic(t *testing.T) {
	run := func() *State {
		s := riftWorld(t, 16)
		s.P.RiftChancePerMyr = 1
		s.P.RiftMinChildShare = 0.02
		step, rift := newRNG(5, "sim.step"), newRNG(5, "sim.rift")
		for range 12 {
			s.Step(step)
			s.Rift(rift, riftParams{share: 0.08, rest: 0})
		}
		return s
	}
	a, b := run(), run()
	if len(a.Plates) < 3 {
		t.Fatalf("expected several rifts, got %d plates", len(a.Plates))
	}
	if len(a.Plates) != len(a.Motions) || len(a.Plates) != len(b.Plates) {
		t.Fatalf("tables: %d plates %d motions, other run %d", len(a.Plates), len(a.Motions), len(b.Plates))
	}
	total := 0.0
	for _, pl := range a.Plates {
		total += pl.Area
	}
	if math.Abs(total-1) > 1e-9 {
		t.Errorf("areas sum to %g", total)
	}
	for i := range a.Labels.Cells {
		if a.Labels.Cells[i] < 0 || int(a.Labels.Cells[i]) >= len(a.Plates) {
			t.Fatalf("label %d out of range at %d", a.Labels.Cells[i], i)
		}
		if a.Labels.Cells[i] != b.Labels.Cells[i] || a.Th.Cells[i] != b.Th.Cells[i] {
			t.Fatalf("runs differ at pixel %d", i)
		}
	}
	for i := range a.Motions {
		if a.Motions[i] != b.Motions[i] {
			t.Fatalf("runs differ at motion %d: %+v vs %+v", i, a.Motions[i], b.Motions[i])
		}
	}
}

// A plate already split into fragments rifts only within the fragment that
// holds the start pixel; its other fragments stay with the parent.
func TestRiftIgnoresOtherFragments(t *testing.T) {
	s := riftWorld(t, 24)
	dirs := Dirs(24)
	capPixels := 0
	for i, d := range dirs {
		if d[2] < -0.9 {
			s.Labels.Cells[i] = 0 // an island of plate 0 inside plate 1
			capPixels++
		}
	}
	if capPixels == 0 {
		t.Fatal("no -z cap pixels")
	}
	s.Plates = PlateStats(s.Th, s.Labels, len(s.Plates))
	if !s.riftPlate(0, startNearPlusY(s), newRNG(9, "rift")) {
		t.Fatal("rift cancelled")
	}
	child := int32(len(s.Plates) - 1)
	for i, d := range dirs {
		if d[2] >= -0.9 {
			continue
		}
		if s.Labels.Cells[i] != 0 {
			t.Fatalf("cap pixel %d relabelled to %d, want it kept by the parent", i, s.Labels.Cells[i])
		}
	}
	n := 0
	for _, l := range s.Labels.Cells {
		if l == child {
			n++
		}
	}
	if child != 2 || n == 0 {
		t.Errorf("child id %d with %d pixels", child, n)
	}
}

func TestDrawRiftParamsInRangeAndSeeded(t *testing.T) {
	p := testParams(t, 16)
	a := drawRiftParams(p, 7)
	if a.share < p.RiftMinShareMin || a.share > p.RiftMinShareMax || a.rest < p.RiftRestMyrMin || a.rest > p.RiftRestMyrMax {
		t.Errorf("draw %+v outside share %g-%g, rest %g-%g", a, p.RiftMinShareMin, p.RiftMinShareMax, p.RiftRestMyrMin, p.RiftRestMyrMax)
	}
	if b := drawRiftParams(p, 7); b != a {
		t.Errorf("same seed drew %+v then %+v", a, b)
	}
}
