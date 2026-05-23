package hyperjump

import (
	"math"
	"testing"
)

const eps = 1e-9

func almostEqual(a, b float64) bool { return math.Abs(a-b) <= 1e-6 }

func TestBearing(t *testing.T) {
	tests := []struct {
		name string
		a, b Vec
		want float64
	}{
		{"+X is 0", Vec{0, 0}, Vec{1, 0}, 0},
		{"+Y is 90", Vec{0, 0}, Vec{0, 1}, 90},
		{"-X is 180", Vec{0, 0}, Vec{-1, 0}, 180},
		{"-Y is 270", Vec{0, 0}, Vec{0, -1}, 270},
		{"diagonal is 45", Vec{1, 1}, Vec{2, 2}, 45},
		{"translated origin", Vec{10, 10}, Vec{10, 5}, 270},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Bearing(tt.a, tt.b)
			if !almostEqual(got, tt.want) {
				t.Errorf("Bearing(%v,%v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestProj(t *testing.T) {
	// rel = (10,0): straight ahead at heading 0 -> proj 10; at heading 90 -> proj 0.
	if got := Proj(Vec{10, 0}, 0); !almostEqual(got, 10) {
		t.Errorf("Proj along heading = %v, want 10", got)
	}
	if got := Proj(Vec{10, 0}, 90); !almostEqual(got, 0) {
		t.Errorf("Proj perpendicular = %v, want 0", got)
	}
	// behind: rel=(-5,0) at heading 0 -> proj -5 (negative => behind)
	if got := Proj(Vec{-5, 0}, 0); !almostEqual(got, -5) {
		t.Errorf("Proj behind = %v, want -5", got)
	}
}

func TestSignedPerp(t *testing.T) {
	// rel=(10,0) aimed at heading 0 -> on the ray, perp 0.
	if got := SignedPerp(Vec{10, 0}, 0); math.Abs(got) > eps {
		t.Errorf("SignedPerp on ray = %v, want 0", got)
	}
	// rel=(10,0) at heading 90: signedPerp = relX*sin - relY*cos = 10.
	if got := SignedPerp(Vec{10, 0}, 90); !almostEqual(got, 10) {
		t.Errorf("SignedPerp = %v, want 10", got)
	}
	// rel=(0,10) at heading 0: signedPerp = 0*0 - 10*1 = -10 (target to the left -> negative).
	if got := SignedPerp(Vec{0, 10}, 0); !almostEqual(got, -10) {
		t.Errorf("SignedPerp left = %v, want -10", got)
	}
}

func TestArcHalfWidth(t *testing.T) {
	tests := []struct {
		name         string
		r, margin    float64
		want         float64
	}{
		{"margin/r = 0.5 -> 30deg", 200, 100, 30},
		{"r == margin -> 90deg", 100, 100, 90},
		{"r < margin clamps to 90", 50, 100, 90},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ArcHalfWidth(tt.r, tt.margin)
			if !almostEqual(got, tt.want) {
				t.Errorf("ArcHalfWidth(%v,%v) = %v, want %v", tt.r, tt.margin, got, tt.want)
			}
		})
	}
}
