package frontier

import "github.com/rsned/spacemolt-kb/pkg/galaxymap"

// PlaceMapSystems readies galaxy-map systems for drawing: a system with an
// estimate takes it, and a system other than Sol still at (0,0) has no known
// position, so it is dropped (with every lane to it) rather than drawn on top
// of Sol. byID loses the dropped systems too. It returns the kept systems.
func PlaceMapSystems(systems []*galaxymap.System, byID map[string]*galaxymap.System, est map[string]Estimate) []*galaxymap.System {
	dropped := map[string]bool{}
	kept := systems[:0:0]
	for _, s := range systems {
		if e, ok := est[s.ID]; ok {
			s.PositionX, s.PositionY = e.X, e.Y
		} else if s.PositionX == 0 && s.PositionY == 0 && s.ID != "sol" {
			dropped[s.ID] = true
			delete(byID, s.ID)
			continue
		}
		kept = append(kept, s)
	}
	if len(dropped) > 0 {
		for _, s := range kept {
			conns := s.Connections[:0]
			for _, c := range s.Connections {
				if !dropped[c.SystemID] {
					conns = append(conns, c)
				}
			}
			s.Connections = conns
		}
	}
	return kept
}
