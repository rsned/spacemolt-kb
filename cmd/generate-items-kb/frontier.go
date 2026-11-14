package main

import (
	"cmp"
	"slices"

	"github.com/rsned/spacemolt-kb/internal/frontier"
)

// frontierPositionsPath holds estimated positions for uncharted frontier
// systems, relative to the kb repo root the generator runs from.
const frontierPositionsPath = frontier.EstimatesPath

// frontierEstimate is one hand-estimated system position.
type frontierEstimate = frontier.Estimate

// loadFrontierEstimates reads the estimates file; a missing or unreadable file
// yields nil.
func loadFrontierEstimates(path string) map[string]frontierEstimate {
	return frontier.LoadEstimates(path)
}

// applyFrontierPositions gives uncharted systems a usable position. Neither
// get_map nor get_system reports one for them, so the database holds (0,0) --
// Sol's spot -- unless the estimates were backfilled into it. An uncharted
// system with an estimate takes it and is flagged approximate either way;
// one with no estimate and no position is flagged as having none and kept
// off the maps. Charted systems are never touched.
func applyFrontierPositions(systems []*System, est map[string]frontierEstimate) {
	for _, s := range systems {
		if !s.Uncharted {
			continue
		}
		if e, ok := est[s.ID]; ok {
			s.PositionX, s.PositionY, s.PositionApprox = e.X, e.Y, true
		} else if s.PositionX == 0 && s.PositionY == 0 {
			s.PositionUnknown = true
		}
	}
}

// sightedSystems makes a Sighted system for every estimate with no database
// row: a frontier system seen only on another player's map. Its name comes
// from the estimate, else from its ID. Sorted by ID for stable output.
func sightedSystems(systems []*System, est map[string]frontierEstimate) []*System {
	known := make(map[string]bool, len(systems))
	for _, s := range systems {
		known[s.ID] = true
	}
	var out []*System
	for id, e := range est {
		if known[id] {
			continue
		}
		name := e.Name
		if name == "" {
			name = titleCase(id)
		}
		out = append(out, &System{ID: id, Name: name, PositionX: e.X, PositionY: e.Y,
			Uncharted: true, PositionApprox: true, Sighted: true})
	}
	slices.SortFunc(out, func(a, b *System) int { return cmp.Compare(a.ID, b.ID) })
	return out
}
