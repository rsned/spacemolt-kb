// Command star-fix checks telescope star sightings against known stars and
// places unknown ones. A Stellar Telescope makes get_location list
// star_sightings: bearing, brightness and temperature, with no name or
// distance. Each sighting is matched to a known star by bearing and by the
// game's temperature range for the star's class; a star no known system
// explains, seen from two systems, is placed where the two bearings cross.
//
// Sightings are logged by hand in data/star_sightings.json. Run from the kb
// repo root:
//
//	go run ./cmd/star-fix
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/internal/frontier"
)

// Observation is the star_sightings list seen from one system.
type Observation struct {
	System string     `json:"system"`
	Seen   string     `json:"seen"`
	Stars  []Sighting `json:"stars"`
}

func main() {
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database")
	logPath := flag.String("sightings", "data/star_sightings.json", "sightings log")
	tol := flag.Float64("tol", 5, "bearing tolerance in degrees")
	flag.Parse()

	var f struct {
		Observations []Observation `json:"observations"`
	}
	data, err := os.ReadFile(*logPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &f); err != nil {
		log.Fatalf("%s: %v", *logPath, err)
	}
	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	pos, stars, err := load(db, frontier.LoadEstimates(frontier.EstimatesPath))
	if err != nil {
		log.Fatal(err)
	}

	type loose struct {
		From Point
		Obs  string
		S    Sighting
	}
	var unknown []loose
	for _, o := range f.Observations {
		at, ok := pos[o.System]
		if !ok {
			fmt.Printf("%s: no position known, skipped\n\n", o.System)
			continue
		}
		fmt.Printf("From %s (%s):\n", o.System, o.Seen)
		for _, s := range o.Stars {
			if m, ok := matchSighting(at, o.System, s, stars, *tol); ok {
				fmt.Printf("  %5.0f°  %6.0f K  b%d  = %s (%s), true bearing %.1f° (off %.1f°), %.0f away\n",
					s.Bearing, s.Temperature, s.Brightness, m.Star.System, m.Star.Class, bearing(at, m.Star.At), m.Err, m.Distance)
				continue
			}
			fmt.Printf("  %5.0f°  %6.0f K  b%d  = UNKNOWN: no known star within %.0f° fits\n", s.Bearing, s.Temperature, s.Brightness, *tol)
			unknown = append(unknown, loose{at, o.System, s})
		}
		fmt.Println()
	}
	// Cross every pair of unknowns seen from different systems at the same
	// temperature: the telescope reports one fixed temperature per star.
	for i := range unknown {
		for j := i + 1; j < len(unknown); j++ {
			a, b := unknown[i], unknown[j]
			if a.Obs == b.Obs || a.S.Temperature != b.S.Temperature {
				continue
			}
			if p, ok := intersect(a.From, a.S.Bearing, b.From, b.S.Bearing); ok {
				fmt.Printf("Unknown %.0f K star: %s %.0f° × %s %.0f° cross at (%.0f, %.0f), %.0f from the centre\n",
					a.S.Temperature, a.Obs, a.S.Bearing, b.Obs, b.S.Bearing, p.X, p.Y, math.Hypot(p.X, p.Y))
			}
		}
	}
}

// load reads system positions (frontier estimates win over the DB's 0,0) and
// every star with its class temperature range.
func load(db *sql.DB, est map[string]frontier.Estimate) (map[string]Point, []Star, error) {
	pos := map[string]Point{}
	rows, err := db.Query(`SELECT id, position_x, position_y FROM systems`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var id string
		var x, y sql.NullFloat64
		if err := rows.Scan(&id, &x, &y); err != nil {
			return nil, nil, err
		}
		if e, ok := est[id]; ok {
			pos[id] = Point{e.X, e.Y}
		} else if x.Valid && (x.Float64 != 0 || y.Float64 != 0 || id == "sol") {
			pos[id] = Point{x.Float64, y.Float64}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	rows, err = db.Query(`SELECT p.system_id, m.star_class, COALESCE(m.temp_range,'')
		FROM pois p JOIN poi_metadata_stars m ON m.poi_id = p.id`)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var stars []Star
	for rows.Next() {
		var sys, class, tr string
		if err := rows.Scan(&sys, &class, &tr); err != nil {
			return nil, nil, err
		}
		at, ok := pos[sys]
		lo, hi := parseTempRange(tr)
		if !ok || hi == 0 {
			continue
		}
		stars = append(stars, Star{System: sys, Class: class, At: at, Lo: lo, Hi: hi})
	}
	return pos, stars, rows.Err()
}
