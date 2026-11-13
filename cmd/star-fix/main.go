// Command star-fix checks telescope star sightings against known stars and
// places unknown ones. A Stellar Telescope makes get_location list
// star_sightings: bearing, brightness and temperature, with no name or
// distance. Each sighting is matched to a known star by bearing and by the
// game's temperature range for the star's class; readings no known star
// clearly explains, seen from two systems, are placed where the bearings
// cross.
//
// Sightings are logged by hand in data/star_sightings.json. Run from the kb
// repo root:
//
//	go run ./cmd/star-fix                 # text report
//	go run ./cmd/star-fix -html map.html  # also a pan/zoom galaxy plot
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
	htmlPath := flag.String("html", "", "also write a galaxy plot of the unclear readings here")
	var lim Limits
	flag.Float64Var(&lim.Tol, "tol", 6, "bearing tolerance for a match, degrees")
	flag.Float64Var(&lim.ClearErr, "clear-err", 3, "a match farther off than this many degrees is doubtful")
	flag.Float64Var(&lim.ClearDist, "clear-dist", 2000, "a match to an ordinary star farther than this is doubtful (0: off)")
	flag.Float64Var(&lim.DwarfDist, "dwarf-dist", 2500, "a match to a white dwarf farther than this is doubtful (0: off)")
	flag.Float64Var(&lim.FitErr, "fit-err", 3.5, "how far a reading may miss a fitted star, degrees")
	flag.Float64Var(&lim.Sigma, "sigma", 2, "assumed bearing error for the error ellipse, degrees")
	flag.Float64Var(&lim.MinCross, "min-cross", 10, "two lines crossing at less than this many degrees place nothing")
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
	est := frontier.LoadEstimates(frontier.EstimatesPath)
	systems, err := load(db, est)
	if err != nil {
		log.Fatal(err)
	}
	pos := map[string]Point{}
	var stars []Star
	for _, s := range systems {
		pos[s.ID] = s.At
		if s.Star.Hi > 0 {
			stars = append(stars, s.Star)
		}
	}
	rs, fits := analyze(f.Observations, pos, stars, lim)
	report(f.Observations, pos, rs, fits, lim)
	if *htmlPath != "" {
		if err := writeHTML(*htmlPath, systems, f.Observations, rs, fits, lim); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("\nwrote %s\n", *htmlPath)
	}
}

// report prints each observation's readings and the crossings.
func report(obs []Observation, pos map[string]Point, rs []Reading, fits []Fit, lim Limits) {
	i := 0
	for _, o := range obs {
		at, ok := pos[o.System]
		if !ok {
			fmt.Printf("%s: no position known, skipped\n\n", o.System)
			continue
		}
		fmt.Printf("From %s (%s):\n", o.System, o.Seen)
		for range o.Stars {
			r := rs[i]
			i++
			if r.Match == nil {
				fmt.Printf("  %5.0f°  %6.0f K  b%d  = UNKNOWN: no known star within %.0f° fits\n", r.S.Bearing, r.S.Temperature, r.S.Brightness, lim.Tol)
				continue
			}
			m := r.Match
			flag := ""
			if r.Status == "doubtful" {
				flag = "  [doubtful]"
			}
			fmt.Printf("  %5.0f°  %6.0f K  b%d  = %s (%s), true bearing %.1f° (off %.1f°), %.0f away%s\n",
				r.S.Bearing, r.S.Temperature, r.S.Brightness, m.Star.System, m.Star.Class, bearing(at, m.Star.At), m.Err, m.Distance, flag)
		}
		fmt.Println()
	}
	for _, f := range fits {
		fmt.Printf("Unknown %.0f K star at (%.0f, %.0f), %.0f from the centre: %d readings, worst %.1f° off, ±%.0f × ±%.0f\n",
			f.Temp, f.At.X, f.At.Y, math.Hypot(f.At.X, f.At.Y), len(f.Members), f.MaxErr, f.Major, f.Minor)
		for _, m := range f.Members {
			fmt.Printf("    %s %.0f° b%d (%.0f away)\n", m.Obs, m.S.Bearing, m.S.Brightness, math.Hypot(f.At.X-m.From.X, f.At.Y-m.From.Y))
		}
	}
}

// System is one positioned system for the plot.
type System struct {
	ID, Name  string
	At        Point
	Estimated bool // position from the frontier estimates file
	Star      Star // Hi == 0 when the star's class is unknown
}

// load reads every positioned system (frontier estimates win over the DB's
// 0,0) with its star's class temperature range.
func load(db *sql.DB, est map[string]frontier.Estimate) ([]System, error) {
	star := map[string]Star{}
	rows, err := db.Query(`SELECT p.system_id, m.star_class, COALESCE(m.temp_range,'')
		FROM pois p JOIN poi_metadata_stars m ON m.poi_id = p.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sys, class, tr string
		if err := rows.Scan(&sys, &class, &tr); err != nil {
			return nil, err
		}
		lo, hi := parseTempRange(tr)
		star[sys] = Star{System: sys, Class: class, Lo: lo, Hi: hi}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	rows, err = db.Query(`SELECT id, COALESCE(name,id), position_x, position_y FROM systems ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []System
	for rows.Next() {
		var id, name string
		var x, y sql.NullFloat64
		if err := rows.Scan(&id, &name, &x, &y); err != nil {
			return nil, err
		}
		s := System{ID: id, Name: name}
		switch e, ok := est[id]; {
		case ok:
			s.At, s.Estimated = Point{e.X, e.Y}, true
		case x.Valid && (x.Float64 != 0 || y.Float64 != 0 || id == "sol"):
			s.At = Point{x.Float64, y.Float64}
		default:
			continue
		}
		if st, ok := star[id]; ok {
			st.At = s.At
			s.Star = st
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
