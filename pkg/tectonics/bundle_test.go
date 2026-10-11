package tectonics

import (
	"math"
	"os"
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

func TestBundleRoundTripsElevation(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 2, 2
	dir := t.TempDir()
	m := NewManifest(p, 5, "")
	var frames []Frame
	err := Run(p, 5, func(f Frame) error {
		fi, err := WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		m.WaterKm3 = f.WaterKm3
		frames = append(frames, f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || got.WaterKm3 != frames[0].WaterKm3 {
		t.Fatalf("manifest version %d water %g", got.Version, got.WaterKm3)
	}
	for k, fi := range got.Frames {
		if fi.SeaLevelKm != frames[k].SeaLevelKm || fi.LandShare != frames[k].LandShare {
			t.Fatalf("frame %d: sea/land rows %g/%g vs %g/%g", k, fi.SeaLevelKm, fi.LandShare, frames[k].SeaLevelKm, frames[k].LandShare)
		}
		if _, err := os.Stat(filepath.Join(dir, fi.Dir, "elevation.png")); err != nil {
			t.Fatal(err)
		}
		fr, err := ReadFrame(dir, fi)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range frames[k].Elev.Cells {
			if math.Abs(fr.Elev.Cells[i]-v) > 0.0006 {
				t.Fatalf("frame %d pixel %d: elevation %.4f read back as %.4f", k, i, v, fr.Elev.Cells[i])
			}
		}
	}
}

func TestReadFrameMissingElevation(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 1, 1
	dir := t.TempDir()
	var fi FrameInfo
	var frame Frame
	if err := Run(p, 7, func(f Frame) error {
		frame = f
		f.Elev = nil // a v1 frame carries no elevation grid
		var err error
		fi, err = WriteFrame(dir, f)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, fi.Dir, "elevation.png")); !os.IsNotExist(err) {
		t.Fatalf("elevation.png should not exist, stat err %v", err)
	}
	got, err := ReadFrame(dir, fi)
	if err != nil {
		t.Fatalf("ReadFrame on a v1 frame directory: %v", err)
	}
	if got.Elev != nil {
		t.Fatal("Elev should be nil when elevation.png is absent")
	}
	if math.Abs(got.Th.Cells[0]-frame.Th.Cells[0]) > 1.0/255+1e-9 {
		t.Fatalf("thickness grid not read: got %g, want %g", got.Th.Cells[0], frame.Th.Cells[0])
	}

	// A real read failure (not "file missing") must still surface.
	if err := os.WriteFile(filepath.Join(dir, fi.Dir, "elevation.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFrame(dir, fi); err == nil {
		t.Fatal("ReadFrame must error on a corrupt elevation.png, not swallow it")
	}
}

func TestElevationPackingClamps(t *testing.T) {
	S := 4
	elev := NewGrid[float64](S)
	elev.Cells[0] = 40.0  // above the +33.536 km range: clamps to 32.767 km
	elev.Cells[1] = -40.0 // below the -32 km range: clamps to -32.768 km
	fr := Frame{Step: 0, Th: NewGrid[float64](S), Labels: NewGrid[int32](S), Feature: NewGrid[uint8](S), Elev: elev}
	dir := t.TempDir()
	fi, err := WriteFrame(dir, fr)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadFrame(dir, fi)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.Elev.Cells[0]-32.767) > 1e-9 {
		t.Errorf("clamp high: %.6f, want 32.767", got.Elev.Cells[0])
	}
	if math.Abs(got.Elev.Cells[1]-(-32.768)) > 1e-9 {
		t.Errorf("clamp low: %.6f, want -32.768", got.Elev.Cells[1])
	}
}

func TestReadManifestAcceptsVersion1(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":1,"archetype":"terran","face":8,"steps":1,"myr_per_step":1,"frames":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(dir); err != nil {
		t.Fatalf("version 1 must still load: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadManifest(dir); err == nil {
		t.Fatal("version 3 must be refused")
	}
}
