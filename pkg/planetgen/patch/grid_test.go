package patch

import "testing"

func TestGridBilinear(t *testing.T) {
	g := NewGrid(2)
	g.Set(0, 0, 0)
	g.Set(1, 0, 1)
	g.Set(0, 1, 2)
	g.Set(1, 1, 3)
	if v := g.Bilinear(0.5, 0.5); v != 1.5 {
		t.Fatalf("center bilinear = %v, want 1.5", v)
	}
	// Clamped outside.
	if v := g.Bilinear(-5, -5); v != 0 {
		t.Fatalf("clamp low = %v, want 0", v)
	}
	if v := g.Bilinear(10, 10); v != 3 {
		t.Fatalf("clamp high = %v, want 3", v)
	}
}

func TestGridClone(t *testing.T) {
	g := NewGrid(4)
	g.Set(1, 1, 7)
	c := g.Clone()
	c.Set(1, 1, 9)
	if g.At(1, 1) != 7 {
		t.Fatal("Clone aliases the original")
	}
}
