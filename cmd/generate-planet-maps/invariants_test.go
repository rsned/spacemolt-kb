package main

import (
	"image/color"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

var invariantTypes = []string{
	"scorched", "arid", "terran", "tundra", "glacial", "ice_world",
	"super_terran", "hothouse", "lava_world", "oceanic",
	"jovian", "ice_giant", "unknown",
}

func TestInvariantsAlphaOpaque(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "InvariantSeed-"+pt, 200, 100)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 255 {
				t.Fatalf("%s: alpha=%d at pixel %d", pt, img.Pix[i], i/4)
			}
		}
	}
}

func TestInvariantsHistogramNonDegenerate(t *testing.T) {
	for _, pt := range invariantTypes {
		img, err := planetgen.GenerateEquirect(pt, "InvariantSeed-"+pt, 200, 100)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		var bucket [16]int
		for i := 0; i < len(img.Pix); i += 4 {
			r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
			lum := (int(r) + int(g) + int(b)) / 3
			bucket[lum/16]++
		}
		distinct := 0
		for _, n := range bucket {
			if n > 0 {
				distinct++
			}
		}
		if distinct < 4 {
			t.Errorf("%s: only %d distinct luminance buckets occupied; want ≥4", pt, distinct)
		}
	}
}

func TestInvariantsTerranOceanLandRatio(t *testing.T) {
	img, err := planetgen.GenerateEquirect("terran", "Earth", 400, 200)
	if err != nil {
		t.Fatal(err)
	}
	var oceanPx, totalPx int
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.RGBA{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
		if c.B > c.R && c.B > c.G && c.B > 100 {
			oceanPx++
		}
		totalPx++
	}
	ratio := float64(oceanPx) / float64(totalPx)
	if ratio < 0.30 || ratio > 0.85 {
		t.Errorf("terran ocean ratio = %.3f, want in [0.30, 0.85]", ratio)
	}
}

// TestInvariantsCubeMapContinuity verifies that the cube-map sampler
// returns nearly-identical colors for two 3D directions that are very
// close to each other (sub-pixel separation). This is a true sphere-
// continuity check: it tests the cube-map sampler, not the equirect
// bake's pixel quantization.
func TestInvariantsCubeMapContinuity(t *testing.T) {
	for _, pt := range invariantTypes {
		cm, err := planetgen.Generate(pt, "ContinuitySeed-"+pt, 256)
		if err != nil {
			t.Fatalf("%s: %v", pt, err)
		}
		// Sample at +X axis with a tiny perturbation. Both directions
		// hit the +X face center; bilinear sample should agree to
		// within a small tolerance.
		const eps = 1e-4
		c1 := cm.Sample(1.0, 0.0, eps)
		c2 := cm.Sample(1.0, 0.0, -eps)
		d := absI(int(c1.R)-int(c2.R)) +
			absI(int(c1.G)-int(c2.G)) +
			absI(int(c1.B)-int(c2.B))
		if d > 8 {
			t.Errorf("%s: +X-axis +/- eps RGB delta = %d, want ≤ 8", pt, d)
		}
	}
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestPhase7PlateInvariants(t *testing.T) {
	archetypes := map[string]int64{
		"terran": 1, "super_terran": 2, "oceanic": 3, "tundra": 4,
		"arid": 5, "glacial": 6, "scorched": 7, "lava_world": 8,
	}
	S := 64
	for name, master := range archetypes {
		t.Run(name, func(t *testing.T) {
			profile := planetgen.Profiles[name]
			pf := field.GeneratePlates(profile, master, S)
			if pf == nil {
				t.Skipf("no plates for %s", name)
			}
			// Number of distinct plate ids equals PlateCount.
			seen := make(map[int16]int)
			for f := range pf.PlateID {
				for _, id := range pf.PlateID[f] {
					if id < 0 {
						t.Fatalf("unfilled pixel in face %d", f)
					}
					seen[id]++
				}
			}
			if len(seen) != profile.PlateCount {
				t.Errorf("got %d distinct plate ids, want %d", len(seen), profile.PlateCount)
			}
			for id, c := range seen {
				if c == 0 {
					t.Errorf("plate %d has 0 pixels", id)
				}
			}
		})
	}
}

func TestPhase7JitterInvariants(t *testing.T) {
	archetypes := map[string]int64{
		"terran": 1, "tundra": 2, "arid": 3, "scorched": 4,
	}
	S := 64
	for name, master := range archetypes {
		t.Run(name, func(t *testing.T) {
			profile := planetgen.Profiles[name]
			jf := noise.GenerateJitter(profile, master, S)
			if jf == nil {
				t.Skipf("jitter disabled for %s", name)
			}
			seen := make(map[int16]int)
			for f := range jf.PerPixel {
				for _, id := range jf.PerPixel[f] {
					seen[id]++
				}
			}
			if len(seen) != len(jf.Cells) {
				t.Errorf("got %d distinct cell ids, want %d", len(seen), len(jf.Cells))
			}
		})
	}
}
