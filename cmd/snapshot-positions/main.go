// Command snapshot-positions keeps an append-only, out-of-band copy of every
// system coordinate we have learned, in data/learned_positions.json.
//
// The knowledge DB is not a safe store for them: when an explorer visits a
// frontier system the scraper marks it "api" and overwrites its drift-measured
// position with 0,0. Each run folds in the DB's non-zero positions and our
// fitted frontier positions; a value once recorded is never dropped, only
// superseded, and the last value from each source is kept. Run from the kb
// repo root after a scrape:
//
//	go run ./cmd/snapshot-positions
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Reading is one coordinate from one source.
type Reading struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	FirstSeen string  `json:"first_seen"`
	LastSeen  string  `json:"last_seen"`
	Note      string  `json:"note,omitempty"`
}

// Entry is everything learned about one system's position, keyed by source
// ("api", "measured", "estimate", "fitted").
type Entry struct {
	Name    string              `json:"name"`
	Sources map[string]*Reading `json:"sources"`
}

func main() {
	dbPath := flag.String("db", "spacemolt-knowledge.db", "knowledge DB")
	frontierPath := flag.String("frontier", "data/frontier_positions.json", "fitted frontier positions")
	out := flag.String("out", "data/learned_positions.json", "append-only snapshot")
	flag.Parse()
	today := time.Now().UTC().Format("2006-01-02")

	snap := map[string]*Entry{}
	if data, err := os.ReadFile(*out); err == nil {
		if err := json.Unmarshal(data, &snap); err != nil {
			log.Fatalf("%s: %v", *out, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	rows, err := db.Query(`SELECT id, name, position_x, position_y, COALESCE(position_source, '') FROM systems`)
	if err != nil {
		log.Fatal(err)
	}
	changed := 0
	for rows.Next() {
		var id, name, src string
		var x, y float64
		if err := rows.Scan(&id, &name, &x, &y, &src); err != nil {
			log.Fatal(err)
		}
		if src == "" {
			src = "api"
		}
		if record(snap, id, name, src, x, y, "", today) {
			changed++
		}
	}
	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}
	_ = rows.Close()
	_ = db.Close()

	var frontier struct {
		Systems map[string]struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Source string  `json:"source"`
		} `json:"systems"`
	}
	data, err := os.ReadFile(*frontierPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.Unmarshal(data, &frontier); err != nil {
		log.Fatalf("%s: %v", *frontierPath, err)
	}
	for id, p := range frontier.Systems {
		name := id
		if e := snap[id]; e != nil {
			name = e.Name
		}
		if record(snap, id, name, "fitted", p.X, p.Y, p.Source, today) {
			changed++
		}
	}

	data, err = json.MarshalIndent(snap, "", "  ") // map keys marshal sorted
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d systems in %s, %d readings new or changed\n", len(snap), *out, changed)
}

// record folds one reading into the snapshot and reports whether it changed
// anything beyond the last-seen date. A 0,0 position is the DB's placeholder
// for "unknown" (except Sol, which really sits at the origin) and is ignored,
// so it can never erase a learned value.
func record(snap map[string]*Entry, id, name, src string, x, y float64, note, today string) bool {
	if x == 0 && y == 0 && id != "sol" {
		return false
	}
	e := snap[id]
	if e == nil {
		e = &Entry{Sources: map[string]*Reading{}}
		snap[id] = e
	}
	e.Name = name
	r := e.Sources[src]
	if r != nil && math.Abs(r.X-x) < 0.5 && math.Abs(r.Y-y) < 0.5 && r.Note == note {
		r.LastSeen = today
		return false
	}
	e.Sources[src] = &Reading{X: x, Y: y, FirstSeen: today, LastSeen: today, Note: note}
	return true
}
