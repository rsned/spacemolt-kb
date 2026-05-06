package main

import (
	"bytes"
	"flag"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
)

var updateGoldens = flag.Bool("update", false, "update golden images instead of comparing")

var goldenPlanets = []struct{ Type, Seed string }{
	{"scorched", "GoldenScorched"},
	{"arid", "GoldenArid"},
	{"terran", "GoldenTerran"},
	{"tundra", "GoldenTundra"},
	{"glacial", "GoldenGlacial"},
	{"ice_world", "GoldenIceWorld"},
	{"super_terran", "GoldenSuperTerran"},
	{"hothouse", "GoldenHothouse"},
	{"lava_world", "GoldenLavaWorld"},
	{"oceanic", "GoldenOceanic"},
	{"jovian", "GoldenJovian"},
	{"ice_giant", "GoldenIceGiant"},
	{"unknown", "GoldenUnknown"},
}

const (
	goldenW       = 400
	goldenH       = 200
	maxMeanDE2000 = 1.5
)

func TestGolden(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			got, err := planetgen.GenerateEquirect(p.Type, p.Seed, goldenW, goldenH)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, p.Type+".png")
			if *updateGoldens {
				if err := writePNG(path, got); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readPNGRGBA(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGolden -update`)", path, err)
			}
			if want.Bounds() != got.Bounds() {
				t.Fatalf("size %v != golden %v", got.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(got.Pix, want.Pix)
			if meanDE > maxMeanDE2000 {
				t.Errorf("mean ΔE2000 = %.3f > %.2f (run with -update after review)",
					meanDE, maxMeanDE2000)
			}
		})
	}
}

// TestGoldenClouds bakes the cloud overlay for each archetype in
// goldenPlanets that has clouds enabled and compares the resulting
// 4×3 cube-cross PNG against testdata/golden/<type>.clouds.cube.png.
//
// Archetypes whose Cloud.Coverage is zero (gas giants, scorched, lava,
// etc.) yield a nil CloudCubeMap and are skipped — RenderCloudCubeMap
// returning nil is the desired "no clouds" signal.
//
// Face size 64 keeps each golden tractable while still exercising the
// per-face noise + storm overlays. Phase 9b can scale up later.
func TestGoldenClouds(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const cloudFaceSize = 64
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			profile := planetgen.GetProfile(p.Type)
			if profile == nil {
				t.Fatalf("unknown planet type %s", p.Type)
			}
			master := planetgen.HashSeedPublic(p.Seed)
			cm := render.RenderCloudCubeMap(profile, master, cloudFaceSize)
			if cm == nil {
				t.Skipf("clouds disabled for %s", p.Type)
			}
			// Materialize to PNG bytes via the same code path the cmd
			// uses, then decode back to RGBA so we can run the same
			// meanDE2000 comparison TestGolden uses.
			var buf bytes.Buffer
			if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
				t.Fatalf("WriteCrossPNGTo: %v", err)
			}
			got, err := decodePNGToRGBA(buf.Bytes())
			if err != nil {
				t.Fatalf("decode generated PNG: %v", err)
			}
			path := filepath.Join(dir, p.Type+".clouds.cube.png")
			if *updateGoldens {
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readPNGRGBA(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGoldenClouds -update`)", path, err)
			}
			if want.Bounds() != got.Bounds() {
				t.Fatalf("size %v != golden %v", got.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(got.Pix, want.Pix)
			if meanDE > maxMeanDE2000 {
				t.Errorf("mean ΔE2000 = %.3f > %.2f (run with -update after review)",
					meanDE, maxMeanDE2000)
			}
		})
	}
}

// TestGoldenNight bakes the Black Marble nightside cube-map for each
// archetype with Civ.Tier > 0 and compares the resulting 4×3 cube-cross
// PNG against testdata/golden/<type>.night.cube.png.
//
// Archetypes whose Civ.Tier == 0 (gas giants, scorched, lava, etc.)
// yield a nil CubeMap from RenderNightCubeMap and are skipped — same
// "disabled → nil" idiom TestGoldenClouds uses.
//
// Face size 64 keeps each golden tractable.
func TestGoldenNight(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const nightFaceSize = 64
	for _, p := range goldenPlanets {
		t.Run(p.Type, func(t *testing.T) {
			profile := planetgen.GetProfile(p.Type)
			if profile == nil {
				t.Fatalf("unknown planet type %s", p.Type)
			}
			master := planetgen.HashSeedPublic(p.Seed)
			cm := render.RenderNightCubeMap(profile, master, nightFaceSize)
			if cm == nil {
				t.Skipf("civ disabled for %s", p.Type)
			}
			var buf bytes.Buffer
			if err := cubemap.WriteCrossPNGTo(cm, &buf); err != nil {
				t.Fatalf("WriteCrossPNGTo: %v", err)
			}
			got, err := decodePNGToRGBA(buf.Bytes())
			if err != nil {
				t.Fatalf("decode generated PNG: %v", err)
			}
			path := filepath.Join(dir, p.Type+".night.cube.png")
			if *updateGoldens {
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("updated %s", path)
				return
			}
			want, err := readPNGRGBA(path)
			if err != nil {
				t.Fatalf("missing/unreadable golden %s: %v (run `go test -run TestGoldenNight -update`)", path, err)
			}
			if want.Bounds() != got.Bounds() {
				t.Fatalf("size %v != golden %v", got.Bounds(), want.Bounds())
			}
			meanDE := meanDE2000(got.Pix, want.Pix)
			if meanDE > maxMeanDE2000 {
				t.Errorf("mean ΔE2000 = %.3f > %.2f (run with -update after review)",
					meanDE, maxMeanDE2000)
			}
		})
	}
}

func decodePNGToRGBA(data []byte) (*image.RGBA, error) {
	return decodePNGReaderToRGBA(bytes.NewReader(data))
}

func decodePNGReaderToRGBA(r io.Reader) (*image.RGBA, error) {
	src, err := png.Decode(r)
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst, nil
}

func writePNG(path string, img *image.RGBA) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

func readPNGRGBA(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return decodePNGReaderToRGBA(f)
}

func meanDE2000(a, b []uint8) float64 {
	n := len(a) / 4
	var sum float64
	for i := 0; i < n; i++ {
		j := i * 4
		ca := colorful.Color{
			R: float64(a[j]) / 255,
			G: float64(a[j+1]) / 255,
			B: float64(a[j+2]) / 255,
		}
		cb := colorful.Color{
			R: float64(b[j]) / 255,
			G: float64(b[j+1]) / 255,
			B: float64(b[j+2]) / 255,
		}
		sum += ca.DistanceCIEDE2000(cb)
	}
	return sum / float64(n)
}
