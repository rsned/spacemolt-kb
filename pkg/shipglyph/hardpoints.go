package shipglyph

import "fmt"

// Hardpoint is a module mounting marker on the hull. Slot counts are
// authoritative from the catalog; the descriptor only supplies the zones
// along which markers may be placed.
type Hardpoint struct {
	// ID is the stable SVG element ID, e.g. "hp-w1".
	ID string
	// Kind is "weapon", "defense" or "utility".
	Kind string
	// Pos is the marker position in glyph space.
	Pos Point
}

// Hardpoints distributes one marker per slot along the descriptor's mount
// zones. Markers alternate starboard and port so pairs read as symmetric
// mounts, and are inset from the hull edge so they always sit inside the
// outline.
func Hardpoints(d Descriptor, s Stats) []Hardpoint {
	var out []Hardpoint
	out = append(out, placeKind(d, "weapon", "w", s.Weapon, d.MountZones.Weapon)...)
	out = append(out, placeKind(d, "defense", "d", s.Defense, d.MountZones.Defense)...)
	out = append(out, placeKind(d, "utility", "u", s.Utility, d.MountZones.Utility)...)
	return out
}

// hardpointInset keeps markers clear of the hull edge, as a fraction of the
// local half-width.
const hardpointInset = 0.55

// placeKind spreads n markers evenly across the given zones.
func placeKind(d Descriptor, kind, prefix string, n int, zones [][2]float64) []Hardpoint {
	if n <= 0 || len(zones) == 0 {
		return nil
	}
	out := make([]Hardpoint, 0, n)
	for i := range n {
		// Spread across the concatenated zones by index.
		z := zones[i%len(zones)]
		var f float64
		if n == 1 {
			f = 0.5
		} else {
			f = float64(i) / float64(n-1)
		}
		t := z[0] + f*(z[1]-z[0])

		side := 1.0
		if i%2 == 1 {
			side = -1.0
		}
		// A lone marker sits on the centerline rather than off to one side.
		y := 0.0
		if n > 1 {
			y = side * hullHalfWidth(d, t) * hardpointInset
		}

		out = append(out, Hardpoint{
			ID:   fmt.Sprintf("hp-%s%d", prefix, i+1),
			Kind: kind,
			Pos:  Point{X: t, Y: y},
		})
	}
	return out
}
