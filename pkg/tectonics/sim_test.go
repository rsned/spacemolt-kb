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
	p = testParams(t, 16)
	p.RepoleEveryMyr = 0
	if err := Validate(p); err == nil {
		t.Error("RepoleEveryMyr 0 must fail")
	}
	p = testParams(t, 16)
	p.DominantMax = 0.1
	if err := Validate(p); err == nil {
		t.Error("DominantMax below DominantMin must fail")
	}
	p = testParams(t, 16)
	p.Steps, p.TimelineMyr, p.RepoleEveryMyr = 400, 800, 25
	if got := repoleSteps(p); got != 13 {
		t.Errorf("repoleSteps = %d, want 13 (25 Myr at 2 Myr per step is 12.5, rounded half up)", got)
	}
	p.RepoleEveryMyr = 0.5
	if got := repoleSteps(p); got != 1 {
		t.Errorf("repoleSteps = %d, want the floor of 1", got)
	}

	for _, bad := range []func(*Params){
		func(p *Params) { p.RiftMinShareMax = p.RiftMinShareMin - 0.01 },
		func(p *Params) { p.RiftRestMyrMin = -1 },
		func(p *Params) { p.RiftChancePerMyr = 1.5 },
		func(p *Params) { p.RiftMinChildShare = 0.5 },
		func(p *Params) { p.RiftThinPower = 0 },
		func(p *Params) { p.RiftMaxPlates = 256 },
		func(p *Params) { p.RiftRetries = 11 },
	} {
		q := testParams(t, 16)
		bad(&q)
		if err := Validate(q); err == nil {
			t.Errorf("Validate accepted bad rift knobs %+v", q)
		}
	}
}

func TestRunRiftsWhenForced(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 20, 20
	p.RiftChancePerMyr, p.RiftMinShareMin, p.RiftMinShareMax = 1, 0.05, 0.05
	p.RiftRestMyrMin, p.RiftRestMyrMax, p.RiftMinChildShare = 0, 0, 0.02
	var last Frame
	if err := Run(p, 77, func(f Frame) error { last = f; return nil }); err != nil {
		t.Fatal(err)
	}
	born := 0
	for _, r := range last.Plates {
		if r.Born > 0 {
			born++
		}
	}
	if born == 0 {
		t.Error("no plates born in a run with forced rifting")
	}
	p.RiftChancePerMyr = 0
	if err := Run(p, 77, func(f Frame) error { last = f; return nil }); err != nil {
		t.Fatal(err)
	}
	for _, r := range last.Plates {
		if r.Born > 0 {
			t.Fatal("a plate was born with RiftChancePerMyr 0")
		}
	}
}
