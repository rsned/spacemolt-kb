package cubemap

import (
	"image"
	"math"
)

// BakeEquirect samples the cube map at every pixel of a width×height
// equirectangular image and returns the result. Latitude maps π/2 (y=0)
// to -π/2 (y=height-1); longitude maps 0 (x=0) to ~2π (x=width-1).
func BakeEquirect(cm *CubeMap, width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for py := range height {
		lat := math.Pi/2 - (float64(py)+0.5)/float64(height)*math.Pi
		cosLat := math.Cos(lat)
		sinLat := math.Sin(lat)
		for px := range width {
			lon := (float64(px) + 0.5) / float64(width) * 2 * math.Pi
			dx := cosLat * math.Cos(lon)
			dy := sinLat
			dz := cosLat * math.Sin(lon)
			img.SetRGBA(px, py, cm.Sample(dx, dy, dz))
		}
	}
	return img
}
