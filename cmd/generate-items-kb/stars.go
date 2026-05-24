package main

import (
	"encoding/json"
	"os"
	"sort"
)

// StarRecord is one system's star as needed by the hyperspace-warp engine:
// position on the galactic plane plus the sun's spectral class (for color/size).
type StarRecord struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Class string  `json:"class"` // Morgan-Keenan class; "" when unknown
}

// starRecords builds the star list for all systems, sorted by id. The class is
// taken from the system's sun POI (type "sun"), or "" if there is none.
func starRecords(systems []*System) []StarRecord {
	recs := make([]StarRecord, 0, len(systems))
	for _, s := range systems {
		class := ""
		for _, p := range s.POIs {
			if p.Type == "sun" {
				class = p.Class
				break
			}
		}
		recs = append(recs, StarRecord{ID: s.ID, Name: s.Name, X: s.PositionX, Y: s.PositionY, Class: class})
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	return recs
}

// starsJSON marshals the star catalog to a compact JSON array.
func starsJSON(systems []*System) ([]byte, error) {
	return json.Marshal(starRecords(systems))
}

// writeStarsJSON writes the galaxy's stars to path as a compact JSON array.
func writeStarsJSON(path string, systems []*System) error {
	data, err := starsJSON(systems)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// writeStarsJS writes the star catalog as a script that assigns the array to a
// global, so pages can load it with a <script> tag and work off the filesystem
// (file:// blocks fetch of local JSON).
func writeStarsJS(path string, systems []*System) error {
	data, err := starsJSON(systems)
	if err != nil {
		return err
	}
	out := append([]byte("window.SPACEMOLT_STARS="), data...)
	out = append(out, ';')
	return os.WriteFile(path, out, 0o644)
}
