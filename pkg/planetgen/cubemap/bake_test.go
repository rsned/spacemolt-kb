package cubemap

import (
	"image/color"
	"testing"
)

func TestBakeIdentity(t *testing.T) {
	// Paint each face a distinct color, bake to a small equirect,
	// and verify each face's color appears in its expected region.
	cm := New(8)
	colors := [NumFaces]color.RGBA{
		FacePosX: {255, 0, 0, 255},
		FaceNegX: {0, 255, 0, 255},
		FacePosY: {0, 0, 255, 255},
		FaceNegY: {255, 255, 0, 255},
		FacePosZ: {0, 255, 255, 255},
		FaceNegZ: {255, 0, 255, 255},
	}
	for face := range Face(NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = colors[face]
		}
	}
	img := BakeEquirect(cm, 64, 32)
	if img.Bounds().Dx() != 64 || img.Bounds().Dy() != 32 {
		t.Fatalf("bake size: got %v", img.Bounds())
	}
	// Equirect (px=W/4, py=H/2) → lon=π/2, lat=0 → dir=(0,0,1) → +Z face.
	c := img.RGBAAt(16, 16)
	if c != colors[FacePosZ] {
		t.Errorf("center-of-+Z bake = %v, want %v", c, colors[FacePosZ])
	}
	// Equirect (px=0, py=H/2) → lon=0, lat=0 → dir=(1,0,0) → +X face.
	c = img.RGBAAt(0, 16)
	if c != colors[FacePosX] {
		t.Errorf("+X-axis bake = %v, want %v", c, colors[FacePosX])
	}
	// Equirect (px=W/2, py=H/2) → lon=π, lat=0 → dir=(-1,0,0) → -X face.
	c = img.RGBAAt(32, 16)
	if c != colors[FaceNegX] {
		t.Errorf("-X-axis bake = %v, want %v", c, colors[FaceNegX])
	}
	// Equirect (px=W/2, py=0) → lat=π/2 (~) → near +Y.
	c = img.RGBAAt(32, 0)
	if c != colors[FacePosY] {
		t.Errorf("+Y-pole bake = %v, want %v", c, colors[FacePosY])
	}
}

func TestBakeSeamWrap(t *testing.T) {
	// Sampling at lon=0 from the right end of the equirect should
	// produce the same color as sampling at lon=0 from the left end,
	// modulo bilinear-edge effects.
	cm := New(16)
	for i := range cm.Faces[FacePosX] {
		cm.Faces[FacePosX][i] = color.RGBA{200, 100, 50, 255}
	}
	img := BakeEquirect(cm, 64, 32)
	cL := img.RGBAAt(0, 16)
	cR := img.RGBAAt(63, 16)
	dr := int(cL.R) - int(cR.R)
	if dr < -8 || dr > 8 {
		t.Errorf("seam Δr = %d, want |Δr| ≤ 8", dr)
	}
}
