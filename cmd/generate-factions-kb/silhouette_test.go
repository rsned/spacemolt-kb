package main

import "testing"

func TestSeedHashStable(t *testing.T) {
	h1, h2 := seedHash("abc"), seedHash("abc")
	if h1 != h2 {
		t.Fatal("seedHash not stable for same input")
	}
	if seedHash("abc") == seedHash("abd") {
		t.Fatal("seedHash collided on distinct inputs")
	}
}

func TestDerivePaletteDeterministicAndNonEmpty(t *testing.T) {
	f1, a1 := derivePalette("player-123")
	f2, a2 := derivePalette("player-123")
	if f1 != f2 || a1 != a2 {
		t.Fatalf("derivePalette not deterministic: (%s,%s) vs (%s,%s)", f1, a1, f2, a2)
	}
	if f1 == "" || a1 == "" {
		t.Fatal("derivePalette returned empty color")
	}
	if g, _ := derivePalette("player-999"); g == f1 {
		t.Fatal("derivePalette gave identical field for different seeds")
	}
}
