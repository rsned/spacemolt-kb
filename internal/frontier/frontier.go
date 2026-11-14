// Package frontier supports uncharted frontier systems (v0.687): systems the
// in-game map and search leave out. get_map omits them and get_system carries
// no position, so the knowledge database stores them at (0,0). Positions come
// from a hand-maintained estimates file instead.
package frontier

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// EstimatesPath is the estimates file, relative to the kb repo root.
const EstimatesPath = "data/frontier_positions.json"

// Color is the pink the in-game map uses for uncharted systems.
const Color = "#F472B6"

// Estimate is one hand-estimated system position.
type Estimate struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Source string  `json:"source"`
	// Name is the display name of a system known only from another
	// player's map (no database row yet); empty otherwise.
	Name string `json:"name,omitempty"`
}

// LoadEstimates reads an estimates file; a missing or unreadable file yields nil.
func LoadEstimates(path string) map[string]Estimate {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Systems map[string]Estimate `json:"systems"`
	}
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	return f.Systems
}

// ChartedIDs returns the IDs of every system in catalogDir/get_map.json, or nil
// when it cannot be read. A system missing from it is uncharted.
func ChartedIDs(catalogDir string) map[string]bool {
	data, err := os.ReadFile(filepath.Join(catalogDir, "get_map.json"))
	if err != nil {
		return nil
	}
	var m struct {
		Systems []struct {
			ID string `json:"system_id"`
		} `json:"systems"`
	}
	if err := json.Unmarshal(data, &m); err != nil || len(m.Systems) == 0 {
		return nil
	}
	out := make(map[string]bool, len(m.Systems))
	for _, s := range m.Systems {
		out[s.ID] = true
	}
	return out
}
