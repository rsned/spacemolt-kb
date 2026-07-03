package jumpmap

import (
	"math"
	"reflect"
	"testing"
)

func close(a, b float64) bool { return math.Abs(a-b) <= 1e-6 }

func TestPolar(t *testing.T) {
	// Server convention: 0deg = +X (right); +Y renders DOWN the page (as the
	// galaxy/empire maps do), so heading 90deg (+Y) points downward on screen.
	cx, cy, r := 100.0, 100.0, 10.0
	tests := []struct {
		deg        float64
		wantX, wantY float64
	}{
		{0, 110, 100},   // right
		{90, 100, 110},  // +Y -> down (larger y)
		{180, 90, 100},  // left
		{270, 100, 90},  // -Y -> up (smaller y)
	}
	for _, tt := range tests {
		x, y := polar(cx, cy, r, tt.deg)
		if !close(x, tt.wantX) || !close(y, tt.wantY) {
			t.Errorf("polar(%v) = (%v,%v), want (%v,%v)", tt.deg, x, y, tt.wantX, tt.wantY)
		}
	}
}

func TestAssignLabelRings(t *testing.T) {
	// Ascending headings; labels within minSep must move to an outer ring.
	got := assignLabelRings([]float64{10, 10.5, 11, 50}, 2)
	want := []int{0, 1, 2, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assignLabelRings = %v, want %v", got, want)
	}
}

func TestAssignLabelRings_allSpread(t *testing.T) {
	// Well-separated headings all stay on ring 0.
	got := assignLabelRings([]float64{0, 20, 40, 60}, 5)
	want := []int{0, 0, 0, 0}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("assignLabelRings = %v, want %v", got, want)
	}
}
