package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestLoadPlayersExcludesNonPlayers guards the realPlayerID predicate. The
// sighting scraper files every entity it sees into seen_players, so creature
// spawns, NPC authorities and named stations all share the table with real
// accounts. Only bare 32-hex IDs are players; creatures belong to kb/wildlife/.
func TestLoadPlayersExcludesNonPlayers(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err = db.Exec(`CREATE TABLE seen_players (
		player_id TEXT PRIMARY KEY, username TEXT NOT NULL, faction_id TEXT,
		faction_tag TEXT, clan_tag TEXT, primary_color TEXT, secondary_color TEXT,
		status_message TEXT, anonymous INTEGER NOT NULL DEFAULT 0,
		first_seen_utc TEXT NOT NULL, last_seen_utc TEXT NOT NULL,
		sighting_count INTEGER NOT NULL DEFAULT 1)`); err != nil {
		t.Fatal(err)
	}

	const hexA = "0123456789abcdef0123456789abcdef"
	const hexB = "fedcba9876543210fedcba9876543210"
	rows := []struct {
		id, username string
		want         bool
	}{
		{hexA, "Real Pilot", true},
		{hexB, "Another Pilot", true},
		{"crt_c33feecd9b437a4346ac5f0917a6408f", "Patina-Grazer", false}, // creature spawn
		{"npc_authority_nebula", "Nebula Trade Federation Authority", false},
		{"mera_sanctum_station", "Mera Sanctum Station", false},
		{"mobile_capital", "The Patchwork", false},
		{"0123456789ABCDEF0123456789ABCDEF", "Uppercase Hex", false}, // not the ID shape
		{"0123456789abcdef0123456789abcde", "Short Hex", false},      // 31 chars
		{hexA[:31] + "g", "Non-Hex Char", false},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT INTO seen_players
			(player_id, username, first_seen_utc, last_seen_utc)
			VALUES (?, ?, '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`,
			r.id, r.username); err != nil {
			t.Fatal(err)
		}
	}

	players, err := loadPlayers(db, nil, nil, nil)
	if err != nil {
		t.Fatalf("loadPlayers: %v", err)
	}
	got := map[string]bool{}
	for _, p := range players {
		got[p.ID] = true
	}
	for _, r := range rows {
		if got[r.id] != r.want {
			t.Errorf("player %q (%s): included = %v, want %v", r.id, r.username, got[r.id], r.want)
		}
	}
	if len(players) != 2 {
		t.Errorf("loaded %d players, want 2", len(players))
	}
}
