package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestSmoothHeightmapZeroRadiusNoOp(t *testing.T) {
	hm := makeBumpyHeightmap(32, 1.0)
	out := SmoothHeightmap(hm, 0, 32)
	for face := range out.Faces {
		for i := range out.Faces[face] {
			if out.Faces[face][i] != hm.Faces[face][i] {
				t.Fatalf("r=0 should be no-op")
			}
		}
	}
}

func TestSmoothHeightmapReducesVariance(t *testing.T) {
	hm := makeBumpyHeightmap(32, 1.0)
	out := SmoothHeightmap(hm, 2, 32)
	varBefore := variance(hm)
	varAfter := variance(out)
	if varAfter >= varBefore {
		t.Errorf("blur should reduce variance: before=%f after=%f", varBefore, varAfter)
	}
}

func TestSmoothHeightmapPreservesMean(t *testing.T) {
	hm := makeBumpyHeightmap(32, 1.0)
	out := SmoothHeightmap(hm, 2, 32)
	meanBefore := mean(hm)
	meanAfter := mean(out)
	diff := meanAfter - meanBefore
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.005 {
		t.Errorf("blur should preserve global mean within 0.005: before=%f after=%f", meanBefore, meanAfter)
	}
}

func mean(cm *cubemap.CubeMapF) float64 {
	sum := 0.0
	n := 0
	for face := range cm.Faces {
		for _, v := range cm.Faces[face] {
			sum += v
			n++
		}
	}
	return sum / float64(n)
}

func variance(cm *cubemap.CubeMapF) float64 {
	m := mean(cm)
	s := 0.0
	n := 0
	for face := range cm.Faces {
		for _, v := range cm.Faces[face] {
			s += (v - m) * (v - m)
			n++
		}
	}
	return s / float64(n)
}
