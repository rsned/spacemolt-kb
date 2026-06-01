package main

import "testing"

func TestGroupSightings(t *testing.T) {
	in := []Sighting{
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: false, LastSeenUTC: "2026-05-31T10:00:00Z"},
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: true, LastSeenUTC: "2026-05-31T12:00:00Z"},
		{SystemID: "vega", POIID: "", ShipClass: "Hauler", InCombat: false, LastSeenUTC: "2026-05-30T09:00:00Z"},
	}
	got := groupSightings(in)
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	// Groups are sorted by LastSeenUTC desc; sol/Frigate is most recent.
	if got[0].SystemID != "sol" || !got[0].InCombat || got[0].LastSeenUTC != "2026-05-31T12:00:00Z" {
		t.Errorf("first group = %+v; want sol/Frigate combat=true latest 12:00", got[0])
	}
	if got[1].SystemID != "vega" {
		t.Errorf("second group = %+v; want vega", got[1])
	}
}
