package main

import "testing"

// chain returns edges for a-b-c-d-e.
func chain() []Edge {
	return []Edge{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "e"}}
}

func TestComputeReachSourcesAreDistanceZero(t *testing.T) {
	r := ComputeReach(chain(), []string{"a", "e"})

	if r.Dist["a"] != 0 || r.Dist["e"] != 0 {
		t.Errorf("sources must be distance 0, got a=%d e=%d", r.Dist["a"], r.Dist["e"])
	}
	if r.Owner["a"] != "a" || r.Owner["e"] != "e" {
		t.Errorf("sources must own themselves, got a=%q e=%q", r.Owner["a"], r.Owner["e"])
	}
}

func TestComputeReachDistancesAlongChain(t *testing.T) {
	r := ComputeReach(chain(), []string{"a"})

	want := map[string]int{"a": 0, "b": 1, "c": 2, "d": 3, "e": 4}
	for id, w := range want {
		if r.Dist[id] != w {
			t.Errorf("dist[%s] = %d, want %d", id, r.Dist[id], w)
		}
	}
	if r.Max != 4 {
		t.Errorf("Max = %d, want 4", r.Max)
	}
}

func TestComputeReachEdgesAreUndirected(t *testing.T) {
	// Only a->b is listed, but reaching a from b must still work.
	r := ComputeReach([]Edge{{"a", "b"}}, []string{"b"})

	if r.Dist["a"] != 1 {
		t.Errorf("dist[a] = %d, want 1 (edges are undirected)", r.Dist["a"])
	}
}

func TestComputeReachTieBrokenByLowestSourceID(t *testing.T) {
	// c is two hops from both a and e.
	r := ComputeReach(chain(), []string{"e", "a"})

	if r.Dist["c"] != 2 {
		t.Fatalf("dist[c] = %d, want 2", r.Dist["c"])
	}
	if r.Owner["c"] != "a" {
		t.Errorf("owner[c] = %q, want %q (ties break to the lowest source ID)", r.Owner["c"], "a")
	}
}

func TestComputeReachDisconnectedNodeIsAbsent(t *testing.T) {
	r := ComputeReach([]Edge{{"a", "b"}, {"y", "z"}}, []string{"a"})

	if _, ok := r.Dist["z"]; ok {
		t.Errorf("unreachable node must be absent from Dist")
	}
	if _, ok := r.Owner["z"]; ok {
		t.Errorf("unreachable node must be absent from Owner")
	}
	if r.Max != 1 {
		t.Errorf("Max = %d, want 1 (unreachable nodes must not inflate Max)", r.Max)
	}
}

func TestComputeReachNoSources(t *testing.T) {
	r := ComputeReach(chain(), nil)

	if len(r.Dist) != 0 {
		t.Errorf("no sources should reach nothing, got %d entries", len(r.Dist))
	}
	if r.Max != 0 {
		t.Errorf("Max = %d, want 0", r.Max)
	}
}

func TestComputeReachDuplicateSourceIsHarmless(t *testing.T) {
	r := ComputeReach(chain(), []string{"a", "a"})

	if r.Dist["b"] != 1 {
		t.Errorf("dist[b] = %d, want 1", r.Dist["b"])
	}
}
