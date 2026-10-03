package tectonics

import "testing"

func TestRunEmitsFramesDeterministically(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 5, 2
	collect := func() []Frame {
		var out []Frame
		if err := Run(p, 123, func(f Frame) error { out = append(out, f); return nil }); err != nil {
			t.Fatal(err)
		}
		return out
	}
	a, b := collect(), collect()
	wantSteps := []int{0, 2, 4, 5}
	if len(a) != len(wantSteps) {
		t.Fatalf("%d frames, want %d", len(a), len(wantSteps))
	}
	for i, f := range a {
		if f.Step != wantSteps[i] {
			t.Errorf("frame %d step %d want %d", i, f.Step, wantSteps[i])
		}
		if f.Myr != float64(f.Step)*p.MyrPerStep() {
			t.Errorf("frame %d Myr %g", i, f.Myr)
		}
		if len(f.Plates) == 0 || f.Th.Len() != 6*16*16 {
			t.Errorf("frame %d incomplete", i)
		}
		for j := range f.Th.Cells {
			if f.Th.Cells[j] != b[i].Th.Cells[j] || f.Feature.Cells[j] != b[i].Feature.Cells[j] {
				t.Fatalf("frame %d pixel %d differs between runs", i, j)
			}
		}
	}
	// frames are snapshots: frame 0 must not reflect later steps
	if a[0].Step != 0 {
		t.Error("first frame is not the initial state")
	}
	for _, v := range a[0].Feature.Cells {
		if v != 0 {
			t.Fatal("frame 0 has features before any step")
		}
	}
}

func TestValidate(t *testing.T) {
	p := testParams(t, 16)
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	p.Steps = 0
	if err := Validate(p); err == nil {
		t.Error("Steps 0 must fail")
	}
	p = testParams(t, 16)
	p.Face = 2048
	if err := Validate(p); err == nil {
		t.Error("Face 2048 must fail")
	}
}
