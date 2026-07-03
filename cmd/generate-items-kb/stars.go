package main

import (
	"encoding/json"
	"os"
	"sort"
)

// StarRecord is one system's star as needed by the hyperspace-warp engine:
// position on the galactic plane plus the sun's spectral class (for color/size).
//
// Suns is set only for multi-star systems (binary/trinary); it lists every
// component star so the warp can render them as a cluster. Single-star systems
// leave it nil and rely on Class alone.
type StarRecord struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	X     float64   `json:"x"`
	Y     float64   `json:"y"`
	Class string    `json:"class"` // Morgan-Keenan class of the headline star; "" when unknown
	Suns  []SunComp `json:"suns,omitempty"`
}

// SunComp is one component star of a multi-star system.
type SunComp struct {
	Name  string `json:"name"`
	Class string `json:"class"`
}

// sunComponents lists a system's component stars, headline first (any black
// hole, then the remaining suns in catalog order). It returns nil when the
// system has one sun or none, so single-star records carry no suns array.
func sunComponents(pois []SystemPOI) []SunComp {
	var bh, rest []SunComp
	for _, p := range pois {
		if p.Type != "sun" {
			continue
		}
		c := SunComp{Name: p.Name, Class: p.Class}
		if p.Class == "BH" {
			bh = append(bh, c)
		} else {
			rest = append(rest, c)
		}
	}
	all := append(bh, rest...)
	if len(all) <= 1 {
		return nil
	}
	return all
}

// sunClass picks a system's representative spectral class from its sun POIs.
// Multi-star systems (e.g. Alzirr) can list several suns; prefer a black hole
// (the headline object) and otherwise the first sun with a real class, so a
// blank-class entry never shadows a classified companion.
func sunClass(pois []SystemPOI) string {
	best := ""
	for _, p := range pois {
		if p.Type != "sun" {
			continue
		}
		if p.Class == "BH" {
			return "BH"
		}
		if best == "" && p.Class != "" {
			best = p.Class
		}
	}
	return best
}

// starRecords builds the star list for all systems, sorted by id.
func starRecords(systems []*System) []StarRecord {
	recs := make([]StarRecord, 0, len(systems))
	for _, s := range systems {
		recs = append(recs, StarRecord{
			ID: s.ID, Name: s.Name, X: s.PositionX, Y: s.PositionY,
			Class: sunClass(s.POIs), Suns: sunComponents(s.POIs),
		})
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
