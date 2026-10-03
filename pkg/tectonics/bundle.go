package tectonics

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
)

const manifestVersion = 1

// FrameInfo is one frame's manifest row: where its grids live on disk and
// the plate table at that step.
type FrameInfo struct {
	Step   int        `json:"step"`
	Myr    float64    `json:"myr"`
	Dir    string     `json:"dir"` // relative to the bundle root
	Plates []PlateRow `json:"plates"`
}

// Manifest describes a keyframe bundle on disk: the run's parameters and
// the ordered list of frames it emitted.
type Manifest struct {
	Version    int         `json:"version"`
	Seed       int64       `json:"seed"`
	Planet     string      `json:"planet,omitempty"`
	Archetype  string      `json:"archetype"`
	Params     Params      `json:"params"`
	Face       int         `json:"face"`
	Steps      int         `json:"steps"`
	MyrPerStep float64     `json:"myr_per_step"`
	Frames     []FrameInfo `json:"frames"`
}

// NewManifest builds an empty Manifest for a run of p seeded by master,
// ready to have FrameInfo rows appended as frames are written.
func NewManifest(p Params, master int64, planet string) *Manifest {
	return &Manifest{Version: manifestVersion, Seed: master, Planet: planet, Archetype: p.Archetype,
		Params: p, Face: p.Face, Steps: p.Steps, MyrPerStep: p.MyrPerStep()}
}

func gridToCross[T any](g *Grid[T], px func(T) color.RGBA) *cubemap.CubeMap {
	cm := cubemap.New(g.S)
	for i, v := range g.Cells {
		f, x, y := g.Addr(i)
		cm.Set(f, x, y, px(v))
	}
	return cm
}

func writeCross(path string, cm *cubemap.CubeMap) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = cubemap.WriteCrossPNGTo(cm, f)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// WriteFrame writes frames/<step:04d>/{thickness,plate,feature}.png under
// bundleDir and returns the manifest row for the frame.
func WriteFrame(bundleDir string, fr Frame) (FrameInfo, error) {
	rel := filepath.Join("frames", fmt.Sprintf("%04d", fr.Step))
	dir := filepath.Join(bundleDir, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return FrameInfo{}, err
	}
	grey := func(v float64) color.RGBA {
		g := uint8(math.Round(math.Max(0, math.Min(1, v)) * 255))
		return color.RGBA{R: g, G: g, B: g, A: 255}
	}
	idc := func(v int32) color.RGBA { return color.RGBA{R: uint8(v), G: 0, B: 0, A: 255} }
	fb := func(v uint8) color.RGBA { return color.RGBA{R: v, G: 0, B: 0, A: 255} }
	if err := writeCross(filepath.Join(dir, "thickness.png"), gridToCross(fr.Th, grey)); err != nil {
		return FrameInfo{}, err
	}
	if err := writeCross(filepath.Join(dir, "plate.png"), gridToCross(fr.Labels, idc)); err != nil {
		return FrameInfo{}, err
	}
	if err := writeCross(filepath.Join(dir, "feature.png"), gridToCross(fr.Feature, fb)); err != nil {
		return FrameInfo{}, err
	}
	return FrameInfo{Step: fr.Step, Myr: fr.Myr, Dir: rel, Plates: fr.Plates}, nil
}

// WriteManifest writes m as manifest.json under bundleDir.
func WriteManifest(bundleDir string, m *Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundleDir, "manifest.json"), b, 0o644)
}

// ReadManifest reads manifest.json from bundleDir.
func ReadManifest(bundleDir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", bundleDir, err)
	}
	if m.Version != manifestVersion {
		return nil, fmt.Errorf("manifest %s: version %d, want %d", bundleDir, m.Version, manifestVersion)
	}
	return &m, nil
}

func readCross(path string) (*cubemap.CubeMap, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return cubemap.ReadCrossPNGFrom(f)
}

func crossToGrid[T any](cm *cubemap.CubeMap, px func(color.RGBA) T) *Grid[T] {
	g := NewGrid[T](cm.Size)
	for i := range g.Cells {
		f, x, y := g.Addr(i)
		g.Cells[i] = px(cm.Get(f, x, y))
	}
	return g
}

// ReadFrame loads one frame's grids. Thickness is 8-bit quantised.
func ReadFrame(bundleDir string, fi FrameInfo) (Frame, error) {
	dir := filepath.Join(bundleDir, fi.Dir)
	th, err := readCross(filepath.Join(dir, "thickness.png"))
	if err != nil {
		return Frame{}, err
	}
	pl, err := readCross(filepath.Join(dir, "plate.png"))
	if err != nil {
		return Frame{}, err
	}
	fe, err := readCross(filepath.Join(dir, "feature.png"))
	if err != nil {
		return Frame{}, err
	}
	return Frame{Step: fi.Step, Myr: fi.Myr, Plates: fi.Plates,
		Th:      crossToGrid(th, func(c color.RGBA) float64 { return float64(c.R) / 255 }),
		Labels:  crossToGrid(pl, func(c color.RGBA) int32 { return int32(c.R) }),
		Feature: crossToGrid(fe, func(c color.RGBA) uint8 { return c.R }),
	}, nil
}
