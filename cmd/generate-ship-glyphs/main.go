// Command generate-ship-glyphs renders a top-down blueprint SVG for every ship
// in the catalog, plus a contact sheet page for reviewing them all at once.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

// renderedGlyph pairs a ship's stats with its finished SVG markup. Task 10's
// contact sheet consumes these.
type renderedGlyph struct {
	Stats shipglyph.Stats
	SVG   string
}

func main() {
	catalogPath := flag.String("catalog", "../spacemolt/data/game-api/latest/catalog_ships.json",
		"path to catalog_ships.json")
	overlayDir := flag.String("overlays", "overlays/shipshapes",
		"directory of hand-authored shape overlays (empty to disable)")
	outDir := flag.String("out", "kb/ships/glyphs", "output directory for glyph SVGs")
	size := flag.Float64("size", 200, "glyph viewBox edge length")
	flag.Parse()

	ships, err := loadShipCatalog(*catalogPath)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	rendered := make([]renderedGlyph, 0, len(ships))
	var overlaid int

	for _, c := range ships {
		s := toStats(c)
		d := shipglyph.Infer(s)

		over, ok, err := shipglyph.LoadOverlay(*overlayDir, c.ID)
		if err != nil {
			log.Fatalf("overlay for %s: %v", c.ID, err)
		}
		if ok {
			d = shipglyph.Merge(d, over)
			overlaid++
		}

		svg := shipglyph.Render(d, s, shipglyph.Options{
			Size:           *size,
			ShowHardpoints: true,
			Title:          c.Name,
		})

		path := filepath.Join(*outDir, c.ID+".svg")
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
		rendered = append(rendered, renderedGlyph{Stats: s, SVG: svg})
	}

	fmt.Printf("Generated %d ship glyphs (%d with overlays) in %s/\n",
		len(rendered), overlaid, *outDir)
}
