package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestGenerateProvincesDisabled(t *testing.T) {
	id, ramp, rfreq := GenerateProvinces(1, types.ProvinceConfig{Count: 0}, 32)
	if id != nil || ramp != nil || rfreq != nil {
		t.Errorf("Count=0 should return all-nil, got id=%v ramp=%v rfreq=%v", id, ramp, rfreq)
	}
}

func TestGenerateProvincesCellCoverage(t *testing.T) {
	cfg := types.ProvinceConfig{Count: 12, Jitter: 0.1}
	id, _, _ := GenerateProvinces(42, cfg, 64)
	if id == nil {
		t.Fatal("nil cell-id field")
	}
	uniq := map[int]struct{}{}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for y := range 64 {
			for x := range 64 {
				uniq[int(id.Get(face, x, y))] = struct{}{}
			}
		}
	}
	// All 12 cells should appear at S=64 (Fibonacci spiral covers the sphere).
	if len(uniq) != 12 {
		t.Errorf("expected all 12 cells visible at S=64, got %d distinct cells", len(uniq))
	}
}

func TestProvinceModulatorRanges(t *testing.T) {
	cfg := types.ProvinceConfig{Count: 8, Jitter: 0.2}
	_, ramp, rfreq := GenerateProvinces(7, cfg, 32)
	for _, f := range []*cubemap.CubeMapF{ramp, rfreq} {
		for face := range cubemap.Face(cubemap.NumFaces) {
			for y := range 32 {
				for x := range 32 {
					v := f.Get(face, x, y)
					if math.IsNaN(v) {
						t.Fatalf("modulator NaN at face=%d x=%d y=%d", face, x, y)
					}
					if v < 1.0-cfg.Jitter-1e-9 || v > 1.0+cfg.Jitter+1e-9 {
						t.Errorf("modulator %v outside [%v, %v]",
							v, 1.0-cfg.Jitter, 1.0+cfg.Jitter)
					}
				}
			}
		}
	}
}

func TestProvinceDeterministic(t *testing.T) {
	cfg := types.ProvinceConfig{Count: 8, Jitter: 0.2, WarpAmp: 0.05}
	a, _, _ := GenerateProvinces(123, cfg, 16)
	b, _, _ := GenerateProvinces(123, cfg, 16)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for y := range 16 {
			for x := range 16 {
				if a.Get(face, x, y) != b.Get(face, x, y) {
					t.Fatalf("non-deterministic cell-id at face=%d x=%d y=%d: %v vs %v",
						face, x, y, a.Get(face, x, y), b.Get(face, x, y))
				}
			}
		}
	}
}
