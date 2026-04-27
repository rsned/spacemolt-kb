package color

import (
	"bufio"
	"fmt"
	"image/color"
	"math"
	"strings"
)

// LUT is a 3D color lookup table (sized N×N×N, typically N=16 or 32).
// Inputs and outputs are RGB ∈ [0,1]³.
type LUT struct {
	Size int       // N
	Data []float64 // length 3*N³, packed (R,G,B) stride
	Name string    // optional, e.g. "terran"
}

// ApplyLUT trilinearly samples the LUT at the input color and returns
// the graded output. Input and output are 8-bit RGBA; alpha is preserved.
func ApplyLUT(lut LUT, in color.RGBA) color.RGBA {
	if lut.Size <= 1 || len(lut.Data) != 3*lut.Size*lut.Size*lut.Size {
		return in
	}
	n := lut.Size
	r := float64(in.R) / 255 * float64(n-1)
	g := float64(in.G) / 255 * float64(n-1)
	b := float64(in.B) / 255 * float64(n-1)
	r0, g0, b0 := int(math.Floor(r)), int(math.Floor(g)), int(math.Floor(b))
	r1 := min(r0+1, n-1)
	g1 := min(g0+1, n-1)
	b1 := min(b0+1, n-1)
	tr, tg, tb := r-float64(r0), g-float64(g0), b-float64(b0)

	c000 := lutFetch(lut, r0, g0, b0)
	c100 := lutFetch(lut, r1, g0, b0)
	c010 := lutFetch(lut, r0, g1, b0)
	c110 := lutFetch(lut, r1, g1, b0)
	c001 := lutFetch(lut, r0, g0, b1)
	c101 := lutFetch(lut, r1, g0, b1)
	c011 := lutFetch(lut, r0, g1, b1)
	c111 := lutFetch(lut, r1, g1, b1)

	c00 := lerp3(c000, c100, tr)
	c10 := lerp3(c010, c110, tr)
	c01 := lerp3(c001, c101, tr)
	c11 := lerp3(c011, c111, tr)
	c0 := lerp3(c00, c10, tg)
	c1 := lerp3(c01, c11, tg)
	c := lerp3(c0, c1, tb)

	return color.RGBA{
		R: uint8(math.Round(c[0] * 255)),
		G: uint8(math.Round(c[1] * 255)),
		B: uint8(math.Round(c[2] * 255)),
		A: in.A,
	}
}

func lutFetch(lut LUT, r, g, b int) [3]float64 {
	idx := 3 * (b*lut.Size*lut.Size + g*lut.Size + r)
	return [3]float64{lut.Data[idx], lut.Data[idx+1], lut.Data[idx+2]}
}

func lerp3(a, b [3]float64, t float64) [3]float64 {
	return [3]float64{
		a[0]*(1-t) + b[0]*t,
		a[1]*(1-t) + b[1]*t,
		a[2]*(1-t) + b[2]*t,
	}
}

// ParseCubeLUT parses a Resolve .cube text-format LUT.
// Recognized header line: `LUT_3D_SIZE N` (e.g., 16, 32).
// Body: N³ lines of "R G B" floats in [0,1], in (R, G, B) ordering with R innermost.
// Comments start with `#` and are ignored.
func ParseCubeLUT(text string) (LUT, error) {
	var size int
	var data []float64
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		switch fields[0] {
		case "LUT_3D_SIZE":
			if _, err := fmt.Sscanf(line, "LUT_3D_SIZE %d", &size); err != nil {
				return LUT{}, fmt.Errorf("invalid LUT_3D_SIZE: %v", err)
			}
		case "TITLE", "DOMAIN_MIN", "DOMAIN_MAX":
			// Optional headers; skip.
			continue
		default:
			if size == 0 {
				return LUT{}, fmt.Errorf("encountered data before LUT_3D_SIZE")
			}
			if len(fields) != 3 {
				return LUT{}, fmt.Errorf("expected 3 floats, got %d", len(fields))
			}
			var r, g, b float64
			if _, err := fmt.Sscanf(line, "%f %f %f", &r, &g, &b); err != nil {
				return LUT{}, fmt.Errorf("parse %q: %v", line, err)
			}
			data = append(data, r, g, b)
		}
	}
	if err := scanner.Err(); err != nil {
		return LUT{}, err
	}
	expected := 3 * size * size * size
	if len(data) != expected {
		return LUT{}, fmt.Errorf("data length %d != expected %d for size=%d", len(data), expected, size)
	}
	return LUT{Size: size, Data: data}, nil
}
