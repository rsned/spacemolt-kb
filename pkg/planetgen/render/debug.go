package render

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
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
	Stages []DebugStage
	Plates *field.PlateField
	Jitter *noise.JitterField
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
	hm, craters := generateRockyHeightmapDebug(profile, seed, S, frame, bypass, jitter)
	_ = colorizeRockyDebug(profile, seed, S, hm, craters, frame, bypass, jitter)
	if plates != nil {
		frame.Stages = append(frame.Stages,
			DebugStage{Name: "Plates: id", Kind: "field", CategoricalAfter: paintCategoricalCubeMap16(plates.PlateID, S)},
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
	return frame
}
