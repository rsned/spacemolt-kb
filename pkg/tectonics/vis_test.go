package tectonics

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

// vis enables PNG renders of before/after fields under testdata/vis/.
var vis = flag.Bool("vis", false, "write testdata/vis/<test>.png isometric renders and index.html")

const visDir = "testdata/vis"

// fillQuad fills the convex quad with corners c (screen coords, in order).
func fillQuad(img *image.RGBA, c [4][2]float64, col color.RGBA) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range c {
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	b := img.Bounds()
	for y := max(b.Min.Y, int(math.Floor(minY))); y <= min(b.Max.Y-1, int(math.Ceil(maxY))); y++ {
		fy := float64(y) + 0.5
		xl, xr := math.Inf(1), math.Inf(-1)
		for k := range 4 {
			p, q := c[k], c[(k+1)%4]
			if (p[1] <= fy) != (q[1] <= fy) {
				x := p[0] + (fy-p[1])*(q[0]-p[0])/(q[1]-p[1])
				xl, xr = math.Min(xl, x), math.Max(xr, x)
			}
		}
		if xl > xr {
			continue
		}
		for x := max(b.Min.X, int(math.Floor(xl))); x <= min(b.Max.X-1, int(math.Ceil(xr))); x++ {
			dst := img.RGBAAt(x, y)
			a := float64(col.A) / 255
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(col.R)*a + float64(dst.R)*(1-a)),
				G: uint8(float64(col.G)*a + float64(dst.G)*(1-a)),
				B: uint8(float64(col.B)*a + float64(dst.B)*(1-a)), A: 255})
		}
	}
}

func shade(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), c.A}
}

func lerpRGB(a, b color.RGBA, t float64) color.RGBA {
	t = math.Max(0, math.Min(1, t))
	return color.RGBA{uint8(float64(a.R) + t*(float64(b.R)-float64(a.R))),
		uint8(float64(a.G) + t*(float64(b.G)-float64(a.G))),
		uint8(float64(a.B) + t*(float64(b.B)-float64(a.B))), 255}
}

// landColour shades height above sea level green → brown → white.
func landColour(above, span float64) color.RGBA {
	t := above / math.Max(span, 1e-9)
	if t < 0.5 {
		return lerpRGB(color.RGBA{60, 140, 60, 255}, color.RGBA{140, 100, 60, 255}, t*2)
	}
	return lerpRGB(color.RGBA{140, 100, 60, 255}, color.RGBA{245, 245, 245, 255}, (t-0.5)*2)
}

// renderIso draws one cube face of h as a 3/4 isometric height field: each
// pixel a shaded column (height × exag screen pixels), land coloured by
// height above seaLevel, a translucent water plane at seaLevel.
func renderIso(h *Grid[float64], face cubemap.Face, seaLevel, exag float64) *image.RGBA {
	S := h.S
	const cw = 10.0 // screen width of one cell
	ch := cw / 2
	lo, hi := math.Inf(1), math.Inf(-1)
	for py := range S {
		for px := range S {
			v := h.Cells[h.Index(face, px, py)]
			lo, hi = math.Min(lo, v), math.Max(hi, v)
		}
	}
	lo = math.Min(lo, seaLevel)
	hi = math.Max(hi, seaLevel+1e-9)
	z := func(v float64) float64 { return (v - lo) * exag }
	top := 10 + z(hi)
	W, H := int(cw*float64(S)+cw), int(top+ch*float64(S)+ch+10)
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{20, 22, 28, 255}}, image.Point{}, draw.Src)
	sx := func(x, y float64) float64 { return (x-y+float64(S))*cw/2 + cw/2 }
	sy := func(x, y, zz float64) float64 { return top + (x+y)*ch/2 - zz }
	column := func(x, y, zz float64, col color.RGBA) {
		// top, then right (x+1) and front (y+1) side faces down to the base plane
		fillQuad(img, [4][2]float64{{sx(x, y), sy(x, y, zz)}, {sx(x+1, y), sy(x+1, y, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, zz)}, {sx(x, y+1), sy(x, y+1, zz)}}, col)
		fillQuad(img, [4][2]float64{{sx(x+1, y), sy(x+1, y, zz)}, {sx(x+1, y+1), sy(x+1, y+1, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, 0)}, {sx(x+1, y), sy(x+1, y, 0)}}, shade(col, 0.75))
		fillQuad(img, [4][2]float64{{sx(x, y+1), sy(x, y+1, zz)}, {sx(x+1, y+1), sy(x+1, y+1, zz)},
			{sx(x+1, y+1), sy(x+1, y+1, 0)}, {sx(x, y+1), sy(x, y+1, 0)}}, shade(col, 0.55))
	}
	for sum := 0; sum <= 2*(S-1); sum++ { // painter's order: back (small px+py) to front
		for px := range S {
			py := sum - px
			if py < 0 || py >= S {
				continue
			}
			v := h.Cells[h.Index(face, px, py)]
			x, y := float64(px), float64(py)
			if v < seaLevel {
				depth := (seaLevel - v) / math.Max(seaLevel-lo, 1e-9)
				column(x, y, z(v), lerpRGB(color.RGBA{40, 90, 160, 255}, color.RGBA{10, 30, 90, 255}, depth))
				column(x, y, z(seaLevel), color.RGBA{60, 120, 220, 110})
			} else {
				column(x, y, z(v), landColour(v-seaLevel, hi-seaLevel))
			}
		}
	}
	return img
}

// diffPanel colours after−before: loss red, gain blue, scaled so |scale| is full intensity.
//
//nolint:unused // shared vis helper; no Task 5 test needs a diff panel yet, future tests will call it
func diffPanel(before, after *Grid[float64], face cubemap.Face, scale float64) *image.RGBA {
	S := before.S
	const cell = 10
	img := image.NewRGBA(image.Rect(0, 0, S*cell, S*cell))
	for py := range S {
		for px := range S {
			d := (after.Cells[after.Index(face, px, py)] - before.Cells[before.Index(face, px, py)]) / scale
			d = math.Max(-1, math.Min(1, d))
			c := color.RGBA{40, 40, 44, 255}
			if d < 0 {
				c = lerpRGB(c, color.RGBA{230, 60, 50, 255}, -d)
			} else if d > 0 {
				c = lerpRGB(c, color.RGBA{60, 120, 230, 255}, d)
			}
			draw.Draw(img, image.Rect(px*cell, py*cell, (px+1)*cell, (py+1)*cell), &image.Uniform{c}, image.Point{}, draw.Src)
		}
	}
	return img
}

// writeVis tiles panels left to right into testdata/vis/<name>.png and
// regenerates index.html. A no-op unless -vis is set.
func writeVis(t *testing.T, name string, panels ...*image.RGBA) {
	t.Helper()
	if !*vis || len(panels) == 0 {
		return
	}
	const gap = 12
	W, H := gap, 0
	for _, p := range panels {
		W += p.Bounds().Dx() + gap
		H = max(H, p.Bounds().Dy())
	}
	out := image.NewRGBA(image.Rect(0, 0, W, H+2*gap))
	draw.Draw(out, out.Bounds(), &image.Uniform{color.RGBA{12, 13, 16, 255}}, image.Point{}, draw.Src)
	x := gap
	for _, p := range panels {
		draw.Draw(out, image.Rect(x, gap, x+p.Bounds().Dx(), gap+p.Bounds().Dy()), p, p.Bounds().Min, draw.Src)
		x += p.Bounds().Dx() + gap
	}
	if err := os.MkdirAll(visDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(visDir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, out); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeVisIndex(t)
}

func writeVisIndex(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(visDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("<!doctype html><meta charset=utf-8><title>tectonics test renders</title>" +
		"<style>body{background:#0c0d10;color:#ccc;font:14px system-ui;margin:16px}h2{margin:24px 0 6px}img{max-width:100%}</style>" +
		"<h1>tectonics test renders</h1><p>panels left to right as named in each test (typically before | after | diff).</p>")
	for _, n := range names {
		fmt.Fprintf(&b, "<h2>%s</h2><img src=%q>", strings.TrimSuffix(n, ".png"), n)
	}
	if err := os.WriteFile(filepath.Join(visDir, "index.html"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
