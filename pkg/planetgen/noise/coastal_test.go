package noise

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func TestCoastalNoOpFarFromCoast(t *testing.T) {
	h := 0.7
	base := h
	out := ApplyCoastal(NewCoastalGen(123), 0, 1, 0, h, 1.0, 0.1, 0.5, 1.0)
	// distance = 1.0 (max), effect must be zero.
	if out != base {
		t.Errorf("far-from-coast pixel changed: got %f, want %f", out, base)
	}
}

func TestCoastalAmpZeroIsIdentity(t *testing.T) {
	if got := ApplyCoastal(NewCoastalGen(7), 0, 1, 0, 0.5, 0.0, 0, 0.5, 1.0); got != 0.5 {
		t.Errorf("Amp=0 should be identity; got %f", got)
	}
}

func TestCoastalDeterministic(t *testing.T) {
	a := ApplyCoastal(NewCoastalGen(99), 0.3, 0.4, 0.5, 0.6, 0.0, 0.1, 0.5, 4.0)
	b := ApplyCoastal(NewCoastalGen(99), 0.3, 0.4, 0.5, 0.6, 0.0, 0.1, 0.5, 4.0)
	if a != b {
		t.Errorf("non-deterministic; %f vs %f", a, b)
	}
	_ = cubemap.FacePosX // keep import
}
