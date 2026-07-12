package patch

import (
	"fmt"
	"image"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// Context holds the immutable-per-render inputs shared by all layers.
type Context struct {
	Sphere  *SphereData
	Fields  *Fields
	Profile *types.PlanetProfile
	Master  int64
	// SeaLevelView overrides the waterline sea level for the
	// waterlines/civ layers; 0 means "use Sphere.SeaLevel".
	SeaLevelView float64
}

// seaLevelView resolves the effective waterline sea level: the
// SeaLevelView slider override when set, else the sphere's post-flow
// quantile. Used by applyWaterlines (layer 11).
func (c *Context) seaLevelView() float64 {
	if c.SeaLevelView > 0 {
		return c.SeaLevelView
	}
	return c.Sphere.SeaLevel
}

// State is the accumulated per-layer output. Layers must treat the
// input State as immutable: copy the struct, replace only the pointers
// for fields they write.
type State struct {
	Height    *Grid
	DistCoast *Grid
	T, M      *Grid
	RainMult  *Grid
	Rivers    []bool
	FlowAccum *Grid
	Craters   []feature.Crater
	Img       *image.RGBA
	Sites     []feature.Site
}

// Layer is one patch pipeline stage.
type Layer struct {
	Index   int
	ID      string
	Name    string
	Params  []string
	Enabled func(*Context) bool
	Apply   func(*Context, *State) *State
}

func always(*Context) bool                  { return true }
func identity(_ *Context, st *State) *State { return st }

// Layers returns the canonical ordered layer list. Layer tasks 8–15
// replace the identity Apply/Enabled placeholders with real stages.
func Layers() []Layer {
	ls := []Layer{
		{ID: "tectonic-base", Name: "Tectonic base", Params: nil},
		{ID: "tectonic-fx", Name: "Tectonic FX", Params: []string{"tectonicFX"}},
		{ID: "control-noise", Name: "Control noise", Params: []string{"ControlConfig"}},
		{ID: "height-smooth", Name: "Height smooth", Params: []string{"HeightSmoothRadius"}},
		{ID: "normalize", Name: "Normalize", Params: nil},
		{ID: "coastal", Name: "Coastal noise", Params: []string{"Coastal"}},
		{ID: "erosion", Name: "Erosion", Params: []string{"Erosion"}},
		{ID: "craters", Name: "Craters", Params: []string{"CraterCount", "CraterMinRadius", "CraterMaxRadius", "CraterDepth", "PowerLawAlpha", "MariaDensityFactor", "SurfaceAge", "SecondaryDensity"}},
		{ID: "flow-rivers", Name: "Rivers", Params: []string{"flow"}},
		{ID: "climate", Name: "Climate", Params: []string{"rainShadow", "ControlConfig.Temperature", "ControlConfig.Humidity"}},
		{ID: "biome-color", Name: "Biome color", Params: []string{"BiomeTable", "Palette", "EquatorialPalette", "PolarPalette", "Warp", "LUT"}},
		{ID: "waterlines", Name: "Waterlines", Params: []string{"SnowLine", "OceanColor", "HasPolarCaps", "PolarCapSize", "PolarCapNoise", "ShadingStrength", "ShadingExaggeration", "seaLevelView"}},
		{ID: "civ", Name: "Civilization", Params: []string{"civ"}},
	}
	ls[0].Apply = applyTectonicBase
	ls[1].Apply = applyTectonicFX
	ls[2].Apply = applyControlNoise
	ls[3].Apply = applyHeightSmooth
	ls[4].Apply = applyNormalize
	ls[5].Apply = applyCoastal
	ls[5].Enabled = coastalEnabled
	ls[6].Apply = applyErosion
	ls[6].Enabled = erosionEnabled
	ls[7].Apply = applyCraters
	ls[7].Enabled = cratersEnabled
	ls[8].Apply = applyFlowRivers
	ls[8].Enabled = flowEnabled
	ls[9].Apply = applyClimate
	ls[10].Apply = applyBiomeColor
	ls[11].Apply = applyWaterlines
	ls[12].Apply = applyCiv
	ls[12].Enabled = civEnabled
	for i := range ls {
		ls[i].Index = i
		if ls[i].Enabled == nil {
			ls[i].Enabled = always
		}
		if ls[i].Apply == nil {
			ls[i].Apply = identity
		}
	}
	return ls
}

// Stack renders layers with per-layer caching and dirty tracking.
type Stack struct {
	ctx       *Context
	layers    []Layer
	cache     []*State // cache[i] = state after layer i
	dirtyFrom int      // first layer that must re-run
}

func NewStack(ctx *Context) *Stack {
	ls := Layers()
	return &Stack{ctx: ctx, layers: ls, cache: make([]*State, len(ls)), dirtyFrom: 0}
}

func (s *Stack) Ctx() *Context { return s.ctx }

// inertParams are profile params the crust-path pipeline never reads.
// Patch Lab sessions are always crust-path (ComputeSphere rejects
// crust-disabled profiles), so edits to these change nothing — mapping
// them to a sphere recompute would spend seconds producing an
// identical SphereData. Matched by path prefix, like Layer.Params.
var inertParams = []string{"OceanLevel", "Continents", "Ridged", "Basin"}

// MarkDirty maps a changed profile param path to the owning layer.
//
// Matching rules:
//  1. Inert params (crust path never reads them): no-op, returns false.
//  2. The edit sits at/under one or more layer patterns: the MOST
//     SPECIFIC (longest) pattern wins, so "ControlConfig.Temperature.Amp"
//     dirties climate (layer 9) while "ControlConfig.Detail.Amp" still
//     dirties control-noise (layer 2).
//  3. The edit is BROADER than some pattern (paramPath is a proper
//     prefix of it, e.g. a whole-"ControlConfig" replacement): several
//     layers may consume parts of the subtree — the EARLIEST matching
//     layer is dirtied.
//
// Returns true when no layer owns the param — the sphere precompute
// does — and the caller must recompute SphereData + Fields, then
// MarkAllDirty.
func (s *Stack) MarkDirty(paramPath string) bool {
	for _, p := range inertParams {
		if strings.HasPrefix(paramPath, p) {
			return false
		}
	}
	bestSpecific, bestLen := -1, -1 // longest pattern containing the edit
	broadEarliest := -1            // earliest layer whose pattern the edit contains
	for i := range s.layers {
		for _, p := range s.layers[i].Params {
			switch {
			case strings.HasPrefix(paramPath, p):
				if len(p) > bestLen {
					bestSpecific, bestLen = i, len(p)
				}
			case strings.HasPrefix(p, paramPath):
				if broadEarliest == -1 {
					broadEarliest = i
				}
			}
		}
	}
	target := bestSpecific
	if broadEarliest != -1 && (target == -1 || broadEarliest < target) {
		target = broadEarliest
	}
	if target == -1 {
		return true
	}
	if target < s.dirtyFrom {
		s.dirtyFrom = target
	}
	return false
}

func (s *Stack) MarkAllDirty() { s.dirtyFrom = 0 }

// RenderTo runs enabled layers up to and including target, reusing
// cached upstream states.
func (s *Stack) RenderTo(target int) (*State, error) {
	if target < 0 || target >= len(s.layers) {
		return nil, fmt.Errorf("patch: layer index %d out of range", target)
	}
	if target < s.dirtyFrom {
		return s.cache[target], nil
	}
	var st *State
	start := s.dirtyFrom
	if start == 0 {
		st = &State{Height: NewGrid(s.ctx.Fields.Window.Size)}
	} else {
		st = s.cache[start-1]
	}
	for i := start; i <= target; i++ {
		if s.layers[i].Enabled(s.ctx) {
			reportProgress("layer:"+s.layers[i].ID, i+1, len(s.layers))
			st = s.layers[i].Apply(s.ctx, st)
		}
		s.cache[i] = st
	}
	if target+1 > s.dirtyFrom {
		s.dirtyFrom = target + 1
	}
	return st, nil
}
