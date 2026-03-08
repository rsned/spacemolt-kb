// Command system-map generates an SVG system map from either a get_system
// JSON response file or from the SQLite knowledge database.
//
// Usage:
//
//	system-map -json path/to/get_system.json
//	system-map -json file.json -map get_map.json
//	system-map -db path/to/knowledge.db -system sol
//	system-map -json file.json -o map.svg
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/systemmap"
)

type getSystemResponse struct {
	System jsonSystem `json:"system"`
}

type jsonSystem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	POIs        []jsonPOI  `json:"pois"`
	Connections []jsonConn `json:"connections"`
}

type jsonPOI struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Position    struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

type jsonConn struct {
	SystemID string `json:"system_id"`
	Name     string `json:"name"`
	Distance int    `json:"distance"`
}

func main() {
	jsonPath := flag.String("json", "", "path to get_system JSON response file")
	mapPath := flag.String("map", "", "path to get_map JSON file (for gate angles in -json mode)")
	dbPath := flag.String("db", "", "path to SQLite knowledge database")
	systemID := flag.String("system", "", "system ID to render (required with -db)")
	outPath := flag.String("o", "", "output file path (default: stdout)")
	flag.Parse()

	hasJSON := *jsonPath != ""
	hasDB := *dbPath != ""

	if hasJSON == hasDB {
		fmt.Fprintln(os.Stderr, "error: exactly one of -json or -db must be provided")
		flag.Usage()
		os.Exit(1)
	}

	var svg string
	var err error

	if hasJSON {
		svg, err = renderFromJSON(*jsonPath, *mapPath)
	} else {
		if *systemID == "" {
			fmt.Fprintln(os.Stderr, "error: -system is required with -db")
			flag.Usage()
			os.Exit(1)
		}
		svg, err = renderFromDB(*dbPath, *systemID)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(svg), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Print(svg)
	}
}

// getMapResponse is the top-level structure of a get_map JSON response.
type getMapResponse struct {
	Systems []mapSystem `json:"systems"`
}

type mapSystem struct {
	SystemID string   `json:"system_id"`
	Name     string   `json:"name"`
	Position struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

func renderFromJSON(path, mapFilePath string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading JSON file: %w", err)
	}

	var resp getSystemResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	sys := convertJSONSystem(resp.System)

	var allSystems map[string]*systemmap.System
	if mapFilePath != "" {
		allSystems, err = loadMapSystems(mapFilePath, sys)
		if err != nil {
			return "", err
		}
	}

	return systemmap.RenderSystemMap(sys, allSystems, true), nil
}

// loadMapSystems reads a get_map JSON file and builds the allSystems lookup
// needed for gate angle computation. It also sets the galaxy coordinates on
// the current system from the map data.
func loadMapSystems(path string, sys *systemmap.System) (map[string]*systemmap.System, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading map file: %w", err)
	}

	var resp getMapResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parsing map JSON: %w", err)
	}

	allSystems := make(map[string]*systemmap.System, len(resp.Systems))
	for _, ms := range resp.Systems {
		s := &systemmap.System{
			ID:        ms.SystemID,
			Name:      ms.Name,
			PositionX: ms.Position.X,
			PositionY: ms.Position.Y,
		}
		allSystems[ms.SystemID] = s

		// Set galaxy coordinates on the current system.
		if ms.SystemID == sys.ID {
			sys.PositionX = ms.Position.X
			sys.PositionY = ms.Position.Y
		}
	}

	// Ensure the current system is in the map.
	allSystems[sys.ID] = sys

	return allSystems, nil
}

func convertJSONSystem(js jsonSystem) *systemmap.System {
	sys := &systemmap.System{
		ID:   js.ID,
		Name: js.Name,
	}

	for _, p := range js.POIs {
		sys.POIs = append(sys.POIs, systemmap.POI{
			ID:          p.ID,
			Name:        p.Name,
			Type:        p.Type,
			Description: p.Description,
			PositionX:   p.Position.X,
			PositionY:   p.Position.Y,
		})
	}

	for _, c := range js.Connections {
		sys.Connections = append(sys.Connections, systemmap.Connection{
			SystemID: c.SystemID,
			Name:     c.Name,
			Distance: c.Distance,
		})
	}

	return sys
}

func renderFromDB(dbPath, sysID string) (string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", fmt.Errorf("opening database: %w", err)
	}
	defer func() { _ = db.Close() }()

	// Load main system.
	var sys systemmap.System
	err = db.QueryRow("SELECT id, name, position_x, position_y FROM systems WHERE id = ?", sysID).
		Scan(&sys.ID, &sys.Name, &sys.PositionX, &sys.PositionY)
	if err != nil {
		return "", fmt.Errorf("querying system %q: %w", sysID, err)
	}

	// Load POIs.
	poiRows, err := db.Query(
		"SELECT id, name, type, COALESCE(description,''), position_x, position_y FROM pois WHERE system_id = ? ORDER BY name",
		sysID,
	)
	if err != nil {
		return "", fmt.Errorf("querying POIs: %w", err)
	}
	defer func() { _ = poiRows.Close() }()

	for poiRows.Next() {
		var p systemmap.POI
		if err := poiRows.Scan(&p.ID, &p.Name, &p.Type, &p.Description, &p.PositionX, &p.PositionY); err != nil {
			return "", fmt.Errorf("scanning POI: %w", err)
		}
		sys.POIs = append(sys.POIs, p)
	}
	if err := poiRows.Err(); err != nil {
		return "", fmt.Errorf("iterating POIs: %w", err)
	}

	// Load connections.
	connRows, err := db.Query(
		"SELECT c.to_system, s.name, c.distance FROM connections c JOIN systems s ON c.to_system = s.id WHERE c.from_system = ?",
		sysID,
	)
	if err != nil {
		return "", fmt.Errorf("querying connections: %w", err)
	}
	defer func() { _ = connRows.Close() }()

	for connRows.Next() {
		var c systemmap.Connection
		if err := connRows.Scan(&c.SystemID, &c.Name, &c.Distance); err != nil {
			return "", fmt.Errorf("scanning connection: %w", err)
		}
		sys.Connections = append(sys.Connections, c)
	}
	if err := connRows.Err(); err != nil {
		return "", fmt.Errorf("iterating connections: %w", err)
	}

	// Load connected systems for gate angle computation.
	allSystems := make(map[string]*systemmap.System)
	allSystems[sys.ID] = &sys

	neighborRows, err := db.Query(
		"SELECT id, name, position_x, position_y FROM systems WHERE id IN (SELECT to_system FROM connections WHERE from_system = ?)",
		sysID,
	)
	if err != nil {
		return "", fmt.Errorf("querying connected systems: %w", err)
	}
	defer func() { _ = neighborRows.Close() }()

	for neighborRows.Next() {
		var ns systemmap.System
		if err := neighborRows.Scan(&ns.ID, &ns.Name, &ns.PositionX, &ns.PositionY); err != nil {
			return "", fmt.Errorf("scanning connected system: %w", err)
		}
		allSystems[ns.ID] = &ns
	}
	if err := neighborRows.Err(); err != nil {
		return "", fmt.Errorf("iterating connected systems: %w", err)
	}

	return systemmap.RenderSystemMap(&sys, allSystems, true), nil
}
