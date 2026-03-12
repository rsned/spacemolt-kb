package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/systemmap"
)

// polarToXY converts polar coordinates (radius, angle in degrees) to Cartesian (x, y)
func polarToXY(radius, angleDegrees float64) (float64, float64) {
	angleRad := angleDegrees * math.Pi / 180.0
	return radius * math.Cos(angleRad), radius * math.Sin(angleRad)
}

func main() {
	// Define all sample systems demonstrating every classification type
	systems := map[string]*systemmap.System{
		// SPECTRAL TYPES (O-Y)
		"spectral-O": {
			ID:   "spectral-O",
			Name: "O-Type Blue Hypergiant (O3 Ia, >30,000K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "O3 Ia", Name: "Zeta Puppis", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-B": {
			ID:   "spectral-B",
			Name: "B-Type Blue-White Giant (B0 III, 10-30K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "B0 III", Name: "Rigel", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-A": {
			ID:   "spectral-A",
			Name: "A-Type White Main Sequence (A0 V, 7.5-10K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "A0 V", Name: "Sirius A", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-F": {
			ID:   "spectral-F",
			Name: "F-Type Yellow-White Subgiant (F5 IV, 6-7.5K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "F5 IV", Name: "Procyon", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-G": {
			ID:   "spectral-G",
			Name: "G-Type Yellow Main Sequence (G2 V, 5.2-6K) - Sun-like",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sol", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Earth", PositionX: 1, PositionY: 0},
			},
		},
		"spectral-G-subtype-gradient": {
			ID:   "spectral-G-subtype-gradient",
			Name: "G-Type Subtype Gradient (G0→G5→G9 showing temperature refinement)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G0 V", Name: "G0 (hottest G, blends toward F)", PositionX: -4, PositionY: 0},
				{ID: "star2", Type: "sun", Class: "G5 V", Name: "G5 (pure G yellow)", PositionX: 0, PositionY: 0},
				{ID: "star3", Type: "sun", Class: "G9 V", Name: "G9 (coolest G, blends toward K)", PositionX: 4, PositionY: 0},
			},
		},
		"spectral-K": {
			ID:   "spectral-K",
			Name: "K-Type Orange Main Sequence (K5 V, 3.7-5.2K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K5 V", Name: "Epsilon Eridani", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-M": {
			ID:   "spectral-M",
			Name: "M-Type Red Dwarf (M3 V, 2.4-3.7K) - Most Common",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M3 V", Name: "Proxima Centauri", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Proxima b", PositionX: 0.5, PositionY: 0},
			},
		},
		"spectral-L": {
			ID:   "spectral-L",
			Name: "L-Type Brown Dwarf (L5 V, 1.3-2.4K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "L5 V", Name: "Teide 1", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-T": {
			ID:   "spectral-T",
			Name: "T-Type Brown Dwarf (T8 V, 700-1,300K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "T8 V", Name: "Gliese 229B", PositionX: 0, PositionY: 0},
			},
		},
		"spectral-Y": {
			ID:   "spectral-Y",
			Name: "Y-Type Brown Dwarf (Y2 V, <700K)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "Y2 V", Name: "WISE 1828+2650", PositionX: 0, PositionY: 0},
			},
		},

		// WHITE DWARF SPECTRAL TYPES
		"wd-DA": {
			ID:   "wd-DA",
			Name: "White Dwarf DA (Hydrogen lines present)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DA", Name: "Sirius B (DA)", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DB": {
			ID:   "wd-DB",
			Name: "White Dwarf DB (Helium I lines)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DB", Name: "DB White Dwarf", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DC": {
			ID:   "wd-DC",
			Name: "White Dwarf DC (Continuous spectrum, no lines)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DC", Name: "Featureless WD", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DO": {
			ID:   "wd-DO",
			Name: "White Dwarf DO (Helium II lines)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DO", Name: "Hot DO White Dwarf", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DQ": {
			ID:   "wd-DQ",
			Name: "White Dwarf DQ (Carbon lines present)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DQ", Name: "DQ White Dwarf", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DZ": {
			ID:   "wd-DZ",
			Name: "White Dwarf DZ (Metal lines present)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DZ", Name: "DZ White Dwarf", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DX": {
			ID:   "wd-DX",
			Name: "White Dwarf DX (Unclear or unclassifiable spectrum)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DX", Name: "Mysterious WD", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DAP": {
			ID:   "wd-DAP",
			Name: "White Dwarf DAP (Magnetic with detectable polarization)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DAP", Name: "Magnetic DA (P)", PositionX: 0, PositionY: 0},
			},
		},
		"wd-DAV": {
			ID:   "wd-DAV",
			Name: "White Dwarf DAV (Variable - ZZ Ceti star)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DAV", Name: "ZZ Ceti Variable", PositionX: 0, PositionY: 0},
			},
		},

		// LUMINOSITY CLASSES (Ia-VII)
		"lum-Ia": {
			ID:   "lum-Ia",
			Name: "Hypergiant (Ia) - Largest (28px)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "O3 Ia", Name: "Eta Carinae", PositionX: 0, PositionY: 0},
			},
		},
		"lum-Ib": {
			ID:   "lum-Ib",
			Name: "Supergiant (Ib) - 24px",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M2 Ib", Name: "Betelgeuse", PositionX: 0, PositionY: 0},
			},
		},
		"lum-II": {
			ID:   "lum-II",
			Name: "Bright Giant (II) - 20px",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G8 II", Name: "Canopus", PositionX: 0, PositionY: 0},
			},
		},
		"lum-III": {
			ID:   "lum-III",
			Name: "Giant (III) - 16px",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K0 III", Name: "Arcturus", PositionX: 0, PositionY: 0},
			},
		},
		"lum-IV": {
			ID:   "lum-IV",
			Name: "Subgiant (IV) - 14px",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G8 IV", Name: "Procyon", PositionX: 0, PositionY: 0},
			},
		},
		"lum-V": {
			ID:   "lum-V",
			Name: "Main Sequence (V) - 10px baseline (90% of stars)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sol", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Earth", PositionX: 1, PositionY: 0},
				{ID: "p2", Type: "planet", Class: "jovian", Name: "Jupiter", PositionX: 5.2, PositionY: 0},
			},
		},
		"lum-VI": {
			ID:   "lum-VI",
			Name: "Subdwarf (VI) - 8px",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G7 VI", Name: "Mu Cassiopeiae", PositionX: 0, PositionY: 0},
			},
		},
		"lum-VII": {
			ID:   "lum-VII",
			Name: "White Dwarf (VII) - Smallest (6px) - Note: White dwarfs use D-prefix classification",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DA", Name: "Sirius B", PositionX: 0, PositionY: 0},
			},
		},

		// PLANET CLASSES (actual game types)
		"planet-arid": {
			ID:   "planet-arid",
			Name: "Arid World (dry, tan-brown #d4b596)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K5 V", Name: "Orange Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "arid", Name: "Arid World", PositionX: 1, PositionY: 0},
			},
		},
		"planet-glacial": {
			ID:   "planet-glacial",
			Name: "Glacial World (icy, blue-white #b0e0e6)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M3 V", Name: "Red Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "glacial", Name: "Glacial World", PositionX: 0.5, PositionY: 0},
			},
		},
		"planet-ice_world": {
			ID:   "planet-ice_world",
			Name: "Ice World (frozen, pale blue #9fc5e8)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "ice_world", Name: "Ice World", PositionX: 15, PositionY: 0},
			},
		},
		"planet-jovian": {
			ID:   "planet-jovian",
			Name: "Jovian Gas Giant with Rings (orange #e8a86c)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "jovian", Name: "Saturn-like", PositionX: 10, PositionY: 0},
			},
		},
		"planet-oceanic": {
			ID:   "planet-oceanic",
			Name: "Oceanic World (water world, deep blue #4a90d9)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "oceanic", Name: "Oceanic World", PositionX: 1.2, PositionY: 0},
			},
		},
		"planet-scorched": {
			ID:   "planet-scorched",
			Name: "Scorched World (burnt, dark red-orange #d46a4e)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "O3 V", Name: "Hot Blue Star", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "scorched", Name: "Scorched World", PositionX: 0.3, PositionY: 0},
			},
		},
		"planet-super_terran": {
			ID:   "planet-super_terran",
			Name: "Super Terran World (large earth-like, deep green #3d8b5d)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "super_terran", Name: "Super Earth", PositionX: 2, PositionY: 0},
			},
		},
		"planet-terran": {
			ID:   "planet-terran",
			Name: "Terran World (earth-like, blue-green #4a9c6d)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Earth-like", PositionX: 1, PositionY: 0},
			},
		},
		"planet-tundra": {
			ID:   "planet-tundra",
			Name: "Tundra World (cold, grayish-green #9cad8e)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K5 V", Name: "Orange Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "tundra", Name: "Tundra World", PositionX: 0.8, PositionY: 0},
			},
		},

		// EDGE CASES
		"edge-unclassified": {
			ID:   "edge-unclassified",
			Name: "Unclassified System (defaults)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "", Name: "Unknown Star", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "", Name: "Unknown Planet", PositionX: 2, PositionY: 0},
			},
		},
		"edge-invalid": {
			ID:   "edge-invalid",
			Name: "Invalid Classification (fallback to default)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "X9 Z", Name: "Invalid", PositionX: 0, PositionY: 0},
			},
		},
		"edge-compact": {
			ID:   "edge-compact",
			Name: "Compact Format 'G2V' (no space)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2V", Name: "Compact Sun", PositionX: 0, PositionY: 0},
			},
		},
		"edge-missing-lum": {
			ID:   "edge-missing-lum",
			Name: "Missing Luminosity 'M9' (defaults to V)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M9", Name: "M-Dwarf", PositionX: 0, PositionY: 0},
			},
		},
		"edge-brown-dwarf-giant": {
			ID:   "edge-brown-dwarf-giant",
			Name: "Brown Dwarf with Giant class (size: 8px fixed)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "L5 III", Name: "Impossible Object", PositionX: 0, PositionY: 0},
			},
		},

		// COMPLEX SYSTEMS
		"complex-triple": {
			ID:   "complex-triple",
			Name: "Complex Multi-Planet System",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Earth-like", PositionX: 0.5, PositionY: 0},
				{ID: "p2", Type: "planet", Class: "jovian", Name: "Gas Giant", PositionX: 2, PositionY: 0},
				{ID: "p3", Type: "planet", Class: "ice_world", Name: "Ice World", PositionX: 4, PositionY: 0},
				{ID: "p4", Type: "planet", Class: "arid", Name: "Arid", PositionX: 0.3, PositionY: 0},
				{ID: "p5", Type: "planet", Class: "oceanic", Name: "Oceanic", PositionX: 8, PositionY: 0},
			},
		},
		"complex-binary": {
			ID:   "complex-binary",
			Name: "Binary Star System (not rendered - both would be at origin)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Primary Sun", PositionX: 0, PositionY: 0},
				{ID: "star2", Type: "sun", Class: "DA VII", Name: "White Dwarf", PositionX: 0.1, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Circumbinary", PositionX: 3, PositionY: 0},
			},
		},

		// SOL SYSTEM (from get_system.sol.json)
		"sol": {
			ID:       "sol",
			Name:     "Sol — Maximum Security (empire capital)",
			Security: 100,
			Connections: []systemmap.Connection{
				{SystemID: "sirius", Name: "Sirius", Distance: 715},
				{SystemID: "alpha_centauri", Name: "Alpha Centauri", Distance: 279},
			},
			POIs: []systemmap.POI{
				{ID: "sol_star", Type: "sun", Class: "G2 V", Name: "Sol Star", PositionX: 0, PositionY: 0},
				{ID: "mercury", Type: "planet", Class: "rocky", Name: "Mercury", PositionX: -0.4, PositionY: 0.1},
				{ID: "venus", Type: "planet", Class: "rocky", Name: "Venus", PositionX: -0.3, PositionY: 0.6},
				{ID: "earth", Type: "planet", Class: "terran", Name: "Earth", PositionX: -0.6, PositionY: -0.8},
				{ID: "sol_central", Type: "station", Name: "Sol Central", PositionX: -0.3, PositionY: -1.1},
				{ID: "mars", Type: "planet", Class: "rocky", Name: "Mars", PositionX: 0.1, PositionY: 1.5},
				{ID: "main_belt", Type: "asteroid_belt", Name: "Main Belt", PositionX: 3, PositionY: 0},
				{ID: "jupiter", Type: "planet", Class: "jovian", Name: "Jupiter", PositionX: 3.3, PositionY: 3.9},
				{ID: "jovian_extraction_zone", Type: "gas_cloud", Name: "Jovian Extraction Zone", PositionX: 3.1, PositionY: 4.4},
				{ID: "saturn", Type: "planet", Class: "jovian", Name: "Saturn", PositionX: -5.1, PositionY: 2.9},
				{ID: "uranus", Type: "planet", Class: "ice_giant", Name: "Uranus", PositionX: -6, PositionY: -2.2},
				{ID: "neptune", Type: "planet", Class: "ice_giant", Name: "Neptune", PositionX: 4.3, PositionY: -5.1},
				{ID: "kuiper_ice_fields", Type: "ice_field", Name: "Kuiper Ice Fields", PositionX: 6.6, PositionY: 2.4},
				{ID: "phantom_wormhole", Type: "wormhole", Name: "Wormhole Anomaly I", PositionX: 2.6, PositionY: -3.4},
				{ID: "ex_wormhole", Type: "collapsed_wormhole", Name: "Faded Wormhole", PositionX: -2.6, PositionY: -3.4},
			},
		},

		// SHIP TRAFFIC
		"ship-traffic": {
			ID:       "ship-traffic",
			Name:     "High Security System with Ship Traffic (Security 75)",
			Security: 75,
			Connections: []systemmap.Connection{
				{SystemID: "neighbor-alpha", Name: "Alpha Gate", Distance: 12},
				{SystemID: "neighbor-beta", Name: "Beta Gate", Distance: 8},
			},
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sol Prime", PositionX: 0, PositionY: 0},
				{ID: "station1", Type: "station", Name: "Haven Station", PositionX: 2, PositionY: 1},
				{ID: "ab1", Type: "asteroid_belt", Name: "Ore Belt", PositionX: 5, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "New Eden", PositionX: 3, PositionY: -1},
			},
		},
		"ship-traffic-pirate": {
			ID:       "ship-traffic-pirate",
			Name:     "Dead-End Pirate Stronghold (Security 0)",
			Security: 0,
			Connections: []systemmap.Connection{
				{SystemID: "neighbor-alpha", Name: "Skull Gate", Distance: 20},
			},
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M3 V", Name: "Blood Eye", PositionX: 0, PositionY: 0},
			},
		},

		// BLACK HOLE FEEDING ON STAR
		"black-hole-feeding": {
			ID:   "black-hole-feeding",
			Name: "Black Hole Consuming a Star (animated accretion stream)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K5 V", Name: "Doomed Star", PositionX: 0, PositionY: 0},
				{ID: "bh1", Type: "black_hole", Name: "Maw of Darkness", PositionX: 0.5, PositionY: 0.25},
				{ID: "ab1", Type: "asteroid_belt", Name: "Debris Ring", PositionX: 3, PositionY: 0},
			},
		},

		// MATRIX TEST PATTERN - All combinations
		"matrix-all": {
			ID:   "matrix-all",
			Name: "MK Classification Matrix (80 stars: 10 spectral × 8 luminosity)",
			POIs: func() []systemmap.POI {
				spectrals := []string{"O", "B", "A", "F", "G", "K", "M", "L", "T", "Y"}
				luminosities := []string{"Ia", "Ib", "II", "III", "IV", "V", "VI", "VII"}

				var pois []systemmap.POI
				// Grid spacing: x from -18 to +18, y from -10 to +10
				xSpacing := 4.0
				ySpacing := 3.0
				xOffset := -18.0
				yOffset := 10.5

				for li, lum := range luminosities {
					for si, spec := range spectrals {
						class := fmt.Sprintf("%s %s", spec, lum)
						x := xOffset + float64(si)*xSpacing
						y := yOffset - float64(li)*ySpacing
						id := fmt.Sprintf("%s_%s", spec, lum)
						pois = append(pois, systemmap.POI{
							ID:         id,
							Type:       "sun",
							Class:      class,
							Name:       fmt.Sprintf("%s %s", spec, lum),
							PositionX:  x,
							PositionY:  y,
						})
					}
				}
				return pois
			}(),
		},

		// MARKERS GRID - All POI markers with subtypes in orbital layout
		"markers-all": {
			ID:   "markers-all",
			Name: "Resources & Structures (stations, asteroid belts, gas clouds, ice fields, nebulae, relics)",
			POIs: func() []systemmap.POI {
				var pois []systemmap.POI

				// Central star
				pois = append(pois, systemmap.POI{
					ID:   "sun1",
					Type: "sun",
					Class: "G2 V",
					Name: "Central Star",
					PositionX: 0,
					PositionY: 0,
				})

				// Asteroid belts at radii 2, 4, 6, 8
				abRadii := []float64{2, 4, 6, 8}
				abTypes := []struct{id, class, name string}{
					{"ab1", "metallic", "Metallic Belt"},
					{"ab2", "mixed", "Mixed Belt"},
					{"ab3", "carbonaceous", "Carbonaceous"},
					{"ab4", "icy", "Icy Belt"},
				}
				for i, ab := range abTypes {
					angle := float64(i) * 90.0 // Spread evenly: 0, 90, 180, 270 degrees
					x, y := polarToXY(abRadii[i], angle)
					pois = append(pois, systemmap.POI{
						ID:   ab.id,
						Type: "asteroid_belt",
						Class: ab.class,
						Name: ab.name,
						PositionX: x,
						PositionY: y,
					})
				}

				// Ice fields at radii 11 and 14
				iceTypes := []struct{id, class, name string; radius float64}{
					{"if1", "kuiper", "Kuiper Belt", 11},
					{"if2", "cometary", "Cometary", 14},
				}
				for i, ice := range iceTypes {
					angle := float64(i) * 180.0 // Opposite sides: 0, 180
					x, y := polarToXY(ice.radius, angle)
					pois = append(pois, systemmap.POI{
						ID:   ice.id,
						Type: "ice_field",
						Class: ice.class,
						Name: ice.name,
						PositionX: x,
						PositionY: y,
					})
				}

				// Gas clouds, nebula, and relics at radius 16 (7 items spread evenly)
				r16Items := []struct{id, ptype, class, name string}{
					{"gc1", "gas_cloud", "molecular_cloud", "Molecular Cloud"},
					{"gc2", "gas_cloud", "emission", "Emission Cloud"},
					{"gc3", "gas_cloud", "atmospheric", "Atmospheric"},
					{"nebula", "nebula", "stellar_nursery", "Stellar Nursery"},
					{"relic1", "relic", "derelict", "Derelict Ship"},
					{"relic2", "relic", "megastructure", "Megastructure"},
					{"relic3", "relic", "alien_artifact", "Alien Artifact"},
				}
				for i, item := range r16Items {
					angle := float64(i) * (360.0 / 7.0) // Evenly spaced: ~51.4 degrees apart
					x, y := polarToXY(16, angle)
					pois = append(pois, systemmap.POI{
						ID:   item.id,
						Type: item.ptype,
						Class: item.class,
						Name: item.name,
						PositionX: x,
						PositionY: y,
					})
				}

				// Add station
				pois = append(pois, systemmap.POI{
					ID:   "station",
					Type: "station",
					Name: "Space Station",
					PositionX: 0,
					PositionY: 10,
				})

				return pois
			}(),
		},

		// MARKERS GRID - All planet types
		"markers-planets": {
			ID:   "markers-planets",
			Name: "All Planet Types (12 classes with size variations)",
			POIs: func() []systemmap.POI {
				var pois []systemmap.POI

				// Grid layout: 3 rows × 4 columns
				xSpacing := 10.0
				ySpacing := 7.0
				xOffset := -15.0
				yOffset := 7.0

				// Row 1: Small planets (4)
				pois = append(pois, systemmap.POI{
					ID: "p_scorched", Type: "planet", Class: "scorched", Name: "Scorched (3px)", PositionX: xOffset, PositionY: yOffset,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_arid", Type: "planet", Class: "arid", Name: "Arid (4.5px)", PositionX: xOffset + xSpacing, PositionY: yOffset,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_tundra", Type: "planet", Class: "tundra", Name: "Tundra (4.5px)", PositionX: xOffset + 2*xSpacing, PositionY: yOffset,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_lava", Type: "planet", Class: "lava_world", Name: "Lava World (4px)", PositionX: xOffset + 3*xSpacing, PositionY: yOffset,
				})

				// Row 2: Medium planets (4)
				planet2Y := yOffset - ySpacing
				pois = append(pois, systemmap.POI{
					ID: "p_terran", Type: "planet", Class: "terran", Name: "Terran (5px)", PositionX: xOffset, PositionY: planet2Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_oceanic", Type: "planet", Class: "oceanic", Name: "Oceanic (5px)", PositionX: xOffset + xSpacing, PositionY: planet2Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_glacial", Type: "planet", Class: "glacial", Name: "Glacial (5px)", PositionX: xOffset + 2*xSpacing, PositionY: planet2Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_hothouse", Type: "planet", Class: "hothouse", Name: "Hothouse (5px)", PositionX: xOffset + 3*xSpacing, PositionY: planet2Y,
				})

				// Row 3: Large planets (4)
				planet3Y := yOffset - 2*ySpacing
				pois = append(pois, systemmap.POI{
					ID: "p_ice_world", Type: "planet", Class: "ice_world", Name: "Ice World (5.5px)", PositionX: xOffset, PositionY: planet3Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_super_terran", Type: "planet", Class: "super_terran", Name: "Super Terran (6.25px)", PositionX: xOffset + xSpacing, PositionY: planet3Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_ice_giant", Type: "planet", Class: "ice_giant", Name: "Ice Giant (7px)", PositionX: xOffset + 2*xSpacing, PositionY: planet3Y,
				})
				pois = append(pois, systemmap.POI{
					ID: "p_jovian", Type: "planet", Class: "jovian", Name: "Jovian (10px)", PositionX: xOffset + 3*xSpacing, PositionY: planet3Y,
				})

				return pois
			}(),
		},

		// MARKERS GRID - Anomalies (wormholes, black holes)
		"markers-anomalies": {
			ID:   "markers-anomalies",
			Name: "Cosmic Anomalies (wormholes, black holes, neutron stars)",
			POIs: func() []systemmap.POI {
				var pois []systemmap.POI

				// Grid layout: 2 rows × 4 columns
				xSpacing := 10.0
				ySpacing := 8.0
				xOffset := -15.0
				yOffset := 7.0

				// Row 1: Wormholes (4)
				pois = append(pois, systemmap.POI{
					ID: "wh1", Type: "wormhole", Name: "Active Wormhole", PositionX: xOffset, PositionY: yOffset,
				})
				pois = append(pois, systemmap.POI{
					ID: "wh2", Type: "collapsed_wormhole", Name: "Collapsed Wormhole", PositionX: xOffset + xSpacing, PositionY: yOffset,
				})
				pois = append(pois, systemmap.POI{
					ID: "star1", Type: "sun", Class: "G2 V", Name: "Nearby Star", PositionX: xOffset + 2*xSpacing, PositionY: yOffset - 2,
				})
				pois = append(pois, systemmap.POI{
					ID: "star2", Type: "sun", Class: "M3 V", Name: "Red Dwarf", PositionX: xOffset + 3*xSpacing, PositionY: yOffset - 2,
				})

				// Row 2: Black holes + Neutron star (4)
				anomalyY := yOffset - ySpacing
				pois = append(pois, systemmap.POI{
					ID: "bh1", Type: "black_hole", Name: "Black Hole", PositionX: xOffset, PositionY: anomalyY,
				})
				pois = append(pois, systemmap.POI{
					ID: "star3", Type: "sun", Class: "G2 V", Name: "Nearby Star", PositionX: xOffset + xSpacing, PositionY: anomalyY - 2,
				})
				pois = append(pois, systemmap.POI{
					ID: "ns1", Type: "sun", Class: "NS", Name: "Neutron Star", PositionX: xOffset + 2*xSpacing, PositionY: anomalyY - 2,
				})
				pois = append(pois, systemmap.POI{
					ID: "star4", Type: "sun", Class: "DA", Name: "White Dwarf", PositionX: xOffset + 3*xSpacing, PositionY: anomalyY - 2,
				})

				return pois
			}(),
		},

		// NEW PLANET TYPES
		"planet-hothouse": {
			ID:   "planet-hothouse",
			Name: "Hothouse World (extreme greenhouse, bright green #58c868)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "F5 V", Name: "Hot Star", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "hothouse", Name: "Hothouse World", PositionX: 0.7, PositionY: 0},
			},
		},
		"planet-lava_world": {
			ID:   "planet-lava_world",
			Name: "Lava World (molten, bright red-orange #ff4500)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M2 V", Name: "Red Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "lava_world", Name: "Lava World", PositionX: 0.2, PositionY: 0},
			},
		},
		"planet-ice_giant": {
			ID:   "planet-ice_giant",
			Name: "Ice Giant (pale blue #9fc5e8, 1.4x)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "ice_giant", Name: "Ice Giant", PositionX: 15, PositionY: 0},
			},
		},

		// POI SUBTYPES
		"asteroid-metallic": {
			ID:   "asteroid-metallic",
			Name: "Metallic Asteroid Belt (silver-gray, most common)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "ab1", Type: "asteroid_belt", Class: "metallic", Name: "Metallic Belt", PositionX: 3, PositionY: 0},
			},
		},
		"asteroid-carbonaceous": {
			ID:   "asteroid-carbonaceous",
			Name: "Carbonaceous Asteroid Belt (dark brown)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "ab1", Type: "asteroid_belt", Class: "carbonaceous", Name: "Carbonaceous Belt", PositionX: 4, PositionY: 0},
			},
		},
		"gas-molecular": {
			ID:   "gas-molecular",
			Name: "Molecular Cloud (purple, dense)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "gc1", Type: "gas_cloud", Class: "molecular_cloud", Name: "Molecular Cloud", PositionX: 5, PositionY: 2},
			},
		},
		"gas-emission": {
			ID:   "gas-emission",
			Name: "Emission Nebula (reddish)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "O3 V", Name: "Hot Star", PositionX: 0, PositionY: 0},
				{ID: "gc1", Type: "gas_cloud", Class: "emission", Name: "Emission Cloud", PositionX: 3, PositionY: -1},
			},
		},
		"ice-kuiper": {
			ID:   "ice-kuiper",
			Name: "Kuiper Belt (pale cyan)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "if1", Type: "ice_field", Class: "kuiper", Name: "Kuiper Belt", PositionX: 8, PositionY: 0},
			},
		},
		"relic-derelict": {
			ID:   "relic-derelict",
			Name: "Derelict Ship (brown hull)",
			POIs: []systemmap.POI{
				{ID: "relic1", Type: "relic", Class: "derelict", Name: "Abandoned Ship", PositionX: 2, PositionY: 1},
			},
		},
		"relic-megastructure": {
			ID:   "relic-megastructure",
			Name: "Megastructure (gold ring)",
			POIs: []systemmap.POI{
				{ID: "relic1", Type: "relic", Class: "megastructure", Name: "Dyson Swarm", PositionX: -3, PositionY: 2},
			},
		},
		"nebula-stellar": {
			ID:   "nebula-stellar",
			Name: "Stellar Nursery Nebula (pink hydrogen emission)",
			POIs: []systemmap.POI{
				{ID: "neb1", Type: "nebula", Class: "stellar_nursery", Name: "Rose Nebula", PositionX: 5, PositionY: 3},
			},
		},

		// BELT ROTATION - Demonstrates animated resource belt orbits
		"belt-rotation": {
			ID:   "belt-rotation",
			Name: "Belt Rotation (inner belt 3AU=1800s, outer belts same linear speed)",
			POIs: func() []systemmap.POI {
				var pois []systemmap.POI

				// Central star
				pois = append(pois, systemmap.POI{
					ID: "sun1", Type: "sun", Class: "G2 V", Name: "Central Star",
					PositionX: 0, PositionY: 0,
				})

				// Inner planet for reference
				pois = append(pois, systemmap.POI{
					ID: "p1", Type: "planet", Class: "terran", Name: "Inner World",
					PositionX: 1.5, PositionY: 0,
				})

				// Asteroid belts at 3, 5, 7 AU — inner belt sets the speed
				// Inner: 1800s, Middle: 1800*5/3=3000s, Outer: 1800*7/3=4200s
				abx3, aby3 := polarToXY(3, 45)
				pois = append(pois, systemmap.POI{
					ID: "ab_inner", Type: "asteroid_belt", Class: "metallic",
					Name: "Inner Belt (3 AU, 1800s)",
					PositionX: abx3, PositionY: aby3,
				})

				abx5, aby5 := polarToXY(5, 160)
				pois = append(pois, systemmap.POI{
					ID: "ab_mid", Type: "asteroid_belt", Class: "mixed",
					Name: "Middle Belt (5 AU, 3000s)",
					PositionX: abx5, PositionY: aby5,
				})

				abx7, aby7 := polarToXY(7, 280)
				pois = append(pois, systemmap.POI{
					ID: "ab_outer", Type: "asteroid_belt", Class: "carbonaceous",
					Name: "Outer Belt (7 AU, 4200s)",
					PositionX: abx7, PositionY: aby7,
				})

				// Ice field at 10 AU — same linear speed, 1800*10/3=6000s
				icex, icey := polarToXY(10, 90)
				pois = append(pois, systemmap.POI{
					ID: "if1", Type: "ice_field", Class: "cometary",
					Name: "Kuiper Ring (10 AU, 6000s)",
					PositionX: icex, PositionY: icey,
				})

				// Station for visual reference (static)
				pois = append(pois, systemmap.POI{
					ID: "st1", Type: "station", Name: "Orbital Station",
					PositionX: 2, PositionY: -1,
				})

				return pois
			}(),
		},
	}

	// Dummy neighbor systems for gate angle computation in ship traffic tests.
	systems["neighbor-alpha"] = &systemmap.System{ID: "neighbor-alpha", Name: "Alpha", PositionX: 10, PositionY: 5}
	systems["neighbor-beta"] = &systemmap.System{ID: "neighbor-beta", Name: "Beta", PositionX: -8, PositionY: 3}
	systems["sirius"] = &systemmap.System{ID: "sirius", Name: "Sirius", PositionX: 6, PositionY: -3}
	systems["alpha_centauri"] = &systemmap.System{ID: "alpha_centauri", Name: "Alpha Centauri", PositionX: -4, PositionY: 7}

	fmt.Printf("Generating %d MK classification test systems...\n\n", len(systems))

	// Generate HTML files (skip dummy neighbor systems).
	for sysID, sys := range systems {
		if strings.HasPrefix(sysID, "neighbor-") || sysID == "sirius" || sysID == "alpha_centauri" {
			continue
		}
		filename := fmt.Sprintf("test-%s.html", sysID)
		f, _ := os.Create(filename)

		// Write HTML header
		_, _ = fmt.Fprintf(f, `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>%s</title>
<style>
body { background: #000; color: #d8dee9; font-family: sans-serif; padding: 20px; }
h1 { color: #88c0d0; }
.info { color: #64748b; font-size: 14px; margin: 20px 0; }
.map-container { padding: 20px; border: 1px solid #4c566a; border-radius: 8px; margin: 20px 0; }
.map-label { font: 11px sans-serif; fill: #ffffff; }
</style>
</head>
<body>
<h1>%s</h1>
<div class="info">%s</div>
<div class="info"><strong>POIs:</strong></div>
`, sys.Name, sys.Name, sys.ID)

		// List POIs
		for _, poi := range sys.POIs {
			classInfo := poi.Class
			if classInfo == "" {
				classInfo = "<em>unclassified</em>"
			}
			_, _ = fmt.Fprintf(f, "<div class=\"info\">  • %s (%s): %s</div>\n", poi.Name, poi.Type, classInfo)
		}

		// Add rendered map
		_, _ = f.WriteString("<div class=\"map-container\">\n")
		mapSVG := systemmap.RenderSystemMap(sys, systems, false)
		_, _ = f.WriteString(mapSVG)
		_, _ = f.WriteString("</div>\n")

		_, _ = f.WriteString("</body>\n</html>\n")
		_ = f.Close()

		fmt.Printf("✓ %s\n", filename)
	}

	fmt.Printf("\nGenerated %d test files covering:\n", len(systems))
	fmt.Println("  • All 10 spectral types (O-Y) with various luminosity classes")
	fmt.Println("  • All 8 luminosity classes (Ia-VII)")
	fmt.Println("  • All 9 white dwarf spectral types (DA, DB, DC, DO, DQ, DZ, DX) with secondary features (P, H, E, V)")
	fmt.Println("  • All 12 planet classes (arid, glacial, hothouse, ice_giant, ice_world, jovian, lava_world, oceanic, scorched, super_terran, terran, tundra)")
	fmt.Println("  • Asteroid belt subtypes (metallic, mixed, carbonaceous, icy)")
	fmt.Println("  • Gas cloud subtypes (molecular_cloud, emission, atmospheric)")
	fmt.Println("  • Ice field subtypes (kuiper, cometary)")
	fmt.Println("  • Relic subtypes (derelict, megastructure, alien_artifact)")
	fmt.Println("  • Nebula types (stellar_nursery)")
	fmt.Println("  • All POI markers in grid layout (stars, planets, stations, anomalies)")
	fmt.Println("  • Edge cases (unclassified, invalid, compact formats, brown dwarf giants)")
	fmt.Println("  • Complex multi-planet systems")
	fmt.Println("\nOpen any HTML file in a browser to see the rendered system map!")
}
