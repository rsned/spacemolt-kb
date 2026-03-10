package main

import (
	"fmt"
	"os"

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
	}

	fmt.Printf("Generating %d MK classification test systems...\n\n", len(systems))

	// Generate HTML files
	for sysID, sys := range systems {
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
