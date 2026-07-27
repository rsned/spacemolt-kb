// Command generate-stronghold-reach renders the "Reach of the Nine
// Strongholds" Did-You-Know page: a galaxy map whose red territory blobs
// grow one jump at a time from the galaxy's pirate strongholds.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// defaultRadius is the frame the page opens on.
const defaultRadius = 5

func main() {
	dbPath := flag.String("db", "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db",
		"path to the knowledge database")
	out := flag.String("out", "kb/did_you_know/stronghold_reach.html", "output HTML path")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	systems, edges, sources, err := loadGalaxy(db)
	if err != nil {
		log.Fatalf("load galaxy: %v", err)
	}
	if len(sources) == 0 {
		log.Fatalf("no systems flagged is_stronghold; nothing to measure reach from")
	}

	reach := ComputeReach(edges, sources)
	if unreachable := len(systems) - len(reach.Dist); unreachable > 0 {
		log.Printf("warning: %d systems have no route to any stronghold", unreachable)
	}

	names := make(map[string]string, len(systems))
	for _, s := range systems {
		names[s.ID] = s.Name
	}

	rows := RadiusRows(reach, edges, len(systems), reach.Max)
	territory := TerritoryRows(reach, names)

	// All systems are passed as "explored" so every one gets a dot and
	// blob geometry; the reach map has no explored/unexplored split.
	svg := galaxymap.Render(systems, nil, systemIndex(systems), galaxymap.Options{
		ShowConnections: true,
		LinkPrefix:      "../",
		HighlightClasses: func(id string) []string {
			d, ok := reach.Dist[id]
			if !ok {
				return nil
			}
			return []string{fmt.Sprintf("sr-%d", d)}
		},
		ReachBlob: &galaxymap.ReachBlob{
			Radius: func(id string) int {
				if d, ok := reach.Dist[id]; ok {
					return d
				}
				return -1
			},
			Max:   reach.Max,
			Color: "#c53030",
		},
	})

	statsJSON, err := json.Marshal(statsByRadius(rows))
	if err != nil {
		log.Fatalf("marshal stats: %v", err)
	}

	data := struct {
		TotalSystems      int
		EdgeCount         int
		MaxRadius         int
		DefaultRadius     int
		Rows              []RadiusRow
		Territory         []TerritoryRow
		MergeStory        string
		TopTerritory      string
		TopTerritoryCount int
		FarthestNames     string
		UnreachableCount  int
		ReachCSS          template.CSS
		MapSVG            template.HTML
		StatsJSON         template.JS
	}{
		TotalSystems:     len(systems),
		EdgeCount:        len(edges),
		MaxRadius:        reach.Max,
		DefaultRadius:    min(defaultRadius, reach.Max),
		Rows:             rows,
		Territory:        territory,
		MergeStory:       mergeStory(rows),
		FarthestNames:    farthestNames(reach, names),
		UnreachableCount: len(systems) - len(reach.Dist),
		ReachCSS:         template.CSS(ReachCSS(reach.Max)),
		MapSVG:           template.HTML(svg),
		StatsJSON:        template.JS(statsJSON),
	}
	if len(territory) > 0 {
		data.TopTerritory = territory[0].Name
		data.TopTerritoryCount = territory[0].Systems
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatalf("create output file: %v", err)
	}
	defer func() { _ = f.Close() }()

	tmpl := template.Must(template.New("stronghold-reach").Parse(pageTemplate))
	if err := tmpl.Execute(f, data); err != nil {
		log.Fatalf("render page: %v", err)
	}

	fmt.Printf("Generated %s (%d systems, max radius %d)\n", *out, len(systems), reach.Max)
}

// systemIndex builds the ID lookup galaxymap.Render needs to resolve
// connection endpoints.
func systemIndex(systems []*galaxymap.System) map[string]*galaxymap.System {
	m := make(map[string]*galaxymap.System, len(systems))
	for _, s := range systems {
		m[s.ID] = s
	}
	return m
}

// radiusStat is the per-frame readout payload handed to the page script.
type radiusStat struct {
	Systems int    `json:"systems"`
	Percent string `json:"percent"`
	Blobs   int    `json:"blobs"`
}

// statsByRadius keys the readout payload by radius for direct lookup in JS.
func statsByRadius(rows []RadiusRow) map[string]radiusStat {
	m := make(map[string]radiusStat, len(rows))
	for _, r := range rows {
		m[fmt.Sprintf("%d", r.Radius)] = radiusStat{
			Systems: r.Systems,
			Percent: fmt.Sprintf("%.1f", r.Percent),
			Blobs:   r.Blobs,
		}
	}
	return m
}

// mergeStory renders the blob-count sequence as prose, e.g.
// "9 at 3 jumps, 8 at 4, 6 at 6, 4 at 7, 2 at 8, and a single blob at 9".
func mergeStory(rows []RadiusRow) string {
	var parts []string
	prev := -1
	for _, r := range rows {
		if r.Blobs == prev {
			continue
		}
		prev = r.Blobs
		if r.Blobs == 1 {
			parts = append(parts, fmt.Sprintf("a single blob at %d", r.Radius))
			break
		}
		parts = append(parts, fmt.Sprintf("%d at %d", r.Blobs, r.Radius))
	}
	return strings.Join(parts, ", ")
}

// farthestNames lists the systems sitting at the maximum reach distance.
func farthestNames(r Reach, names map[string]string) string {
	var out []string
	for id, d := range r.Dist {
		if d == r.Max {
			n := names[id]
			if n == "" {
				n = id
			}
			out = append(out, n)
		}
	}
	slices.Sort(out)
	return strings.Join(out, ", ")
}
