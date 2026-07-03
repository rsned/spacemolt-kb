package seed

import "testing"

func TestDomainDeterminism(t *testing.T) {
	for _, name := range []string{"warp.x", "control.peaks", "biome.t"} {
		a := Domain(42, name)
		b := Domain(42, name)
		if a != b {
			t.Errorf("Domain(42, %q) returned %d then %d", name, a, b)
		}
	}
}

func TestDomainOrthogonality(t *testing.T) {
	if Domain(42, "warp.x") == Domain(42, "warp.y") {
		t.Error("different domain names should produce different seeds")
	}
}

func TestDomainMasterPropagation(t *testing.T) {
	if Domain(0, "warp.x") == Domain(1, "warp.x") {
		t.Error("different master seeds should produce different domain seeds")
	}
}
