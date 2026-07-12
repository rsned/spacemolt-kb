package main

import "testing"

func TestFormatEmpire(t *testing.T) {
	cases := []struct {
		name       string
		empire     string
		tick       int
		police     int
		stronghold bool
		want       string
	}{
		{"unknown when never seen", "solarian", 0, 5, false, "Unknown"},
		{"neutral when empire empty", "", 100, 5, false, "Neutral"},
		{"titlecased empire under police", "solarian", 100, 5, false, "Solarian"},
		{"lawless drifting empire reads independent", "crimson", 100, 0, false, "Independent"},
		{"stronghold with no police is not lawless", "nebula", 100, 0, true, "Nebula"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatEmpire(tc.empire, tc.tick, tc.police, tc.stronghold)
			if got != tc.want {
				t.Fatalf("formatEmpire(%q,%d,%d,%v) = %q, want %q",
					tc.empire, tc.tick, tc.police, tc.stronghold, got, tc.want)
			}
		})
	}
}
