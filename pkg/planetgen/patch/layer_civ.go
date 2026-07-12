package patch

import (
	"container/heap"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/fogleman/delaunay"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/feature"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
)

// roadSlopeWeight is the per-step slope-cost coefficient used by the
// flat-grid A* pathfinder, matching feature/roads.go's roadSlopeWeight
// (spec §4.6).
const roadSlopeWeight = 5.0

// civSlopeWeight is the MST edge-weight slope coefficient (spec
// §4.6): dist * (1 + civSlopeWeight*avgSlopeAlongStraightLine).
const civSlopeWeight = 3.0

// civMSTSamples is the number of evenly spaced height samples taken
// along a candidate MST edge's straight-line segment to estimate its
// average slope.
const civMSTSamples = 8

// civHabFloor is the lower habitability threshold below which a pixel
// is never considered for site placement, mirroring
// feature.poissonHabFloor.
const civHabFloor = 0.05

var (
	roadColor = color.RGBA{R: 95, G: 82, B: 62, A: 255}
	siteColor = color.RGBA{R: 210, G: 200, B: 160, A: 255}
)

func civEnabled(ctx *Context) bool { return ctx.Profile.Civ.Tier > 0 }

// patchHabitability scores every pixel with the shared production
// formula (feature.HabitabilityScoreAt); convergent distance comes
// from the min of the three cropped convergent-class FX distances
// (belt/subduction/arc).
func patchHabitability(ctx *Context, st *State) *Grid {
	f := ctx.Fields
	size := st.Height.Size
	sea := ctx.seaLevelView()
	hab := NewGrid(size)
	for i := range hab.Data {
		convKm := math.Min(f.BeltDist.Data[i], math.Min(f.SubdDist.Data[i], f.ArcDist.Data[i]))
		onRiver := st.Rivers != nil && st.Rivers[i]
		hab.Data[i] = feature.HabitabilityScoreAt(
			st.Height.Data[i], st.T.Data[i], st.M.Data[i], st.RainMult.Data[i],
			onRiver, convKm, sea)
	}
	return hab
}

// civSite pairs a feature.Site with its fractional pixel position on
// the patch grid. feature.Site itself carries no pixel field (it's
// shared with the spherical civ pipeline), so the pixel coordinate
// rides alongside in this patch-local wrapper rather than growing the
// shared type.
type civSite struct {
	Site feature.Site
	X, Y float64
}

// poissonPatch runs Bridson-style dart throwing on the flat grid with
// a habitability-modulated radius (in pixels). Deterministic via the
// "patch.civ.sites" / "patch.civ.sites.stream" seed domains.
func poissonPatch(ctx *Context, hab *Grid) []civSite {
	cfg := ctx.Profile.Civ
	size := hab.Size
	w := ctx.Fields.Window
	// Radii in virtual pixels: SiteMinDistRad/SiteMaxDistRad are
	// angular; one virtual pixel ~ (pi/2)/SProd radians.
	pxRad := w.PxRad()
	rMin := cfg.SiteMinDistRad / pxRad
	rMax := cfg.SiteMaxDistRad / pxRad
	if rMin <= 0 || rMax < rMin {
		return nil
	}
	rng := rand.New(rand.NewPCG(
		uint64(seed.Domain(ctx.Master, "patch.civ.sites")),        //nolint:gosec // deterministic domain hash, not security-sensitive
		uint64(seed.Domain(ctx.Master, "patch.civ.sites.stream")), //nolint:gosec // deterministic domain hash, not security-sensitive
	))

	type pt struct{ x, y float64 }
	var placed []pt
	var sites []civSite
	tooClose := func(x, y, r float64) bool {
		for _, p := range placed {
			if math.Hypot(p.x-x, p.y-y) < r {
				return true
			}
		}
		return false
	}
	attempts := size * size / 2
	for range attempts {
		x := rng.Float64() * float64(size-1)
		y := rng.Float64() * float64(size-1)
		h := hab.Bilinear(x, y)
		if h < civHabFloor {
			continue
		}
		r := rMin + (1-h)*(rMax-rMin)
		if tooClose(x, y, r) {
			continue
		}
		placed = append(placed, pt{x: x, y: y})
		dx, dy, dz := w.Dir(int(x), int(y))
		sites = append(sites, civSite{
			Site: feature.Site{Dir: [3]float64{dx, dy, dz}, Habitability: h},
			X:    x, Y: y,
		})
	}
	return sites
}

// delaunayEdges triangulates the flat pixel positions of sites and
// returns the unique undirected edges (i<j) of the triangulation,
// sorted for determinism. Returns nil for fewer than 3 sites or when
// the triangulator fails (collinear/degenerate input) — callers must
// treat "no edges" as "no roads", not an error.
func delaunayEdges(sites []civSite) (edges [][2]int) {
	if len(sites) < 3 {
		return nil
	}
	pts := make([]delaunay.Point, len(sites))
	for i, s := range sites {
		pts[i] = delaunay.Point{X: s.X, Y: s.Y}
	}
	// The vendored triangulator is not guaranteed panic-free on
	// pathological (e.g. fully collinear) input; treat a panic the
	// same as a returned error — no roads, not a crash. Narrowed to
	// just the Triangulate call (rather than the whole function) so a
	// bug in our own edge-dedup/sort logic below panics loudly
	// instead of being silently swallowed, matching production's
	// feature.SphericalDelaunay, which has no recover at all.
	tri, err := func() (t *delaunay.Triangulation, err error) {
		defer func() {
			if r := recover(); r != nil {
				t, err = nil, fmt.Errorf("delaunay: triangulate panic: %v", r)
			}
		}()
		return delaunay.Triangulate(pts)
	}()
	if err != nil || tri == nil {
		return nil
	}
	seen := make(map[[2]int]struct{}, len(tri.Triangles))
	for i := 0; i+2 < len(tri.Triangles); i += 3 {
		a, b, c := tri.Triangles[i], tri.Triangles[i+1], tri.Triangles[i+2]
		for _, pair := range [3][2]int{{a, b}, {b, c}, {a, c}} {
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
	edges = make([][2]int, 0, len(seen))
	for e := range seen {
		edges = append(edges, e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i][0] != edges[j][0] {
			return edges[i][0] < edges[j][0]
		}
		return edges[i][1] < edges[j][1]
	})
	return edges
}

// mstEdgeWeight samples height at civMSTSamples evenly spaced points
// along the straight-line segment between sites[i] and sites[j] and
// returns the Kruskal edge weight: great-line pixel distance times
// (1 + civSlopeWeight * average absolute slope between consecutive
// samples).
func mstEdgeWeight(sites []civSite, height *Grid, i, j int) float64 {
	ax, ay := sites[i].X, sites[i].Y
	bx, by := sites[j].X, sites[j].Y
	dist := math.Hypot(bx-ax, by-ay)

	var heights [civMSTSamples]float64
	for k := range civMSTSamples {
		t := float64(k) / float64(civMSTSamples-1)
		heights[k] = height.Bilinear(ax+t*(bx-ax), ay+t*(by-ay))
	}
	var sumAbs float64
	for k := 1; k < civMSTSamples; k++ {
		sumAbs += math.Abs(heights[k] - heights[k-1])
	}
	avgSlope := sumAbs / float64(civMSTSamples-1)
	return dist * (1 + civSlopeWeight*avgSlope)
}

// kruskalMST returns the subset of edges forming a minimum spanning
// tree over sites, weighted by mstEdgeWeight. Ties break on (i, j) for
// determinism.
func kruskalMST(sites []civSite, height *Grid, edges [][2]int) [][2]int {
	if len(edges) == 0 || len(sites) < 2 {
		return nil
	}
	type weightedEdge struct {
		i, j int
		w    float64
	}
	we := make([]weightedEdge, len(edges))
	for k, e := range edges {
		we[k] = weightedEdge{i: e[0], j: e[1], w: mstEdgeWeight(sites, height, e[0], e[1])}
	}
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

	out := make([][2]int, 0, len(sites)-1)
	for _, e := range we {
		ri, rj := find(e.i), find(e.j)
		if ri != rj {
			parent[ri] = rj
			out = append(out, [2]int{e.i, e.j})
		}
	}
	return out
}

// civAstarNode is a single open-set entry in the flat-grid A* priority
// queue.
type civAstarNode struct {
	x, y  int
	g, f  float64
	tie   int
	index int
}

type civAstarHeap []*civAstarNode

func (h civAstarHeap) Len() int { return len(h) }
func (h civAstarHeap) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	// Deterministic tiebreak when f-scores match.
	return h[i].tie < h[j].tie
}
func (h civAstarHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *civAstarHeap) Push(x any) {
	n, _ := x.(*civAstarNode)
	n.index = len(*h)
	*h = append(*h, n)
}
func (h *civAstarHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	x.index = -1
	*h = old[:n-1]
	return x
}

// civNeighbors8 lists the 8-connected pixel offsets with their
// Euclidean step distance.
var civNeighbors8 = [8]struct {
	dx, dy int
	dist   float64
}{
	{-1, -1, math.Sqrt2}, {0, -1, 1}, {1, -1, math.Sqrt2},
	{-1, 0, 1}, {1, 0, 1},
	{-1, 1, math.Sqrt2}, {0, 1, 1}, {1, 1, math.Sqrt2},
}

// astarRoad finds the minimum-cost 8-connected pixel path between the
// (rounded) pixel positions of two sites on the flat height grid.
// Per-step cost is dist * (1 + roadSlopeWeight*|Δh|*SProd); pixels
// with height below seaLevel are refused. Returns nil when no path
// exists (e.g. the sites are separated by ocean) or when either
// endpoint is itself underwater.
func astarRoad(height *Grid, seaLevel float64, sProd int, ax, ay, bx, by float64) []image.Point {
	size := height.Size
	clamp := func(v float64) int {
		iv := int(math.Round(v))
		if iv < 0 {
			return 0
		}
		if iv > size-1 {
			return size - 1
		}
		return iv
	}
	sx, sy := clamp(ax), clamp(ay)
	ex, ey := clamp(bx), clamp(by)
	if seaLevel > 0 && (height.At(sx, sy) < seaLevel || height.At(ex, ey) < seaLevel) {
		return nil
	}
	if sx == ex && sy == ey {
		return []image.Point{{X: sx, Y: sy}}
	}

	tie := func(x, y int) int { return y*size + x }
	heuristic := func(x, y int) float64 { return math.Hypot(float64(ex-x), float64(ey-y)) }

	open := &civAstarHeap{}
	heap.Init(open)
	type key struct{ x, y int }
	gScore := map[key]float64{{sx, sy}: 0}
	cameFrom := map[key]key{}
	closed := map[key]bool{}
	heap.Push(open, &civAstarNode{x: sx, y: sy, g: 0, f: heuristic(sx, sy), tie: tie(sx, sy)})

	for open.Len() > 0 {
		cur, _ := heap.Pop(open).(*civAstarNode)
		if cur.x == ex && cur.y == ey {
			path := []image.Point{{X: cur.x, Y: cur.y}}
			k := key{cur.x, cur.y}
			for {
				prev, ok := cameFrom[k]
				if !ok {
					break
				}
				path = append(path, image.Point{X: prev.x, Y: prev.y})
				k = prev
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		ck := key{cur.x, cur.y}
		if closed[ck] {
			continue
		}
		closed[ck] = true
		curH := height.At(cur.x, cur.y)

		for _, n := range civNeighbors8 {
			nx, ny := cur.x+n.dx, cur.y+n.dy
			if nx < 0 || ny < 0 || nx >= size || ny >= size {
				continue
			}
			nk := key{nx, ny}
			if closed[nk] {
				continue
			}
			nH := height.At(nx, ny)
			if seaLevel > 0 && nH < seaLevel {
				continue
			}
			step := n.dist * (1 + roadSlopeWeight*math.Abs(nH-curH)*float64(sProd))
			tentative := cur.g + step
			if existing, ok := gScore[nk]; ok && tentative >= existing {
				continue
			}
			gScore[nk] = tentative
			cameFrom[nk] = ck
			heap.Push(open, &civAstarNode{x: nx, y: ny, g: tentative, f: tentative + heuristic(nx, ny), tie: tie(nx, ny)})
		}
	}
	return nil
}

// paintRoad stamps 1px-wide road pixels onto img in the dirt-road
// brown.
func paintRoad(img *image.RGBA, path []image.Point) {
	for _, p := range path {
		o := img.PixOffset(p.X, p.Y)
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = roadColor.R, roadColor.G, roadColor.B, roadColor.A
	}
}

// paintSite stamps a filled disc for a site, radius scaled by its
// population relative to the most populous site on the patch.
func paintSite(img *image.RGBA, cx, cy float64, population, maxPop float64) {
	size := img.Bounds().Dx()
	r := 1
	if maxPop > 0 {
		r += int(2 * population / maxPop)
	}
	ix, iy := int(math.Round(cx)), int(math.Round(cy))
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy > r*r {
				continue
			}
			x, y := ix+dx, iy+dy
			if x < 0 || y < 0 || x >= size || y >= size {
				continue
			}
			o := img.PixOffset(x, y)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = siteColor.R, siteColor.G, siteColor.B, siteColor.A
		}
	}
}

// applyCiv (layer 12, final): habitability -> Poisson-disc sites ->
// Zipf populations -> Delaunay/Kruskal MST -> per-edge A* roads,
// overlaid onto Img. Publishes Sites (spec §4.6). Enabled only when
// ctx.Profile.Civ.Tier > 0.
func applyCiv(ctx *Context, st *State) *State {
	w := ctx.Fields.Window
	sea := ctx.seaLevelView()

	ns := *st
	ns.Img = image.NewRGBA(image.Rect(0, 0, w.Size, w.Size))
	copy(ns.Img.Pix, st.Img.Pix)

	hab := patchHabitability(ctx, st)
	raw := poissonPatch(ctx, hab)
	if len(raw) == 0 {
		ns.Sites = nil
		return &ns
	}

	// Sort the combined (Site, pixel-position) slice by habitability
	// desc BEFORE calling AssignPopulations, so its internal stable
	// sort on the extracted []feature.Site is a no-op reorder and the
	// pixel positions in raw stay index-aligned with the populations
	// written into the extracted slice.
	sort.SliceStable(raw, func(i, j int) bool {
		return raw[i].Site.Habitability > raw[j].Site.Habitability
	})
	sites := make([]feature.Site, len(raw))
	for i, r := range raw {
		sites[i] = r.Site
	}
	feature.AssignPopulations(sites, ctx.Profile.Civ)

	if len(sites) >= 2 {
		edges := delaunayEdges(raw)
		mst := kruskalMST(raw, st.Height, edges)
		for _, e := range mst {
			path := astarRoad(st.Height, sea, w.SProd, raw[e[0]].X, raw[e[0]].Y, raw[e[1]].X, raw[e[1]].Y)
			if len(path) == 0 {
				continue
			}
			paintRoad(ns.Img, path)
		}
	}

	maxPop := 0.0
	for _, s := range sites {
		if s.Population > maxPop {
			maxPop = s.Population
		}
	}
	for i, s := range sites {
		paintSite(ns.Img, raw[i].X, raw[i].Y, s.Population, maxPop)
	}

	ns.Sites = sites
	return &ns
}
