// Package main implements a side-by-side image diff CLI for the
// planet generator's golden image harness.
//
// Usage:
//
//	planet-image-diff <a.png> <b.png> [<diff.png>]
//
// Prints the mean ΔE2000 between corresponding pixels and (if a
// third argument is given) writes a diff image with A on the left,
// B in the middle, and an amplified per-pixel difference on the right.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/lucasb-eyer/go-colorful"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: planet-image-diff <a.png> <b.png> [<diff.png>]")
		os.Exit(2)
	}
	a := readPNG(os.Args[1])
	b := readPNG(os.Args[2])
	if a.Bounds() != b.Bounds() {
		log.Fatalf("size mismatch: %v vs %v", a.Bounds(), b.Bounds())
	}
	w, h := a.Bounds().Dx(), a.Bounds().Dy()

	var sumDE float64
	var n int
	maxChannelDelta := 0.0
	for y := range h {
		for x := range w {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			ca := colorful.Color{
				R: float64(ar) / 65535,
				G: float64(ag) / 65535,
				B: float64(ab) / 65535,
			}
			cb := colorful.Color{
				R: float64(br) / 65535,
				G: float64(bg) / 65535,
				B: float64(bb) / 65535,
			}
			sumDE += ca.DistanceCIEDE2000(cb)
			n++
			cd := math.Max(
				math.Max(math.Abs(float64(ar)-float64(br)),
					math.Abs(float64(ag)-float64(bg))),
				math.Abs(float64(ab)-float64(bb))) / 257
			if cd > maxChannelDelta {
				maxChannelDelta = cd
			}
		}
	}
	meanDE := sumDE / float64(n)
	fmt.Printf("mean ΔE2000 = %.3f\n", meanDE)
	fmt.Printf("max channel delta = %.0f / 255\n", maxChannelDelta)

	if len(os.Args) >= 4 {
		writeDiffImage(a, b, os.Args[3])
		fmt.Printf("diff image: %s\n", os.Args[3])
	}
}

func readPNG(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		log.Fatalf("decode %s: %v", path, err)
	}
	return img
}

func writeDiffImage(a, b image.Image, path string) {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	out := image.NewRGBA(image.Rect(0, 0, 3*w, h))
	for y := range h {
		for x := range w {
			ca := a.At(x, y)
			cb := b.At(x, y)
			out.Set(x, y, ca)
			out.Set(w+x, y, cb)
			ar, ag, ab, _ := ca.RGBA()
			br, bg, bbB, _ := cb.RGBA()
			dr := abs8(int(ar)-int(br)) >> 8
			dg := abs8(int(ag)-int(bg)) >> 8
			db := abs8(int(ab)-int(bbB)) >> 8
			m := dr
			if dg > m {
				m = dg
			}
			if db > m {
				m = db
			}
			scaled := m * 4
			if scaled > 255 {
				scaled = 255
			}
			amp := uint8(scaled)
			out.Set(2*w+x, y, color.RGBA{R: amp, G: amp, B: amp, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, out); err != nil {
		log.Fatal(err)
	}
}

func abs8(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
