package main

import (
	"strings"
	"testing"
)

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

func TestSilhouetteSVGDeterministic(t *testing.T) {
	a := string(silhouetteSVG("id-1", "#112233", "#445566"))
	b := string(silhouetteSVG("id-1", "#112233", "#445566"))
	if a != b {
		t.Fatal("silhouetteSVG not byte-identical for same input")
	}
	if !strings.Contains(a, "<svg class=\"silhouette\"") {
		t.Fatalf("missing svg root: %s", a)
	}
	if !strings.Contains(a, "#112233") || !strings.Contains(a, "#445566") {
		t.Fatal("provided colors not used in SVG")
	}
}

func TestSilhouetteSVGVariesBySeed(t *testing.T) {
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		seen[string(silhouetteSVG(id, "", ""))] = true
	}
	if len(seen) < 2 {
		t.Fatalf("silhouettes did not vary across seeds: %d distinct", len(seen))
	}
}

func TestSilhouetteSVGEmptyColorsUsesDerivedPalette(t *testing.T) {
	out := string(silhouetteSVG("id-x", "", ""))
	if !strings.Contains(out, "hsl(") {
		t.Fatal("empty colors should fall back to derived hsl palette, got none")
	}
}
