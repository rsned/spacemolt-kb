package field

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestQuantileSeaLevelUniform(t *testing.T) {
	const S = 64
	hm := cubemap.NewF(S)
	rng := rand.New(rand.NewPCG(1, 2))
	for f := range hm.Faces {
		for i := range hm.Faces[f] {
			hm.Faces[f][i] = rng.Float64()
		}
	}
	for _, oceanFrac := range []float64{0.3, 0.5, 0.7, 0.9} {
		lvl := QuantileSeaLevel(hm, oceanFrac)
		var below, total int
		for f := range hm.Faces {
			for _, h := range hm.Faces[f] {
				if h < lvl {
					below++
				}
				total++
			}
		}
		got := float64(below) / float64(total)
		if math.Abs(got-oceanFrac) > 0.01 {
			t.Errorf("oceanFrac %v: %v of pixels below level %v", oceanFrac, got, lvl)
		}
	}
}

func TestQuantileSeaLevelEdgeCases(t *testing.T) {
	const S = 8
	hm := cubemap.NewF(S)
	if lvl := QuantileSeaLevel(hm, 0.5); lvl < 0 || lvl > 1 {
		t.Errorf("flat heightmap level %v out of [0,1]", lvl)
	}
	if lvl := QuantileSeaLevel(hm, 0); lvl != 0 {
		t.Errorf("oceanFrac 0 → level %v, want 0", lvl)
	}
}
