// Command generate-galaxy-map creates a full galaxy map page with all explored systems.
package main

import (
	"cmp"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"os"
	"slices"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
)

// System holds galaxy map data.
type System = galaxymap.System

// Connection is a jump gate connection.
type Connection = galaxymap.Connection

// POI is a point of interest.
type POI struct {
	SystemID string
	Type     string
	Name     string
}

func main() {
	knowledgeDBPath := "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db"

	db, err := sql.Open("sqlite", knowledgeDBPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	systems, err := loadSystems(db)
	if err != nil {
		log.Fatalf("load systems: %v", err)
	}

	pois, err := loadPOIs(db)
	if err != nil {
		log.Printf("warning: failed to load POIs: %v", err)
		pois = make(map[string][]POI) // Continue without POI data
	}

	// Create output directory.
	outDir := "kb"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	// Generate the galaxy map page.
	if err := writeGalaxyMapPage(outDir, systems, pois); err != nil {
		log.Fatalf("write galaxy map page: %v", err)
	}

	fmt.Printf("Generated galaxy map page in %s/\n", outDir)
}

func loadSystems(db *sql.DB) ([]*System, error) {
	rows, err := db.Query(`SELECT id, name, position_x, position_y, police_level, COALESCE(empire,''), is_stronghold, last_updated_tick FROM systems ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	systemMap := make(map[string]*System)
	var systems []*System
	for rows.Next() {
		var s System
		if err := rows.Scan(&s.ID, &s.Name, &s.PositionX, &s.PositionY, &s.PoliceLevel, &s.Empire, &s.IsStronghold, &s.LastUpdatedTick); err != nil {
			return nil, err
		}
		if s.ID == "" {
			continue
		}
		systemMap[s.ID] = &s
		systems = append(systems, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load connections.
	connRows, err := db.Query(`SELECT from_system, to_system, distance FROM connections ORDER BY from_system`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = connRows.Close() }()

	for connRows.Next() {
		var fromID, toID string
		var distance int
		if err := connRows.Scan(&fromID, &toID, &distance); err != nil {
			return nil, err
		}
		if from, ok := systemMap[fromID]; ok {
			toName := toID
			if to, ok := systemMap[toID]; ok {
				toName = to.Name
			}
			from.Connections = append(from.Connections, Connection{
				SystemID: toID,
				Name:     toName,
				Distance: distance,
			})
		}
	}
	if err := connRows.Err(); err != nil {
		return nil, err
	}

	// Sort connections by name.
	for _, s := range systems {
		slices.SortFunc(s.Connections, func(a, b Connection) int {
			return cmp.Compare(a.Name, b.Name)
		})
	}

	return systems, nil
}

func loadPOIs(db *sql.DB) (map[string][]POI, error) {
	rows, err := db.Query(`SELECT system_id, type, name FROM pois WHERE type = 'station'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	poiMap := make(map[string][]POI)
	for rows.Next() {
		var poi POI
		if err := rows.Scan(&poi.SystemID, &poi.Type, &poi.Name); err != nil {
			return nil, err
		}
		poiMap[poi.SystemID] = append(poiMap[poi.SystemID], poi)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return poiMap, nil
}

func writeGalaxyMapPage(outDir string, systems []*System, pois map[string][]POI) error {
	// Separate explored and unexplored systems.
	var explored, unexplored []*System
	for _, s := range systems {
		if s.LastUpdatedTick > 0 {
			explored = append(explored, s)
		} else {
			unexplored = append(unexplored, s)
		}
	}

	// Build system lookup and identify pirate strongholds.
	systemMap := make(map[string]*System, len(systems))
	for _, s := range systems {
		// Mark pirate strongholds (systems with stations named "Stronghold", "Fortress", or " Port")
		if !s.IsStronghold {
			for _, poi := range pois[s.ID] {
				if poi.Type == "station" {
					name := strings.ToLower(poi.Name)
					if strings.Contains(name, "stronghold") || strings.Contains(name, "fortress") || strings.Contains(name, " port") {
						s.IsStronghold = true
						break
					}
				}
			}
		}
		systemMap[s.ID] = s
	}

	// Calculate stats.
	totalSystems := len(systems)
	exploredCount := len(explored)
	explorationPct := 0.0
	if totalSystems > 0 {
		explorationPct = 100.0 * float64(exploredCount) / float64(totalSystems)
	}

	data := struct {
		TotalSystems   int
		ExploredCount  int
		UnexploredCount int
		ExplorationPct float64
		MapSVG         template.HTML
	}{
		TotalSystems:    totalSystems,
		ExploredCount:   exploredCount,
		UnexploredCount: len(unexplored),
		ExplorationPct:  explorationPct,
		MapSVG: template.HTML(galaxymap.Render(explored, unexplored, systemMap, galaxymap.Options{
			ShowEmpireBlobs: true,
			ShowConnections: true,
			LinkPrefix:      "",
		})),
	}

	tmpl := template.Must(template.New("galaxy-map").Parse(galaxyMapTemplate))
	path := outDir + "/galaxy-map.html"
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

var galaxyMapTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Galaxy Map - Spacemolt KB</title>
    <link rel="stylesheet" href="smui.css">
    <link rel="stylesheet" href="system.css">
    <style>
        body { margin: 0; padding: 0; background: #0a0e1a; }
        .galaxy-container { width: 100%; min-height: 100vh; display: flex; flex-direction: column; }
        .galaxy-header {
            background: var(--bg-card, #1a1f2e);
            padding: 15px 20px;
            border-bottom: 1px solid var(--border, #2d3748);
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: sticky;
            top: 0;
            z-index: 100;
        }
        .galaxy-stats { display: flex; gap: 20px; align-items: center; }
        .galaxy-stat { text-align: center; }
        .galaxy-stat .num { font-size: 1.4em; font-weight: 700; }
        .galaxy-stat .label { font-size: 0.75em; color: var(--text-muted, #718096); text-transform: uppercase; }
        .galaxy-map-wrapper { display: flex; justify-content: center; padding: 20px; }
        .galaxy-map-svg { width: 100%; max-width: 2000px; height: auto; }
        .galaxy-sys-dot { cursor: pointer; transition: r 0.2s; }
        .galaxy-sys-dot:hover { r: 6; }
        .galaxy-sys-label { font-family: system-ui, -apple-system, sans-serif; pointer-events: none; text-shadow: 1px 1px 2px rgba(0,0,0,0.8); }
        .back-link { color: var(--link, #63b3ed); text-decoration: none; }
        .back-link:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="galaxy-container">
        <div class="galaxy-header">
            <div>
                <h1 style="margin:0; font-size:1.3em">Galaxy Map</h1>
                <p style="margin:5px 0 0 0; font-size:0.9em; color:var(--text-muted,#718096)">Full explored galaxy view</p>
            </div>
            <div class="galaxy-stats">
                <div class="galaxy-stat">
                    <div class="num">{{.TotalSystems}}</div>
                    <div class="label">Total Systems</div>
                </div>
                <div class="galaxy-stat">
                    <div class="num">{{.ExploredCount}}</div>
                    <div class="label">Explored</div>
                </div>
                <div class="galaxy-stat">
                    <div class="num">{{printf "%.1f" .ExplorationPct}}%</div>
                    <div class="label">Explored</div>
                </div>
                <div style="margin-left:20px">
                    <a href="systems/index.html" class="back-link">← Back to Systems</a>
                </div>
            </div>
        </div>
        <div class="galaxy-map-wrapper">
            {{.MapSVG}}
        </div>
    </div>
</body>
</html>
`
