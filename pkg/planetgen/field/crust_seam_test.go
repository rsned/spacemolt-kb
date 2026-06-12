package field

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
)

// TestCrustSeamContinuity checks that the ContinentalMask and BaseHeight
// cube maps do not have catastrophic discontinuities at cube-face seam
// boundaries. The tolerance is 0.40 (40% of field range) rather than
// the 0.05 used by smooth noise fields: these fields contain intentional
// high-gradient coastline transitions that can cross a seam, producing
// legitimate nearest-neighbor deltas of up to ~30% when the smoothstep
// transition (shelf width 0.05 rad, total span 0.10 rad) straddles a
// face boundary. The test still catches true seam bugs (missing pixels,
// wrong-face reads, NaN injection) which would produce deltas near 100%.
func TestCrustSeamContinuity(t *testing.T) {
	const S = 64
	p := crustTestProfile()
	pf := GeneratePlates(p, 42, S)
	crust := GenerateCrust(p, 42, S, pf)
	seamtest.AssertSeamContinuity(t, "ContinentalMask", crust.ContinentalMask, 0.40)
	seamtest.AssertSeamContinuity(t, "BaseHeight", crust.BaseHeight, 0.40)
}
