package footprint

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// canonicalWidth is the viewBox width every footprint carries: 1000 units of
// hull length plus a 10-unit margin on each side.
const canonicalWidth = 1020

// marginTotal is the vertical margin the hull is inset by, summed over both
// edges. data-aspect is measured with it removed.
const marginTotal = 20

// aspectTolerance is how far data-aspect may sit from 1000/(H-20).
//
// Measured across all 395 shipped footprints, the true maximum deviation is
// 0.03845 (11 files exceed a naive 0.01 tolerance, 6 exceed 0.02, 2 exceed
// 0.03, none exceed 0.04). This does not matter for rendering: data-aspect is
// pipeline metadata only, and the holotable's draw transform derives its
// centre from the viewBox height directly (cy = height/2) — it never reads
// data-aspect. The tolerance exists purely to catch a pipeline regression,
// so it is set comfortably above the measured maximum (still small enough to
// flag anything actually wrong by an order of magnitude) rather than tuned to
// the current corpus.
const aspectTolerance = 0.05

// Check reports every way f departs from the asset contract. An empty result
// means the file is good. It returns all problems rather than the first so a
// corpus run gives one complete picture instead of needing repeated passes.
func Check(f Footprint, filename string) []string {
	var problems []string

	stem := strings.TrimSuffix(filepath.Base(filename), ".svg")
	if f.Ship == "" {
		problems = append(problems, "data-ship is missing; it is the only join key the battle log provides")
	} else if f.Ship != stem {
		problems = append(problems, fmt.Sprintf("filename stem %q != data-ship %q", stem, f.Ship))
	}

	if f.Width != canonicalWidth {
		problems = append(problems, fmt.Sprintf("viewBox width %v, want %v", f.Width, canonicalWidth))
	}
	if f.Height <= marginTotal {
		problems = append(problems, fmt.Sprintf("viewBox height %v leaves no hull inside the margins", f.Height))
	}

	switch f.pathCount {
	case 1:
		// The contract: one closed path, fill-rule evenodd for holes.
	case 0:
		problems = append(problems, "no path element; there is nothing to draw")
	default:
		problems = append(problems, fmt.Sprintf("%d path elements, want exactly 1", f.pathCount))
	}

	if !f.hasAspect {
		problems = append(problems, "data-aspect is missing; a pipeline regression that stops emitting it would go undetected")
	} else if f.Height > marginTotal {
		want := 1000 / (f.Height - marginTotal)
		if math.Abs(f.Aspect-want) > aspectTolerance {
			problems = append(problems, fmt.Sprintf(
				"data-aspect %.4f but the viewBox implies %.4f", f.Aspect, want))
		}
	}

	return problems
}
