package noise_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/noise"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// TestJitterTransformDirSeamSymmetric verifies that direction-based
// jitter cell lookup (JitterField.Transform) is seam-symmetric. The
// per-face PerPixel raster used by the deprecated TransformPixel cuts
// cell boundaries at face seams, producing 8.7-46.6% Detail-field
// discontinuity. With Transform the cell identity is a function of
// direction alone, so adjacent face-seam pixels that map to nearly
// identical sphere directions produce nearly identical outputs.
//
// The test walks every seam pixel pair and asserts:
//
//   - When both sides of the seam pair resolve to the same Voronoi
//     cell, the transformed directions agree to within a tight angular
//     bound (Rodrigues' rotation is Lipschitz; small input drift maps
//     to bounded output drift).
//   - The fraction of seam pairs that straddle a cell boundary is
//     small (proportional to cell-boundary density / pixel density),
//     not 100% as it effectively was with per-face raster lookup.
func TestJitterTransformDirSeamSymmetric(t *testing.T) {
	profile := &types.PlanetProfile{
		JitterEnabled:   true,
		JitterCellCount: 120,
		JitterRotMax:    math.Pi / 4,
		JitterOffsetMax: 0.05,
	}
	S := 64
	jf := noise.GenerateJitter(profile, 1, S)
	if jf == nil {
		t.Fatal("nil JitterField for enabled profile")
	}

	type sample struct {
		out [3]float64
		dir [3]float64
		id  int
	}

	// Build per-face direction grids and the corresponding Transform
	// outputs at every pixel, plus the cell-id selected by the
	// direction-based lookup.
	var grid [cubemap.NumFaces][]sample
	for f := range cubemap.NumFaces {
		face := cubemap.Face(f)
		grid[face] = make([]sample, S*S)
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				tx, ty, tz := jf.Transform(dx, dy, dz)
				bestI := 0
				bestDot := -2.0
				for i := range jf.Cells {
					c := jf.Cells[i].Center
					d := c[0]*dx + c[1]*dy + c[2]*dz
					if d > bestDot {
						bestDot = d
						bestI = i
					}
				}
				grid[face][py*S+px] = sample{
					out: [3]float64{tx, ty, tz},
					dir: [3]float64{dx, dy, dz},
					id:  bestI,
				}
			}
		}
	}

	const sameCellTolRad = 0.05 // ~2.9°: bounds Lipschitz drift from half-pixel direction snap through Rodrigues.
	var sameCellPairs, diffCellPairs int
	var maxSameCellAng, maxDiffCellAng float64
	var firstSameCellViolation string
	seamtest.WalkSeams(grid, S, func(face cubemap.Face, edge seamtest.Edge, idx int, here, there sample) {
		ang := angBetween(here.out, there.out)
		if here.id == there.id {
			sameCellPairs++
			if ang > maxSameCellAng {
				maxSameCellAng = ang
			}
			if ang > sameCellTolRad && firstSameCellViolation == "" {
				firstSameCellViolation = fmt.Sprintf(
					"face=%v edge=%v idx=%d cell=%d dirA=%v dirB=%v outA=%v outB=%v ang=%.4f rad",
					face, edge, idx, here.id, here.dir, there.dir, here.out, there.out, ang)
			}
		} else {
			diffCellPairs++
			if ang > maxDiffCellAng {
				maxDiffCellAng = ang
			}
		}
	})

	totalPairs := sameCellPairs + diffCellPairs
	if totalPairs == 0 {
		t.Fatal("no seam pairs walked")
	}

	t.Logf("seam pairs: total=%d same-cell=%d (%.1f%%) diff-cell=%d (%.1f%%)",
		totalPairs, sameCellPairs, 100*float64(sameCellPairs)/float64(totalPairs),
		diffCellPairs, 100*float64(diffCellPairs)/float64(totalPairs))
	t.Logf("max angle: same-cell=%.4f rad (%.2f°)  diff-cell=%.4f rad (%.2f°)",
		maxSameCellAng, maxSameCellAng*180/math.Pi,
		maxDiffCellAng, maxDiffCellAng*180/math.Pi)

	if firstSameCellViolation != "" {
		t.Errorf("same-cell seam pair exceeds %.3f rad tolerance: %s", sameCellTolRad, firstSameCellViolation)
	}

	// Direction-based lookup should leave well under 25% of seam pairs
	// straddling a cell boundary at S=64 with 120 cells. The previous
	// per-face raster regime had ~100% effective disagreement at the
	// rotation+offset level. The cap is generous to remain stable
	// across seeds.
	diffFrac := float64(diffCellPairs) / float64(totalPairs)
	if diffFrac > 0.25 {
		t.Errorf("too many seam pairs straddle a cell boundary: %.2f%% > 25%%", diffFrac*100)
	}
}

func angBetween(a, b [3]float64) float64 {
	aLen := math.Sqrt(a[0]*a[0] + a[1]*a[1] + a[2]*a[2])
	bLen := math.Sqrt(b[0]*b[0] + b[1]*b[1] + b[2]*b[2])
	if aLen == 0 || bLen == 0 {
		return math.NaN()
	}
	dot := (a[0]*b[0] + a[1]*b[1] + a[2]*b[2]) / (aLen * bLen)
	if dot > 1 {
		dot = 1
	} else if dot < -1 {
		dot = -1
	}
	return math.Acos(dot)
}
