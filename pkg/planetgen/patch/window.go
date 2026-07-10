package patch

import (
	"fmt"
	"math"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// Window locates a patch: a Size×Size pixel window on one cube face of
// a virtual full-resolution cube map with SProd pixels per face edge.
// Patch pixel (ix,iy) is virtual face pixel (X0+ix, Y0+iy), so its
// sphere direction — and therefore every 3D noise sample — is exactly
// what the production render computes for that pixel at S=SProd.
type Window struct {
	Face  cubemap.Face `json:"face"`
	X0    int          `json:"x0"`
	Y0    int          `json:"y0"`
	Size  int          `json:"size"`
	SProd int          `json:"sProd"`
}

func (w Window) Valid() error {
	if w.Face < 0 || w.Face >= cubemap.NumFaces {
		return fmt.Errorf("patch: invalid face %d", w.Face)
	}
	if w.Size <= 0 || w.SProd <= 0 {
		return fmt.Errorf("patch: non-positive size %d / sProd %d", w.Size, w.SProd)
	}
	if w.X0 < 0 || w.Y0 < 0 || w.X0+w.Size > w.SProd || w.Y0+w.Size > w.SProd {
		return fmt.Errorf("patch: window (%d,%d)+%d overflows face of %d", w.X0, w.Y0, w.Size, w.SProd)
	}
	return nil
}

// Dir returns the unit sphere direction at the CENTER of patch pixel
// (ix, iy) — identical to the production cube path's direction for the
// same virtual pixel.
func (w Window) Dir(ix, iy int) (x, y, z float64) {
	return cubemap.FacePixelToDir(w.Face, w.X0+ix, w.Y0+iy, w.SProd)
}

// PxRad is the angular size of one virtual production pixel in
// radians: a cube face spans π/2 radians across SProd pixels.
func (w Window) PxRad() float64 { return (math.Pi / 2) / float64(w.SProd) }

// Sampler returns a direction-space sampler over a patch grid — the
// patch analog of CubeMapF.Sample. Directions off the window clamp to
// its border (open-patch edge policy).
func (w Window) Sampler(g *Grid) func(x, y, z float64) float64 {
	return func(x, y, z float64) float64 {
		u, v := cubemap.ForceFaceUV(w.Face, x, y, z)
		px := u*float64(w.SProd) - 0.5 - float64(w.X0)
		py := v*float64(w.SProd) - 0.5 - float64(w.Y0)
		return g.Bilinear(px, py)
	}
}
