package main

import (
	"image/color"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
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
