package patch

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

func terranProfile(t *testing.T) *types.PlanetProfile {
	t.Helper()
	p := planetgen.GetProfile("terran")
	if p.Crust.MajorPlates <= 0 {
		t.Fatal("terran profile is expected to be crust-enabled")
	}
	return p
}

func TestComputeSphereTerran(t *testing.T) {
	sd, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sd.Plates == nil || sd.Crust == nil || sd.FX == nil {
		t.Fatal("missing tectonic fields")
	}
	if !(sd.SeaLevel0 > 0 && sd.SeaLevel0 < 1) || !(sd.SeaLevel > 0 && sd.SeaLevel < 1) {
		t.Fatalf("sea levels out of range: %v %v", sd.SeaLevel0, sd.SeaLevel)
	}
	if sd.HMax <= sd.HMin {
		t.Fatalf("degenerate normalize bounds: [%v, %v]", sd.HMin, sd.HMax)
	}
	// Determinism.
	sd2, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	if sd2.SeaLevel != sd.SeaLevel || sd2.HMin != sd.HMin || sd2.HMax != sd.HMax {
		t.Fatal("ComputeSphere is not deterministic")
	}
}

func TestComputeSphereRejectsLegacy(t *testing.T) {
	p := planetgen.GetProfile("scorched") // no crust path
	if _, err := ComputeSphere(p, 1, 32); err == nil {
		t.Fatal("legacy (non-crust) profile must be rejected")
	}
}
