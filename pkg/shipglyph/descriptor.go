package shipglyph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// HullPart is one section of the hull along the spine. Kind selects which
// geometry routine interprets the remaining fields; unused fields are zero.
type HullPart struct {
	// Kind is one of: beam, box, container_stack, bay_rack, engine_cone,
	// open_frame, drum, disc, lobe_cluster.
	Kind string `json:"kind"`
	// Span is the [start, end] spine range this part occupies, in t.
	Span [2]float64 `json:"span"`
	// Points are [t, halfwidth] control points, used by kind "beam".
	Points [][2]float64 `json:"points,omitempty"`
	// Grid is [across, deep] container counts, used by kind "container_stack".
	Grid [2]int `json:"grid,omitempty"`
	// Cells is the number of bays, used by kind "bay_rack".
	Cells int `json:"cells,omitempty"`
	// Bells is the number of engine nozzles, used by kind "engine_cone".
	Bells int `json:"bells,omitempty"`
	// Lobes is the number of organic lobes, used by kind "lobe_cluster".
	Lobes int `json:"lobes,omitempty"`
	// Radius is the half-width, used by kinds "drum" and "disc".
	Radius float64 `json:"radius,omitempty"`
	// Half is the constant half-width, used by kind "box".
	Half float64 `json:"half,omitempty"`
	// Seat marks an exposed pilot seat, used by kind "open_frame".
	Seat bool `json:"seat,omitempty"`
}

// Appendage is a feature attached to the hull rather than part of the spine.
type Appendage struct {
	// Kind is one of: wing, sponson, nacelle, outrigger, boom, drone_rack,
	// tow_arm, antenna_mast.
	Kind string `json:"kind"`
	// At is the spine position, in t, where the appendage attaches.
	At float64 `json:"at"`
	// Sweep is the trailing sweep angle in degrees, for wings.
	Sweep float64 `json:"sweep,omitempty"`
	// Span is how far the appendage extends from the hull, in y units.
	Span float64 `json:"span,omitempty"`
	// Side is "both", "port" or "starboard". Empty means "both".
	Side string `json:"side,omitempty"`
}

// MountZones are the spine ranges along which hardpoint markers of each slot
// type may be placed. Each entry is a [start, end] range in t.
type MountZones struct {
	Weapon  [][2]float64 `json:"weapon,omitempty"`
	Defense [][2]float64 `json:"defense,omitempty"`
	Utility [][2]float64 `json:"utility,omitempty"`
}

// Descriptor is a complete declarative description of a ship's top-down shape.
// Every field is optional; absent fields are supplied by the inferred layer.
type Descriptor struct {
	ID string `json:"id,omitempty"`
	// Aspect is length divided by maximum beam.
	Aspect float64 `json:"aspect,omitempty"`
	// Symmetry is "bilateral", "asymmetric" or "radial".
	Symmetry   string      `json:"symmetry,omitempty"`
	Hull       []HullPart  `json:"hull,omitempty"`
	Appendages []Appendage `json:"appendages,omitempty"`
	MountZones MountZones  `json:"mountZones,omitempty"`
	// Greeble is "none", "light" or "heavy" surface detail density.
	Greeble string `json:"greeble,omitempty"`
}

// Merge overlays over onto base field by field. A field set in over replaces
// the corresponding field in base; a zero-valued field in over leaves base
// untouched. Slice fields are replaced wholesale, never appended, so an
// overlay that specifies Hull fully controls the hull.
func Merge(base, over Descriptor) Descriptor {
	out := base
	if over.ID != "" {
		out.ID = over.ID
	}
	if over.Aspect != 0 {
		out.Aspect = over.Aspect
	}
	if over.Symmetry != "" {
		out.Symmetry = over.Symmetry
	}
	if len(over.Hull) > 0 {
		out.Hull = over.Hull
	}
	if len(over.Appendages) > 0 {
		out.Appendages = over.Appendages
	}
	if len(over.MountZones.Weapon) > 0 {
		out.MountZones.Weapon = over.MountZones.Weapon
	}
	if len(over.MountZones.Defense) > 0 {
		out.MountZones.Defense = over.MountZones.Defense
	}
	if len(over.MountZones.Utility) > 0 {
		out.MountZones.Utility = over.MountZones.Utility
	}
	if over.Greeble != "" {
		out.Greeble = over.Greeble
	}
	return out
}

// LoadOverlay reads dir/<id>.json. It reports ok=false with a nil error when
// no overlay exists for the ship, which is the common case.
func LoadOverlay(dir, id string) (Descriptor, bool, error) {
	var d Descriptor
	if dir == "" {
		return d, false, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return d, false, nil
	}
	if err != nil {
		return d, false, fmt.Errorf("read overlay for %q: %w", id, err)
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return d, false, fmt.Errorf("parse overlay for %q: %w", id, err)
	}
	return d, true, nil
}
