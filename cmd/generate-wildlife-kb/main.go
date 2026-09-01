// Command generate-wildlife-kb renders the Wildlife section of the KB from
// the scanned field guide in the knowledge database: an index modelled on
// the Resources page (species, estimated populations by system, galaxy map)
// and one detail page per species.
//
//	generate-wildlife-kb [-db ../spacemolt-knowledge.db] [-lore data/mesh_bakeoff/wildlife/FORMS_LORE.md] [-out kb/wildlife]
//
// Hero art is optional: a species page shows kb/wildlife/images/<species>.png
// when the file exists and a labelled placeholder frame otherwise.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
	"github.com/rsned/spacemolt-kb/pkg/galaxymap"
	"github.com/rsned/spacemolt-kb/pkg/wildlife"
)

func main() {
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database")
	lorePath := flag.String("lore", "data/mesh_bakeoff/wildlife/FORMS_LORE.md", "wildlife lore document (Part 1 roster is used)")
	codexPath := flag.String("codex", "data/wildlife/codex.json", "hand-recorded official scan descriptions, used when the DB has none")
	combatPath := flag.String("combat-stats", "data/wildlife/combat_stats.json", "per-species combat observations from exported battle logs (scripts/wildlife_combat_stats.py)")
	statsPath := flag.String("battle-stats", "data/wildlife/battle_stats.json", "per-species battle records from the bulk data feed (scripts/wildlife_battle_stats.py)")
	outDir := flag.String("out", "kb/wildlife", "output directory")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open %s: %v", *dbPath, err)
	}
	defer func() { _ = db.Close() }()

	guide, err := wildlife.Load(db)
	if err != nil {
		log.Fatalf("load wildlife: %v", err)
	}
	codex, err := wildlife.LoadCodex(*codexPath)
	if err != nil {
		log.Fatalf("load codex: %v", err)
	}
	codex.Apply(guide.Species)
	withCodex := 0
	for _, s := range guide.Species {
		if s.Description != "" {
			withCodex++
		}
	}
	log.Printf("Codex: %d of %d species have an official description (%d hand-recorded in %s)", withCodex, len(guide.Species), len(codex), *codexPath)

	lore := wildlife.Lore{}
	if data, err := os.ReadFile(*lorePath); err != nil {
		log.Printf("warning: lore unavailable (%v); species pages render without field notes", err)
	} else {
		lore = wildlife.ParseLore(data)
	}

	var stats *wildlife.BattleStats
	if bs, err := wildlife.LoadBattleStats(*statsPath); err != nil {
		log.Printf("warning: battle stats unavailable (%v); pages render without danger ratings", err)
	} else {
		stats = bs
		log.Printf("Battle records: %d species from feed months %v", len(bs.Species), bs.Months)
	}

	var combat *wildlife.CombatStats
	if cs, err := wildlife.LoadCombatStats(*combatPath); err != nil {
		log.Printf("warning: combat stats unavailable (%v); pages render without attack data", err)
	} else {
		combat = cs
		log.Printf("Combat observations: %d species (%s)", len(cs.Species), cs.Source)
	}

	if err := render(guide, lore, stats, combat, *outDir); err != nil {
		log.Fatal(err)
	}
}

// speciesView is a Species plus everything the templates derive from it.
type speciesView struct {
	wildlife.Species
	Slug      string
	Combat    *wildlife.CreatureCombat
	Record    *wildlife.BattleRecord
	Lore      *wildlife.LoreEntry
	HasImage  bool
	ImagePath string // relative to the section directory
}

func render(guide *wildlife.Guide, lore wildlife.Lore, stats *wildlife.BattleStats, combat *wildlife.CombatStats, outDir string) error {
	if err := os.MkdirAll(filepath.Join(outDir, "images"), 0o755); err != nil {
		return err
	}
	// Clean previous pages, keeping images/.
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".html") {
			_ = os.Remove(filepath.Join(outDir, e.Name()))
		}
	}

	views := make([]speciesView, 0, len(guide.Species))
	matched := 0
	for _, s := range guide.Species {
		v := speciesView{Species: s, Slug: wildlife.Slug(s.ID)}
		if e, ok := lore.Lookup(s.Name); ok {
			e := e
			v.Lore = &e
			matched++
			if v.Description == "" && e.Codex != "" {
				v.Description = e.Codex
				v.CodexSource = "lore"
			}
		}
		if stats != nil {
			if r, ok := stats.Species[s.Name]; ok {
				v.Record = &r
			}
		}
		if combat != nil {
			if c, ok := combat.Species[s.Name]; ok {
				v.Combat = &c
			}
		}
		v.ImagePath = "images/" + s.ID + ".png"
		if _, err := os.Stat(filepath.Join(outDir, v.ImagePath)); err == nil {
			v.HasImage = true
		}
		views = append(views, v)
	}
	if len(lore) > 0 {
		var unmatched []string
		for _, e := range lore {
			if !slices.ContainsFunc(views, func(v speciesView) bool { return v.Lore != nil && v.Lore.Name == e.Name }) {
				unmatched = append(unmatched, e.Name)
			}
		}
		slices.Sort(unmatched)
		log.Printf("Lore: %d of %d species have field notes; %d entries match no scanned species %v", matched, len(views), len(unmatched), unmatched)
	}

	// Galaxy map with one highlight class per species.
	classes := make(map[string][]string)
	for _, v := range views {
		for _, p := range v.Places {
			classes[p.SystemID] = append(classes[p.SystemID], "w-"+v.Slug)
		}
	}
	for id := range classes {
		slices.Sort(classes[id])
	}
	var explored, unexplored []*galaxymap.System
	for _, s := range guide.MapSystems {
		if s.LastUpdatedTick > 0 {
			explored = append(explored, s)
		} else {
			unexplored = append(unexplored, s)
		}
	}
	mapSVG := galaxymap.Render(explored, unexplored, guide.MapByID, galaxymap.Options{
		ShowConnections:  true,
		LinkPrefix:       "../",
		HighlightClasses: func(id string) []string { return classes[id] },
	})
	var css strings.Builder
	for _, v := range views {
		if len(v.Places) > 0 {
			fmt.Fprintf(&css, "#wl-map[data-active=\"%s\"] .w-%s{fill:#ffc832;r:9;stroke:#fff0b8;stroke-width:1.5}\n", v.Slug, v.Slug)
		}
	}

	type roleGroup struct {
		Role    string
		Species []speciesView
	}
	var groups []roleGroup
	for _, v := range views {
		if n := len(groups); n == 0 || groups[n-1].Role != v.Role {
			groups = append(groups, roleGroup{Role: v.Role})
		}
		groups[len(groups)-1].Species = append(groups[len(groups)-1].Species, v)
	}
	firstSlug := ""
	for _, v := range views {
		if len(v.Places) > 0 {
			firstSlug = v.Slug
			break
		}
	}
	sighted := 0
	for _, v := range views {
		if len(v.Places) > 0 {
			sighted++
		}
	}

	statsMonths := ""
	if stats != nil {
		switch {
		case stats.Since != "":
			statsMonths = "since " + stats.Since[:10] + ", post-balance-patch"
		case len(stats.Months) > 0:
			statsMonths = stats.Months[0]
			if n := len(stats.Months); n > 1 {
				statsMonths += " to " + stats.Months[n-1]
			}
		}
	}

	funcs := templateFuncs()
	idx := htmltpl.Must(htmltpl.New("index").Funcs(funcs).Parse(indexTemplate))
	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	err = idx.Execute(f, map[string]any{
		"Header":       htmltpl.HTML(kbnav.Header("../")), //nolint:gosec // site header, generated internally
		"Species":      views,
		"Groups":       groups,
		"Coverage":     guide.Coverage,
		"Sighted":      sighted,
		"Estimated":    guide.EstimatedCreatures(),
		"MapSVG":       htmltpl.HTML(mapSVG),      //nolint:gosec // generated internally from trusted DB data
		"HighlightCSS": htmltpl.CSS(css.String()), //nolint:gosec // generated internally from trusted DB data
		"FirstSlug":    firstSlug,
		"StatsMonths":  statsMonths,
		"Generated":    time.Now().UTC().Format("2006-01-02"),
	})
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("render index: %w", err)
	}

	page := htmltpl.Must(htmltpl.New("species").Funcs(funcs).Parse(speciesTemplate))
	for _, v := range views {
		f, err := os.Create(filepath.Join(outDir, v.ID+".html"))
		if err != nil {
			return err
		}
		err = page.Execute(f, map[string]any{
			"Header":      htmltpl.HTML(kbnav.Header("../")), //nolint:gosec // site header, generated internally
			"S":           v,
			"StatsMonths": statsMonths,
		})
		_ = f.Close()
		if err != nil {
			return fmt.Errorf("render %s: %w", v.ID, err)
		}
	}
	log.Printf("Wildlife: %d species (%d sighted), %d systems with wildlife, ~%d creatures; wrote %s/", len(views), sighted, guide.Coverage.SystemsWithWildlife, guide.EstimatedCreatures(), outDir)
	return nil
}

func templateFuncs() htmltpl.FuncMap {
	return htmltpl.FuncMap{
		"title": func(s string) string {
			if s == "" {
				return ""
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"habitat": func(h string) string { return strings.ReplaceAll(h, "_", " ") },
		"roleClass": func(role string) string {
			switch role {
			case "predator":
				return "badge-red"
			case "scavenger":
				return "badge-orange"
			case "grazer":
				return "badge-green"
			}
			return ""
		},
		"intRange": func(lo, hi int) string {
			if lo == hi {
				return fmt.Sprintf("%d", lo)
			}
			return fmt.Sprintf("%d\u2013%d", lo, hi)
		},
		"ratingClass": func(rating string) string {
			switch rating {
			case "extreme":
				return "badge-red"
			case "high":
				return "badge-orange"
			case "moderate":
				return "badge-yellow"
			}
			return ""
		},
		"yesno": func(b bool) string {
			if b {
				return "Yes"
			}
			return "No"
		},
		"date": func(s string) string {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t.Format("2006-01-02")
			}
			return s
		},
		"datetime": func(s string) string {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t.Format("2006-01-02 15:04") + " UTC"
			}
			return s
		},
		"fmtF": func(f float64) string {
			if f == float64(int64(f)) {
				return fmt.Sprintf("%d", int64(f))
			}
			return fmt.Sprintf("%.2f", f)
		},
		"plural": func(n int, s string) string {
			if n == 1 {
				return s
			}
			return s + "s"
		},
		"poiList": func(pois []wildlife.POISighting) string {
			parts := make([]string, 0, len(pois))
			for _, p := range pois {
				parts = append(parts, fmt.Sprintf("%s (%d)", p.Name, p.Count))
			}
			return strings.Join(parts, ", ")
		},
	}
}
