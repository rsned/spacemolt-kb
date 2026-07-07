package main

import "testing"

func TestFmtCompact(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{999, "999"},
		{999.5, "1K"},
		{1500, "2K"},
		{999_500, "1.0M"},
		{999_999, "1.0M"},
		{12_300_000, "12.3M"},
		{999_999_999, "1.0B"},
		{-57_700_000, "-57.7M"},
	}
	for _, c := range cases {
		if got := fmtCompact(c.in); got != c.want {
			t.Errorf("fmtCompact(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFmtPct(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{50, "+50.0%"},
		{-3.14, "-3.1%"},
		{0, "+0.0%"},
	}
	for _, c := range cases {
		if got := fmtPct(c.in); got != c.want {
			t.Errorf("fmtPct(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
