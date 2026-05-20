package render

import (
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/biome"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/field"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// DebugBypass is the set of stage names the debug renderer should
// dry-run. The renderer still consumes any rng/noise draws the stage
// would have made (so downstream noise streams stay stable), but skips
// the stage's contribution.
type DebugBypass map[string]bool

// DebugFrame collects per-stage cube maps so the slider tool can show
// the operator each step of the rocky pipeline. Populated by
// RenderRockyDebug (added in T3).
type DebugFrame struct {
	Stages     []DebugStage
	Plates     *field.PlateField
	Jitter     *noise.JitterField
	Flow       *field.FlowField
	RainShadow *biome.RainShadowField
	Civ        *feature.CivField
	// Final is the fully-composited day-side cube map returned by the
	// colorize pass — exactly what RenderRocky would return for the
	// same (profile, seed, bypass). Consumers that want "the final
	// picture" (e.g. the planet-explorer cube/sphere preview) should
	// read this directly rather than walking Stages, since later
	// debug-only entries (Cloud: shaded, Civ: <part>) are partial
	// overlays, not composited images.
	Final *cubemap.CubeMap
}

// DebugStage is one row in the pipeline visualization.
//
// Kind is "height" for heightmap stages and "color" for colorization
// stages. For height stages, RawFbm/InputBands/OutputBands/SumAfter are
// populated as available. For color stages, ColorAfter holds the cube
// map after the stage applied; the other fields are nil.
type DebugStage struct {
	Name        string
	Kind        string
	RawFbm      *cubemap.CubeMapF
	InputBands  *cubemap.CubeMap
	OutputBands *cubemap.CubeMap
	SumAfter    *cubemap.CubeMapF
	ColorAfter  *cubemap.CubeMap
	Skipped     bool
	// CategoricalAfter renders each unique int16 id as a distinct hue
	// via golden-ratio HSV stepping. Negative ids map to black.
	CategoricalAfter *cubemap.CubeMap
	// BooleanAfter is a two-tone raster (e.g. oceanic vs continental).
	BooleanAfter *cubemap.CubeMap
	// ScalarAfter is a single-channel float field (e.g. plate SDF in km).
	ScalarAfter *cubemap.CubeMapF
	// Combined is the "planet so far" context image for a field-kind
	// stage — set by the debug-frame post-pass to either the heightmap
	// grayscale (for pre-color stages: Plates, Jitter, Flow, RainShadow)
	// or the final composited day image (for post-pipeline stages:
	// Cloud, Civ debug). Lets the debug grid show every layer next to
	// the planet context, matching the asymmetry the height/color rows
	// already provide via SumAfter / ColorAfter.
	Combined *cubemap.CubeMap
}

// RenderRockyDebug runs the rocky pipeline (heightmap + colorize) with
// per-stage intermediates captured into a DebugFrame. bypass is a set
// of stage names to dry-run; pass nil to run all stages normally.
func RenderRockyDebug(profile *types.PlanetProfile, seed int64, S int, bypass DebugBypass) *DebugFrame {
	jitter := noise.GenerateJitter(profile, seed, S)
	plates := field.GeneratePlates(profile, seed, S)
	frame := &DebugFrame{
		Plates: plates,
		Jitter: jitter,
	}
	// Plates and Jitter are foundation data — they're generated upfront
	// and consumed by every downstream stage (Ridged anchors mountain
	// belts to convergent plate boundaries; Continents seeds from
	// continental plate centroids; Civ scores habitability against
	// plates; the noise pipeline routes through jitter cells). Surface
	// them FIRST in the debug grid so the operator reads the pipeline
	// top-to-bottom in the order the renderer actually runs.
	if plates != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "Plates: id", Kind: "field", CategoricalAfter: paintPlateIDWithArrows(plates, S)},
			DebugStage{Name: "Plates: oceanic", Kind: "field", BooleanAfter: paintOceanicMask(plates, S)},
			DebugStage{Name: "Plates: convergent", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Convergent, S)},
			DebugStage{Name: "Plates: divergent", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Divergent, S)},
			DebugStage{Name: "Plates: transform", Kind: "field", ScalarAfter: scalarFromKmPerFace(plates.Transform, S)},
		)
	}
	if jitter != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "Jitter: cells", Kind: "field", CategoricalAfter: paintCategoricalCubeMap16(jitter.PerPixel, S)},
		)
	}
	hm, craters := generateRockyHeightmapDebug(profile, seed, S, frame, bypass, jitter, plates)
	frame.Final = colorizeRockyDebug(profile, seed, S, hm, craters, frame, bypass, jitter, plates, frame.Flow)
	// Cache a grayscale heightmap snapshot so pre-color field stages
	// (Plates, Jitter, Flow, RainShadow) get a meaningful "planet
	// shape" context image. Post-pipeline field stages (Cloud, Civ
	// debug) instead get the final composited day image.
	heightmapImg := cubemap.GrayscaleFromF(hm)
	if frame.Flow != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "Flow: rivers", Kind: "field", BooleanAfter: paintBooleanCubeMap(frame.Flow.Rivers, S)},
			DebugStage{Name: "Flow: accum", Kind: "field", ScalarAfter: scalarFromAccum(frame.Flow.Accum, S)},
		)
	}
	if frame.RainShadow != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "RainShadow", Kind: "field", ScalarAfter: scalarFromMultiplier(frame.RainShadow.Multiplier, S)},
		)
	}
	if cf := feature.GenerateClouds(profile, seed, S); cf != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "Cloud: alpha", Kind: "field", ScalarAfter: scalarFromCubeFaces(cf.Alpha, S)},
			DebugStage{Name: "Cloud: density", Kind: "field", ScalarAfter: scalarFromCubeFaces(cf.Density, S)},
			DebugStage{Name: "Cloud: shaded", Kind: "color", ColorAfter: paintCloudShaded(cf, S)},
		)
	}
	if frame.Civ != nil {
		civ := frame.Civ
		stages := []DebugStage{
			{Name: "Civ: habitability", Kind: "field", ScalarAfter: scalarFromCubeFaces(civ.Habitability.Score, S)},
			{Name: "Civ: sites", Kind: "color", ColorAfter: paintCivSites(civ.Sites, S)},
			{Name: "Civ: roads", Kind: "field", ScalarAfter: scalarFromCubeFaces(civ.Roads, S)},
			{Name: "Civ: day overlay", Kind: "color", ColorAfter: civ.DayColor},
			{Name: "Civ: night lights", Kind: "color", ColorAfter: civ.Night},
		}
		frame.Stages = append(frame.Stages, stages...)
	}
	// Post-pass: attach a "planet so far" Combined image to every
	// field-kind stage. Pre-color stages (plates, jitter, flow,
	// rainshadow) get the heightmap grayscale so the operator sees
	// the shape their layer is informing; post-pipeline stages
	// (cloud, civ) get the final composited image so the operator
	// sees the layer in context. Pointers are shared rather than
	// cloned — consumers treat these as read-only.
	for i := range frame.Stages {
		st := &frame.Stages[i]
		if st.Kind != "field" {
			continue
		}
		switch {
		case strings.HasPrefix(st.Name, "Cloud:"), strings.HasPrefix(st.Name, "Civ:"):
			st.Combined = frame.Final
		default:
			st.Combined = heightmapImg
		}
	}
	return frame
}
