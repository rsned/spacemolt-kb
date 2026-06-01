package main

import (
	"fmt"
	"time"
)

// relativeTime renders utc (RFC3339) as a short human delta from now:
// "just now", "30m ago", "2h ago", "2d ago". Future or unparseable times
// return "just now" / "—" respectively so templates never error.
func relativeTime(now time.Time, utc string) string {
	if utc == "" {
		return "—"
	}
	ts, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		return "—"
	}
	d := now.Sub(ts)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
}
