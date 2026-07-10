package patch

import "testing"

func TestPickDeterministicAndValid(t *testing.T) {
	sd := testSphere(t)
	a := Pick(sd, 64, 128, 5)
	b := Pick(sd, 64, 128, 5)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("Pick returned %d/%d candidates", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("candidate %d differs between runs", i)
		}
		if err := a[i].Window.Valid(); err != nil {
			t.Fatalf("candidate %d invalid: %v", i, err)
		}
	}
	// Ranked descending.
	for i := 1; i < len(a); i++ {
		if a[i].Score > a[i-1].Score {
			t.Fatal("candidates not sorted by score desc")
		}
	}
	// The top window on a terran world should straddle land AND ocean.
	f, err := ExtractFields(sd, a[0].Window)
	if err != nil {
		t.Fatal(err)
	}
	land, ocean := 0, 0
	for _, v := range f.ContinentalMask.Data {
		if v > 0.5 {
			land++
		} else {
			ocean++
		}
	}
	if land == 0 || ocean == 0 {
		t.Fatalf("top window is single-domain: land=%d ocean=%d", land, ocean)
	}
}
