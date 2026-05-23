package hyperjump

import "math"

// SweepRange is a contiguous run of whole-degree headings from an origin that all
// land on the same system (or all miss, when LandsAt is empty). It is the
// collapsed form of a full 0..359 heading sweep.
type SweepRange struct {
	StartDeg       int     `json:"startDeg"`       // inclusive
	EndDeg         int     `json:"endDeg"`         // inclusive
	LandsAt        string  `json:"landsAt"`        // system id, or "" for void
	LandsAtStation bool    `json:"landsAtStation"` // landing system has a station
	Distance       float64 `json:"distance"`       // travel proj at mid-arc heading (0 if void)
	Ticks          int     `json:"ticks"`          // ceil(Distance/10), 0 if void
}

// HeadingSweep evaluates every whole-degree heading (0..359) from origin and
// returns the landing system for each, collapsed into contiguous ranges. A range
// with an empty LandsAt is a void window (the jump intersects nothing). Distance
// and Ticks are taken at the range's mid-arc heading. Headings are not merged
// across the 0/360 boundary.
func HeadingSweep(origin System, systems []System, margin float64) []SweepRange {
	type cell struct {
		id      string
		station bool
		proj    float64
	}
	cells := make([]cell, 360)
	for d := range 360 {
		heading := float64(d)
		var (
			bestID      string
			bestProj    float64
			bestStation bool
			have        bool
		)
		for _, s := range systems {
			if s.ID == origin.ID {
				continue
			}
			rel := Vec{s.Pos.X - origin.Pos.X, s.Pos.Y - origin.Pos.Y}
			proj := Proj(rel, heading)
			if proj <= 0 {
				continue
			}
			if math.Abs(SignedPerp(rel, heading)) > margin {
				continue
			}
			if !have || proj < bestProj {
				have, bestID, bestProj, bestStation = true, s.ID, proj, s.HasStation
			}
		}
		cells[d] = cell{bestID, bestStation, bestProj}
	}

	var out []SweepRange
	for i := 0; i < 360; {
		j := i
		for j+1 < 360 && cells[j+1].id == cells[i].id {
			j++
		}
		r := SweepRange{StartDeg: i, EndDeg: j, LandsAt: cells[i].id, LandsAtStation: cells[i].station}
		if cells[i].id != "" {
			r.Distance = cells[(i+j)/2].proj
			r.Ticks = int(math.Ceil(r.Distance / 10))
		}
		out = append(out, r)
		i = j + 1
	}
	return out
}
