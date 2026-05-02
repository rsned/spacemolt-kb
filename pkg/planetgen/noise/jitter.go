// Voronoi cell-coordinate jitter on the unit sphere.
//
// JitterField rotates and offsets sample coordinates per Voronoi cell
// to break visible repetition in fbm-based detail noise. Adjacent
// cells produce slightly-different rotated views of the same fbm
// field, so what would otherwise be a tiled pattern shows natural
// variation across cell boundaries.
//
// The discontinuity at cell boundaries is small (≤ JitterOffsetMax in
// sample-space, ≤ JitterRotMax around the cell center) and reads as
// natural variation given the underlying fbm continuity.
package noise

import (
	"math"
	"math/rand/v2"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// JitterCell encodes a per-cell rotation and translation applied at
// sample time when looking up fbm noise inside the cell's Voronoi
// region.
type JitterCell struct {
	Center   [3]float64 // unit vector
	RotAxis  [3]float64 // unit vector
	RotAngle float64    // radians
	Offset   [3]float64 // each component in [-JitterOffsetMax, +JitterOffsetMax]
}

// JitterField is the per-planet jitter state. PerPixel[face][py*S+px]
// holds the cell id (0..len(Cells)-1) that contains that pixel.
type JitterField struct {
	Size     int
	Cells    []JitterCell
	PerPixel [cubemap.NumFaces][]int16
}

// GenerateJitter returns a JitterField for a planet at face size S.
// Returns nil when profile.JitterEnabled is false (the renderer skips
// the jitter transform when nil — no per-call flag needed).
func GenerateJitter(profile *types.PlanetProfile, master int64, S int) *JitterField {
	if !profile.JitterEnabled || profile.JitterCellCount <= 0 {
		return nil
	}
	n := profile.JitterCellCount
	jf := &JitterField{Size: S, Cells: make([]JitterCell, n)}
	rngCells := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "jitter.cells")),        //nolint:gosec
		uint64(seed.Domain(master, "jitter.cells.stream")), //nolint:gosec
	))
	rngRot := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "jitter.rot")),        //nolint:gosec
		uint64(seed.Domain(master, "jitter.rot.stream")), //nolint:gosec
	))
	rngOff := rand.New(rand.NewPCG(
		uint64(seed.Domain(master, "jitter.offset")),        //nolint:gosec
		uint64(seed.Domain(master, "jitter.offset.stream")), //nolint:gosec
	))
	goldenAngle := math.Pi * (3.0 - math.Sqrt(5))
	for i := range n {
		y := 1.0 - 2.0*(float64(i)+0.5)/float64(n)
		radius := math.Sqrt(1.0 - y*y)
		theta := goldenAngle * float64(i)
		// Tiny per-cell jitter on the spiral so different planets at
		// the same JitterCellCount don't share cell layout.
		jitter := (rngCells.Float64() - 0.5) * (math.Pi / 60)
		x := math.Cos(theta+jitter) * radius
		z := math.Sin(theta+jitter) * radius
		jf.Cells[i].Center = [3]float64{x, y, z}

		// Random rotation axis (Marsaglia).
		for {
			a := 2*rngRot.Float64() - 1
			b := 2*rngRot.Float64() - 1
			s := a*a + b*b
			if s >= 1 {
				continue
			}
			f := 2 * math.Sqrt(1-s)
			jf.Cells[i].RotAxis = [3]float64{a * f, b * f, 1 - 2*s}
			break
		}
		jf.Cells[i].RotAngle = (rngRot.Float64()*2 - 1) * profile.JitterRotMax
		jf.Cells[i].Offset = [3]float64{
			(rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
			(rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
			(rngOff.Float64()*2 - 1) * profile.JitterOffsetMax,
		}
	}

	// Per-pixel nearest-center search.
	for f := range cubemap.NumFaces {
		face := cubemap.Face(f)
		jf.PerPixel[face] = make([]int16, S*S)
		for py := range S {
			for px := range S {
				dx, dy, dz := cubemap.FacePixelToDir(face, px, py, S)
				bestI := int16(0)
				bestDot := -2.0
				for i := range jf.Cells {
					c := jf.Cells[i].Center
					dot := c[0]*dx + c[1]*dy + c[2]*dz
					if dot > bestDot {
						bestDot = dot
						bestI = int16(i)
					}
				}
				jf.PerPixel[face][py*S+px] = bestI
			}
		}
	}
	return jf
}

// At returns the JitterCell whose Voronoi region contains the unit
// direction (dx, dy, dz). O(N) nearest-center search; N is small
// (~120) so this is fine for sample-time use.
func (jf *JitterField) At(dx, dy, dz float64) *JitterCell {
	if jf == nil {
		return nil
	}
	bestI := 0
	bestDot := -2.0
	for i := range jf.Cells {
		c := jf.Cells[i].Center
		dot := c[0]*dx + c[1]*dy + c[2]*dz
		if dot > bestDot {
			bestDot = dot
			bestI = i
		}
	}
	return &jf.Cells[bestI]
}

// Transform applies the cell-local rotation + offset to direction
// (px, py, pz). Returns the original direction unchanged if jf is nil.
func (jf *JitterField) Transform(px, py, pz float64) (float64, float64, float64) {
	if jf == nil {
		return px, py, pz
	}
	cell := jf.At(px, py, pz)
	// Rotate (p − Center) around RotAxis by RotAngle (Rodrigues' formula).
	// p' = p·cos + (k×p)·sin + k·(k·p)·(1−cos)
	dx, dy, dz := px-cell.Center[0], py-cell.Center[1], pz-cell.Center[2]
	rx, ry, rz := cell.RotAxis[0], cell.RotAxis[1], cell.RotAxis[2]
	cosA := math.Cos(cell.RotAngle)
	sinA := math.Sin(cell.RotAngle)
	dot := dx*rx + dy*ry + dz*rz
	// cx, cy, cz = k × p_local (cross product of RotAxis and local offset)
	cx := ry*dz - rz*dy
	cy := rz*dx - rx*dz
	cz := rx*dy - ry*dx
	nx := dx*cosA + cx*sinA + rx*dot*(1-cosA)
	ny := dy*cosA + cy*sinA + ry*dot*(1-cosA)
	nz := dz*cosA + cz*sinA + rz*dot*(1-cosA)
	// Translate back and offset.
	return nx + cell.Center[0] + cell.Offset[0],
		ny + cell.Center[1] + cell.Offset[1],
		nz + cell.Center[2] + cell.Offset[2]
}

// TransformPixel is the same as Transform but uses the precomputed
// PerPixel cell-id raster to find the cell in O(1) instead of O(N).
// Use this from Detail-field and biome-jitter inner loops where face
// and pixel coords are already in scope. Returns the input unchanged
// if jf is nil.
func (jf *JitterField) TransformPixel(face cubemap.Face, px, py int, tx, ty, tz float64) (float64, float64, float64) {
	if jf == nil {
		return tx, ty, tz
	}
	cell := &jf.Cells[jf.PerPixel[face][py*jf.Size+px]]
	// Rotate (p − Center) around RotAxis by RotAngle (Rodrigues' formula).
	dx, dy, dz := tx-cell.Center[0], ty-cell.Center[1], tz-cell.Center[2]
	rx, ry, rz := cell.RotAxis[0], cell.RotAxis[1], cell.RotAxis[2]
	cosA := math.Cos(cell.RotAngle)
	sinA := math.Sin(cell.RotAngle)
	dot := dx*rx + dy*ry + dz*rz
	cx := ry*dz - rz*dy
	cy := rz*dx - rx*dz
	cz := rx*dy - ry*dx
	nx := dx*cosA + cx*sinA + rx*dot*(1-cosA)
	ny := dy*cosA + cy*sinA + ry*dot*(1-cosA)
	nz := dz*cosA + cz*sinA + rz*dot*(1-cosA)
	return nx + cell.Center[0] + cell.Offset[0],
		ny + cell.Center[1] + cell.Offset[1],
		nz + cell.Center[2] + cell.Offset[2]
}
