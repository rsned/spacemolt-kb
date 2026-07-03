// Command gen-archetype-luts writes per-archetype 16³ LUTs as
// Resolve .cube files into pkg/planetgen/color/luts/. Run once to
// regenerate after edits to the per-archetype color shifts.
package main

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/lucasb-eyer/go-colorful"
)

type archetype struct {
	name string
	// Hue shift in degrees, saturation multiplier, value multiplier.
	hueDeg, satMul, valMul float64
}

var archetypes = []archetype{
	{"scorched", -8, 0.85, 0.95},
	{"arid", -5, 1.05, 1.0},
	{"terran", 0, 1.0, 1.0},
	{"tundra", +5, 0.85, 1.05},
	{"glacial", +10, 0.8, 1.05},
	{"ice_world", +15, 0.85, 1.05},
	{"super_terran", -3, 1.1, 0.95},
	{"hothouse", -12, 1.05, 0.95},
	{"lava_world", -20, 1.15, 0.85},
	{"oceanic", +5, 1.05, 1.0},
	{"jovian", -10, 1.1, 0.95},
	{"ice_giant", +20, 0.9, 1.05},
	{"unknown", 0, 0.95, 0.95},
}

func main() {
	dir := "pkg/planetgen/color/luts"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}
	const N = 16
	for _, a := range archetypes {
		path := filepath.Join(dir, a.name+".cube")
		f, err := os.Create(path)
		if err != nil {
			panic(err)
		}
		if _, err := fmt.Fprintf(f, "TITLE \"%s archetype LUT\"\n", a.name); err != nil {
			panic(err)
		}
		if _, err := fmt.Fprintf(f, "LUT_3D_SIZE %d\n", N); err != nil {
			panic(err)
		}
		for b := 0; b < N; b++ {
			for g := 0; g < N; g++ {
				for r := 0; r < N; r++ {
					rIn := float64(r) / float64(N-1)
					gIn := float64(g) / float64(N-1)
					bIn := float64(b) / float64(N-1)
					out := shift(color.RGBA{
						R: uint8(rIn * 255),
						G: uint8(gIn * 255),
						B: uint8(bIn * 255),
						A: 255,
					}, a.hueDeg, a.satMul, a.valMul)
					rOut := float64(out.R) / 255
					gOut := float64(out.G) / 255
					bOut := float64(out.B) / 255
					if _, err := fmt.Fprintf(f, "%.6f %.6f %.6f\n", rOut, gOut, bOut); err != nil {
						panic(err)
					}
				}
			}
		}
		_ = f.Close()
		fmt.Printf("wrote %s\n", path)
	}
}

func shift(c color.RGBA, hueDeg, satMul, valMul float64) color.RGBA {
	cf := colorful.Color{R: float64(c.R) / 255, G: float64(c.G) / 255, B: float64(c.B) / 255}
	h, s, v := cf.Hsv()
	h = math.Mod(h+hueDeg+360, 360)
	s *= satMul
	v *= valMul
	if s > 1 {
		s = 1
	}
	if v > 1 {
		v = 1
	}
	out := colorful.Hsv(h, s, v).Clamped()
	return color.RGBA{
		R: uint8(math.Round(out.R * 255)),
		G: uint8(math.Round(out.G * 255)),
		B: uint8(math.Round(out.B * 255)),
		A: 255,
	}
}
