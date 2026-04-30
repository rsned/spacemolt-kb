package field

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestErodeNoOpZeroDroplets(t *testing.T) {
	hm := cubemap.NewF(32)
	for face := range hm.Faces {
		for i := range hm.Faces[face] {
			hm.Faces[face][i] = 0.5
		}
	}
	cfg := types.ErosionConfig{Droplets: 0}
	out := Erode(123, hm, cfg, 32)
	for face := range out.Faces {
		for i, v := range out.Faces[face] {
			if v != 0.5 {
				t.Fatalf("0 droplets should be a no-op; face %d idx %d got %f", face, i, v)
			}
		}
	}
}

func TestErodeDeterministic(t *testing.T) {
	hm := makeBumpyHeightmap(32, 1.0)
	cfg := types.ErosionConfig{Droplets: 500, Inertia: 0.05, Capacity: 4, ErosionRate: 0.3, Deposition: 0.3, Evaporation: 0.01, MaxStepsPerDrop: 50}
	a := Erode(7, hm.Clone(), cfg, 32)
	b := Erode(7, hm.Clone(), cfg, 32)
	for face := range a.Faces {
		for i := range a.Faces[face] {
			if a.Faces[face][i] != b.Faces[face][i] {
				t.Fatalf("non-deterministic at face %d idx %d", face, i)
			}
		}
	}
}

func TestErodeLowersPeaks(t *testing.T) {
	hm := makePyramid(32, 0.9, 0.4) // peak at +X face center, base height 0.4
	peakBefore := hm.Get(cubemap.FacePosX, 16, 16)
	cfg := types.ErosionConfig{Droplets: 5000, Inertia: 0.05, Capacity: 4, ErosionRate: 0.5, Deposition: 0.2, Evaporation: 0.01, MaxStepsPerDrop: 60, Gravity: 4}
	out := Erode(11, hm.Clone(), cfg, 32)
	peakAfter := out.Get(cubemap.FacePosX, 16, 16)
	if peakAfter >= peakBefore {
		t.Errorf("peak should be lower after erosion; before=%f after=%f", peakBefore, peakAfter)
	}
}

func makeBumpyHeightmap(S int, _ float64) *cubemap.CubeMapF {
	out := cubemap.NewF(S)
	for face := range out.Faces {
		for py := range S {
			for px := range S {
				u := float64(px)/float64(S-1) - 0.5
				v := float64(py)/float64(S-1) - 0.5
				out.Set(cubemap.Face(face), px, py, 0.5+0.2*math.Sin(u*8)*math.Cos(v*8))
			}
		}
	}
	return out
}

func makePyramid(S int, peak, base float64) *cubemap.CubeMapF {
	out := cubemap.NewF(S)
	for face := range out.Faces {
		for i := range out.Faces[face] {
			out.Faces[face][i] = base
		}
	}
	cx, cy := S/2, S/2
	for r := 0; r < S/2; r++ {
		h := peak * (1 - float64(r)/float64(S/2))
		if h < base {
			h = base
		}
		for py := cy - r; py <= cy+r; py++ {
			for px := cx - r; px <= cx+r; px++ {
				if px < 0 || px >= S || py < 0 || py >= S {
					continue
				}
				if out.Get(cubemap.FacePosX, px, py) < h {
					out.Set(cubemap.FacePosX, px, py, h)
				}
			}
		}
	}
	return out
}
