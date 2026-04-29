package color

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	_ "image/png"
)

//go:embed luts/jupiter_ramp.png
var jupiterRampPNG []byte

var jupiterRamp [256]color.RGBA

func init() {
	img, _, err := image.Decode(bytes.NewReader(jupiterRampPNG))
	if err != nil {
		return
	}
	b := img.Bounds()
	if b.Dx() < 256 {
		return
	}
	for i := range 256 {
		r, g, b2, a := img.At(b.Min.X+i, b.Min.Y).RGBA()
		jupiterRamp[i] = color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b2 >> 8), A: uint8(a >> 8)}
	}
}

// JupiterRamp returns the embedded Jupiter latitude-band palette sampled by
// latitude fraction f in [0, 1] (0 = equator, 1 = pole).
// Values outside [0,1] are clamped. Alpha is always 255.
func JupiterRamp(f float64) color.RGBA {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	x := f * 255
	i0 := int(x)
	if i0 >= 255 {
		return jupiterRamp[255]
	}
	t := x - float64(i0)
	a := jupiterRamp[i0]
	b := jupiterRamp[i0+1]
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}
