package main

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
)

// loadShips returns, per player_id, the distinct ship classes sighted (sorted),
// and the full ShipSeen records per player for player pages.
func loadShips(db *sql.DB) (classes map[string][]string, detail map[string][]ShipSeen, err error) {
	rows, err := db.Query(`SELECT player_id, ship_class, first_seen_utc, last_seen_utc FROM seen_player_ships`)
	if err != nil {
		return nil, nil, fmt.Errorf("query seen_player_ships: %w", err)
	}
	defer func() { _ = rows.Close() }()
	classes = map[string][]string{}
	detail = map[string][]ShipSeen{}
	for rows.Next() {
		var pid string
		var class, first, last sql.NullString
		if err := rows.Scan(&pid, &class, &first, &last); err != nil {
			return nil, nil, fmt.Errorf("scan ship: %w", err)
		}
		classes[pid] = append(classes[pid], class.String)
		detail[pid] = append(detail[pid], ShipSeen{
			Class:        class.String,
			FirstSeenUTC: first.String,
			LastSeenUTC:  last.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate seen_player_ships: %w", err)
	}
	for pid := range classes {
		sort.Strings(classes[pid])
	}
	return classes, detail, nil
}

// loadSightings returns grouped sightings per player_id, resolving system slugs
// against knownSystemSlugs (system_id -> slug; "" means no page exists).
func loadSightings(db *sql.DB, knownSystemSlugs map[string]string) (map[string][]Sighting, error) {
	rows, err := db.Query(`SELECT player_id, system_id, poi_id, ship_class, in_combat, last_seen_utc FROM seen_player_sightings`)
	if err != nil {
		return nil, fmt.Errorf("query seen_player_sightings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	raw := map[string][]Sighting{}
	for rows.Next() {
		var pid, sysID string
		var poi, ship, last sql.NullString
		var combat sql.NullInt64
		if err := rows.Scan(&pid, &sysID, &poi, &ship, &combat, &last); err != nil {
			return nil, fmt.Errorf("scan sighting: %w", err)
		}
		raw[pid] = append(raw[pid], Sighting{
			SystemID:    sysID,
			SystemSlug:  knownSystemSlugs[sysID],
			POIID:       poi.String,
			ShipClass:   ship.String,
			InCombat:    combat.Int64 != 0,
			LastSeenUTC: last.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate seen_player_sightings: %w", err)
	}
	out := map[string][]Sighting{}
	for pid, list := range raw {
		out[pid] = groupSightings(list)
	}
	return out, nil
}

// loadPlayers loads all real players (NPC/station rows excluded), attaching
// their ships and grouped sightings. factionSlugByID maps faction_id -> slug
// for back-links ("" when untracked).
func loadPlayers(db *sql.DB, shipDetail map[string][]ShipSeen, sightings map[string][]Sighting, factionSlugByID map[string]string) ([]*Player, error) {
	rows, err := db.Query(`
		SELECT player_id, username, faction_id, faction_tag, clan_tag,
		       primary_color, secondary_color, status_message,
		       first_seen_utc, last_seen_utc
		FROM seen_players
		WHERE username NOT LIKE '[%' AND player_id NOT LIKE 'npc%'`)
	if err != nil {
		return nil, fmt.Errorf("query seen_players: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var players []*Player
	for rows.Next() {
		var pid string
		var uname, fid, ftag, clan, pc, sc, status, first, last sql.NullString
		if err := rows.Scan(&pid, &uname, &fid, &ftag, &clan, &pc, &sc, &status, &first, &last); err != nil {
			return nil, fmt.Errorf("scan player: %w", err)
		}
		p := &Player{
			ID:             pid,
			Username:       uname.String,
			Slug:           playerSlug(uname.String, pid),
			FactionID:      fid.String,
			FactionTag:     ftag.String,
			FactionSlug:    factionSlugByID[fid.String],
			ClanTag:        clan.String,
			PrimaryColor:   pc.String,
			SecondaryColor: sc.String,
			StatusMessage:  status.String,
			FirstSeenUTC:   first.String,
			LastSeenUTC:    last.String,
			Ships:          shipDetail[pid],
			Sightings:      sightings[pid],
		}
		sort.SliceStable(p.Ships, func(i, j int) bool { return p.Ships[i].Class < p.Ships[j].Class })
		players = append(players, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(players, func(i, j int) bool {
		if players[i].LastSeenUTC != players[j].LastSeenUTC {
			return players[i].LastSeenUTC > players[j].LastSeenUTC
		}
		return players[i].Username < players[j].Username
	})
	return players, nil
}

// knownSystemSlugs scans kb/systems/ for existing per-system page directories so
// sighting rows can link to them. The KB writes systems as kb/systems/<id>/.
func knownSystemSlugs(systemsDir string) map[string]string {
	out := map[string]string{}
	entries, err := os.ReadDir(systemsDir)
	if err != nil {
		return out // no systems dir -> no links, not an error
	}
	for _, e := range entries {
		if e.IsDir() {
			out[e.Name()] = e.Name()
		}
	}
	return out
}
