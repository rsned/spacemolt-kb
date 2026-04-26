package cubemap

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestDirToFaceUVKnownDirections(t *testing.T) {
	cases := []struct {
		x, y, z  float64
		wantFace Face
	}{
		{1, 0, 0, FacePosX},
		{-1, 0, 0, FaceNegX},
		{0, 1, 0, FacePosY},
		{0, -1, 0, FaceNegY},
		{0, 0, 1, FacePosZ},
		{0, 0, -1, FaceNegZ},
	}
	for _, tc := range cases {
		face, u, v := DirToFaceUV(tc.x, tc.y, tc.z)
		if face != tc.wantFace {
			t.Errorf("DirToFaceUV(%v,%v,%v) face = %d, want %d",
				tc.x, tc.y, tc.z, face, tc.wantFace)
		}
		if math.Abs(u-0.5) > 1e-9 || math.Abs(v-0.5) > 1e-9 {
			t.Errorf("DirToFaceUV(%v,%v,%v) u,v = %f,%f, want 0.5,0.5",
				tc.x, tc.y, tc.z, u, v)
		}
	}
}

func TestDirFaceUVRoundtrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	const N = 10_000
	maxErr := 0.0
	for range N {
		// uniform on sphere
		u1 := rng.Float64()
		u2 := rng.Float64()
		theta := 2 * math.Pi * u1
		z := 2*u2 - 1
		r := math.Sqrt(1 - z*z)
		x := r * math.Cos(theta)
		y := r * math.Sin(theta)

		face, fu, fv := DirToFaceUV(x, y, z)
		x2, y2, z2 := FaceUVToDir(face, fu, fv)
		dx, dy, dz := x-x2, y-y2, z-z2
		d := math.Sqrt(dx*dx + dy*dy + dz*dz)
		if d > maxErr {
			maxErr = d
		}
	}
	if maxErr > 1e-12 {
		t.Errorf("max roundtrip error = %g, want < 1e-12", maxErr)
	}
}

func TestFacePixelToDirCoverage(t *testing.T) {
	const S = 8
	for face := range Face(NumFaces) {
		for py := 0; py < S; py++ {
			for px := 0; px < S; px++ {
				x, y, z := FacePixelToDir(face, px, py, S)
				mag := math.Sqrt(x*x + y*y + z*z)
				if math.Abs(mag-1.0) > 1e-12 {
					t.Errorf("face %d (%d,%d): |dir| = %f, want 1",
						face, px, py, mag)
				}
			}
		}
	}
}

func TestFacePixelToDirCenter(t *testing.T) {
	// The center pixel of FacePosX (at S/2-1, S/2-1 with bilinear-ish
	// rounding) should be very close to (1, 0, 0). With S=8 and
	// pixel-center sampling, pixel (3,3) center is at u=v=0.4375, which
	// gives sc=tc=-0.125 → direction roughly (1, 0.125, 0.125)
	// pre-normalisation. Just check we get the right *face* and that
	// the dominant axis is correct.
	x, _, _ := FacePixelToDir(FacePosX, 3, 3, 8)
	if x < 0.95 {
		t.Errorf("FacePosX (3,3) S=8 dir.x = %f, expected near 1", x)
	}
}
