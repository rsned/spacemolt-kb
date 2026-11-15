package main

import "testing"

func TestFitOfMarksOnlyFitMembers(t *testing.T) {
	a := Reading{Obs: "brushfield", S: Sighting{Bearing: 160, Brightness: 6, Temperature: 60000}, Status: "unknown"}
	b := Reading{Obs: "clear_burn", S: Sighting{Bearing: 305, Brightness: 8, Temperature: 60000}, Status: "unknown"}
	c := Reading{Obs: "clear_burn", S: Sighting{Bearing: 20, Brightness: 2, Temperature: 20000}, Status: "unknown"}
	// Same observer and bearing as a, different temperature: not a member.
	d := Reading{Obs: "brushfield", S: Sighting{Bearing: 160, Brightness: 6, Temperature: 4000}, Status: "unknown"}
	fits := []Fit{{Temp: 9000}, {Temp: 60000, Members: []Reading{a, b}}}
	got := fitOf([]Reading{a, b, c, d}, fits)
	if got[0] != 2 || got[1] != 2 || len(got) != 2 {
		t.Errorf("fitOf = %v, want readings 0 and 1 in fit 2 only", got)
	}
}
