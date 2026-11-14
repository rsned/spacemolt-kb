package frontier

import (
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

func TestPlaceMapSystems(t *testing.T) {
	sol := &galaxymap.System{ID: "sol"}
	far := &galaxymap.System{ID: "fallow_span", PositionX: 685, PositionY: -8215}
	lost := &galaxymap.System{ID: "lost"}
	sol.Connections = []galaxymap.Connection{{SystemID: "fallow_span"}, {SystemID: "lost"}}
	byID := map[string]*galaxymap.System{"sol": sol, "fallow_span": far, "lost": lost}
	got := PlaceMapSystems([]*galaxymap.System{sol, far, lost}, byID, map[string]Estimate{"fallow_span": {X: 683, Y: -8312}})
	if len(got) != 2 || got[0] != sol || got[1] != far {
		t.Fatalf("kept = %v, want sol and fallow_span", got)
	}
	if far.PositionX != 683 || far.PositionY != -8312 {
		t.Errorf("fallow_span at (%v, %v), want the estimate (683, -8312)", far.PositionX, far.PositionY)
	}
	if _, ok := byID["lost"]; ok {
		t.Error("an unpositioned system should leave byID")
	}
	if len(sol.Connections) != 1 || sol.Connections[0].SystemID != "fallow_span" {
		t.Errorf("sol lanes = %v, want only fallow_span", sol.Connections)
	}
}
