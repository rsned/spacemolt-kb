package feature

import (
	"container/heap"
	"math"
	"sort"

	"github.com/fogleman/delaunay"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// roadSlopeWeight is the per-step slope-cost coefficient used by the
// A* pathfinder. A neighbour transition costs 1 + roadSlopeWeight *
// |Δheight|, so a 0.1 height step adds 0.5 to the per-step cost. The
// constant matches the "5*max(0, h - 0.7)" terrain factor used by the
// MST so the two stages share a sense of "mountainous = 5x worse".
const roadSlopeWeight = 5.0

// roadCenterIntensity is the per-pixel intensity contribution at each
// step of a rasterised road. Subsequent overlapping paths add to the
// same pixel, capped by roadIntensityCap.
const roadCenterIntensity = 1.0

// roadNeighborIntensity is the contribution to each 4-connected
// neighbour of a path pixel. 0.3 produces a gently anti-aliased edge
// when overlaid by the colorizer (T5).
const roadNeighborIntensity = 0.3

// roadIntensityCap limits per-pixel intensity. A pathological set of
// many overlapping paths could otherwise drive the value arbitrarily
// high; the colorizer clamps the visible value to 1, but capping here
// keeps the field's float range bounded for downstream consumers.
const roadIntensityCap = 2.0

// RoadField holds per-pixel road intensity in [0, roadIntensityCap].
// Faces are flat row-major Size*Size slices, matching cubemap.CubeMapF
// layout. The colorizer overlays this onto the daytime palette at low
// alpha (T5).
type RoadField struct {
	Size      int
	Intensity [cubemap.NumFaces][]float64
}

// SphericalDelaunay returns the Delaunay-triangulation edges of the
// sites projected via stereographic from the antipode of the centroid
// onto a plane through the centroid. Edges are returned as (i, j)
// index pairs with i < j, deduplicated, and sorted by (i, j) for
// determinism.
//
// Returns nil when len(sites) < 3, when the projection degenerates
// (e.g. the centroid is the zero vector), or when the underlying
// triangulator returns an error.
func SphericalDelaunay(sites []Site) [][2]int {
	if len(sites) < 3 {
		return nil
	}

	// Centroid of the input directions, normalised. Stereographic
	// projects through the antipode of c onto the tangent plane at c.
	var cx, cy, cz float64
	for _, s := range sites {
		cx += s.Dir[0]
		cy += s.Dir[1]
		cz += s.Dir[2]
	}
	cn := math.Sqrt(cx*cx + cy*cy + cz*cz)
	if cn < 1e-12 {
		return nil
	}
	c := [3]float64{cx / cn, cy / cn, cz / cn}

	// Tangent basis (e1, e2) ⟂ c, shared with poisson_sphere.go.
	e1, e2 := tangentBasis(c)

	// Stereographic projection from -c onto the plane at c. For a
	// site direction d, scale s = 2 / (1 + d·c); planar coords are
	// (s * d·e1, s * d·e2). Sites coincident with the projection
	// point (-c) blow up to infinity — we reject the rare case where
	// 1 + d·c is below a small floor.
	pts := make([]delaunay.Point, len(sites))
	for i, s := range sites {
		d := s.Dir
		denom := 1 + d[0]*c[0] + d[1]*c[1] + d[2]*c[2]
		if denom < 1e-9 {
			// Site at the antipode of the centroid; projection
			// is ill-defined. Fall back to placing it far out
			// along e1 to keep triangulation alive without an
			// infinity. The site will end up on the convex hull
			// and its incident edges may be longer than ideal,
			// but they remain valid input for the planar
			// triangulator.
			pts[i] = delaunay.Point{X: 1e9, Y: 0}
			continue
		}
		scale := 2.0 / denom
		x := scale * (d[0]*e1[0] + d[1]*e1[1] + d[2]*e1[2])
		y := scale * (d[0]*e2[0] + d[1]*e2[1] + d[2]*e2[2])
		pts[i] = delaunay.Point{X: x, Y: y}
	}

	tri, err := delaunay.Triangulate(pts)
	if err != nil || tri == nil {
		return nil
	}

	// Collect unique edges from the triangle list. Each triangle
	// contributes three edges; the dedup map keys by the sorted
	// pair so cross-triangle duplicates collapse.
	seen := make(map[[2]int]struct{}, len(tri.Triangles))
	for i := 0; i+2 < len(tri.Triangles); i += 3 {
		a, b, cidx := tri.Triangles[i], tri.Triangles[i+1], tri.Triangles[i+2]
		for _, pair := range [3][2]int{{a, b}, {b, cidx}, {a, cidx}} {
			lo, hi := pair[0], pair[1]
			if lo > hi {
				lo, hi = hi, lo
			}
			if lo == hi {
				continue
			}
			seen[[2]int{lo, hi}] = struct{}{}
		}
	}

	edges := make([][2]int, 0, len(seen))
	for e := range seen {
		edges = append(edges, e)
	}
	// Deterministic ordering — map iteration is randomised in Go.
	sort.Slice(edges, func(i, j int) bool {
		if edges[i][0] != edges[j][0] {
			return edges[i][0] < edges[j][0]
		}
		return edges[i][1] < edges[j][1]
	})
	return edges
}

// edgeWeight returns the great-circle distance between sites[i] and
// sites[j], multiplied by a terrain factor sampled at the midpoint
// direction. The terrain factor is 1 when heightmap is nil or when
// the midpoint height is below 0.7; above 0.7 it climbs linearly so
// a peak at h=0.9 carries a 2x multiplier. Matches the formula in
// the Phase 9b plan (item 18 step 2).
func edgeWeight(sites []Site, i, j int, heightmap *cubemap.CubeMapF) float64 {
	a := sites[i].Dir
	b := sites[j].Dir
	cosAngle := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
	if cosAngle > 1 {
		cosAngle = 1
	} else if cosAngle < -1 {
		cosAngle = -1
	}
	dist := math.Acos(cosAngle)

	terrain := 1.0
	if heightmap != nil {
		mx := a[0] + b[0]
		my := a[1] + b[1]
		mz := a[2] + b[2]
		mn := math.Sqrt(mx*mx + my*my + mz*mz)
		if mn > 1e-12 {
			mx /= mn
			my /= mn
			mz /= mn
			face, px, py := cubemap.DirToFacePixel(mx, my, mz, heightmap.Size)
			h := heightmap.Get(face, px, py)
			if h > 0.7 {
				terrain = 1 + 5*(h-0.7)
			}
		}
	}
	return dist * terrain
}

// MinSpanningTree returns the subset of the Delaunay edges that form a
// Kruskal minimum spanning tree, weighted by great-circle distance
// times a midpoint terrain factor. heightmap may be nil, in which case
// the terrain factor is 1.
//
// Output edges are returned in the same orientation (i, j) as the
// input — this function does not normalise the pair order.
func MinSpanningTree(sites []Site, edges [][2]int, heightmap *cubemap.CubeMapF) [][2]int {
	if len(edges) == 0 || len(sites) < 2 {
		return nil
	}

	type weightedEdge struct {
		i, j int
		w    float64
	}
	we := make([]weightedEdge, len(edges))
	for k, e := range edges {
		we[k] = weightedEdge{i: e[0], j: e[1], w: edgeWeight(sites, e[0], e[1], heightmap)}
	}
	// Sort by weight asc; tiebreak by (i, j) so equal weights resolve
	// deterministically regardless of the input order.
	sort.Slice(we, func(a, b int) bool {
		if we[a].w != we[b].w {
			return we[a].w < we[b].w
		}
		if we[a].i != we[b].i {
			return we[a].i < we[b].i
		}
		return we[a].j < we[b].j
	})

	parent := make([]int, len(sites))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) bool {
		ra, rb := find(a), find(b)
		if ra == rb {
			return false
		}
		parent[ra] = rb
		return true
	}

	out := make([][2]int, 0, len(sites)-1)
	for _, e := range we {
		if union(e.i, e.j) {
			out = append(out, [2]int{e.i, e.j})
		}
	}
	return out
}

// astarNode is a single open-set entry in the A* priority queue.
type astarNode struct {
	addr  cubemap.PixelAddr
	g, f  float64
	tie   int // deterministic tiebreak key
	index int // heap.Interface bookkeeping
}

type astarHeap []*astarNode

func (h astarHeap) Len() int { return len(h) }
func (h astarHeap) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	// Deterministic tiebreak when f-scores match; without this the
	// heap can return either of two equal-priority entries based on
	// internal layout, which (combined with later pops) causes
	// path-equivalent but pixel-different results across runs.
	return h[i].tie < h[j].tie
}
func (h astarHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *astarHeap) Push(x any) {
	n := x.(*astarNode)
	n.index = len(*h)
	*h = append(*h, n)
}
func (h *astarHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	x.index = -1
	*h = old[:n-1]
	return x
}

// addrTie returns a deterministic linearised key for a pixel address
// (face * S^2 + py * S + px). Used as the heap tiebreak so identical
// f-scores resolve in a fixed order.
func addrTie(addr cubemap.PixelAddr, S int) int {
	return int(addr.Face)*S*S + addr.PY*S + addr.PX
}

// arcDistance returns the great-circle (in radians) between two unit
// vectors. Used as the A* heuristic — admissible because it's a
// strict lower bound on the per-pixel cost (1 + slope*|Δh| ≥ 1) and
// the per-pixel angular step approximates 1/S, so the arc-distance in
// radians is always ≤ the path's accumulated cost.
func arcDistance(a, b [3]float64) float64 {
	cosAngle := a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
	if cosAngle > 1 {
		cosAngle = 1
	} else if cosAngle < -1 {
		cosAngle = -1
	}
	return math.Acos(cosAngle)
}

// AStarPath returns the minimum-cost 8-connected pixel path between
// start and end on the cube-map heightmap. Per-step cost is 1 +
// slopeWeight * |height(neighbor) - height(current)|. The heuristic
// is the great-circle arc-distance from the current pixel direction
// to the end pixel direction, which is admissible (≤ true remaining
// cost) and consistent.
//
// Cube-face seams are crossed via cubemap.FacePixelNeighbors8. Returns
// nil when heightmap is nil or no path exists. The returned slice
// includes both endpoints.
func AStarPath(start, end cubemap.PixelAddr, heightmap *cubemap.CubeMapF, slopeWeight float64) []cubemap.PixelAddr {
	if heightmap == nil {
		return nil
	}
	S := heightmap.Size
	if S <= 0 {
		return nil
	}
	if start == end {
		return []cubemap.PixelAddr{start}
	}

	endX, endY, endZ := cubemap.FacePixelToDir(end.Face, end.PX, end.PY, S)
	endDir := [3]float64{endX, endY, endZ}

	open := &astarHeap{}
	heap.Init(open)

	gScore := map[cubemap.PixelAddr]float64{start: 0}
	cameFrom := map[cubemap.PixelAddr]cubemap.PixelAddr{}
	closed := map[cubemap.PixelAddr]struct{}{}

	startX, startY, startZ := cubemap.FacePixelToDir(start.Face, start.PX, start.PY, S)
	startH := arcDistance([3]float64{startX, startY, startZ}, endDir)
	heap.Push(open, &astarNode{addr: start, g: 0, f: startH, tie: addrTie(start, S)})

	for open.Len() > 0 {
		cur := heap.Pop(open).(*astarNode)
		if cur.addr == end {
			// Reconstruct path.
			path := []cubemap.PixelAddr{cur.addr}
			node := cur.addr
			for {
				prev, ok := cameFrom[node]
				if !ok {
					break
				}
				path = append(path, prev)
				node = prev
			}
			// Reverse in place so path[0] = start.
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		if _, done := closed[cur.addr]; done {
			continue
		}
		closed[cur.addr] = struct{}{}

		curH := heightmap.Get(cur.addr.Face, cur.addr.PX, cur.addr.PY)

		neighbors := cubemap.FacePixelNeighbors8(cur.addr.Face, cur.addr.PX, cur.addr.PY, S)
		for _, n := range neighbors {
			if _, done := closed[n]; done {
				continue
			}
			nH := heightmap.Get(n.Face, n.PX, n.PY)
			step := 1.0 + slopeWeight*math.Abs(nH-curH)
			tentativeG := cur.g + step
			if existing, ok := gScore[n]; ok && tentativeG >= existing {
				continue
			}
			gScore[n] = tentativeG
			cameFrom[n] = cur.addr
			nx, ny, nz := cubemap.FacePixelToDir(n.Face, n.PX, n.PY, S)
			h := arcDistance([3]float64{nx, ny, nz}, endDir)
			heap.Push(open, &astarNode{
				addr: n,
				g:    tentativeG,
				f:    tentativeG + h,
				tie:  addrTie(n, S),
			})
		}
	}
	return nil
}

// rasterizePath stamps each pixel in path into rf at +roadCenterIntensity,
// and bleeds +roadNeighborIntensity into the 4-connected neighbours via
// cubemap.OffsetPixel (so seams are handled correctly). All writes are
// saturated at roadIntensityCap.
func rasterizePath(rf *RoadField, path []cubemap.PixelAddr) {
	S := rf.Size
	addAt := func(addr cubemap.PixelAddr, v float64) {
		idx := addr.PY*S + addr.PX
		cur := rf.Intensity[addr.Face][idx]
		nv := cur + v
		if nv > roadIntensityCap {
			nv = roadIntensityCap
		}
		rf.Intensity[addr.Face][idx] = nv
	}
	for _, p := range path {
		addAt(p, roadCenterIntensity)
		// 4-connected neighbours via OffsetPixel — seam-aware.
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			f, nx, ny := cubemap.OffsetPixel(p.Face, p.PX, p.PY, d[0], d[1], S)
			addAt(cubemap.PixelAddr{Face: f, PX: nx, PY: ny}, roadNeighborIntensity)
		}
	}
}

// GenerateRoads runs the full Delaunay → MST → A* → rasterise pipeline
// and returns a RoadField sized to match heightmap. Returns nil when
// sites is empty, heightmap is nil, cfg.Tier <= 0, or fewer than two
// sites are placed (a single site has no edges to draw).
//
// The slope weight inside A* is the package constant roadSlopeWeight
// (5.0); a future task may make it a CivConfig knob.
func GenerateRoads(sites []Site, heightmap *cubemap.CubeMapF, cfg types.CivConfig) *RoadField {
	if len(sites) < 2 || heightmap == nil || cfg.Tier <= 0 {
		return nil
	}

	S := heightmap.Size
	rf := &RoadField{Size: S}
	for face := range cubemap.Face(cubemap.NumFaces) {
		rf.Intensity[face] = make([]float64, S*S)
	}

	edges := SphericalDelaunay(sites)
	if len(edges) == 0 {
		return rf
	}
	mst := MinSpanningTree(sites, edges, heightmap)
	if len(mst) == 0 {
		return rf
	}

	for _, e := range mst {
		startFace, sx, sy := cubemap.DirToFacePixel(sites[e[0]].Dir[0], sites[e[0]].Dir[1], sites[e[0]].Dir[2], S)
		endFace, ex, ey := cubemap.DirToFacePixel(sites[e[1]].Dir[0], sites[e[1]].Dir[1], sites[e[1]].Dir[2], S)
		path := AStarPath(
			cubemap.PixelAddr{Face: startFace, PX: sx, PY: sy},
			cubemap.PixelAddr{Face: endFace, PX: ex, PY: ey},
			heightmap, roadSlopeWeight,
		)
		if len(path) == 0 {
			continue
		}
		rasterizePath(rf, path)
	}
	return rf
}
