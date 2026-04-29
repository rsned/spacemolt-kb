package render

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
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
}
