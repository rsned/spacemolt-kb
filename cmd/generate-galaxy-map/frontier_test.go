package main

import (
	"testing"

	"github.com/rsned/spacemolt-kb/internal/frontier"
)

func TestPlaceFrontier(t *testing.T) {
	sol := &System{ID: "sol", Connections: []Connection{{SystemID: "lost"}}}
	ln := &System{ID: "long_night", Connections: []Connection{{SystemID: "still_point"}, {SystemID: "lost"}}}
	lost := &System{ID: "lost"}
	kept, ids := placeFrontier([]*System{sol, ln, lost}, map[string]bool{"sol": true},
		map[string]frontier.Estimate{"long_night": {X: -6334, Y: 4615}})
	if len(kept) != 2 || kept[1].PositionX != -6334 || !ids["long_night"] || ids["sol"] {
		t.Fatalf("kept=%d ids=%v pos=%v", len(kept), ids, kept[1].PositionX)
	}
	if len(sol.Connections) != 0 || len(ln.Connections) != 1 {
		t.Errorf("connections to the dropped system survive: sol=%v ln=%v", sol.Connections, ln.Connections)
	}
	if kept, ids := placeFrontier([]*System{sol, lost}, nil, nil); len(kept) != 2 || len(ids) != 0 {
		t.Error("with no map nothing should change")
	}
}
