package tectonics

import (
	"math"
	"testing"
)

func TestDegPerStep(t *testing.T) {
	// 5 cm/yr for 5 Myr = 250 km; on a 6371 km sphere that is 2.248°.
	if got := DegPerStep(5, 6371, 5); math.Abs(got-2.248) > 0.01 {
		t.Errorf("DegPerStep = %g", got)
	}
}

func TestRotateIsRightHanded(t *testing.T) {
	v := rotate([3]float64{1, 0, 0}, [3]float64{0, 0, 1}, 90)
	if math.Abs(v[0]) > 1e-9 || math.Abs(v[1]-1) > 1e-9 {
		t.Errorf("rotate x about z by 90 = %v, want +y", v)
	}
}

// twoHemispheres splits the sphere at z=0: plate 0 is z>=0 (thick), plate 1
// is z<0 (thin). Shared by the motion and step tests.
func twoHemispheres(S int, thNorth, thSouth float64) (*Grid[float64], *Grid[int32], []Plate) {
	th := NewGrid[float64](S)
	labels := NewGrid[int32](S)
	for i, d := range Dirs(S) {
		if d[2] >= 0 {
			th.Cells[i], labels.Cells[i] = thNorth, 0
		} else {
			th.Cells[i], labels.Cells[i] = thSouth, 1
		}
	}
	return th, labels, PlateStats(th, labels, 2)
}

func TestInitMotionPushesAwayFromThinSeam(t *testing.T) {
	// Make the northern plate's equatorial rim thin on the +x side only, so
	// its push direction must point toward -x.
	S := 16
	th, labels, plates := twoHemispheres(S, 0.8, 0.3)
	for i, d := range Dirs(S) {
		if labels.Cells[i] == 0 && d[2] < 0.3 && d[0] > 0.3 {
			th.Cells[i] = 0.1
		}
	}
	p := testParams(t, S)
	p.PushJitterDeg = 0
	ms := InitMotion(th, labels, plates, p, 1)
	v := velocityAt(ms[0], plates[0].Centroid)
	if v[0] >= 0 {
		t.Errorf("plate 0 velocity %v should point to -x", v)
	}
	if ms[0].DegPerStep <= 0 || ms[0].SpeedCmYr < p.SpeedMinCmYr || ms[0].SpeedCmYr > p.SpeedMaxCmYr {
		t.Errorf("motion %+v", ms[0])
	}
	if math.Abs(dot(ms[0].Pole, ms[0].Pole)-1) > 1e-9 {
		t.Error("pole not unit")
	}
}

func TestReaimBlendsTowardNewPush(t *testing.T) {
	S := 16
	th, labels, plates := twoHemispheres(S, 0.8, 0.3)
	p := testParams(t, S)
	p.PushJitterDeg = 0
	ms := InitMotion(th, labels, plates, p, 1)
	before := ms[0]
	// thin the -x rim instead: push should swing toward +x
	for i, d := range Dirs(S) {
		if labels.Cells[i] == 0 && d[2] < 0.3 && d[0] < -0.3 {
			th.Cells[i] = 0.05
		}
	}
	Reaim(th, labels, plates, ms, p, newRNG(1, "test"))
	vb, va := velocityAt(before, plates[0].Centroid), velocityAt(ms[0], plates[0].Centroid)
	if va[0] <= vb[0] {
		t.Errorf("reaim did not swing toward +x: before %v after %v", vb, va)
	}
	if ms[0].SpeedCmYr != before.SpeedCmYr {
		t.Error("reaim must not change speed")
	}
}
