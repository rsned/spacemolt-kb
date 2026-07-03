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
)

// System holds galaxy map data.
type System struct {
	ID              string
	Name            string
	PositionX       float64
	PositionY       float64
	PoliceLevel     int
	Empire          string
	IsStronghold    bool
	LastUpdatedTick int
	Connections     []Connection
}

// Connection is a jump gate connection.
type Connection struct {
	SystemID string
	Name     string
	Distance int
}

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
		MapSVG:          template.HTML(renderGalaxyMap(explored, unexplored, systemMap)),
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

// renderGalaxyMap generates a full galaxy SVG map.
func renderGalaxyMap(explored, unexplored []*System, systemMap map[string]*System) string {
	if len(explored) == 0 {
		return `<p style="padding:20px;text-align:center">No explored systems to display.</p>`
	}

	// Build explored set for fast lookup.
	exploredSet := make(map[string]bool, len(explored))
	for _, s := range explored {
		exploredSet[s.ID] = true
	}

	// Compute bounding box of explored systems.
	minX, minY := explored[0].PositionX, explored[0].PositionY
	maxX, maxY := minX, minY
	for _, s := range explored[1:] {
		if s.PositionX < minX {
			minX = s.PositionX
		}
		if s.PositionX > maxX {
			maxX = s.PositionX
		}
		if s.PositionY < minY {
			minY = s.PositionY
		}
		if s.PositionY > maxY {
			maxY = s.PositionY
		}
	}

	// Add padding.
	padX := (maxX - minX) * 0.10
	padY := (maxY - minY) * 0.10
	if padX < 50 {
		padX = 50
	}
	if padY < 50 {
		padY = 50
	}
	minX -= padX
	minY -= padY
	maxX += padX
	maxY += padY

	rangeX := maxX - minX
	rangeY := maxY - minY
	if rangeX < 1 {
		rangeX = 1
	}
	if rangeY < 1 {
		rangeY = 1
	}

	// Use 2000x2000 canvas as requested.
	const svgSize = 2000.0
	scale := svgSize / max(rangeX, rangeY)

	// Center the explored region in the canvas.
	offsetX := (svgSize - rangeX*scale) / 2
	offsetY := (svgSize - rangeY*scale) / 2

	// Transform galaxy coords to SVG coords.
	tx := func(x float64) float64 {
		return (x-minX)*scale + offsetX
	}
	ty := func(y float64) float64 {
		return (y-minY)*scale + offsetY
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<svg viewBox="0 0 %.0f %.0f" xmlns="http://www.w3.org/2000/svg" class="galaxy-map-svg">`, svgSize, svgSize))

	// Dark background for space.
	b.WriteString(`<rect width="100%" height="100%" fill="#0a0e1a"/>`)

	// Metaball filter for explored territory blob.
	b.WriteString(`<defs><filter id="goo-galaxy" x="-20%" y="-20%" width="140%" height="140%" colorInterpolationFilters="sRGB">`)
	b.WriteString(`<feGaussianBlur in="SourceGraphic" stdDeviation="18" result="blur"/>`)
	b.WriteString(`<feColorMatrix in="blur" type="matrix" values="1 0 0 0 0  0 1 0 0 0  0 0 1 0 0  0 0 0 30 -12" result="blob"/>`)
	b.WriteString(`<feComponentTransfer in="blob" result="fill"><feFuncA type="linear" slope="0.25" intercept="0"/></feComponentTransfer>`)
	b.WriteString(`</filter></defs>`)

	// Territory blob - only for explored systems and their connections.
	const blobColor = "#E8E8E8" // Light white/grey
	blobR := 28.0
	b.WriteString(`<g filter="url(#goo-galaxy)">`)

	// Thick connection lines between explored systems.
	drawnBlob := make(map[string]bool)
	for _, s := range explored {
		for _, conn := range s.Connections {
			if !exploredSet[conn.SystemID] {
				continue // Skip connections to unexplored systems
			}
			key := s.ID + "|" + conn.SystemID
			rev := conn.SystemID + "|" + s.ID
			if drawnBlob[key] || drawnBlob[rev] {
				continue
			}
			drawnBlob[key] = true
			target := systemMap[conn.SystemID]
			if target == nil {
				continue
			}
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.0f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY), blobColor, blobR*1.2))
		}
	}

	// Circles at each explored system position.
	for _, s := range explored {
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.0f" fill="%s"/>`, tx(s.PositionX), ty(s.PositionY), blobR, blobColor))
	}
	b.WriteString(`</g>`)

	// Visible connection lines between explored systems (on top of blob).
	b.WriteString(`<g stroke="#63b3ed" stroke-width="2" opacity="0.6">`)
	drawn := make(map[string]bool)
	for _, s := range explored {
		for _, conn := range s.Connections {
			if !exploredSet[conn.SystemID] {
				continue
			}
			key := s.ID + "|" + conn.SystemID
			rev := conn.SystemID + "|" + s.ID
			if drawn[key] || drawn[rev] {
				continue
			}
			drawn[key] = true
			target := systemMap[conn.SystemID]
			if target == nil {
				continue
			}
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
		}
	}
	b.WriteString(`</g>`)

	// Outgoing connections to unexplored systems (dashed, brighter).
	b.WriteString(`<g stroke="#a0aec0" stroke-width="2" opacity="0.8" stroke-dasharray="4,4">`)
	drawnUnexplored := make(map[string]bool)

	// Connections from explored to unexplored
	for _, s := range explored {
		for _, conn := range s.Connections {
			if exploredSet[conn.SystemID] {
				continue
			}
			target := systemMap[conn.SystemID]
			if target == nil {
				continue
			}
			key := s.ID + "|" + conn.SystemID
			drawnUnexplored[key] = true
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
		}
	}

	// Connections between unexplored systems
	for _, s := range unexplored {
		for _, conn := range s.Connections {
			if exploredSet[conn.SystemID] {
				continue
			}
			target := systemMap[conn.SystemID]
			if target == nil {
				continue
			}
			key := s.ID + "|" + conn.SystemID
			rev := conn.SystemID + "|" + s.ID
			if drawnUnexplored[key] || drawnUnexplored[rev] {
				continue
			}
			drawnUnexplored[key] = true
			b.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f"/>`,
				tx(s.PositionX), ty(s.PositionY), tx(target.PositionX), ty(target.PositionY)))
		}
	}
	b.WriteString(`</g>`)

	// Explored system dots.
	for _, s := range explored {
		sx, sy := tx(s.PositionX), ty(s.PositionY)

		// Dot color based on empire.
		dotColor := blobColor
		if s.Empire != "" {
			switch s.Empire {
			case "solarian":
				dotColor = "#FFD700"
			case "voidborn":
				dotColor = "#9932CC"
			case "crimson":
				dotColor = "#DC143C"
			case "nebula":
				dotColor = "#00CED1"
			case "outerrim":
				dotColor = "#2E8B57"
			}
		}

		if s.IsStronghold {
			dotColor = "#FF0000"
		}

		b.WriteString(fmt.Sprintf(`<a href="systems/%s/"><circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#000" stroke-width="0.5" class="galaxy-sys-dot"><title>%s</title></circle>`,
			s.ID, sx, sy, dotColor, s.Name))

		// Label for major systems (capitals or strongholds).
		if s.IsStronghold || isCapital(s.ID) {
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="galaxy-sys-label" fill="#d8dee9" font-size="12" font-weight="bold">%s</text></a>`,
				sx+8, sy+4, s.Name))
		}
		b.WriteString(`</a>`)
	}

	// Unexplored system dots (same style as explored non-empire stars).
	for _, s := range unexplored {
		sx, sy := tx(s.PositionX), ty(s.PositionY)
		b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="#000" stroke-width="0.5" opacity="0.7"><title>%s (Unexplored)</title></circle>`,
			sx, sy, blobColor, s.Name))
	}

	b.WriteString(`</svg>`)
	return b.String()
}

// isCapital returns true if the system is an empire capital.
func isCapital(systemID string) bool {
	capitals := map[string]bool{
		"sol":         true, // Solarian
		"nexus_prime": true, // Voidborn
		"krynn":       true, // Crimson
		"haven":       true, // Nebula
		"frontier":    true, // Outerrim
	}
	return capitals[systemID]
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
