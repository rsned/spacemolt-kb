package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/systemmap"
)

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
			Name: "White Dwarf (VII) - Smallest (6px)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "DA VII", Name: "Sirius B", PositionX: 0, PositionY: 0},
			},
		},

		// PLANET CLASSES
		"planet-terran": {
			ID:   "planet-terran",
			Name: "Terran World (blue-green #4a9c6d)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "terran", Name: "Earth-like", PositionX: 1, PositionY: 0},
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
		"planet-ice_giant": {
			ID:   "planet-ice_giant",
			Name: "Ice Giant (pale blue #9fc5e8)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "ice_giant", Name: "Neptune-like", PositionX: 15, PositionY: 0},
			},
		},
		"planet-gas_dwarf": {
			ID:   "planet-gas_dwarf",
			Name: "Gas Dwarf / Mini-Neptune (tan #d4b596)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "M3 V", Name: "Red Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "gas_dwarf", Name: "Sub-Neptune", PositionX: 0.5, PositionY: 0},
			},
		},
		"planet-rocky": {
			ID:   "planet-rocky",
			Name: "Rocky Planet (brown-gray #8b7355)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "K5 V", Name: "Orange Dwarf", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "rocky", Name: "Mercury-like", PositionX: 0.8, PositionY: 0},
			},
		},
		"planet-dwarf": {
			ID:   "planet-dwarf",
			Name: "Dwarf Planet (tan-brown #c4a484)",
			POIs: []systemmap.POI{
				{ID: "star", Type: "sun", Class: "G2 V", Name: "Sun", PositionX: 0, PositionY: 0},
				{ID: "p1", Type: "planet", Class: "dwarf", Name: "Pluto-like", PositionX: 6, PositionY: 0},
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
				{ID: "p3", Type: "planet", Class: "ice_giant", Name: "Ice Giant", PositionX: 4, PositionY: 0},
				{ID: "p4", Type: "planet", Class: "rocky", Name: "Rocky", PositionX: 0.3, PositionY: 0},
				{ID: "p5", Type: "planet", Class: "dwarf", Name: "Dwarf", PositionX: 8, PositionY: 0},
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
		f.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>%s</title>
<style>
body { background: #000; color: #d8dee9; font-family: sans-serif; padding: 20px; }
h1 { color: #88c0d0; }
.info { color: #64748b; font-size: 14px; margin: 20px 0; }
.map-container { padding: 20px; border: 1px solid #4c566a; border-radius: 8px; margin: 20px 0; }
</style>
</head>
<body>
<h1>%s</h1>
<div class="info">%s</div>
<div class="info"><strong>POIs:</strong></div>
`, sys.Name, sys.Name, sys.ID))

		// List POIs
		for _, poi := range sys.POIs {
			classInfo := poi.Class
			if classInfo == "" {
				classInfo = "<em>unclassified</em>"
			}
			f.WriteString(fmt.Sprintf("<div class=\"info\">  • %s (%s): %s</div>\n", poi.Name, poi.Type, classInfo))
		}

		// Add rendered map
		f.WriteString("<div class=\"map-container\">\n")
		mapSVG := systemmap.RenderSystemMap(sys, systems, false)
		f.WriteString(mapSVG)
		f.WriteString("</div>\n")

		f.WriteString("</body>\n</html>\n")
		f.Close()

		fmt.Printf("✓ %s\n", filename)
	}

	fmt.Printf("\nGenerated %d test files covering:\n", len(systems))
	fmt.Println("  • All 10 spectral types (O-Y) with various luminosity classes")
	fmt.Println("  • All 8 luminosity classes (Ia-VII)")
	fmt.Println("  • 8 planet classes (terran, jovian, ice_giant, gas_dwarf, rocky, dwarf, proto, captured)")
	fmt.Println("  • Edge cases (unclassified, invalid, compact formats, brown dwarf giants)")
	fmt.Println("  • Complex multi-planet systems")
	fmt.Println("\nOpen any HTML file in a browser to see the rendered system map!")
}
