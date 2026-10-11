package tectonics

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestSeaLevelAtDatum(t *testing.T) {
	p := testParams(t, 32)
	th := GenerateThickness(p, 2026)
	elev := Elevation(th, InitialAges(th, p), p)
	hollow := HollowVolume(elev, 0, p)
	if hollow <= 0 {
		t.Fatal("a remapped world must have hollows below the datum")
	}
	L := SeaLevelFor(elev, hollow, p)
	if math.Abs(L) > 0.002 {
		t.Fatalf("sea level at factor 1 is %.4f km, want 0 ± 0.002", L)
	}
	L2 := SeaLevelFor(elev, hollow*1.2, p)
	if L2 <= L {
		t.Fatalf("more water must raise the sea: %.4f vs %.4f", L2, L)
	}
	writeVis(t, "sealevel_factor_1_vs_1.2", renderIso(elev, cubemap.FacePosZ, L, 6), renderIso(elev, cubemap.FacePosZ, L2, 6))
	if got := HollowVolume(elev, L2, p); math.Abs(got-hollow*1.2)/(hollow*1.2) > 0.001 {
		t.Fatalf("filled volume %.6g, want %.6g within 0.1%%", got, hollow*1.2)
	}
	if ls := LandShare(elev, L); ls <= 0 || ls >= 1 {
		t.Fatalf("land share %.3f out of range", ls)
	}
	if LandShare(elev, L2) >= LandShare(elev, L) {
		t.Fatal("a higher sea must leave less land")
	}
}

func TestSeaLevelZeroWaterIsLowestPixel(t *testing.T) {
	p := testParams(t, 8)
	elev := NewGrid[float64](8)
	for i := range elev.Cells {
		elev.Cells[i] = 1 + float64(i%5)
	}
	if L := SeaLevelFor(elev, 0, p); math.Abs(L-1) > 0.0011 {
		t.Fatalf("zero water: sea level %.4f, want the lowest pixel (1 km) within a bin", L)
	}
}

func TestRunCarriesWaterAndSeaLevel(t *testing.T) {
	p := testParams(t, 16)
	p.Steps, p.KeyframeEvery = 4, 2
	var frames []Frame
	if err := Run(p, 11, func(f Frame) error { frames = append(frames, f); return nil }); err != nil {
		t.Fatal(err)
	}
	for _, f := range frames {
		if f.Elev == nil || f.Elev.Len() != f.Th.Len() {
			t.Fatalf("frame %d: no elevation grid", f.Step)
		}
		if f.WaterKm3 <= 0 || math.IsNaN(f.SeaLevelKm) || f.LandShare <= 0 || f.LandShare >= 1 {
			t.Fatalf("frame %d: water %g sea %g land %g", f.Step, f.WaterKm3, f.SeaLevelKm, f.LandShare)
		}
		if math.Abs(f.LandShare-LandShare(f.Elev, f.SeaLevelKm)) > 1e-12 {
			t.Fatalf("frame %d: land share %g disagrees with its own grid", f.Step, f.LandShare)
		}
	}
	if frames[0].WaterKm3 != frames[len(frames)-1].WaterKm3 {
		t.Fatal("the water budget must not change during a run")
	}
}
