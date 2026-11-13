package main

import "time"

// activeWindow is how recently a player must have been seen to count as
// active.
const activeWindow = 30 * 24 * time.Hour

// activity renders a player's last sighting (RFC3339) as "Active recently"
// (seen within the last 30 days) or "Inactive". An exact age would change on
// every regen and churn thousands of player pages; this only changes when a
// player crosses the line. Unparseable or empty times return "—".
func activity(now time.Time, utc string) string {
	ts, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		return "—"
	}
	if now.Sub(ts) < activeWindow {
		return "Active recently"
	}
	return "Inactive"
}
