package noise

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateJitterCellCount(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 120, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
	}
	jf := GenerateJitter(profile, 7, 32)
	if jf == nil {
		t.Fatal("nil JitterField for enabled profile")
	}
	if len(jf.Cells) != 120 {
		t.Errorf("got %d cells, want 120", len(jf.Cells))
	}
}

func TestGenerateJitterDisabled(t *testing.T) {
	profile := &types.PlanetProfile{JitterEnabled: false}
	if jf := GenerateJitter(profile, 7, 32); jf != nil {
		t.Errorf("disabled profile should yield nil, got %+v", jf)
	}
}

func TestGenerateJitterPerPixelInRange(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 32, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
	}
	S := 16
	jf := GenerateJitter(profile, 5, S)
	seen := make(map[int16]int)
	for f := range jf.PerPixel {
		for _, id := range jf.PerPixel[f] {
			if id < 0 || int(id) >= len(jf.Cells) {
				t.Fatalf("cell id %d out of range [0, %d)", id, len(jf.Cells))
			}
			seen[id]++
		}
	}
	if len(seen) < len(jf.Cells)/2 {
		t.Errorf("only %d/%d cells visited", len(seen), len(jf.Cells))
	}
	_ = cubemap.NumFaces
}

func TestJitterTransformIdentityWhenZero(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 8, JitterRotMax: 0, JitterOffsetMax: 0,
	}
	jf := GenerateJitter(profile, 3, 16)
	p := [3]float64{0.5, 0.5, math.Sqrt2 / 2}
	px, py, pz := jf.Transform(p[0], p[1], p[2])
	if math.Abs(px-p[0]) > 1e-9 || math.Abs(py-p[1]) > 1e-9 || math.Abs(pz-p[2]) > 1e-9 {
		t.Errorf("zero-jitter transform changed p: in=%v out=(%f,%f,%f)", p, px, py, pz)
	}
}

func TestGenerateJitterDeterministic(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 64, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.1,
	}
	a := GenerateJitter(profile, 99, 16)
	b := GenerateJitter(profile, 99, 16)
	for f := range a.PerPixel {
		for i := range a.PerPixel[f] {
			if a.PerPixel[f][i] != b.PerPixel[f][i] {
				t.Fatalf("non-deterministic at face %d idx %d", f, i)
			}
		}
	}
	for i := range a.Cells {
		if a.Cells[i] != b.Cells[i] {
			t.Errorf("cell %d non-deterministic: %+v vs %+v", i, a.Cells[i], b.Cells[i])
		}
	}
}

func TestJitterTransformPixelMatchesTransform(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 16, JitterRotMax: math.Pi / 4, JitterOffsetMax: 0.05,
	}
	S := 8
	jf := GenerateJitter(profile, 42, S)
	for f := range cubemap.NumFaces {
		face := cubemap.Face(f)
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				ax, ay, az := jf.Transform(dx, dy, dz)
				bx, by, bz := jf.TransformPixel(face, px, py, dx, dy, dz)
				if math.Abs(ax-bx) > 1e-12 || math.Abs(ay-by) > 1e-12 || math.Abs(az-bz) > 1e-12 {
					t.Fatalf("face %v px=%d py=%d: Transform(%.4f,%.4f,%.4f)=(%.6f,%.6f,%.6f) vs TransformPixel=(%.6f,%.6f,%.6f)",
						face, px, py, dx, dy, dz, ax, ay, az, bx, by, bz)
				}
			}
		}
	}
}

func TestJitterTransformPixelNilSafe(t *testing.T) {
	var jf *JitterField
	px, py, pz := jf.TransformPixel(cubemap.FacePosX, 0, 0, 0.5, 0.5, 0.7)
	if px != 0.5 || py != 0.5 || pz != 0.7 {
		t.Errorf("nil receiver should pass through; got (%f, %f, %f)", px, py, pz)
	}
}

func TestJitterTransformMagnitudePreserved(t *testing.T) {
	// With OffsetMax=0, rotation around the cell center preserves the
	// distance from the cell center for every input point. This catches
	// sign errors in the cross product or dot-product coefficient that
	// pass the identity-when-zero test but fail at non-zero rotations.
	profile := &types.PlanetProfile{
		JitterEnabled: true, JitterCellCount: 1, JitterRotMax: math.Pi / 3, JitterOffsetMax: 0,
	}
	jf := GenerateJitter(profile, 42, 16)
	cellCenter := jf.Cells[0].Center
	for _, p := range [][3]float64{
		{1, 0, 0}, {0, 1, 0}, {0.6, 0.8, 0}, {0.5, 0.5, math.Sqrt2 / 2},
	} {
		ox, oy, oz := p[0]-cellCenter[0], p[1]-cellCenter[1], p[2]-cellCenter[2]
		inDist := math.Sqrt(ox*ox + oy*oy + oz*oz)
		rx, ry, rz := jf.Transform(p[0], p[1], p[2])
		tx, ty, tz := rx-cellCenter[0], ry-cellCenter[1], rz-cellCenter[2]
		outDist := math.Sqrt(tx*tx + ty*ty + tz*tz)
		if math.Abs(outDist-inDist) > 1e-9 {
			t.Errorf("rotation changed distance from cell center: in=%.6f out=%.6f for p=%v", inDist, outDist, p)
		}
	}
}
