package tectonics

import (
	"math"
	"path/filepath"
	"testing"
)

func TestBundleRoundTrip(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 3, 1
	dir := t.TempDir()
	m := NewManifest(p, 99, "test_planet")
	var frames []Frame
	err := Run(p, 99, func(f Frame) error {
		fi, err := WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	m2, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m2.Seed != 99 || m2.Planet != "test_planet" || m2.Face != 16 || len(m2.Frames) != 4 || m2.Frames[3].Step != 3 {
		t.Fatalf("manifest %+v", m2)
	}
	if m2.Frames[1].Dir != filepath.Join("frames", "0001") {
		t.Errorf("frame dir %q", m2.Frames[1].Dir)
	}
	for i, fi := range m2.Frames {
		got, err := ReadFrame(dir, fi)
		if err != nil {
			t.Fatal(err)
		}
		want := frames[i]
		for j := range want.Th.Cells {
			if math.Abs(got.Th.Cells[j]-want.Th.Cells[j]) > 1.0/255+1e-9 {
				t.Fatalf("frame %d pixel %d thickness %g vs %g", i, j, got.Th.Cells[j], want.Th.Cells[j])
			}
			if got.Labels.Cells[j] != want.Labels.Cells[j] || got.Feature.Cells[j] != want.Feature.Cells[j] {
				t.Fatalf("frame %d pixel %d label/feature mismatch", i, j)
			}
		}
		if len(fi.Plates) != len(want.Plates) {
			t.Errorf("frame %d plate rows %d vs %d", i, len(fi.Plates), len(want.Plates))
		}
	}
}
