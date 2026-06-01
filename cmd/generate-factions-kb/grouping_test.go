package main

import "testing"

func TestGroupSightingsTiebreak(t *testing.T) {
	// Two sightings with identical LastSeenUTC but different SystemIDs should
	// be sorted by SystemID ascending ("alpha" before "zeta").
	in := []Sighting{
		{SystemID: "zeta", POIID: "p1", ShipClass: "Frigate", LastSeenUTC: "2026-05-31T10:00:00Z"},
		{SystemID: "alpha", POIID: "p2", ShipClass: "Hauler", LastSeenUTC: "2026-05-31T10:00:00Z"},
	}
	got := groupSightings(in)
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}
	if got[0].SystemID != "alpha" {
		t.Errorf("first group SystemID = %q, want %q", got[0].SystemID, "alpha")
	}
	if got[1].SystemID != "zeta" {
		t.Errorf("second group SystemID = %q, want %q", got[1].SystemID, "zeta")
	}
}

func TestGroupSightingsCombatOr(t *testing.T) {
	// When the first sighting has InCombat:true and the second has InCombat:false,
	// the merged group must still have InCombat == true.
	in := []Sighting{
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: true, LastSeenUTC: "2026-05-31T10:00:00Z"},
		{SystemID: "sol", POIID: "stationA", ShipClass: "Frigate", InCombat: false, LastSeenUTC: "2026-05-31T11:00:00Z"},
	}
	got := groupSightings(in)
	if len(got) != 1 {
		t.Fatalf("got %d groups, want 1", len(got))
	}
	if !got[0].InCombat {
		t.Errorf("merged group InCombat = false, want true")
	}
}

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
