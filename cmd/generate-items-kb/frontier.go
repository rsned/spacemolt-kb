package main

import "github.com/rsned/spacemolt-kb/internal/frontier"

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
