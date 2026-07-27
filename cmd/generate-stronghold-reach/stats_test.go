package main

import (
	"slices"
	"testing"
)

// twoStars returns two disjoint 3-node chains: a-b-c and x-y-z, plus a
// bridge c-z that only closes at higher radius.
//
//	a - b - c - z - y - x
func twoStars() []Edge {
	return []Edge{{"a", "b"}, {"b", "c"}, {"c", "z"}, {"z", "y"}, {"y", "x"}}
}

func TestComponentCountCountsIsolatedNodes(t *testing.T) {
	// Only a and x are in the set, and they share no edge.
	got := componentCount(twoStars(), map[string]bool{"a": true, "x": true})
	if got != 2 {
		t.Errorf("componentCount = %d, want 2", got)
	}
}

func TestComponentCountJoinsAcrossEdges(t *testing.T) {
	got := componentCount(twoStars(), map[string]bool{"a": true, "b": true, "c": true})
	if got != 1 {
		t.Errorf("componentCount = %d, want 1", got)
	}
}

func TestComponentCountIgnoresEdgesLeavingTheSet(t *testing.T) {
	// c-z exists but z is out of the set, so it must not merge anything.
	got := componentCount(twoStars(), map[string]bool{"c": true, "x": true})
	if got != 2 {
		t.Errorf("componentCount = %d, want 2", got)
	}
}

func TestRadiusRowsCoverageAndBlobs(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// dist: a=0 x=0 b=1 y=1 c=2 z=2
	rows := RadiusRows(r, edges, 6, 3)

	if len(rows) != 3 {
		t.Fatalf("len(rows) = %d, want 3", len(rows))
	}
	want := []struct{ systems, blobs int }{
		{4, 2}, // R=1: a,b,x,y
		{6, 1}, // R=2: all six, c-z bridges them
		{6, 1}, // R=3: unchanged
	}
	for i, w := range want {
		if rows[i].Radius != i+1 {
			t.Errorf("rows[%d].Radius = %d, want %d", i, rows[i].Radius, i+1)
		}
		if rows[i].Systems != w.systems {
			t.Errorf("rows[%d].Systems = %d, want %d", i, rows[i].Systems, w.systems)
		}
		if rows[i].Blobs != w.blobs {
			t.Errorf("rows[%d].Blobs = %d, want %d", i, rows[i].Blobs, w.blobs)
		}
	}
}

func TestRadiusRowsPercentUsesTotal(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 8, 1)

	// 4 of 8 systems in reach at R=1.
	if rows[0].Percent != 50.0 {
		t.Errorf("Percent = %v, want 50", rows[0].Percent)
	}
}

func TestRadiusRowsMergedFlag(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 6, 3)

	if rows[0].Merged {
		t.Errorf("first row must never be Merged")
	}
	if !rows[1].Merged {
		t.Errorf("row at R=2 should be Merged (2 blobs -> 1)")
	}
	if rows[2].Merged {
		t.Errorf("row at R=3 should not be Merged (blob count unchanged)")
	}
}

func TestRadiusRowsBlobCountNeverIncreases(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := RadiusRows(r, edges, 6, 5)

	for i := 1; i < len(rows); i++ {
		if rows[i].Blobs > rows[i-1].Blobs {
			t.Errorf("blob count rose from %d to %d at R=%d",
				rows[i-1].Blobs, rows[i].Blobs, rows[i].Radius)
		}
	}
}

func TestRadiusRowsZeroTotalDoesNotDivideByZero(t *testing.T) {
	rows := RadiusRows(Reach{Dist: map[string]int{}}, nil, 0, 2)

	for _, row := range rows {
		if row.Percent != 0 {
			t.Errorf("Percent = %v, want 0 when total is 0", row.Percent)
		}
	}
}

func TestTerritoryRowsSortedByCountThenName(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// a owns a,b,c ; x owns x,y,z
	rows := TerritoryRows(r, map[string]string{"a": "Alpha", "x": "Xray"})

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	// Equal counts (3 each), so name ascending: Alpha before Xray.
	if rows[0].Name != "Alpha" || rows[1].Name != "Xray" {
		t.Errorf("got %q,%q; want Alpha,Xray", rows[0].Name, rows[1].Name)
	}
	if rows[0].Systems != 3 || rows[1].Systems != 3 {
		t.Errorf("counts = %d,%d; want 3,3", rows[0].Systems, rows[1].Systems)
	}
}

func TestTerritoryRowsLargestFirst(t *testing.T) {
	// a and x are both sources, but a is force-fed y and z below so its
	// territory outweighs x's.
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// Force an imbalance by making x own nothing but itself.
	r.Owner["y"] = "a"
	r.Owner["z"] = "a"
	rows := TerritoryRows(r, map[string]string{"a": "Alpha", "x": "Xray"})

	if rows[0].Name != "Alpha" || rows[0].Systems != 5 {
		t.Errorf("rows[0] = %+v, want Alpha with 5", rows[0])
	}
}

func TestTerritoryRowsDeterministicTieBreak(t *testing.T) {
	// a and x already own exactly 3 systems each (see twoStars); give them
	// the same display name so the count and name tie-breaks both wash
	// out and SystemID is the only thing left to order by.
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	rows := TerritoryRows(r, map[string]string{"a": "Same", "x": "Same"})

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	// Both have 3 systems and "Same" name; should order by SystemID.
	// "a" < "x" lexicographically, so "a" should come first.
	if rows[0].SystemID != "a" || rows[1].SystemID != "x" {
		t.Errorf("got SystemIDs %q,%q; want a,x (deterministic SystemID ordering)",
			rows[0].SystemID, rows[1].SystemID)
	}
}

func TestRadiusRowsNegativeMaxRadiusNoPanic(t *testing.T) {
	edges := twoStars()
	r := ComputeReach(edges, []string{"a", "x"})
	// Negative maxRadius should not panic; should return empty slice.
	rows := RadiusRows(r, edges, 6, -1)

	if rows == nil {
		t.Errorf("RadiusRows with negative maxRadius returned nil, want empty non-nil slice")
	}
	if len(rows) != 0 {
		t.Errorf("RadiusRows with negative maxRadius returned %d rows, want 0", len(rows))
	}
}

// threeStrongholdBridge builds three chains anchored on the sources p, q,
// and r, joined by two bridges that only enter the reach set at higher
// radius:
//
//	p - p1 - p2 - p3
//	                \
//	                 q2 - q1 - q
//	                  \
//	                   r1 - r
//
// The q2-r1 bridge merges the q and r chains at radius 2; the p3-q2 bridge
// merges the p chain onto that pair at radius 3, the graph's max distance.
// This gives blob counts 3 -> 2 -> 1 across three sources, unlike twoStars
// (used by the tests above), which only ever goes 2 -> 1 with a single pair
// of sources.
func threeStrongholdBridge() (edges []Edge, sources []string) {
	edges = []Edge{
		{"p", "p1"}, {"p1", "p2"}, {"p2", "p3"},
		{"q", "q1"}, {"q1", "q2"},
		{"r", "r1"},
		{"q2", "r1"},
		{"p3", "q2"},
	}
	sources = []string{"p", "q", "r"}
	return edges, sources
}

// componentsOf groups the members of inSet into connected components using
// only the in-set edges. It is a separate, membership-returning
// implementation from componentCount's union-find (which only counts), so
// this test does not rely on the correctness of the code it is checking.
func componentsOf(edges []Edge, inSet map[string]bool) []map[string]bool {
	parent := make(map[string]string, len(inSet))
	for id := range inSet {
		parent[id] = id
	}
	var find func(string) string //nolint:staticcheck // S1021 false positive: self-referential closure needs the two-step declare-then-assign
	find = func(x string) string {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, e := range edges {
		if !inSet[e.A] || !inSet[e.B] {
			continue
		}
		ra, rb := find(e.A), find(e.B)
		if ra != rb {
			parent[ra] = rb
		}
	}

	groups := make(map[string]map[string]bool)
	for id := range inSet {
		root := find(id)
		if groups[root] == nil {
			groups[root] = make(map[string]bool)
		}
		groups[root][id] = true
	}
	out := make([]map[string]bool, 0, len(groups))
	for _, g := range groups {
		out = append(out, g)
	}
	return out
}

// TestEveryReachComponentContainsAStronghold proves the invariant the
// design doc committed to: at every radius, every connected component of
// the "<=radius" in-set contains at least one stronghold. This is what
// guarantees RadiusRows' blob count can only fall as radius grows — the
// page's central claim.
func TestEveryReachComponentContainsAStronghold(t *testing.T) {
	edges, sources := threeStrongholdBridge()
	sourceSet := make(map[string]bool, len(sources))
	for _, s := range sources {
		sourceSet[s] = true
	}

	r := ComputeReach(edges, sources)
	if r.Max < 2 {
		t.Fatalf("test fixture is too shallow: Max = %d, want >= 2 to exercise multiple merges", r.Max)
	}

	for radius := 1; radius <= r.Max; radius++ {
		inSet := make(map[string]bool)
		for id, d := range r.Dist {
			if d <= radius {
				inSet[id] = true
			}
		}

		for _, comp := range componentsOf(edges, inSet) {
			hasSource := false
			for id := range comp {
				if sourceSet[id] {
					hasSource = true
					break
				}
			}
			if !hasSource {
				ids := make([]string, 0, len(comp))
				for id := range comp {
					ids = append(ids, id)
				}
				slices.Sort(ids)
				t.Errorf("radius %d: component %v contains no stronghold", radius, ids)
			}
		}
	}
}
