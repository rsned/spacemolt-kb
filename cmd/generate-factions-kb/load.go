package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

// loadFactions loads every faction with its reconstructed roster (from
// seen_players, overlaid with official faction_members), bases, relations, and
// facilities. shipsByPlayer maps player_id -> distinct ship class names.
func loadFactions(db *sql.DB, shipsByPlayer map[string][]string) ([]*Faction, error) {
	rows, err := db.Query(`
		SELECT faction_id, name, tag, leader_username, treasury, member_count,
		       owned_bases, description, charter, emblem,
		       primary_color, secondary_color, founded_utc
		FROM factions`)
	if err != nil {
		return nil, fmt.Errorf("query factions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var factions []*Faction
	seenSlug := map[string]string{} // slug -> faction_id, to detect collisions
	for rows.Next() {
		f := &Faction{}
		var name, tag, leader, desc, charter, emblem, pc, sc, founded sql.NullString
		var treasury, memberCount, bases sql.NullInt64
		if err := rows.Scan(&f.ID, &name, &tag, &leader, &treasury, &memberCount,
			&bases, &desc, &charter, &emblem, &pc, &sc, &founded); err != nil {
			return nil, fmt.Errorf("scan faction: %w", err)
		}
		f.Name = name.String
		f.Tag = tag.String
		f.LeaderName = leader.String
		f.Treasury = treasury.Int64
		f.OfficialMemberCount = int(memberCount.Int64)
		f.OwnedBases = int(bases.Int64)
		f.Description = desc.String
		f.Charter = charter.String
		f.Emblem = emblem.String
		f.PrimaryColor = pc.String
		f.SecondaryColor = sc.String
		f.FoundedUTC = founded.String

		// URL path is the lowercase, underscore-normalized tag; fall back to the
		// faction_id when the tag is empty or collides after normalization.
		slug := factionSlug(f.Tag, f.ID)
		if other, dup := seenSlug[slug]; dup {
			log.Printf("warning: faction tag slug %q (%s) collides with %s; using id path", slug, f.ID, other)
			slug = f.ID
		}
		seenSlug[slug] = f.ID
		f.Slug = slug

		factions = append(factions, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate factions: %w", err)
	}

	byID := map[string]*Faction{}
	for _, f := range factions {
		byID[f.ID] = f
	}

	if err := loadRosters(db, byID, shipsByPlayer); err != nil {
		return nil, err
	}
	if err := loadBases(db, byID); err != nil {
		return nil, err
	}
	if err := loadRelations(db, byID); err != nil {
		return nil, err
	}
	if err := loadFacilities(db, byID); err != nil {
		return nil, err
	}

	// Roster ordering: alphabetical by username (case-insensitive).
	for _, f := range factions {
		sort.SliceStable(f.Members, func(i, j int) bool {
			return strings.ToLower(f.Members[i].Username) < strings.ToLower(f.Members[j].Username)
		})
	}
	// Faction ordering: largest reconstructed roster first.
	sort.SliceStable(factions, func(i, j int) bool {
		if factions[i].MemberCount() != factions[j].MemberCount() {
			return factions[i].MemberCount() > factions[j].MemberCount()
		}
		return factions[i].Name < factions[j].Name
	})
	return factions, nil
}

// loadRosters fills each faction's Members from seen_players, overlaying role /
// online state from faction_members where present.
func loadRosters(db *sql.DB, byID map[string]*Faction, shipsByPlayer map[string][]string) error {
	// Official overlay: (faction_id, player_id) -> (role, online).
	type overlay struct {
		role   string
		online bool
	}
	ov := map[string]overlay{} // key: factionID + "|" + playerID
	orows, err := db.Query(`SELECT faction_id, player_id, role, is_online FROM faction_members`)
	if err != nil {
		return fmt.Errorf("query faction_members: %w", err)
	}
	for orows.Next() {
		var fid, pid string
		var role sql.NullString
		var online sql.NullInt64
		if err := orows.Scan(&fid, &pid, &role, &online); err != nil {
			_ = orows.Close()
			return fmt.Errorf("scan faction_members: %w", err)
		}
		ov[fid+"|"+pid] = overlay{role: role.String, online: online.Int64 != 0}
	}
	_ = orows.Close()
	if err := orows.Err(); err != nil {
		return fmt.Errorf("iterate faction_members: %w", err)
	}

	rows, err := db.Query(`
		SELECT faction_id, player_id, username, last_seen_utc
		FROM seen_players
		WHERE faction_id IS NOT NULL AND faction_id != '' AND username NOT LIKE '[%' AND player_id NOT LIKE 'npc%'`)
	if err != nil {
		return fmt.Errorf("query seen_players rosters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid, pid string
		var uname, lastSeen sql.NullString
		if err := rows.Scan(&fid, &pid, &uname, &lastSeen); err != nil {
			return fmt.Errorf("scan roster: %w", err)
		}
		f, ok := byID[fid]
		if !ok {
			continue // sighted faction we have no factions-table row for
		}
		m := &Member{
			PlayerID:    pid,
			Username:    uname.String,
			Slug:        pid,
			LastSeenUTC: lastSeen.String,
			Ships:       shipsByPlayer[pid],
		}
		if o, ok := ov[fid+"|"+pid]; ok {
			m.Role = o.role
			m.IsOnline = o.online
		}
		f.Members = append(f.Members, m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate seen_players: %w", err)
	}
	// Link each faction's leader to their player page when the leader appears in
	// the roster (which guarantees a generated page exists to link to).
	for _, f := range byID {
		if f.LeaderName == "" {
			continue
		}
		for _, m := range f.Members {
			if m.Username == f.LeaderName {
				f.LeaderSlug = m.Slug
				break
			}
		}
	}
	return nil
}

func loadBases(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, base_name, system_name, services_json FROM faction_bases`)
	if err != nil {
		return fmt.Errorf("query faction_bases: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var name, sys, svc sql.NullString
		if err := rows.Scan(&fid, &name, &sys, &svc); err != nil {
			return fmt.Errorf("scan base: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Bases = append(f.Bases, Base{
				Name:       name.String,
				SystemName: sys.String,
				Services:   parseServices(svc.String),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate faction_bases: %w", err)
	}
	return nil
}

func loadRelations(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, kind, target_name, target_tag, reason, our_kills, their_kills FROM faction_relations`)
	if err != nil {
		return fmt.Errorf("query faction_relations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var kind, tname, ttag, reason sql.NullString
		var ours, theirs sql.NullInt64
		if err := rows.Scan(&fid, &kind, &tname, &ttag, &reason, &ours, &theirs); err != nil {
			return fmt.Errorf("scan relation: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Relations = append(f.Relations, Relation{
				Kind:       kind.String,
				TargetName: tname.String,
				TargetTag:  ttag.String,
				Reason:     reason.String,
				OurKills:   int(ours.Int64),
				TheirKills: int(theirs.Int64),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate faction_relations: %w", err)
	}
	return nil
}

func loadFacilities(db *sql.DB, byID map[string]*Faction) error {
	rows, err := db.Query(`SELECT faction_id, facility_type, category, level, status FROM faction_facilities`)
	if err != nil {
		return fmt.Errorf("query faction_facilities: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var fid string
		var ftype, cat, status sql.NullString
		var level sql.NullInt64
		if err := rows.Scan(&fid, &ftype, &cat, &level, &status); err != nil {
			return fmt.Errorf("scan facility: %w", err)
		}
		if f, ok := byID[fid]; ok {
			f.Facilities = append(f.Facilities, Facility{
				Type:     ftype.String,
				Category: cat.String,
				Level:    int(level.Int64),
				Status:   status.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate faction_facilities: %w", err)
	}
	return nil
}

// parseServices renders services_json best-effort. Accepts a JSON array of
// strings, a JSON object (uses its keys), or falls back to the raw trimmed
// string. Never errors.
func parseServices(raw string) string {
	if raw == "" {
		return ""
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return strings.Join(arr, ", ")
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return strings.Join(keys, ", ")
	}
	return strings.TrimSpace(raw)
}
