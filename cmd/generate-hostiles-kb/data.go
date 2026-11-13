package main

import (
	"cmp"
	"database/sql"
	"slices"
	"strings"
)

// HostileType is one hostile class (a repeater Monolith, a pirate class, ...).
type HostileType struct {
	ID         string // hostile_type, e.g. repeater_monolith
	Kind       string // repeater | pirate
	Class      string
	Name       string
	MaxHull    int // 0 = not captured yet
	MaxShield  int
	Sightings  int // sighting rows
	Units      int // distinct units seen (ids change each appearance)
	Systems    []SystemSighting
	Weapons    []Weapon
	Battles    int
	HardestHit float64
	FirstSeen  string
	LastSeen   string
}

// Slug is the section anchor.
func (h HostileType) Slug() string { return strings.ReplaceAll(h.ID, "_", "-") }

// Threat orders the overview: hull + shield when captured.
func (h HostileType) Threat() int { return h.MaxHull + h.MaxShield }

// SystemSighting is one type's record in one system.
type SystemSighting struct {
	SystemID, SystemName string
	Units                int // distinct units seen there
	MaxAtOnce            int // most units of this type seen at one tick
	Sightings            int
	InBattle             int // sightings while fighting
	FirstSeen, LastSeen  string
}

// BattlePct is the share of sightings taken mid-battle.
func (s SystemSighting) BattlePct() int {
	if s.Sightings == 0 {
		return 0
	}
	return 100 * s.InBattle / s.Sightings
}

// Weapon is one weapon (or effect) a type fires, summed over battles.
type Weapon struct {
	Name, DamageType, ShotKind string
	Shots, Hits, Battles       int
	Min, Max, Total            float64
}

// HitPct is hits per shot, as a percentage.
func (w Weapon) HitPct() int {
	if w.Shots == 0 {
		return 0
	}
	return 100 * w.Hits / w.Shots
}

// kindOf infers a type's kind from its id when hostile_types has no row.
func kindOf(id string) string {
	if strings.HasPrefix(id, "pirate") {
		return "pirate"
	}
	return "repeater"
}

// nameOf builds a display name from a type id: repeater_monolith → Repeater Monolith.
func nameOf(id string) string {
	words := strings.Fields(strings.ReplaceAll(id, "_", " "))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// loadTypes gathers every hostile type known from any of the three tables,
// with its sightings by system and its weapons.
func loadTypes(db *sql.DB) ([]*HostileType, error) {
	types := map[string]*HostileType{}
	get := func(id string) *HostileType {
		t, ok := types[id]
		if !ok {
			t = &HostileType{ID: id, Kind: kindOf(id), Name: nameOf(id)}
			types[id] = t
		}
		return t
	}

	rows, err := db.Query(`SELECT hostile_type, kind, class, name, max_hull, max_shield, first_seen_utc, last_seen_utc FROM hostile_types`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, kind, class, name, first, last string
		var hull, shield int
		if err := rows.Scan(&id, &kind, &class, &name, &hull, &shield, &first, &last); err != nil {
			return nil, err
		}
		t := get(id)
		if kind != "" {
			t.Kind = kind
		}
		if name != "" {
			t.Name = name
		}
		t.Class, t.MaxHull, t.MaxShield, t.FirstSeen, t.LastSeen = class, hull, shield, first, last
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	names := map[string]string{}
	if rows, err = db.Query(`SELECT id, name FROM systems`); err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, n string
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		names[id] = n
	}
	_ = rows.Close()

	// Sightings by type and system. Unit ids change each appearance, so
	// "at once" is the most distinct ids recorded at a single tick.
	rows, err = db.Query(`
		SELECT hostile_type, system_id, COUNT(*), COUNT(DISTINCT hostile_id), SUM(in_battle),
		       MIN(observed_utc), MAX(observed_utc),
		       (SELECT MAX(n) FROM (SELECT COUNT(DISTINCT s2.hostile_id) n FROM hostile_sightings s2
		         WHERE s2.hostile_type = s.hostile_type AND s2.system_id = s.system_id GROUP BY s2.game_tick))
		FROM hostile_sightings s GROUP BY hostile_type, system_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var s SystemSighting
		var atOnce sql.NullInt64
		if err := rows.Scan(&id, &s.SystemID, &s.Sightings, &s.Units, &s.InBattle, &s.FirstSeen, &s.LastSeen, &atOnce); err != nil {
			return nil, err
		}
		s.MaxAtOnce = int(atOnce.Int64)
		s.SystemName = names[s.SystemID]
		if s.SystemName == "" {
			s.SystemName = s.SystemID
		}
		t := get(id)
		t.Systems = append(t.Systems, s)
		t.Sightings += s.Sightings
		t.Units += s.Units
		if t.FirstSeen == "" || (s.FirstSeen != "" && s.FirstSeen < t.FirstSeen) {
			t.FirstSeen = s.FirstSeen
		}
		t.LastSeen = max(t.LastSeen, s.LastSeen)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	rows, err = db.Query(`
		SELECT hostile_type, weapon_name, damage_type, shot_kind, SUM(shots), SUM(hits),
		       MIN(damage_min), MAX(damage_max), SUM(damage_total), COUNT(DISTINCT battle_id)
		FROM hostile_attacks GROUP BY hostile_type, weapon_name, damage_type, shot_kind`)
	if err != nil {
		return nil, err
	}
	battles := map[string]map[string]bool{}
	for rows.Next() {
		var id string
		var w Weapon
		if err := rows.Scan(&id, &w.Name, &w.DamageType, &w.ShotKind, &w.Shots, &w.Hits, &w.Min, &w.Max, &w.Total, &w.Battles); err != nil {
			return nil, err
		}
		t := get(id)
		t.Weapons = append(t.Weapons, w)
		t.HardestHit = max(t.HardestHit, w.Max)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if rows, err = db.Query(`SELECT DISTINCT hostile_type, battle_id FROM hostile_attacks`); err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, b string
		if err := rows.Scan(&id, &b); err != nil {
			return nil, err
		}
		if battles[id] == nil {
			battles[id] = map[string]bool{}
		}
		battles[id][b] = true
	}
	_ = rows.Close()

	out := make([]*HostileType, 0, len(types))
	for id, t := range types {
		t.Battles = len(battles[id])
		slices.SortFunc(t.Systems, func(a, b SystemSighting) int {
			return cmp.Or(cmp.Compare(b.Sightings, a.Sightings), cmp.Compare(a.SystemName, b.SystemName))
		})
		slices.SortFunc(t.Weapons, func(a, b Weapon) int {
			return cmp.Or(cmp.Compare(b.Total, a.Total), cmp.Compare(a.Name, b.Name))
		})
		out = append(out, t)
	}
	sortByThreat(out)
	return out, nil
}

// sortByThreat puts the most dangerous first: hull + shield when captured,
// then the hardest hit seen, then name.
func sortByThreat(ts []*HostileType) {
	slices.SortFunc(ts, func(a, b *HostileType) int {
		return cmp.Or(cmp.Compare(b.Threat(), a.Threat()), cmp.Compare(b.HardestHit, a.HardestHit), cmp.Compare(a.Name, b.Name))
	})
}

// ofKind filters types by kind, keeping order.
func ofKind(ts []*HostileType, kind string) []*HostileType {
	var out []*HostileType
	for _, t := range ts {
		if t.Kind == kind {
			out = append(out, t)
		}
	}
	return out
}
