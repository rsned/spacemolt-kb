package main

import "slices"

// Edge is an undirected jump-gate link between two systems.
type Edge struct {
	A, B string
}

// Reach is the result of a multi-source breadth-first search from the
// stronghold set.
type Reach struct {
	// Dist maps a system ID to the number of jumps to the nearest
	// source. Systems that cannot reach any source are absent.
	Dist map[string]int
	// Owner maps a system ID to the ID of its nearest source. Ties break
	// toward the lowest-sorting source ID.
	Owner map[string]string
	// Max is the largest value in Dist, or 0 when Dist is empty.
	Max int
}

// ComputeReach runs a multi-source breadth-first search from sources over
// the undirected graph formed by edges.
//
// Sources are seeded in sorted order and adjacency lists are sorted, so the
// result is deterministic and equidistant systems are owned by the
// lowest-sorting source.
func ComputeReach(edges []Edge, sources []string) Reach {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.A] = append(adj[e.A], e.B)
		adj[e.B] = append(adj[e.B], e.A)
	}
	for id := range adj {
		slices.Sort(adj[id])
		adj[id] = slices.Compact(adj[id])
	}

	r := Reach{Dist: make(map[string]int), Owner: make(map[string]string)}

	srcs := slices.Clone(sources)
	slices.Sort(srcs)
	queue := make([]string, 0, len(srcs))
	for _, s := range srcs {
		if _, seen := r.Dist[s]; seen {
			continue
		}
		r.Dist[s] = 0
		r.Owner[s] = s
		queue = append(queue, s)
	}

	// The queue grows while it is being walked, so this cannot be a range
	// loop: range would fix the bound at the initial length.
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		for _, v := range adj[u] {
			if _, seen := r.Dist[v]; seen {
				continue
			}
			r.Dist[v] = r.Dist[u] + 1
			r.Owner[v] = r.Owner[u]
			queue = append(queue, v)
		}
	}

	for _, d := range r.Dist {
		if d > r.Max {
			r.Max = d
		}
	}
	return r
}
