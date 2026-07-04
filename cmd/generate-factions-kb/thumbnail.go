package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"golang.org/x/image/draw"
)

// thumbName is the fixed filename of a passenger landing-page thumbnail, written
// alongside the hero image in each passenger's output dir.
const thumbName = "thumb.png"

// thumbSize is the square edge length (px) of a landing-page thumbnail. Shown at
// half this on the page, so 64px stays crisp on high-DPI displays while keeping
// each file to a few KB (vs the ~330KB 512px hero it is derived from).
const thumbSize = 64

// writeThumbnail decodes the image at srcPath, downscales it to a thumbSize
// square using Catmull-Rom resampling, and writes it as a PNG to destPath.
// Sources are already square portraits, so no cropping is needed.
func writeThumbnail(srcPath, destPath string, size int) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decode %s: %w", srcPath, err)
	}

	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(out, dst); err != nil {
		return fmt.Errorf("encode %s: %w", destPath, err)
	}
	return nil
}
