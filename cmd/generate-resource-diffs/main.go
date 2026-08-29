// Command generate-resource-diffs snapshots the values on the KB Resources
// page and renders reports of what the survey agents have found since the
// previous regeneration and since the last server content update.
//
// Run it right after generate-items-kb, once per regen:
//
//	generate-resource-diffs                      # snapshot from the knowledge DB
//	generate-resource-diffs --from-html page.html --date 20260827 --content-version 0.566.0
//
// Snapshots live in data/resource-snapshots/<YYYY-MM-DD>.json (committed;
// they are the only record of the page at that time) with a manifest.json
// that tracks which snapshot is the baseline for the current content patch.
// Reports are rendered into kb/resources/changes/.
package main

import (
	"cmp"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/rsned/spacemolt-kb/pkg/resourcediff"
)

func main() {
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database to snapshot")
	fromHTML := flag.String("from-html", "", "snapshot a generated kb/resources/index.html instead of the database (bootstrap/backfill)")
	dateFlag := flag.String("date", "", "snapshot date as YYYYMMDD (default: today)")
	snapDir := flag.String("snapshots", "data/resource-snapshots", "snapshot storage directory")
	outDir := flag.String("output", "kb/resources/changes", "output directory for HTML reports")
	gameAPI := flag.String("game-api", "../spacemolt/data/game-api", "game-api scrape root, read for get_version.json feeds")
	diffsDir := flag.String("diffs", "kb/diffs", "catalog diff reports, linked from baseline sections")
	serverVersion := flag.String("server-version", "", "server version at snapshot time (default: newest get_version.json in --game-api)")
	contentVersion := flag.String("content-version", "", "content patch this baseline tracks (default: newest resource-related patch in the feed)")
	forceBaseline := flag.Bool("baseline", false, "pin this snapshot as the content-update baseline")
	flag.Parse()

	snapDate := time.Now()
	if *dateFlag != "" {
		var err error
		if snapDate, err = time.Parse("20060102", *dateFlag); err != nil {
			log.Fatalf("invalid --date %q: %v", *dateFlag, err)
		}
	}
	date := snapDate.Format("2006-01-02")

	feed, err := resourcediff.LoadVersionFeed(*gameAPI)
	if err != nil {
		log.Fatalf("load version feed: %v", err)
	}

	// 1. Build the snapshot.
	snap, err := buildSnapshot(*fromHTML, *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	snap.Date = date
	snap.ServerVersion = cmp.Or(*serverVersion, feed.Current())
	log.Printf("Snapshot %s (%s): %d types, %d deposits, %d/%d systems explored, server v%s",
		date, snap.Source, snap.Summary.Types, snap.Summary.Deposits, snap.Summary.Explored, snap.Summary.Systems, snap.ServerVersion)

	// 2. Place it in the manifest.
	man, err := resourcediff.LoadManifest(*snapDir)
	if err != nil {
		log.Fatalf("load manifest: %v", err)
	}
	man.Snapshots = slices.DeleteFunc(man.Snapshots, func(e resourcediff.ManifestEntry) bool { return e.Date == date })
	prev := man.PreviousBefore(date)
	entry := resourcediff.ManifestEntry{Date: date, ServerVersion: snap.ServerVersion, Source: snap.Source}

	prevVersion := ""
	var reason string
	switch {
	case prev == nil:
		reason = "first snapshot"
	case *forceBaseline:
		reason = "pinned with --baseline"
	default:
		prevVersion = prev.ServerVersion
		prevSnap, err := resourcediff.LoadSnapshot(*snapDir, prev.Date)
		if err != nil {
			log.Fatalf("load previous snapshot %s: %v", prev.Date, err)
		}
		c := resourcediff.Diff(prevSnap, snap)
		if len(c.NewTypes)+len(c.RemovedTypes) > 0 {
			reason = fmt.Sprintf("resource catalog changed since %s: +%d/-%d types", prev.Date, len(c.NewTypes), len(c.RemovedTypes))
		} else if hints := feed.ResourcePatches(prev.ServerVersion, snap.ServerVersion); len(hints) > 0 {
			log.Printf("note: patch v%s mentions resources (%q) but the type list is unchanged; re-run with --baseline to pin a new baseline", hints[0].Version, hints[0].ResourceNotes()[0])
		}
	}
	if reason != "" {
		entry.Baseline = true
		entry.Reason = reason
		entry.ContentVersion = *contentVersion
		if entry.ContentVersion == "" {
			if patches := feed.ResourcePatches(prevVersion, snap.ServerVersion); len(patches) > 0 {
				entry.ContentVersion = patches[0].Version
			} else {
				entry.ContentVersion = snap.ServerVersion
				log.Printf("warning: no resource-related patch found in the version feed; using server v%s as the content version (override with --content-version)", snap.ServerVersion)
			}
		}
		if p := feed.Patch(entry.ContentVersion); p != nil {
			entry.ContentReleaseDate = p.ReleaseDate
			entry.ContentNotes = p.ResourceNotes()
		}
		log.Printf("Baseline: content patch v%s (%s)", entry.ContentVersion, reason)
	}
	man.Snapshots = append(man.Snapshots, entry)
	slices.SortFunc(man.Snapshots, func(a, b resourcediff.ManifestEntry) int { return cmp.Compare(a.Date, b.Date) })
	man.Latest = man.Snapshots[len(man.Snapshots)-1].Date
	for _, e := range man.Snapshots {
		if e.Baseline {
			man.Baseline = e.Date
		}
	}

	// 3. Persist.
	if err := os.MkdirAll(*snapDir, 0o755); err != nil {
		log.Fatalf("create snapshot dir: %v", err)
	}
	data, err := snap.Marshal()
	if err != nil {
		log.Fatalf("encode snapshot: %v", err)
	}
	if err := os.WriteFile(resourcediff.SnapshotPath(*snapDir, date), data, 0o644); err != nil {
		log.Fatalf("write snapshot: %v", err)
	}
	if err := man.Save(*snapDir); err != nil {
		log.Fatalf("write manifest: %v", err)
	}
	log.Printf("Snapshot saved to %s", resourcediff.SnapshotPath(*snapDir, date))

	// 4. Render every report from the manifest (cheap, and keeps prev/next
	// navigation and baseline sections consistent without patching files).
	if err := renderAll(man, *snapDir, *outDir, *diffsDir); err != nil {
		log.Fatal(err)
	}
}

func buildSnapshot(fromHTML, dbPath string) (*resourcediff.Snapshot, error) {
	if fromHTML != "" {
		page, err := os.ReadFile(fromHTML)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", fromHTML, err)
		}
		snap, err := resourcediff.FromHTML(page)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", fromHTML, err)
		}
		return snap, nil
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", dbPath, err)
	}
	defer func() { _ = db.Close() }()
	snap, err := resourcediff.FromDB(db)
	if err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", dbPath, err)
	}
	return snap, nil
}

// renderAll writes one report per manifest entry plus the index.
func renderAll(man *resourcediff.Manifest, snapDir, outDir, diffsDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	snaps := make(map[string]*resourcediff.Snapshot, len(man.Snapshots))
	for _, e := range man.Snapshots {
		s, err := resourcediff.LoadSnapshot(snapDir, e.Date)
		if err != nil {
			return fmt.Errorf("load snapshot %s: %w", e.Date, err)
		}
		snaps[e.Date] = s
	}

	var index []resourcediff.IndexEntry
	var baseline *resourcediff.ManifestEntry
	for i := range man.Snapshots {
		e := &man.Snapshots[i]
		if e.Baseline {
			baseline = e
		}
		if baseline == nil {
			// Cannot happen when the first entry is a baseline, but keep the
			// renderer total: treat the entry as its own baseline.
			baseline = e
		}
		rep := resourcediff.DayReport{
			Snapshot:           snaps[e.Date],
			IsBaseline:         e.Baseline,
			BaselineDate:       baseline.Date,
			ContentVersion:     baseline.ContentVersion,
			ContentReleaseDate: baseline.ContentReleaseDate,
			ContentNotes:       baseline.ContentNotes,
			BaselineReason:     baseline.Reason,
			CatalogDiffURL:     catalogDiffURL(diffsDir, baseline.ContentReleaseDate),
		}
		if i > 0 {
			rep.PrevDate = man.Snapshots[i-1].Date
			rep.VsPrevious = resourcediff.Diff(snaps[rep.PrevDate], rep.Snapshot)
		}
		if i < len(man.Snapshots)-1 {
			rep.NextDate = man.Snapshots[i+1].Date
		}
		if !e.Baseline {
			rep.VsBaseline = resourcediff.Diff(snaps[baseline.Date], rep.Snapshot)
		}
		html, err := resourcediff.RenderDayReport(rep)
		if err != nil {
			return fmt.Errorf("render %s: %w", e.Date, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, e.Date+".html"), []byte(html), 0o644); err != nil {
			return fmt.Errorf("write report %s: %w", e.Date, err)
		}

		ie := resourcediff.IndexEntry{Date: e.Date, ServerVersion: e.ServerVersion, IsBaseline: e.Baseline, ContentVersion: baseline.ContentVersion, VsPrevious: "First snapshot"}
		if rep.VsPrevious != nil {
			ie.VsPrevious = rep.VsPrevious.SummaryLine()
		}
		if rep.VsBaseline != nil {
			ie.VsBaseline = rep.VsBaseline.SummaryLine()
		}
		index = append(index, ie)
	}
	slices.Reverse(index)
	html, err := resourcediff.RenderIndex(index)
	if err != nil {
		return fmt.Errorf("render index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0o644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	log.Printf("Rendered %d reports + index to %s", len(man.Snapshots), outDir)
	return nil
}

// catalogDiffURL links the baseline section to the first catalog diff report
// on or after the content patch's release date (the scrape that picked the
// patch up), if one exists within two weeks. Relative to the output dir.
func catalogDiffURL(diffsDir, releaseDate string) string {
	if releaseDate == "" {
		return ""
	}
	entries, err := os.ReadDir(diffsDir)
	if err != nil {
		return ""
	}
	limit, err := time.Parse("2006-01-02", releaseDate)
	if err != nil {
		return ""
	}
	limitStr := limit.AddDate(0, 0, 14).Format("2006-01-02")
	best := ""
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".html")
		if name == e.Name() || name == "index" || name < releaseDate || name > limitStr {
			continue
		}
		if best == "" || name < best {
			best = name
		}
	}
	if best == "" {
		return ""
	}
	return "../../diffs/" + best + ".html"
}
