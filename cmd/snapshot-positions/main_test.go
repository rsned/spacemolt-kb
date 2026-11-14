package main

import "testing"

func TestRecordNeverErasesWithZero(t *testing.T) {
	snap := map[string]*Entry{}
	if !record(snap, "deep_watch", "Deep Watch", "measured", -3400, 6100, "", "2026-11-01") {
		t.Fatal("first reading not recorded")
	}
	// Visited: the DB now says api 0,0. The measured value must survive.
	if record(snap, "deep_watch", "Deep Watch", "api", 0, 0, "", "2026-11-14") {
		t.Error("0,0 placeholder recorded")
	}
	r := snap["deep_watch"].Sources["measured"]
	if r == nil || r.X != -3400 || r.LastSeen != "2026-11-01" {
		t.Errorf("measured reading = %+v", r)
	}
	if _, ok := snap["deep_watch"].Sources["api"]; ok {
		t.Error("api source created from 0,0")
	}
}

func TestRecordUpdatesDatesAndSupersedes(t *testing.T) {
	snap := map[string]*Entry{}
	record(snap, "elbow", "Elbow", "fitted", 1976, 8405, "a", "2026-11-13")
	if record(snap, "elbow", "Elbow", "fitted", 1976.2, 8405, "a", "2026-11-14") {
		t.Error("same reading reported as a change")
	}
	r := snap["elbow"].Sources["fitted"]
	if r.FirstSeen != "2026-11-13" || r.LastSeen != "2026-11-14" {
		t.Errorf("dates = %s..%s", r.FirstSeen, r.LastSeen)
	}
	if !record(snap, "elbow", "Elbow", "fitted", 1980, 8400, "b", "2026-11-15") || snap["elbow"].Sources["fitted"].X != 1980 {
		t.Error("new fit did not supersede")
	}
}

func TestRecordKeepsSolAtOrigin(t *testing.T) {
	snap := map[string]*Entry{}
	if !record(snap, "sol", "Sol", "api", 0, 0, "", "2026-11-14") {
		t.Error("Sol at origin dropped")
	}
}
