package patch

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"

	planetcolor "github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// HeightPNG renders st.Height as a grayscale PNG, clamping [0,1] to
// 0..255.
func HeightPNG(st *State) ([]byte, error) {
	size := st.Height.Size
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i, v := range st.Height.Data {
		g := uint8(min(255, max(0, int(v*255))))
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = g, g, g, 255
	}
	return encodePNG(img)
}

// ColorPNG renders st.Img — the biome/waterline-colored render — as a
// PNG. Layers before biome-color (index < 10) leave Img nil, so this
// falls back to HeightPNG.
func ColorPNG(st *State) ([]byte, error) {
	if st.Img == nil {
		return HeightPNG(st)
	}
	return encodePNG(st.Img)
}

// fxClass is one tectonic FX class's cropped patch fields and debug
// tint color, shared by TectonicDebugPNG (patch-resolution Dist/Mag
// grids) and minimapTint (sphere-resolution CubeMapF fields).
type fxClass struct {
	Dist, Mag *Grid
	Tint      color.RGBA
}

// fxTintEnvKm is the distance (in km) beyond which an FX class's tint
// contributes nothing — the boundary "activity" envelope described in
// the task brief (Dist < 300km).
const fxTintEnvKm = 300.0

// fxTint blends an FX class's tint over base using alpha = env*mag,
// env = max(0, 1 - dist/fxTintEnvKm).
func fxTint(base color.RGBA, dist, mag float64, tint color.RGBA) color.RGBA {
	env := 1 - dist/fxTintEnvKm
	if env <= 0 {
		return base
	}
	alpha := env * mag
	if alpha <= 0 {
		return base
	}
	return planetcolor.Blend(base, tint, alpha)
}

// TectonicDebugPNG renders the height grayscale tinted by proximity to
// each of the five tectonic FX classes (belt=red, subduction=orange,
// arc=yellow, ridge=cyan, rift=magenta), with dark lines drawn along
// PlateID boundaries. Used as the layer-0/1 debug view and the
// wizard's "tectonic preview".
func TectonicDebugPNG(ctx *Context, st *State) ([]byte, error) {
	f := ctx.Fields
	size := f.Window.Size
	classes := []fxClass{
		{f.BeltDist, f.BeltMag, color.RGBA{R: 200, G: 40, B: 40, A: 255}},
		{f.SubdDist, f.SubdMag, color.RGBA{R: 230, G: 120, B: 30, A: 255}},
		{f.ArcDist, f.ArcMag, color.RGBA{R: 230, G: 210, B: 60, A: 255}},
		{f.RidgeDist, f.RidgeMag, color.RGBA{R: 60, G: 200, B: 220, A: 255}},
		{f.RiftDist, f.RiftMag, color.RGBA{R: 200, G: 60, B: 200, A: 255}},
	}

	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for iy := range size {
		for ix := range size {
			i := iy*size + ix
			g := uint8(min(255, max(0, int(st.Height.At(ix, iy)*255))))
			c := color.RGBA{R: g, G: g, B: g, A: 255}
			for _, cls := range classes {
				c = fxTint(c, cls.Dist.Data[i], cls.Mag.Data[i], cls.Tint)
			}
			if plateBoundary(f.PlateID, size, ix, iy) {
				c = planetcolor.Blend(c, color.RGBA{A: 255}, 0.6)
			}
			img.SetRGBA(ix, iy, c)
		}
	}
	return encodePNG(img)
}

// plateBoundary reports whether patch pixel (ix,iy) sits adjacent (in
// the row-major PlateID grid) to a pixel of a different plate.
func plateBoundary(plateID []int16, size, ix, iy int) bool {
	id := plateID[iy*size+ix]
	if ix+1 < size && plateID[iy*size+ix+1] != id {
		return true
	}
	if iy+1 < size && plateID[(iy+1)*size+ix] != id {
		return true
	}
	return false
}

// MinimapPNG bakes an equirectangular map of the sphere's continental
// mask, tinted with the same FX classes as TectonicDebugPNG, with the
// patch window's footprint outlined in white.
func MinimapPNG(sd *SphereData, w Window, width, height int) ([]byte, error) {
	base := cubemap.GrayscaleFromF(sd.Crust.ContinentalMask)
	classes := []struct {
		Dist, Mag *cubemap.CubeMapF
		Tint      color.RGBA
	}{
		{sd.FX.BeltDist, sd.FX.BeltMag, color.RGBA{R: 200, G: 40, B: 40, A: 255}},
		{sd.FX.SubdDist, sd.FX.SubdMag, color.RGBA{R: 230, G: 120, B: 30, A: 255}},
		{sd.FX.ArcDist, sd.FX.ArcMag, color.RGBA{R: 230, G: 210, B: 60, A: 255}},
		{sd.FX.RidgeDist, sd.FX.RidgeMag, color.RGBA{R: 60, G: 200, B: 220, A: 255}},
		{sd.FX.RiftDist, sd.FX.RiftMag, color.RGBA{R: 200, G: 60, B: 200, A: 255}},
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		for py := range base.Size {
			for px := range base.Size {
				i := py*base.Size + px
				c := base.Get(face, px, py)
				for _, cls := range classes {
					c = fxTint(c, cls.Dist.Faces[face][i], cls.Mag.Faces[face][i], cls.Tint)
				}
				base.Set(face, px, py, c)
			}
		}
	}

	img := cubemap.BakeEquirect(base, width, height)

	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	borderPixel := func(ix, iy int) {
		x, y, z := cubemap.FacePixelToDir(w.Face, w.X0+ix, w.Y0+iy, w.SProd)
		px, py := equirectPixel(x, y, z, width, height)
		img.SetRGBA(px, py, white)
	}
	for ix := range w.Size {
		borderPixel(ix, 0)
		borderPixel(ix, w.Size-1)
	}
	for iy := range w.Size {
		borderPixel(0, iy)
		borderPixel(w.Size-1, iy)
	}

	return encodePNG(img)
}

// equirectPixel is the inverse of the lat/lon projection in
// cubemap.BakeEquirect: given a unit direction, returns the nearest
// equirect pixel coordinates.
func equirectPixel(x, y, z float64, width, height int) (px, py int) {
	lat := math.Asin(y)
	lon := math.Atan2(z, x)
	if lon < 0 {
		lon += 2 * math.Pi
	}
	fpy := (math.Pi/2-lat)/math.Pi*float64(height) - 0.5
	fpx := lon/(2*math.Pi)*float64(width) - 0.5
	px = int(math.Round(fpx))
	py = int(math.Round(fpy))
	px = min(width-1, max(0, px))
	py = min(height-1, max(0, py))
	return px, py
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StateHash is the byte-exact per-layer regression fingerprint. It
// folds in every field a layer can produce, in this fixed order so the
// hash is deterministic and every layer's output is actually covered
// (a golden gate that only hashed Height/Img would miss drift in
// DistCoast, T/M/RainMult, Rivers, FlowAccum, Craters, or Sites):
//
//  1. Height — float64 bits, little-endian, row-major.
//  2. DistCoast, T, M, RainMult — each skipped entirely when nil, else
//     hashed the same way as Height.
//  3. Rivers — skipped when nil, else one byte per element (0 or 1).
//  4. FlowAccum — skipped when nil, else float64 bits like Height.
//  5. Craters — skipped when empty, else each crater's Lat, Lon,
//     Radius, Age float64 bits in that order. (feature.Crater has no
//     Depth field — craters carry Age, not a separate depth scalar —
//     so Age is hashed in Depth's place.)
//  6. Sites — skipped when empty, else each site's Dir[0], Dir[1],
//     Dir[2], Habitability, Population float64 bits in that order.
//  7. Img — pixels, when present, hashed last.
//
// Returned as hex.
func StateHash(st *State) string {
	h := fnv.New64a()
	var b [8]byte
	writeFloat := func(v float64) {
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(v))
		h.Write(b[:])
	}
	writeGrid := func(g *Grid) {
		if g == nil {
			return
		}
		for _, v := range g.Data {
			writeFloat(v)
		}
	}

	writeGrid(st.Height)
	writeGrid(st.DistCoast)
	writeGrid(st.T)
	writeGrid(st.M)
	writeGrid(st.RainMult)

	if st.Rivers != nil {
		rb := make([]byte, len(st.Rivers))
		for i, v := range st.Rivers {
			if v {
				rb[i] = 1
			}
		}
		h.Write(rb)
	}

	writeGrid(st.FlowAccum)

	for _, c := range st.Craters {
		writeFloat(c.Lat)
		writeFloat(c.Lon)
		writeFloat(c.Radius)
		writeFloat(c.Age)
	}

	for _, s := range st.Sites {
		writeFloat(s.Dir[0])
		writeFloat(s.Dir[1])
		writeFloat(s.Dir[2])
		writeFloat(s.Habitability)
		writeFloat(s.Population)
	}

	if st.Img != nil {
		h.Write(st.Img.Pix)
	}
	return fmt.Sprintf("%016x", h.Sum64())
}
