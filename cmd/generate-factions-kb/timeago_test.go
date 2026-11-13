package main

import (
	"testing"
	"time"
)

func TestActivity(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct{ utc, want string }{
		{"", "—"},
		{"garbage", "—"},
		{"2026-11-13T11:00:00Z", "Active recently"},
		{"2026-10-15T12:00:00Z", "Active recently"}, // 29 days
		{"2026-10-14T12:00:00Z", "Inactive"},        // exactly 30 days
		{"2026-01-01T00:00:00Z", "Inactive"},
	}
	for _, c := range cases {
		if got := activity(now, c.utc); got != c.want {
			t.Errorf("activity(%q) = %q, want %q", c.utc, got, c.want)
		}
	}
}
