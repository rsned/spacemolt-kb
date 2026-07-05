package patch

import (
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

func testSphere(t *testing.T) *SphereData {
	t.Helper()
	sd, err := ComputeSphere(terranProfile(t), 4242, 64)
	if err != nil {
		t.Fatal(err)
	}
	return sd
}

func TestExtractFieldsMatchesSample(t *testing.T) {
	sd := testSphere(t)
	w := Window{Face: cubemap.FacePosZ, X0: 32, Y0: 32, Size: 64, SProd: 128}
	f, err := ExtractFields(sd, w)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range [][2]int{{0, 0}, {63, 63}, {10, 50}} {
		x, y, z := w.Dir(p[0], p[1])
		want := sd.Crust.BaseHeight.Sample(x, y, z)
		got := f.BaseHeight.At(p[0], p[1])
		if math.Abs(got-want) > 1e-12 {
			t.Fatalf("BaseHeight at (%d,%d): got %v want %v", p[0], p[1], got, want)
		}
	}
	// PlateID is nearest-neighbor and must be a real plate id.
	if f.PlateID[0] < 0 || int(f.PlateID[0]) >= len(sd.Plates.Plates) {
		t.Fatalf("PlateID[0] out of range: %d", f.PlateID[0])
	}
}

func TestExtractFieldsRejectsInvalidWindow(t *testing.T) {
	sd := testSphere(t)
	if _, err := ExtractFields(sd, Window{Face: cubemap.FacePosZ, X0: 100, Y0: 0, Size: 64, SProd: 128}); err == nil {
		t.Fatal("invalid window accepted")
	}
}
