package shipglyph

// Stats is the subset of catalog ship data that drives shape inference. It is
// deliberately decoupled from the catalog JSON structs so the package has no
// dependency on the generator.
type Stats struct {
	ID       string
	Name     string
	Class    string
	Category string
	Faction  string
	Tier     int
	Scale    int
	Weapon   int
	Defense  int
	Utility  int
	Cargo    int
}

// classArchetype maps catalog ship classes to shape families. Classes absent
// from this table fall back to "spine".
var classArchetype = map[string]string{
	"Liner": "needle", "Courier": "needle", "Shuttle": "needle",
	"Yacht": "needle", "Scout": "needle", "Explorer": "needle",

	"Cruiser": "spine", "Battlecruiser": "spine", "Dreadnought": "spine",
	"Command": "spine", "Patrol": "spine", "Assault": "spine",
	"Raider": "spine", "Interdictor": "spine",

	"Freighter": "slab", "Bulk Hauler": "slab", "Hazmat Freighter": "slab",

	"Tanker": "drum", "Refinery": "drum",
	"Gas Harvester": "drum", "Ice Harvester": "drum",

	"Miner": "rig", "Salvager": "rig",

	"Carrier": "rack", "Fleet Carrier": "rack",
	"Drone Carrier": "rack", "Logistics": "rack",

	"Fighter": "dart", "Heavy Fighter": "dart",

	"Research": "pod", "Intelligence": "pod", "Electronic Warfare": "pod",
}

// archetypeOf returns the shape family for a catalog ship class.
func archetypeOf(class string) string {
	if a, ok := classArchetype[class]; ok {
		return a
	}
	return "spine"
}

// archetypeAspect is the base length-to-beam ratio per family.
var archetypeAspect = map[string]float64{
	"needle": 6.5, "dart": 2.6, "spine": 3.6, "slab": 2.4,
	"drum": 2.8, "rig": 2.2, "rack": 2.9, "pod": 2.7,
}

// Infer builds a complete Descriptor from catalog stats alone. It always
// returns usable geometry, so a ship with no overlay and no lore still renders.
func Infer(s Stats) Descriptor {
	fam := archetypeOf(s.Class)

	// Larger hulls read as slightly stubbier; scale 3 is neutral.
	aspect := archetypeAspect[fam] * (1 - 0.06*float64(s.Scale-3))
	if aspect < 1.2 {
		aspect = 1.2
	}

	d := Descriptor{
		ID:         s.ID,
		Aspect:     aspect,
		Symmetry:   "bilateral",
		Hull:       archetypeHull(fam, s),
		Appendages: archetypeAppendages(fam, s),
		MountZones: archetypeZones(fam),
		Greeble:    greebleFor(s),
	}
	if s.Faction == "outerrim" || s.Faction == "pirate" {
		d.Symmetry = "asymmetric"
	}
	return d
}

// archetypeHull returns the hull part list for a shape family.
func archetypeHull(fam string, s Stats) []HullPart {
	switch fam {
	case "needle":
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.01}, {0.25, 0.06}, {0.5, 0.10}, {0.8, 0.07}, {1, 0.03}},
		}}
	case "dart":
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.03}, {0.35, 0.16}, {0.7, 0.20}, {1, 0.10}},
		}}
	case "spine":
		return []HullPart{
			{Kind: "beam", Span: [2]float64{0, 0.82},
				Points: [][2]float64{{0, 0.09}, {0.15, 0.20}, {0.55, 0.22}, {0.82, 0.18}}},
			{Kind: "engine_cone", Span: [2]float64{0.82, 1}, Bells: enginesFor(s)},
		}
	case "slab":
		return []HullPart{
			{Kind: "box", Span: [2]float64{0.08, 0.80}, Half: 0.26},
			{Kind: "engine_cone", Span: [2]float64{0.80, 1}, Bells: enginesFor(s)},
			{Kind: "open_frame", Span: [2]float64{0, 0.08}, Seat: s.Utility == 0},
		}
	case "drum":
		return []HullPart{
			{Kind: "drum", Span: [2]float64{0.14, 0.78}, Radius: 0.28},
			{Kind: "engine_cone", Span: [2]float64{0.78, 1}, Bells: enginesFor(s)},
			{Kind: "beam", Span: [2]float64{0, 0.14},
				Points: [][2]float64{{0, 0.05}, {0.14, 0.16}}},
		}
	case "rig":
		return []HullPart{
			{Kind: "box", Span: [2]float64{0.20, 0.78}, Half: 0.24},
			{Kind: "engine_cone", Span: [2]float64{0.78, 1}, Bells: enginesFor(s)},
			{Kind: "open_frame", Span: [2]float64{0, 0.20}},
		}
	case "rack":
		return []HullPart{
			{Kind: "bay_rack", Span: [2]float64{0.10, 0.84}, Cells: bayCells(s)},
			{Kind: "engine_cone", Span: [2]float64{0.84, 1}, Bells: enginesFor(s)},
		}
	case "pod":
		return []HullPart{
			{Kind: "disc", Span: [2]float64{0.05, 0.62}, Radius: 0.30},
			{Kind: "beam", Span: [2]float64{0.62, 1},
				Points: [][2]float64{{0.62, 0.14}, {1, 0.08}}},
		}
	default:
		return []HullPart{{
			Kind: "beam", Span: [2]float64{0, 1},
			Points: [][2]float64{{0, 0.10}, {0.5, 0.20}, {1, 0.12}},
		}}
	}
}

// archetypeAppendages returns the non-spine features for a shape family.
func archetypeAppendages(fam string, s Stats) []Appendage {
	switch fam {
	case "needle":
		return []Appendage{{Kind: "wing", At: 0.66, Sweep: 42, Span: 0.34, Side: "both"}}
	case "dart":
		return []Appendage{{Kind: "wing", At: 0.58, Sweep: 28, Span: 0.42, Side: "both"}}
	case "rig":
		return []Appendage{{Kind: "tow_arm", At: 0.16, Span: 0.20, Side: "both"}}
	case "rack":
		return []Appendage{{Kind: "drone_rack", At: 0.46, Span: 0.16, Side: "both"}}
	case "drum":
		return []Appendage{{Kind: "nacelle", At: 0.70, Span: 0.16, Side: "both"}}
	case "pod":
		return []Appendage{{Kind: "antenna_mast", At: 0.30, Span: 0.22, Side: "both"}}
	default:
		if s.Weapon >= 4 {
			return []Appendage{{Kind: "sponson", At: 0.50, Span: 0.12, Side: "both"}}
		}
		return nil
	}
}

// archetypeZones returns default hardpoint mount ranges for a shape family.
func archetypeZones(fam string) MountZones {
	switch fam {
	case "needle", "dart":
		return MountZones{
			Weapon:  [][2]float64{{0.10, 0.45}},
			Defense: [][2]float64{{0.35, 0.70}},
			Utility: [][2]float64{{0.55, 0.92}},
		}
	case "rack":
		return MountZones{
			Weapon:  [][2]float64{{0.08, 0.30}},
			Defense: [][2]float64{{0.20, 0.80}},
			Utility: [][2]float64{{0.30, 0.90}},
		}
	default:
		return MountZones{
			Weapon:  [][2]float64{{0.06, 0.40}},
			Defense: [][2]float64{{0.30, 0.72}},
			Utility: [][2]float64{{0.50, 0.94}},
		}
	}
}

// enginesFor returns a plausible engine nozzle count from hull scale.
func enginesFor(s Stats) int {
	n := 1 + s.Scale/2
	return min(max(n, 1), 4)
}

// bayCells returns the number of carrier bays from hull scale.
func bayCells(s Stats) int {
	n := 2 + s.Scale
	return min(max(n, 2), 7)
}

// greebleFor picks surface detail density from faction and scale.
func greebleFor(s Stats) string {
	switch s.Faction {
	case "outerrim", "pirate":
		return "heavy"
	case "nebula":
		return "none"
	}
	if s.Scale >= 4 {
		return "heavy"
	}
	return "light"
}
