package color

import (
	"image/color"
	"testing"
)

func TestApplyIdentityLUT(t *testing.T) {
	// Identity LUT: output = input.
	lut := identityLUT(8)
	in := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	out := ApplyLUT(lut, in)
	dr := int(in.R) - int(out.R)
	dg := int(in.G) - int(out.G)
	db := int(in.B) - int(out.B)
	if abs(dr) > 4 || abs(dg) > 4 || abs(db) > 4 {
		t.Errorf("identity LUT changed pixel: %v → %v", in, out)
	}
}

func TestParseCubeFile(t *testing.T) {
	cube := `LUT_3D_SIZE 2
0 0 0
1 0 0
0 1 0
1 1 0
0 0 1
1 0 1
0 1 1
1 1 1
`
	lut, err := ParseCubeLUT(cube)
	if err != nil {
		t.Fatal(err)
	}
	if lut.Size != 2 {
		t.Errorf("size = %d, want 2", lut.Size)
	}
	if len(lut.Data) != 24 {
		t.Errorf("data len = %d, want 24", len(lut.Data))
	}
}

func identityLUT(n int) LUT {
	data := make([]float64, 3*n*n*n)
	idx := 0
	for b := 0; b < n; b++ {
		for g := 0; g < n; g++ {
			for r := 0; r < n; r++ {
				data[idx+0] = float64(r) / float64(n-1)
				data[idx+1] = float64(g) / float64(n-1)
				data[idx+2] = float64(b) / float64(n-1)
				idx += 3
			}
		}
	}
	return LUT{Size: n, Data: data}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
