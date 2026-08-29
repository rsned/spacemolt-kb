package wildlife

import (
	"cmp"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// Load reads the field guide from the knowledge database.
func Load(db *sql.DB) (*Guide, error) {
	g := &Guide{}
	systems, byID, err := loadMapSystems(db)
	if err != nil {
		return nil, fmt.Errorf("systems: %w", err)
	}
	g.MapSystems, g.MapByID = systems, byID
	g.Coverage.TotalSystems = len(systems)
	sysName := func(id string) string {
		if s, ok := byID[id]; ok {
			return s.Name
		}
		return id
	}
	pois, err := loadPOIs(db)
	if err != nil {
		return nil, fmt.Errorf("pois: %w", err)
	}

	if g.Species, err = loadSpecies(db); err != nil {
		return nil, fmt.Errorf("species: %w", err)
	}
	index := make(map[string]*Species, len(g.Species))
	for i := range g.Species {
		index[g.Species[i].ID] = &g.Species[i]
	}

	if err := loadPlaces(db, index, sysName, pois); err != nil {
		return nil, fmt.Errorf("sightings: %w", err)
	}
	if err := loadAttacks(db, index); err != nil {
		return nil, fmt.Errorf("attacks: %w", err)
	}
	if err := loadKills(db, index, sysName, pois); err != nil {
		return nil, fmt.Errorf("kills: %w", err)
	}
	if err := loadCoverage(db, &g.Coverage); err != nil {
		return nil, fmt.Errorf("surveys: %w", err)
	}
	withWildlife := make(map[string]bool)
	for _, s := range g.Species {
		for _, p := range s.Places {
			withWildlife[p.SystemID] = true
		}
	}
	g.Coverage.SystemsWithWildlife = len(withWildlife)
	return g, nil
}

type poiInfo struct {
	SystemID, Name, Type string
}

func loadPOIs(db *sql.DB) (map[string]poiInfo, error) {
	rows, err := db.Query(`SELECT id, system_id, name, type FROM pois`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]poiInfo)
	for rows.Next() {
		var id string
		var p poiInfo
		if err := rows.Scan(&id, &p.SystemID, &p.Name, &p.Type); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

func loadSpecies(db *sql.DB) ([]Species, error) {
	rows, err := db.Query(`
		SELECT species, name, role, max_hull, max_shield, danger, habitats, ranchable,
		       scan_traits, scan_revealed, first_seen_utc, last_seen_utc
		FROM wildlife_species`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Species
	for rows.Next() {
		var s Species
		var habitats string
		if err := rows.Scan(&s.ID, &s.Name, &s.Role, &s.MaxHull, &s.MaxShield, &s.Danger, &habitats, &s.Ranchable,
			&s.ScanTraits, &s.ScanRevealed, &s.FirstSeen, &s.LastSeen); err != nil {
			return nil, err
		}
		if s.Name == "" {
			s.Name = s.ID
		}
		for _, h := range strings.Split(habitats, ",") {
			if h = strings.TrimSpace(h); h != "" {
				s.Habitats = append(s.Habitats, h)
			}
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b Species) int {
		return cmp.Or(cmp.Compare(RoleRank(a.Role), RoleRank(b.Role)), cmp.Compare(a.Name, b.Name))
	})
	return out, nil
}

// sighting is one raw ledger row.
type sighting struct {
	Species, SystemID, POIID, Source string
	Count                            int
	Abundance, Bloom                 string
	Ranched, Branded, InCombat       bool
	Tick                             int
	Observed                         string
}

func (s sighting) newerThan(o sighting) bool {
	if s.Tick != o.Tick {
		return s.Tick > o.Tick
	}
	return s.Observed > o.Observed
}

// loadPlaces folds the sighting ledger into one Place per species/system:
// the latest system-level survey supplies the estimate and abundance, the
// latest row per POI supplies the breakdown. A system with only POI-level
// sightings is estimated as the sum of its latest POI counts.
func loadPlaces(db *sql.DB, index map[string]*Species, sysName func(string) string, pois map[string]poiInfo) error {
	rows, err := db.Query(`
		SELECT species, system_id, poi_id, source, observed_count, abundance,
		       ranched, branded, in_combat, bloom_status, game_tick, observed_utc
		FROM wildlife_sightings WHERE system_id <> ''`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type key struct{ species, system string }
	type acc struct {
		system   *sighting            // latest system-level row
		pois     map[string]*sighting // latest row per POI
		count    int
		lastTick int
		lastSeen string
		ranched  bool
		branded  bool
		combat   bool
	}
	accs := make(map[key]*acc)
	for rows.Next() {
		var s sighting
		if err := rows.Scan(&s.Species, &s.SystemID, &s.POIID, &s.Source, &s.Count, &s.Abundance,
			&s.Ranched, &s.Branded, &s.InCombat, &s.Bloom, &s.Tick, &s.Observed); err != nil {
			return err
		}
		if _, ok := index[s.Species]; !ok {
			continue
		}
		k := key{s.Species, s.SystemID}
		a, ok := accs[k]
		if !ok {
			a = &acc{pois: make(map[string]*sighting)}
			accs[k] = a
		}
		a.count++
		if s.Tick > a.lastTick || (s.Tick == a.lastTick && s.Observed > a.lastSeen) {
			a.lastTick, a.lastSeen = s.Tick, s.Observed
		}
		a.ranched = a.ranched || s.Ranched
		a.branded = a.branded || s.Branded
		a.combat = a.combat || s.InCombat
		row := s
		if s.POIID == "" {
			if a.system == nil || row.newerThan(*a.system) {
				a.system = &row
			}
			continue
		}
		if cur := a.pois[s.POIID]; cur == nil || row.newerThan(*cur) {
			a.pois[s.POIID] = &row
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for k, a := range accs {
		p := Place{
			SystemID: k.system, SystemName: sysName(k.system),
			Ranched: a.ranched, Branded: a.branded, InCombat: a.combat,
			LastTick: a.lastTick, LastSeen: a.lastSeen, Sightings: a.count,
		}
		for id, s := range a.pois {
			info := pois[id]
			name := info.Name
			if name == "" {
				name = id
			}
			p.POIs = append(p.POIs, POISighting{ID: id, Name: name, Type: info.Type, Count: s.Count, Bloom: s.Bloom, LastTick: s.Tick})
			if s.Bloom != "" && p.Bloom == "" {
				p.Bloom = s.Bloom
			}
		}
		slices.SortFunc(p.POIs, func(x, y POISighting) int {
			return cmp.Or(cmp.Compare(y.Count, x.Count), cmp.Compare(x.Name, y.Name))
		})
		if a.system != nil {
			p.Count = a.system.Count
			p.Abundance = a.system.Abundance
			if a.system.Bloom != "" && a.system.Bloom != "dormant" || p.Bloom == "" {
				p.Bloom = a.system.Bloom
			}
		} else {
			for _, poi := range p.POIs {
				p.Count += poi.Count
			}
		}
		if p.Bloom == "dormant" {
			p.Bloom = ""
		}
		sp := index[k.species]
		sp.Places = append(sp.Places, p)
	}
	for _, sp := range index {
		slices.SortFunc(sp.Places, func(a, b Place) int {
			return cmp.Or(cmp.Compare(a.SystemName, b.SystemName), cmp.Compare(a.SystemID, b.SystemID))
		})
	}
	return nil
}

func loadAttacks(db *sql.DB, index map[string]*Species) error {
	rows, err := db.Query(`
		SELECT species, weapon_name, damage_type, shot_kind,
		       COUNT(DISTINCT battle_id), SUM(shots), SUM(hits), SUM(damage_total),
		       MIN(CASE WHEN damage_min > 0 THEN damage_min END), MAX(damage_max), MAX(observed_utc)
		FROM wildlife_attacks
		GROUP BY species, weapon_name, damage_type, shot_kind
		ORDER BY species, SUM(damage_total) DESC, weapon_name`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var species string
		var a Attack
		var dmin sql.NullFloat64
		if err := rows.Scan(&species, &a.Weapon, &a.DamageType, &a.ShotKind, &a.Battles, &a.Shots, &a.Hits,
			&a.DamageTotal, &dmin, &a.DamageMax, &a.LastSeen); err != nil {
			return err
		}
		a.DamageMin = dmin.Float64
		if sp, ok := index[species]; ok {
			sp.Attacks = append(sp.Attacks, a)
		}
	}
	return rows.Err()
}

func loadKills(db *sql.DB, index map[string]*Species, sysName func(string) string, pois map[string]poiInfo) error {
	rows, err := db.Query(`
		SELECT creature_id, game_tick, species, system_id, poi_id, duration_ticks,
		       damage_dealt, damage_taken, salvage_value, killed_utc
		FROM wildlife_kills ORDER BY killed_utc DESC, game_tick DESC`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type killKey struct {
		creature string
		tick     int
	}
	kills := make(map[killKey]*Kill)
	var order []killKey
	for rows.Next() {
		var k Kill
		var species string
		var tick int
		if err := rows.Scan(&k.CreatureID, &tick, &species, &k.SystemID, &k.POIID, &k.DurationTicks,
			&k.DamageDealt, &k.DamageTaken, &k.SalvageValue, &k.KilledAt); err != nil {
			return err
		}
		sp, ok := index[species]
		if !ok {
			continue
		}
		k.SystemName = sysName(k.SystemID)
		if info, ok := pois[k.POIID]; ok {
			k.POIName = info.Name
		} else {
			k.POIName = k.POIID
		}
		sp.Kills = append(sp.Kills, k)
		kk := killKey{k.CreatureID, tick}
		kills[kk] = &sp.Kills[len(sp.Kills)-1]
		order = append(order, kk)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(order) == 0 {
		return nil
	}
	drows, err := db.Query(`
		SELECT d.creature_id, d.game_tick, d.item_id, COALESCE(i.name, d.item_id), COALESCE(i.category, ''), d.quantity
		FROM wildlife_kill_drops d LEFT JOIN items i ON i.id = d.item_id
		ORDER BY d.quantity DESC, d.item_id`)
	if err != nil {
		return err
	}
	defer func() { _ = drows.Close() }()
	for drows.Next() {
		var kk killKey
		var d Drop
		if err := drows.Scan(&kk.creature, &kk.tick, &d.ItemID, &d.ItemName, &d.ItemCategory, &d.Quantity); err != nil {
			return err
		}
		if k, ok := kills[kk]; ok {
			k.Drops = append(k.Drops, d)
		}
	}
	return drows.Err()
}

func loadCoverage(db *sql.DB, c *Coverage) error {
	return db.QueryRow(`
		SELECT COUNT(DISTINCT system_id), COUNT(DISTINCT system_id || '|' || poi_id)
		FROM wildlife_surveys WHERE system_id <> ''`).Scan(&c.SystemsSurveyed, &c.PlacesSurveyed)
}

// loadMapSystems loads system geometry and jump connections for the galaxy map.
func loadMapSystems(db *sql.DB) ([]*galaxymap.System, map[string]*galaxymap.System, error) {
	rows, err := db.Query(`
		SELECT id, name, position_x, position_y, police_level,
		       COALESCE(empire, ''), is_stronghold, last_updated_tick
		FROM systems ORDER BY name`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var systems []*galaxymap.System
	byID := make(map[string]*galaxymap.System)
	for rows.Next() {
		var s galaxymap.System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY,
			&s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, nil, err
		}
		if s.ID == "" {
			continue
		}
		systems = append(systems, &s)
		byID[s.ID] = &s
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	connRows, err := db.Query(`SELECT from_system, to_system, distance FROM connections`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = connRows.Close() }()
	for connRows.Next() {
		var fromID, toID string
		var distance int
		if err := connRows.Scan(&fromID, &toID, &distance); err != nil {
			return nil, nil, err
		}
		from, ok := byID[fromID]
		if !ok {
			continue
		}
		name := toID
		if to, ok := byID[toID]; ok {
			name = to.Name
		}
		from.Connections = append(from.Connections, galaxymap.Connection{SystemID: toID, Name: name, Distance: distance})
	}
	return systems, byID, connRows.Err()
}
