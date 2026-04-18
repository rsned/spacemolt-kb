// Command generate-diffs manages game data snapshots and generates HTML diff
// reports. It compares fresh JSON catalog files against the previous snapshot,
// writes an HTML report page, and rotates the snapshot symlinks.
//
// Usage:
//
//	generate-diffs --input /path/to/fresh/json
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/gamediff"
)

// catalogs defines the 5 tracked catalog files in display order.
var catalogs = []struct {
	name  string // display name
	file  string // filename
	isMap bool   // true for get_map.json (uses system_id key)
}{
	{"Recipes", "catalog_recipes.json", false},
	{"Items", "catalog_items.json", false},
	{"Ships", "catalog_ships.json", false},
	{"Skills", "catalog_skills.json", false},
	{"Map", "get_map.json", true},
}

func main() {
	inputDir := flag.String("input", "", "directory containing fresh JSON catalog files")
	snapshotDir := flag.String("snapshots", "data/snapshots", "base directory for snapshot storage")
	outputDir := flag.String("output", "kb/diffs", "output directory for HTML reports")
	dateFlag := flag.String("date", "", "override date as YYYYMMDD (default: today)")
	flag.Parse()

	if *inputDir == "" {
		fmt.Fprintln(os.Stderr, "Usage: generate-diffs --input <dir>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	var reportTime time.Time
	if *dateFlag != "" {
		var err error
		reportTime, err = time.Parse("20060102", *dateFlag)
		if err != nil {
			log.Fatalf("invalid --date %q: %v", *dateFlag, err)
		}
	} else {
		reportTime = time.Now()
	}
	today := reportTime.Format("20060102")
	todayDash := reportTime.Format("2006-01-02")

	// 1. Create today's snapshot directory and copy files.
	todayDir := filepath.Join(*snapshotDir, today)
	if err := os.MkdirAll(todayDir, 0o755); err != nil {
		log.Fatalf("create snapshot dir: %v", err)
	}
	for _, cat := range catalogs {
		src := filepath.Join(*inputDir, cat.file)
		dst := filepath.Join(todayDir, cat.file)
		data, err := os.ReadFile(src)
		if err != nil {
			log.Printf("warning: %s not found in input, skipping", cat.file)
			continue
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			log.Fatalf("write snapshot %s: %v", cat.file, err)
		}
	}
	log.Printf("Snapshot saved to %s", todayDir)

	// 2. Find a previous snapshot to diff against.
	// Prefer the "previous" symlink; fall back to "latest" if it points to
	// a different directory than today; otherwise scan for the most recent
	// dated directory before today.
	prevDir := ""
	prevLink := filepath.Join(*snapshotDir, "previous")
	latestLink := filepath.Join(*snapshotDir, "latest")

	// Prefer "latest" symlink (the most recent completed snapshot).
	// Fall back to "previous" if latest points to today (already saved above).
	if target, err := os.Readlink(latestLink); err == nil && filepath.Base(target) != today {
		candidate := filepath.Join(*snapshotDir, filepath.Base(target))
		if snapshotHasFiles(candidate) {
			prevDir = candidate
		}
	}
	if prevDir == "" {
		if target, err := os.Readlink(prevLink); err == nil && filepath.Base(target) != today {
			candidate := filepath.Join(*snapshotDir, filepath.Base(target))
			if snapshotHasFiles(candidate) {
				prevDir = candidate
			}
		}
	}
	if prevDir == "" {
		// Scan dated directories for the most recent one before today
		// that actually contains catalog files.
		entries, _ := os.ReadDir(*snapshotDir)
		for i := len(entries) - 1; i >= 0; i-- {
			name := entries[i].Name()
			if entries[i].IsDir() && name < today && len(name) == 8 {
				candidate := filepath.Join(*snapshotDir, name)
				if snapshotHasFiles(candidate) {
					prevDir = candidate
					break
				}
			}
		}
	}

	if prevDir == "" {
		log.Println("No previous snapshot found (first run). Creating latest symlink.")
		_ = os.Remove(latestLink)
		if err := os.Symlink(today, latestLink); err != nil {
			log.Fatalf("create latest symlink: %v", err)
		}
		log.Println("Run again after next scrape to generate first diff report.")
		return
	}
	log.Printf("Diffing against %s", filepath.Base(prevDir))

	// 3. Diff each catalog.
	var diffs []gamediff.CatalogDiff
	for _, cat := range catalogs {
		oldPath := filepath.Join(prevDir, cat.file)
		newPath := filepath.Join(todayDir, cat.file)

		oldData, err := os.ReadFile(oldPath)
		if err != nil {
			log.Printf("warning: cannot read previous %s, skipping", cat.file)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}
		newData, err := os.ReadFile(newPath)
		if err != nil {
			log.Printf("warning: cannot read new %s, skipping", cat.file)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}

		var result *gamediff.CatalogDiff
		if cat.isMap {
			result, err = gamediff.DiffMap(oldData, newData)
		} else {
			result, err = gamediff.DiffCatalog(oldData, newData)
		}
		if err != nil {
			log.Printf("warning: diff %s failed: %v", cat.file, err)
			diffs = append(diffs, gamediff.CatalogDiff{Name: cat.name, File: cat.file})
			continue
		}
		result.Name = cat.name
		result.File = cat.file
		diffs = append(diffs, *result)
	}

	// 4. Build the day report.
	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	// Scan existing report files to find prev/next dates.
	existingDates := scanReportDates(*outputDir)
	existingDates = append(existingDates, todayDash)
	slices.Sort(existingDates)
	existingDates = slices.Compact(existingDates)

	todayIdx := slices.Index(existingDates, todayDash)
	prevDate := ""
	nextDate := ""
	if todayIdx > 0 {
		prevDate = existingDates[todayIdx-1]
	}
	if todayIdx < len(existingDates)-1 {
		nextDate = existingDates[todayIdx+1]
	}

	// Extract the date of the snapshot we compared against for the page title.
	compareDate := ""
	if base := filepath.Base(prevDir); len(base) == 8 {
		if t, err := time.Parse("20060102", base); err == nil {
			compareDate = t.Format("2006-01-02")
		}
	}

	day := gamediff.DayReport{
		Date:        reportTime,
		CompareDate: compareDate,
		PrevDate:    prevDate,
		NextDate:    nextDate,
		Catalogs:    diffs,
	}

	html, err := gamediff.RenderDayReport(day)
	if err != nil {
		log.Fatalf("render day report: %v", err)
	}
	dayFile := filepath.Join(*outputDir, todayDash+".html")
	if err := os.WriteFile(dayFile, []byte(html), 0o644); err != nil {
		log.Fatalf("write day report: %v", err)
	}
	log.Printf("Report written to %s", dayFile)

	// 5. Update prev/next links on adjacent reports.
	if prevDate != "" {
		updateAdjacentReport(*outputDir, prevDate, existingDates)
	}
	if nextDate != "" {
		updateAdjacentReport(*outputDir, nextDate, existingDates)
	}

	// 6. Regenerate index.
	var entries []gamediff.IndexEntry
	for i := len(existingDates) - 1; i >= 0; i-- {
		d := existingDates[i]
		summary := summarizeReport(*outputDir, d)
		entries = append(entries, gamediff.IndexEntry{Date: d, Summary: summary})
	}
	idxHTML, err := gamediff.RenderIndex(entries)
	if err != nil {
		log.Fatalf("render index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "index.html"), []byte(idxHTML), 0o644); err != nil {
		log.Fatalf("write index: %v", err)
	}
	log.Println("Index updated")

	// 7. Rotate symlinks: previous -> old latest, latest -> today.
	oldLatest, _ := os.Readlink(latestLink)
	_ = os.Remove(prevLink)
	if oldLatest != "" {
		if err := os.Symlink(filepath.Base(oldLatest), prevLink); err != nil {
			log.Fatalf("update previous symlink: %v", err)
		}
	}
	_ = os.Remove(latestLink)
	if err := os.Symlink(today, latestLink); err != nil {
		log.Fatalf("update latest symlink: %v", err)
	}
	if oldLatest != "" {
		log.Printf("Symlinks updated: previous -> %s, latest -> %s",
			filepath.Base(oldLatest), today)
	} else {
		log.Printf("Symlinks updated: latest -> %s", today)
	}
}

// scanReportDates returns all YYYY-MM-DD dates that have report HTML files.
func scanReportDates(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".html") && name != "index.html" {
			date := strings.TrimSuffix(name, ".html")
			if _, err := time.Parse("2006-01-02", date); err == nil {
				dates = append(dates, date)
			}
		}
	}
	return dates
}

// summarizeReport reads a report HTML file and extracts the summary text.
func summarizeReport(dir, date string) string {
	path := filepath.Join(dir, date+".html")
	data, err := os.ReadFile(path)
	if err != nil {
		return "Report unavailable"
	}
	content := string(data)
	if strings.Contains(content, "No changes detected") {
		return "No changes"
	}
	start := strings.Index(content, `<div class="diff-summary">`)
	if start < 0 {
		return "Changes detected"
	}
	end := strings.Index(content[start:], `</div>`)
	if end < 0 {
		return "Changes detected"
	}
	summary := content[start+len(`<div class="diff-summary">`): start+end]
	summary = strings.TrimSpace(summary)
	for strings.Contains(summary, "<") {
		s := strings.Index(summary, "<")
		e := strings.Index(summary, ">")
		if e < 0 {
			break
		}
		summary = summary[:s] + summary[e+1:]
	}
	return strings.TrimSpace(summary)
}

// snapshotHasFiles returns true if the directory contains at least one expected catalog JSON file.
func snapshotHasFiles(dir string) bool {
	for _, cat := range catalogs {
		if _, err := os.Stat(filepath.Join(dir, cat.file)); err == nil {
			return true
		}
	}
	return false
}

// updateAdjacentReport rewrites the prev/next navigation in an existing report.
func updateAdjacentReport(dir, date string, allDates []string) {
	idx := slices.Index(allDates, date)
	if idx < 0 {
		return
	}
	prev := ""
	next := ""
	if idx > 0 {
		prev = allDates[idx-1]
	}
	if idx < len(allDates)-1 {
		next = allDates[idx+1]
	}

	path := filepath.Join(dir, date+".html")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)

	navStart := strings.Index(content, `<div class="diff-nav">`)
	if navStart < 0 {
		return
	}
	navEnd := strings.Index(content[navStart:], `</div>`)
	if navEnd < 0 {
		return
	}
	navEnd += navStart + len(`</div>`)

	var newNav strings.Builder
	newNav.WriteString(`<div class="diff-nav">`)
	newNav.WriteString("\n    <span>")
	if prev != "" {
		fmt.Fprintf(&newNav, `<a href="%s.html">&larr; %s</a>`, prev, prev)
	} else {
		newNav.WriteString("&nbsp;")
	}
	newNav.WriteString("</span>\n    ")
	newNav.WriteString(`<a href="index.html">All Reports</a>`)
	newNav.WriteString("\n    <span>")
	if next != "" {
		fmt.Fprintf(&newNav, `<a href="%s.html">%s &rarr;</a>`, next, next)
	} else {
		newNav.WriteString("&nbsp;")
	}
	newNav.WriteString("</span>\n</div>")

	newContent := content[:navStart] + newNav.String() + content[navEnd:]
	_ = os.WriteFile(path, []byte(newContent), 0o644)
}
