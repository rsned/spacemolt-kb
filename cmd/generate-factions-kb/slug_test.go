package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"STRG":                "strg",
		"End of Line":         "end-of-line",
		"Oberste Raumbehörde": "oberste-raumbehorde",
		"  Hex  Collective  ": "hex-collective",
		"[CUSTOMS]":           "customs",
		"!!!":                 "",
		"":                    "",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlayerSlug(t *testing.T) {
	got := playerSlug("Alice", "17a08149befb15b51a1fcf8bca325c36")
	want := "alice-17a08149"
	if got != want {
		t.Errorf("playerSlug = %q, want %q", got, want)
	}
	// Empty username falls back to the id8 prefix only.
	if got := playerSlug("!!!", "deadbeefcafef00d0000000000000000"); got != "deadbeef" {
		t.Errorf("playerSlug empty-name = %q, want %q", got, "deadbeef")
	}
}
