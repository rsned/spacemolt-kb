// Command system-map generates an SVG system map from either a get_system
// JSON response file or from the SQLite knowledge database.
//
// Usage:
//
//	system-map -json path/to/get_system.json
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
		svg, err = renderFromJSON(*jsonPath)
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

func renderFromJSON(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading JSON file: %w", err)
	}

	var resp getSystemResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parsing JSON: %w", err)
	}

	sys := convertJSONSystem(resp.System)
	return systemmap.RenderSystemMap(sys, nil, true), nil
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
