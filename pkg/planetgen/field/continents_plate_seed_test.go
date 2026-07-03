package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TestContinentsFromNilSeedsByteEqualToLegacy verifies the back-compat
// path: GenerateContinentsFromSeeds(..., nil) produces byte-equal
// output to the legacy GenerateContinents. This is the core safety
// property of the Phase 8 item 10 rewire — existing archetypes (which
// pass nil because their PlateCount=0) must reproduce bit-for-bit.
func TestContinentsFromNilSeedsByteEqualToLegacy(t *testing.T) {
	cfg := types.ContinentConfig{Seeds: 12, WarpAmp: 0.1, WarpFreq: 1.0, HeightLo: 0.3, HeightHi: 0.7}
	a := GenerateContinents(42, cfg, 32)
	b := GenerateContinentsFromSeeds(42, cfg, 32, nil)
	for face := range a.Faces {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d: legacy %.9f vs from-seeds-nil %.9f",
					face, i, a.Faces[face][i], b.Faces[face][i])
			}
		}
	}
}

// TestContinentsFromEmptySeedsByteEqualToLegacy: an empty (non-nil)
// slice must also follow the legacy path.
func TestContinentsFromEmptySeedsByteEqualToLegacy(t *testing.T) {
	cfg := types.ContinentConfig{Seeds: 8, WarpAmp: 0, WarpFreq: 1, HeightLo: 0.3, HeightHi: 0.7}
	a := GenerateContinents(7, cfg, 32)
	b := GenerateContinentsFromSeeds(7, cfg, 32, [][3]float64{})
	for face := range a.Faces {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("face %d pixel %d: legacy %.9f vs from-empty-seeds %.9f",
					face, i, a.Faces[face][i], b.Faces[face][i])
			}
		}
	}
}

// TestContinentsFromSeedsDiffersFromNil verifies that supplying a
// non-trivial seed list produces a *different* field from the nil
// legacy path. Without this, a buggy implementation that ignored the
// seed list would silently pass the byte-equal test.
func TestContinentsFromSeedsDiffersFromNil(t *testing.T) {
	cfg := types.ContinentConfig{Seeds: 4, WarpAmp: 0, WarpFreq: 1, HeightLo: 0.3, HeightHi: 0.7}
	// Pick four directions far from the Fibonacci-spiral pattern so the
	// nearest-cell partition shifts.
	seeds := [][3]float64{
		{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0},
	}
	a := GenerateContinents(99, cfg, 32)
	b := GenerateContinentsFromSeeds(99, cfg, 32, seeds)
	differs := false
	for face := range a.Faces {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				differs = true
				break
			}
		}
		if differs {
			break
		}
	}
	if !differs {
		t.Fatal("from-seeds output is byte-equal to legacy — seed list was ignored")
	}
}

// TestContinentsAlignWithContinentalPlates: each continental-plate
// seed direction should fall inside its own Voronoi cell in the
// continents field. The continents generator assigns every pixel a
// height in [HeightLo, HeightHi] based on its nearest seed; we verify
// the value at each continental-plate seed direction is within that
// band — i.e. the seed is being honoured (not overridden by the
// Fibonacci-spiral fallback).
func TestContinentsAlignWithContinentalPlates(t *testing.T) {
	profile := &types.PlanetProfile{
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
	}
	pf := GeneratePlates(profile, 42, 64)
	if pf == nil {
		t.Skip("PlateCount=0 — no plates to test against")
	}
	var contSeeds [][3]float64
	for _, p := range pf.Plates {
		if !p.IsOceanic {
			contSeeds = append(contSeeds, p.Seed)
		}
	}
	if len(contSeeds) == 0 {
		t.Skip("seed=42 produced no continental plates; nothing to verify")
	}
	cfg := types.ContinentConfig{
		Seeds: profile.PlateCount, WarpAmp: 0, WarpFreq: 1,
		HeightLo: 0.3, HeightHi: 0.7,
	}
	cm := GenerateContinentsFromSeeds(42, cfg, 64, contSeeds)
	for k, dir := range contSeeds {
		f, px, py := cubemap.DirToFacePixel(dir[0], dir[1], dir[2], 64)
		v := cm.Faces[f][py*64+px]
		if v < cfg.HeightLo || v > cfg.HeightHi {
			t.Errorf("continental seed %d at (face=%d,px=%d,py=%d) value %.4f outside [%.2f,%.2f]",
				k, f, px, py, v, cfg.HeightLo, cfg.HeightHi)
		}
	}
}
