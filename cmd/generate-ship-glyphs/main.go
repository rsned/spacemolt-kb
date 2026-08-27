// Command generate-ship-glyphs renders a top-down blueprint SVG for every ship
// in the catalog, plus a contact sheet page for reviewing them all at once.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/shipglyph"
)

// renderedGlyph pairs a ship's stats with its finished SVG markup. Task 10's
// contact sheet consumes these.
type renderedGlyph struct {
	Stats  shipglyph.Stats
	SVG    string
	Legacy bool
}

func main() {
	catalogPath := flag.String("catalog", "../spacemolt/data/game-api/latest/catalog_ships.json",
		"path to catalog_ships.json")
	overlayDir := flag.String("overlays", "overlays/shipshapes",
		"directory of hand-authored shape overlays (empty to disable)")
	legacyPath := flag.String("legacy", legacyShipsOverlay,
		"discontinued-hull overlay to merge into the catalog (empty to disable)")
	outDir := flag.String("out", "kb/ships/glyphs", "output directory for glyph SVGs")
	size := flag.Float64("size", 200, "glyph viewBox edge length")
	flag.Parse()

	ships, err := loadShipCatalog(*catalogPath)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if *legacyPath != "" {
		ships = appendLegacyShips(ships, *legacyPath)
	}
	if err := validateCatalog(ships); err != nil {
		log.Fatalf("validate catalog: %v", err)
	}
	if err := cleanGeneratedFiles(*outDir); err != nil {
		log.Fatalf("clean output dir: %v", err)
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

		// The contact sheet inlines every ship's SVG into one page, so its
		// copy needs IDs scoped per-ship to avoid colliding with the other
		// 334 ships. The standalone file above keeps plain IDs, which are
		// the contract a future dashboard selects on.
		sheetSVG := shipglyph.Render(d, s, shipglyph.Options{
			Size:           *size,
			ShowHardpoints: true,
			Title:          c.Name,
			IDPrefix:       c.ID + "-",
		})
		rendered = append(rendered, renderedGlyph{Stats: s, SVG: sheetSVG, Legacy: c.Legacy})
	}

	if err := writeContactSheet(*outDir, rendered); err != nil {
		log.Fatalf("write contact sheet: %v", err)
	}

	fmt.Printf("Generated %d ship glyphs (%d with overlays) in %s/\n",
		len(rendered), overlaid, *outDir)
}

// cleanGeneratedFiles removes stale glyph SVGs left over from a previous run,
// so a ship removed or renamed in the catalog doesn't leave an orphan file
// behind forever. Only *.svg is removed: glyphs.css is hand-written and must
// survive, and index.html is rewritten unconditionally by writeContactSheet
// so it needs no special handling here. Mirrors cleanGeneratedFiles in
// cmd/generate-items-kb/main.go.
func cleanGeneratedFiles(outDir string) error {
	entries, err := os.ReadDir(outDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".svg") {
			continue
		}
		if err := os.Remove(filepath.Join(outDir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
