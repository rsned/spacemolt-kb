package planetgen

// This file re-exports symbols whose canonical home has moved into
// subpackages. Callers inside the root planetgen package will be
// migrated incrementally; the shim is removed at the end of Task 14.

import (
	"github.com/rsned/spacemolt-kb/pkg/planetgen/color"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
)

// ColorStop is a legacy alias for color.ColorStop.
type ColorStop = color.ColorStop

// NoiseGenerator is a legacy alias for noise.Generator.
type NoiseGenerator = noise.Generator

// NewNoiseGenerator is a legacy alias for noise.New.
var NewNoiseGenerator = noise.New
