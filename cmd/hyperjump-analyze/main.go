// Command hyperjump-analyze computes Pathfinder Drive (direct hyper-jump)
// reachability between galactic systems from the KB: bearings to every other
// system, which systems interrupt a direct path, the heading margin of error for
// clean paths, and per-system void escape directions.
//
// See docs/plans/2026-05-23-hyperjump-analyze-design.md.
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/rsned/spacemolt-kb/pkg/hyperjump"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "../spacemolt/data/spacemolt-knowledge.db", "path to the SQLite knowledge base")
	margin := flag.Float64("margin", 100, "landing margin in galactic units")
	out := flag.String("out", "", "write the full per-system JSON report to this path")
	system := flag.String("system", "", "restrict printed/JSON detail to a single origin system id")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	systems, err := loadSystems(db)
	if err != nil {
		log.Fatalf("load systems: %v", err)
	}

	reports := hyperjump.Analyze(systems, *margin)

	printSummary(hyperjump.Summarize(reports), *margin, len(systems))

	if *system != "" {
		r, ok := findOrigin(reports, *system)
		if !ok {
			log.Fatalf("system %q not found", *system)
		}
		printOriginDetail(r)
		reports = []hyperjump.OriginReport{r}
	}

	if *out != "" {
		if err := writeJSON(*out, reports); err != nil {
			log.Fatalf("write json: %v", err)
		}
		fmt.Printf("\nWrote %d origin report(s) to %s\n", len(reports), *out)
	}
}

// loadSystems reads all systems (id, name, position) from the KB, ordered by id.
func loadSystems(db *sql.DB) ([]hyperjump.System, error) {
	rows, err := db.Query(`SELECT id, name, position_x, position_y FROM systems ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var systems []hyperjump.System
	for rows.Next() {
		var s hyperjump.System
		if err := rows.Scan(&s.ID, &s.Name, &s.Pos.X, &s.Pos.Y); err != nil {
			return nil, err
		}
		systems = append(systems, s)
	}
	return systems, rows.Err()
}

func findOrigin(reports []hyperjump.OriginReport, id string) (hyperjump.OriginReport, bool) {
	for _, r := range reports {
		if r.System == id {
			return r, true
		}
	}
	return hyperjump.OriginReport{}, false
}

func printSummary(s hyperjump.Summary, margin float64, n int) {
	fmt.Printf("Hyper-jump analysis  (%d systems, margin %.0f GU)\n", n, margin)
	fmt.Printf("==================================================\n")
	fmt.Printf("Q1  Bearings computed for %d directed pairs\n", s.DirectedPairs)
	fmt.Printf("Q2  Reachable directly: %d (%.1f%%)   Blocked by an intervening system: %d (%.1f%%)\n",
		s.Reachable, pct(s.Reachable, s.DirectedPairs), s.Blocked, pct(s.Blocked, s.DirectedPairs))
	fmt.Printf("Q3  Heading margin over clean paths (deg):  min %.3f   median %.3f   max %.3f\n",
		s.MinMargin, s.MedianMargin, s.MaxMargin)
	fmt.Printf("Void  Systems with at least one escape direction: %d of %d\n", s.SystemsWithEscape, s.Systems)
}

func printOriginDetail(r hyperjump.OriginReport) {
	fmt.Printf("\nOrigin: %s\n", r.System)
	fmt.Printf("--------------------------------------------------\n")
	reachable := 0
	for _, p := range r.Pairs {
		if p.Reachable {
			reachable++
		}
	}
	fmt.Printf("Reachable destinations: %d of %d\n", reachable, len(r.Pairs))
	fmt.Printf("Void coverage: %.3f%%   escape gaps: %d\n", r.CoveragePct*100, len(r.Gaps))
	for i, g := range r.Gaps {
		if i >= 5 {
			fmt.Printf("  ... and %d more\n", len(r.Gaps)-5)
			break
		}
		fmt.Printf("  gap %7.3f deg wide  heading %7.3f -> %7.3f  (center %.3f)\n",
			g.WidthDeg, g.StartDeg, g.EndDeg, g.CenterDeg)
	}

	// Tightest clean paths (smallest margin of error) are the most fragile.
	clean := make([]hyperjump.Pair, 0, reachable)
	for _, p := range r.Pairs {
		if p.Reachable {
			clean = append(clean, p)
		}
	}
	sort.Slice(clean, func(i, j int) bool { return clean[i].AngularMargin < clean[j].AngularMargin })
	fmt.Printf("Tightest clean jumps (smallest heading margin):\n")
	for i, p := range clean {
		if i >= 5 {
			break
		}
		fmt.Printf("  -> %-20s bearing %7.3f  dist %9.1f  margin %.3f deg  clearance %.1f GU\n",
			p.To, p.Bearing, p.Distance, p.AngularMargin, p.Clearance)
	}
}

func writeJSON(path string, reports []hyperjump.OriginReport) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(reports)
}

func pct(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}
