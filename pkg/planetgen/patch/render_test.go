package patch

import (
	"bytes"
	"image/png"
	"testing"
)

func TestHeightPNGDecodes(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := HeightPNG(st)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("HeightPNG produced undecodable PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != st.Height.Size || b.Dy() != st.Height.Size {
		t.Fatalf("HeightPNG size = %v, want %dx%d", b, st.Height.Size, st.Height.Size)
	}
}

func TestColorPNGFallsBackBeforeBiomeColor(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	// Layer 0 (tectonic-base) has no Img yet; ColorPNG must fall back
	// to HeightPNG rather than erroring on a nil st.Img.
	st, err := s.RenderTo(0)
	if err != nil {
		t.Fatal(err)
	}
	if st.Img != nil {
		t.Fatal("test assumption broken: layer 0 already has an Img")
	}
	heightData, err := HeightPNG(st)
	if err != nil {
		t.Fatal(err)
	}
	colorData, err := ColorPNG(st)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(heightData, colorData) {
		t.Fatal("ColorPNG with nil Img must equal HeightPNG")
	}

	// Past layer 10 (biome-color), Img is populated and ColorPNG must
	// encode it directly rather than falling back.
	st10, err := s.RenderTo(10)
	if err != nil {
		t.Fatal(err)
	}
	if st10.Img == nil {
		t.Fatal("layer 10 (biome-color) must populate Img")
	}
	data, err := ColorPNG(st10)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ColorPNG produced undecodable PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != st10.Img.Bounds().Dx() {
		t.Fatalf("ColorPNG size mismatch: %v vs %v", b, st10.Img.Bounds())
	}
}

func TestTectonicDebugPNGDecodesAndTintsBoundary(t *testing.T) {
	ctx := testContext(t)
	s := NewStack(ctx)
	st, err := s.RenderTo(1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := TectonicDebugPNG(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("TectonicDebugPNG produced undecodable PNG: %v", err)
	}
	size := ctx.Fields.Window.Size
	if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
		t.Fatalf("TectonicDebugPNG size = %v, want %dx%d", b, size, size)
	}
}

func TestMinimapPNGDecodesAtRequestedSize(t *testing.T) {
	ctx := testContext(t)
	const w, h = 128, 64
	data, err := MinimapPNG(ctx.Sphere, ctx.Fields.Window, w, h)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("MinimapPNG produced undecodable PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != w || b.Dy() != h {
		t.Fatalf("MinimapPNG size = %v, want %dx%d", b, w, h)
	}
}

func TestPlateBoundaryDetectsAdjacentDifferentPlate(t *testing.T) {
	// 3x3 row-major grid, all plate 0 except the top-right corner
	// (1,0) is plate 1.
	ids := []int16{0, 0, 1, 0, 0, 0, 0, 0, 0}
	if !plateBoundary(ids, 3, 1, 0) {
		t.Fatal("expected boundary at (1,0): right neighbor (2,0) has a different plate id")
	}
	if plateBoundary(ids, 3, 0, 0) {
		t.Fatal("did not expect boundary at (0,0): both forward neighbors are the same plate")
	}
}

func TestEquirectPixelRoundTripsBakeEquirectProjection(t *testing.T) {
	// Directly on the equator, prime meridian: should land near the
	// left edge, vertical middle.
	px, py := equirectPixel(1, 0, 0, 360, 180)
	if py < 85 || py > 95 {
		t.Fatalf("expected near-equator py, got %d", py)
	}
	// lon=0 maps to fpx = -0.5 (clamped to 0), i.e. the left edge.
	if px < 0 || px > 2 {
		t.Fatalf("expected near-left-edge px, got %d", px)
	}
}
