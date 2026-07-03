package noise

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func TestWarpZeroAmp(t *testing.T) {
	w := NewWarper(42, types.WarpConfig{Amp: 0, Freq: 1, Octaves: 1, Lacunarity: 2, Persistence: 0.5})
	x, y, z := w.Warp(0.5, 0.5, 0.5)
	if math.Abs(x-0.5) > 1e-12 || math.Abs(y-0.5) > 1e-12 || math.Abs(z-0.5) > 1e-12 {
		t.Errorf("zero-amp warp moved point: (%f, %f, %f)", x, y, z)
	}
}

func TestWarpDeterministic(t *testing.T) {
	cfg := types.WarpConfig{Amp: 0.3, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5}
	w := NewWarper(42, cfg)
	x1, y1, z1 := w.Warp(0.1, 0.2, 0.3)
	w2 := NewWarper(42, cfg)
	x2, y2, z2 := w2.Warp(0.1, 0.2, 0.3)
	if x1 != x2 || y1 != y2 || z1 != z2 {
		t.Errorf("non-deterministic: (%f,%f,%f) vs (%f,%f,%f)", x1, y1, z1, x2, y2, z2)
	}
}

func TestWarpDisplacementBoundedByAmp(t *testing.T) {
	cfg := types.WarpConfig{Amp: 0.3, Freq: 1, Octaves: 2, Lacunarity: 2, Persistence: 0.5}
	w := NewWarper(42, cfg)
	for i := 0; i < 50; i++ {
		x, y, z := float64(i)*0.05, float64(i)*0.07, float64(i)*0.11
		wx, wy, wz := w.Warp(x, y, z)
		dx, dy, dz := wx-x, wy-y, wz-z
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		// fbm output is in [0,1] per generator's normalization, so per-axis
		// displacement is at most cfg.Amp. Magnitude is at most sqrt(3)*Amp.
		if d > cfg.Amp*math.Sqrt(3)*1.1 {
			t.Errorf("displacement %f exceeds bound %f", d, cfg.Amp*math.Sqrt(3))
		}
	}
}
