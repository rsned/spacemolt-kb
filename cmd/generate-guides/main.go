// Command generate-guides renders hand-written guides (guides/*.md: YAML
// frontmatter + markdown) into kb/guides/. A guide's build steps name
// facility ids; their costs, upkeep and power/life-support figures come from
// the facility catalog at generation time, so authors never type them.
package main

import (
	"bytes"
	"flag"
	"fmt"
	htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/catalog"
	"github.com/rsned/spacemolt-kb/pkg/stationplan"
)

func main() {
	guidesDir := flag.String("guides", "guides", "directory of guide markdown files")
	snapshotsDir := flag.String("snapshots", "data/snapshots", "root of the game-API catalog snapshots")
	outDir := flag.String("out", "kb/guides", "output directory")
	flag.Parse()

	warnings, err := generate(*guidesDir, *snapshotsDir, *outDir)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if err != nil {
		log.Fatal(err)
	}
}

// generate renders every guide in guidesDir plus an index into outDir. All
// guides are parsed and checked before anything is written, so a guide that
// names an unknown facility fails the run and leaves outDir untouched.
func generate(guidesDir, snapshotsDir, outDir string) ([]string, error) {
	facs, err := catalog.LoadFacilities(snapshotsDir)
	if err != nil {
		return nil, fmt.Errorf("load facilities: %w", err)
	}
	items, err := catalog.LoadItems(snapshotsDir)
	if err != nil {
		return nil, fmt.Errorf("load items: %w", err)
	}
	byID := make(map[string]catalog.Facility, len(facs))
	for _, f := range facs {
		byID[f.ID] = f
	}
	r := &renderer{facilities: byID, links: stationplan.NewLinker(facs, items, "../")}

	files, err := filepath.Glob(filepath.Join(guidesDir, "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	pages := map[string][]byte{}
	var index []indexEntry
	for _, path := range files {
		g, err := loadGuide(path, filepath.Base(path))
		if err != nil {
			return r.warnings, err
		}
		steps := make([]stationplan.StepInput, len(g.Steps))
		for i, s := range g.Steps {
			for _, id := range append([]string{s.Facility}, s.Alternatives...) {
				if _, ok := byID[id]; id != "" && !ok {
					return r.warnings, fmt.Errorf("%s: step %q: unknown facility %q", g.file, s.Title, id)
				}
			}
			steps[i].FacilityID = s.Facility
			for _, f := range s.Facts {
				steps[i].ExtraCredits += f.Credits
			}
		}
		totals, err := stationplan.Accumulate(byID, steps)
		if err != nil {
			return r.warnings, fmt.Errorf("%s: %w", g.file, err)
		}
		pv, err := r.page(g, totals)
		if err != nil {
			return r.warnings, fmt.Errorf("%s: %w", g.file, err)
		}
		var buf bytes.Buffer
		if err := guideTemplate.Execute(&buf, pv); err != nil {
			return r.warnings, fmt.Errorf("%s: render: %w", g.file, err)
		}
		pages[g.slug+".html"] = buf.Bytes()
		index = append(index, indexEntry{Slug: g.slug, Title: g.Title, Summary: strings.TrimSpace(g.Summary)})
	}

	var buf bytes.Buffer
	if err := indexTemplate.Execute(&buf, map[string]any{
		"Header": htmltpl.HTML(kbnav.Header("../")), //nolint:gosec // site header, generated internally
		"Title":  "Guides",
		"Guides": index,
	}); err != nil {
		return r.warnings, fmt.Errorf("render index: %w", err)
	}
	pages["index.html"] = buf.Bytes()

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return r.warnings, err
	}
	for name, b := range pages {
		if err := os.WriteFile(filepath.Join(outDir, name), b, 0o644); err != nil { //nolint:gosec // static site output
			return r.warnings, err
		}
	}
	return r.warnings, nil
}
