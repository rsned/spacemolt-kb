package main

import "github.com/rsned/spacemolt-kb/internal/frontier"

// catalogDir is the newest game-api scrape, for the list of charted systems.
const catalogDir = "../spacemolt/data/game-api/latest"

// placeFrontier puts uncharted frontier systems where they belong. The
// database holds them at (0,0) -- Sol's spot -- because neither get_map nor
// get_system reports a position for them. Those with an estimate take it;
// those without are dropped, along with every connection to them. It returns
// the kept systems and the set of frontier system IDs (for the pink dots).
// With no map scrape (charted empty) nothing is treated as frontier.
func placeFrontier(systems []*System, charted map[string]bool, est map[string]frontier.Estimate) ([]*System, map[string]bool) {
	ids := map[string]bool{}
	if len(charted) == 0 {
		return systems, ids
	}
	dropped := map[string]bool{}
	kept := make([]*System, 0, len(systems))
	for _, s := range systems {
		if charted[s.ID] {
			kept = append(kept, s)
			continue
		}
		e, ok := est[s.ID]
		if !ok {
			dropped[s.ID] = true
			continue
		}
		s.PositionX, s.PositionY = e.X, e.Y
		ids[s.ID] = true
		kept = append(kept, s)
	}
	for _, s := range kept {
		conns := s.Connections[:0]
		for _, c := range s.Connections {
			if !dropped[c.SystemID] {
				conns = append(conns, c)
			}
		}
		s.Connections = conns
	}
	return kept, ids
}
