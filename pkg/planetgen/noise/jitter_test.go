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
