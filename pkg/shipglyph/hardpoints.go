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
// zones. Markers are placed in mirrored pairs: each station along the zone
// carries one starboard and one port marker at the same longitudinal
// position, so a pair reads as a single symmetric mount. Markers are inset
// from the hull edge so they always sit inside the outline.
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

// placeKind spreads n markers evenly across the given zones as mirrored
// pairs. n markers occupy ceil(n/2) stations; every station but a trailing
// odd one carries a starboard and a port marker at the same X. An odd
// leftover marker takes the final station on the centerline.
//
// Stations are spread across the concatenated zones: consecutive stations
// walk the zone list, and each zone is subdivided among the stations that
// land in it, so multiple zones per kind stay in longitudinal order.
func placeKind(d Descriptor, kind, prefix string, n int, zones [][2]float64) []Hardpoint {
	if n <= 0 || len(zones) == 0 {
		return nil
	}
	stations := (n + 1) / 2
	out := make([]Hardpoint, 0, n)
	for i := range n {
		j := i / 2
		t := stationAt(zones, j, stations)

		// The trailing marker of an odd count has no partner, so it sits on
		// the centerline rather than off to one side.
		y := 0.0
		if i+1 < n || n%2 == 0 {
			side := 1.0
			if i%2 == 1 {
				side = -1.0
			}
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

// stationAt returns the longitudinal position of station j of total across
// the given zones. Stations are dealt to zones in order, so zone z takes
// every station whose index maps to it, and those stations spread evenly
// within that zone.
func stationAt(zones [][2]float64, j, total int) float64 {
	nz := len(zones)
	zi := min(j*nz/total, nz-1)
	z := zones[zi]

	// Stations k with k*nz/total == zi belong to this zone; that is the
	// half-open index range [ceil(zi*total/nz), ceil((zi+1)*total/nz)).
	first := (zi*total + nz - 1) / nz
	count := ((zi+1)*total+nz-1)/nz - first
	if count <= 1 {
		return z[0] + 0.5*(z[1]-z[0])
	}
	f := float64(j-first) / float64(count-1)
	return z[0] + f*(z[1]-z[0])
}
