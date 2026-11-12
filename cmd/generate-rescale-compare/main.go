// Command generate-rescale-compare builds the one-off "Great Rescale" Did You
// Know pages: every system's layout before and after the v0.698.0 real-scale
// rescale, side by side.
//
// The "before" side is a saved baseline (data/rescale/before.json), taken once
// from the system pages of the last regen before the rescale:
//
//	go run ./cmd/generate-rescale-compare -extract-before c43191a717
//
// The "after" side is the knowledge database. Rerun the plain command as
// agents rescan; systems move from "awaiting rescan" to linked. Run from the
// kb repo root. Not part of the regular regen chain.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func main() {
	extract := flag.String("extract-before", "", "git revision whose kb/systems pages become the saved baseline, then exit")
	baselinePath := flag.String("baseline", "data/rescale/before.json", "saved before-side baseline")
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database (after side)")
	outDir := flag.String("out", "kb/did_you_know/rescale", "output directory")
	flag.Parse()

	if *extract != "" {
		b, err := extractBaseline(*extract)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(*baselinePath), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := saveBaseline(*baselinePath, b); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("saved %d systems from %s to %s\n", len(b.Systems), *extract, *baselinePath)
		return
	}

	base, err := loadBaseline(*baselinePath)
	if err != nil {
		log.Fatalf("load baseline (run with -extract-before first): %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	after, err := loadAfter(db)
	if err != nil {
		log.Fatal(err)
	}
	systems := compareSystems(base.Systems, after)
	n, err := writePages(*outDir, systems)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote rescale compare: %d of %d systems rescanned → %s\n", n, len(systems), *outDir)
}

// writePages replaces outDir's pages with the index and one page per
// rescanned system. It returns how many systems were rescanned.
func writePages(outDir string, systems []System) (int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(outDir, e.Name()))
		}
	}
	rescanned := 0
	for _, s := range systems {
		if !s.Rescanned {
			continue
		}
		rescanned++
		page, err := render(detailTmpl, map[string]any{"Title": s.Name + " - The Great Rescale", "Header": header(), "S": s})
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(outDir, s.ID+".html"), page, 0o644); err != nil {
			return 0, err
		}
	}
	idx, err := render(indexTmpl, map[string]any{"Title": "The Great Rescale", "Header": header(), "Systems": systems, "Rescanned": rescanned})
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), idx, 0o644); err != nil {
		return 0, err
	}
	return rescanned, os.WriteFile(filepath.Join(outDir, "rescale.css"), []byte(css), 0o644)
}
