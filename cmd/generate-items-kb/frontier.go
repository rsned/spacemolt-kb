package main

import (
	"encoding/json"
	"os"
)

// frontierPositionsPath holds estimated positions for uncharted frontier
// systems, relative to the kb repo root the generator runs from.
const frontierPositionsPath = "data/frontier_positions.json"

// frontierEstimate is one hand-estimated system position.
type frontierEstimate struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Source string  `json:"source"`
}

// loadFrontierEstimates reads the estimates file; a missing or unreadable file
// yields nil.
func loadFrontierEstimates(path string) map[string]frontierEstimate {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var f struct {
		Systems map[string]frontierEstimate `json:"systems"`
	}
	if json.Unmarshal(data, &f) != nil {
		return nil
	}
	return f.Systems
}

// applyFrontierPositions gives uncharted systems a usable position. Neither
// get_map nor get_system reports one for them, so the database holds (0,0) --
// Sol's spot. An estimate, when there is one, is used and flagged approximate;
// otherwise the system is flagged as having no position and kept off maps.
// Charted systems are never touched.
func applyFrontierPositions(systems []*System, est map[string]frontierEstimate) {
	for _, s := range systems {
		if !s.Uncharted || s.PositionX != 0 || s.PositionY != 0 {
			continue
		}
		if e, ok := est[s.ID]; ok {
			s.PositionX, s.PositionY, s.PositionApprox = e.X, e.Y, true
		} else {
			s.PositionUnknown = true
		}
	}
}

// withPosition returns the systems that can be placed on a map.
func withPosition(systems []*System) []*System {
	out := make([]*System, 0, len(systems))
	for _, s := range systems {
		if !s.PositionUnknown {
			out = append(out, s)
		}
	}
	return out
}
