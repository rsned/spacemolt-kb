package main

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		utc  string
		want string
	}{
		{"2026-05-31T11:59:30Z", "just now"},
		{"2026-05-31T11:30:00Z", "30m ago"},
		{"2026-05-31T10:00:00Z", "2h ago"},
		{"2026-05-29T12:00:00Z", "2d ago"},
		{"2026-05-31T12:30:00Z", "just now"}, // future clamps to "just now"
		{"", "—"},
		{"not-a-date", "—"},
	}
	for _, c := range cases {
		if got := relativeTime(now, c.utc); got != c.want {
			t.Errorf("relativeTime(%q) = %q, want %q", c.utc, got, c.want)
		}
	}
}
