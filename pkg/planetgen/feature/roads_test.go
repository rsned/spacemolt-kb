package feature

import (
	"math"
	"reflect"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// flatHeightmap returns a cube-map with every pixel at v.
func flatHeightmap(size int, v float64) *cubemap.CubeMapF {
	hm := cubemap.NewF(size)
	for face := range cubemap.Face(cubemap.NumFaces) {
		row := hm.Faces[face]
		for i := range row {
			row[i] = v
		}
	}
	return hm
}

// dirSites turns a list of (x,y,z) into Site slices with unit-normalised
// directions and a default population of 0.5.
func dirSites(dirs ...[3]float64) []Site {
	out := make([]Site, len(dirs))
	for i, d := range dirs {
		n := math.Sqrt(d[0]*d[0] + d[1]*d[1] + d[2]*d[2])
		out[i] = Site{
			Dir:          [3]float64{d[0] / n, d[1] / n, d[2] / n},
			Population:   0.5,
			Habitability: 0.8,
		}
	}
	return out
}

// pixelAddrOf returns the cube-map pixel that the site direction maps to.
func pixelAddrOf(s Site, size int) cubemap.PixelAddr {
	face, px, py := cubemap.DirToFacePixel(s.Dir[0], s.Dir[1], s.Dir[2], size)
	return cubemap.PixelAddr{Face: face, PX: px, PY: py}
}

// roadIntensity reads the per-pixel intensity at addr.
func roadIntensity(rf *RoadField, addr cubemap.PixelAddr) float64 {
	return rf.Intensity[addr.Face][addr.PY*rf.Size+addr.PX]
}

// TestRoadsConnectAllSites builds 8 sites scattered across multiple faces
// on a flat heightmap, runs GenerateRoads, then BFS-walks the resulting
// road mask from the first site's pixel and asserts all other site
// pixels are reachable.
func TestRoadsConnectAllSites(t *testing.T) {
	const size = 64
	sites := dirSites(
		[3]float64{1, 0.1, 0.2},   // +X face
		[3]float64{1, -0.3, 0.1},  // +X face
		[3]float64{0.1, 1, 0.2},   // +Y face
		[3]float64{-0.2, 1, -0.1}, // +Y face
		[3]float64{0.1, 0.2, 1},   // +Z face
		[3]float64{-0.3, 0.1, 1},  // +Z face
		[3]float64{-1, 0.1, 0.0},  // -X face
		[3]float64{0.0, -1, 0.1},  // -Y face
	)
	hm := flatHeightmap(size, 0.4)

	rf := GenerateRoads(sites, hm, types.CivConfig{Tier: 1}, 0)
	if rf == nil {
		t.Fatalf("GenerateRoads returned nil")
	}

	// BFS from sites[0]'s pixel via 8-connected neighbours, only
	// stepping onto pixels with intensity > threshold.
	const threshold = 1e-6
	start := pixelAddrOf(sites[0], size)
	visited := map[cubemap.PixelAddr]struct{}{start: {}}
	queue := []cubemap.PixelAddr{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range cubemap.FacePixelNeighbors8(cur.Face, cur.PX, cur.PY, size) {
			if _, ok := visited[n]; ok {
				continue
			}
			if roadIntensity(rf, n) <= threshold {
				continue
			}
			visited[n] = struct{}{}
			queue = append(queue, n)
		}
	}

	// Each subsequent site must be reachable.
	for i := 1; i < len(sites); i++ {
		addr := pixelAddrOf(sites[i], size)
		if _, ok := visited[addr]; !ok {
			t.Fatalf("site[%d] pixel %+v not reachable from site[0] via road network", i, addr)
		}
	}
}

// TestRoadsAvoidPeaks builds two sites at low elevation with a tall
// ridge between them. The mean heightmap value sampled along the road
// path should be lower than the mean along the great-circle arc that
// crosses the ridge.
func TestRoadsAvoidPeaks(t *testing.T) {
	const size = 96
	hm := flatHeightmap(size, 0.3)

	// Carve a tall ridge straight down the middle column of +Z.
	const peak = 0.95
	for py := range size {
		// Three-pixel-wide ridge.
		for dx := -1; dx <= 1; dx++ {
			px := size/2 + dx
			if px < 0 || px >= size {
				continue
			}
			hm.Set(cubemap.FacePosZ, px, py, peak)
		}
	}

	// Two sites on +Z, one on each side of the ridge. Place them
	// well clear of the ridge so the A* pathfinder has room to
	// detour around an end.
	sites := []Site{
		{Habitability: 0.8, Population: 0.5},
		{Habitability: 0.8, Population: 0.5},
	}
	{
		dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, 8, size/2, size)
		n := math.Sqrt(dx*dx + dy*dy + dz*dz)
		sites[0].Dir = [3]float64{dx / n, dy / n, dz / n}
	}
	{
		dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, size-8, size/2, size)
		n := math.Sqrt(dx*dx + dy*dy + dz*dz)
		sites[1].Dir = [3]float64{dx / n, dy / n, dz / n}
	}

	startAddr := pixelAddrOf(sites[0], size)
	endAddr := pixelAddrOf(sites[1], size)

	path := AStarPath(startAddr, endAddr, hm, 5.0, 0)
	if len(path) == 0 {
		t.Fatalf("AStarPath returned empty path")
	}

	// Mean heightmap value along the road.
	var sumRoad float64
	for _, p := range path {
		sumRoad += hm.Get(p.Face, p.PX, p.PY)
	}
	meanRoad := sumRoad / float64(len(path))

	// Sample mean along the great-circle arc from start to end. Use
	// the same number of samples as the A* path for a fair comparison.
	cosAngle := sites[0].Dir[0]*sites[1].Dir[0] +
		sites[0].Dir[1]*sites[1].Dir[1] +
		sites[0].Dir[2]*sites[1].Dir[2]
	if cosAngle > 1 {
		cosAngle = 1
	}
	if cosAngle < -1 {
		cosAngle = -1
	}
	angle := math.Acos(cosAngle)
	sinAngle := math.Sin(angle)

	steps := len(path)
	var sumGC float64
	for i := range steps {
		t := float64(i) / float64(steps-1)
		// Spherical linear interpolation.
		var pt [3]float64
		if sinAngle < 1e-9 {
			pt = sites[0].Dir
		} else {
			a := math.Sin((1-t)*angle) / sinAngle
			b := math.Sin(t*angle) / sinAngle
			pt = [3]float64{
				a*sites[0].Dir[0] + b*sites[1].Dir[0],
				a*sites[0].Dir[1] + b*sites[1].Dir[1],
				a*sites[0].Dir[2] + b*sites[1].Dir[2],
			}
		}
		face, px, py := cubemap.DirToFacePixel(pt[0], pt[1], pt[2], size)
		sumGC += hm.Get(face, px, py)
	}
	meanGC := sumGC / float64(steps)

	t.Logf("meanRoad=%.4f meanGC=%.4f path-len=%d", meanRoad, meanGC, len(path))
	if meanRoad >= meanGC {
		t.Fatalf("road mean %.4f did not undercut great-circle mean %.4f", meanRoad, meanGC)
	}
}

// TestRoadsDeterministic runs GenerateRoads twice on identical inputs
// and asserts the output Intensity arrays compare DeepEqual.
func TestRoadsDeterministic(t *testing.T) {
	const size = 48
	hm := flatHeightmap(size, 0.4)

	sites := dirSites(
		[3]float64{1, 0.1, 0.2},
		[3]float64{1, -0.3, 0.1},
		[3]float64{0.1, 1, 0.2},
		[3]float64{-0.2, 1, -0.1},
		[3]float64{0.1, 0.2, 1},
	)

	a := GenerateRoads(sites, hm, types.CivConfig{Tier: 1}, 0)
	b := GenerateRoads(sites, hm, types.CivConfig{Tier: 1}, 0)

	if a == nil || b == nil {
		t.Fatalf("GenerateRoads returned nil; a=%v b=%v", a, b)
	}
	if a.Size != b.Size {
		t.Fatalf("size mismatch: %d vs %d", a.Size, b.Size)
	}
	for face := range cubemap.Face(cubemap.NumFaces) {
		if !reflect.DeepEqual(a.Intensity[face], b.Intensity[face]) {
			t.Fatalf("face %d intensity differs between runs", face)
		}
	}
}

// TestRoadsAvoidOcean places two sites on land separated by a vertical
// strip of ocean down the middle of +Z. The A* path with oceanLevel=0.3
// must either route around the ocean strip (longer, all on land) or
// fail to find a path; what's NOT acceptable is the path containing
// pixels below oceanLevel.
func TestRoadsAvoidOcean(t *testing.T) {
	const size = 64
	const oceanLevel = 0.30
	// Land elevation everywhere by default.
	hm := flatHeightmap(size, 0.50)

	// Carve a vertical ocean strip down the middle column of +Z (and
	// nowhere else). The strip is 4 pixels wide and runs the full
	// height of the face; sites are placed on +Z to either side.
	const oceanH = 0.10
	for py := range size {
		for dx := -2; dx < 2; dx++ {
			px := size/2 + dx
			if px < 0 || px >= size {
				continue
			}
			hm.Set(cubemap.FacePosZ, px, py, oceanH)
		}
	}

	// Two sites on +Z, well clear of the ocean strip.
	sites := []Site{
		{Habitability: 0.8, Population: 0.5},
		{Habitability: 0.8, Population: 0.5},
	}
	{
		dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, 8, size/2, size)
		n := math.Sqrt(dx*dx + dy*dy + dz*dz)
		sites[0].Dir = [3]float64{dx / n, dy / n, dz / n}
	}
	{
		dx, dy, dz := cubemap.FacePixelToDir(cubemap.FacePosZ, size-8, size/2, size)
		n := math.Sqrt(dx*dx + dy*dy + dz*dz)
		sites[1].Dir = [3]float64{dx / n, dy / n, dz / n}
	}

	startAddr := pixelAddrOf(sites[0], size)
	endAddr := pixelAddrOf(sites[1], size)

	path := AStarPath(startAddr, endAddr, hm, 5.0, oceanLevel)
	// Path may be nil if A* can't route around (acceptable); but if a
	// path exists, NO pixel may be below oceanLevel.
	for _, p := range path {
		h := hm.Get(p.Face, p.PX, p.PY)
		if h < oceanLevel {
			t.Fatalf("path pixel %+v has height=%.4f < oceanLevel=%.2f", p, h, oceanLevel)
		}
	}
	t.Logf("path-len=%d (nil=path refused, ok)", len(path))
}
