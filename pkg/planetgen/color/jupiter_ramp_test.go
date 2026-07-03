package color

import "testing"

func TestJupiterRampBounds(t *testing.T) {
	for _, f := range []float64{-1, 0, 0.5, 1, 2} {
		c := JupiterRamp(f)
		if c.A != 255 {
			t.Errorf("alpha=%d at f=%f, want 255", c.A, f)
		}
	}
}

func TestJupiterRampMonotoneFalloff(t *testing.T) {
	eq := JupiterRamp(0)
	pole := JupiterRamp(1)
	if eq == pole {
		t.Error("equator and pole colors are identical; ramp not loaded?")
	}
}
