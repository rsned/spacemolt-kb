package wildlife

import (
	"cmp"
	"database/sql"
	"slices"
)

// tableExists reports whether the knowledge DB has a table with this name,
// without erroring when it doesn't — older knowledge DBs predate some
// optional ledgers.
func tableExists(db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// loadStages groups wildlife_observations into per-species, per-stage hull
// statistics: each creature's max_hull at its latest sighting (highest
// game_tick, ties broken by observed_utc), grouped by stage. The table was
// added once species started reporting life stages (capture began
// 2026-10-14); it is entirely optional, so a knowledge DB without it leaves
// every species with no stage data.
func loadStages(db *sql.DB, index map[string]*Species) error {
	ok, err := tableExists(db, "wildlife_observations")
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	rows, err := db.Query(`
		SELECT creature_id, species, stage, max_hull, game_tick, observed_utc
		FROM wildlife_observations`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type latest struct {
		stage    string
		hull     int
		tick     int
		observed string
	}
	type creatureKey struct{ species, creature string }
	bestByCreature := make(map[creatureKey]latest)
	minObserved := make(map[string]string) // species -> earliest observed_utc across every row

	for rows.Next() {
		var ck creatureKey
		var l latest
		if err := rows.Scan(&ck.creature, &ck.species, &l.stage, &l.hull, &l.tick, &l.observed); err != nil {
			return err
		}
		if _, ok := index[ck.species]; !ok {
			continue
		}
		if cur, seen := minObserved[ck.species]; !seen || l.observed < cur {
			minObserved[ck.species] = l.observed
		}
		cur, seen := bestByCreature[ck]
		if !seen || l.tick > cur.tick || (l.tick == cur.tick && l.observed > cur.observed) {
			bestByCreature[ck] = l
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	type groupKey struct{ species, stage string }
	groups := make(map[groupKey][]int)
	for ck, l := range bestByCreature {
		stage := l.stage
		if stage == "" {
			stage = "adult"
		}
		groups[groupKey{ck.species, stage}] = append(groups[groupKey{ck.species, stage}], l.hull)
	}

	bySpecies := make(map[string][]StageStats)
	for gk, hulls := range groups {
		slices.Sort(hulls)
		bySpecies[gk.species] = append(bySpecies[gk.species], StageStats{
			Stage:      gk.stage,
			Creatures:  len(hulls),
			Min:        hulls[0],
			Max:        hulls[len(hulls)-1],
			Median:     medianInt(hulls),
			HullValues: hulls,
		})
	}
	for species, stages := range bySpecies {
		slices.SortFunc(stages, func(a, b StageStats) int { return cmp.Compare(a.Median, b.Median) })
		sp, ok := index[species]
		if !ok {
			continue
		}
		sp.Stages = stages
		sp.StagesSince = minObserved[species]
	}
	return nil
}

// medianInt returns the median of a sorted, non-empty slice of ints. An even
// count averages the two middle values, which may land on a half-integer.
func medianInt(sorted []int) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}
