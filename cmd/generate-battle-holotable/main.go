// Command generate-battle-holotable turns an exported battle replay into a
// holotable page: the replay JSON, a hull pack naming every ship class that
// appears, and a thin HTML page that loads both plus the shared renderer.
//
//	go run ./cmd/generate-battle-holotable --replay data/battles/<id>.json
//
// Design: docs/superpowers/specs/2026-08-16-battle-holotable-visualizer-design.md
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"

	_ "modernc.org/sqlite"
)

// battleIDPattern is every battle_id this generator has ever seen: 32 lowercase
// hex characters. Enforced before the id reaches filepath.Join so a replay
// carrying a path-traversal payload (e.g. "../../etc") can't write outside
// --out. The inputs today are trusted local fixtures, so this is hygiene
// rather than a live vulnerability — but it costs two lines.
var battleIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func main() {
	replayPath := flag.String("replay", "", "exported replay model JSON (required)")
	outDir := flag.String("out", "kb/battles", "directory to write the page and its data into")
	footprints := flag.String("footprints", "data/footprints/hy3d-svg", "directory of hy3d footprint SVGs")
	dbPath := flag.String("db", "spacemolt-knowledge.db", "knowledge database, for ship scale")
	flag.Parse()

	if *replayPath == "" {
		fmt.Fprintln(os.Stderr, "usage: generate-battle-holotable --replay data/battles/<id>.json")
		os.Exit(2)
	}

	raw, err := os.ReadFile(*replayPath)
	if err != nil {
		log.Fatalf("read replay: %v", err)
	}

	var rep Replay
	if err := json.Unmarshal(raw, &rep); err != nil {
		log.Fatalf("decode replay: %v", err)
	}
	if rep.BattleID == "" {
		log.Fatalf("replay %s has no battle_id", *replayPath)
	}
	if !battleIDPattern.MatchString(rep.BattleID) {
		log.Fatalf("replay %s has battle_id %q, want 32 lowercase hex characters", *replayPath, rep.BattleID)
	}

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	scales, err := LoadScales(db)
	if err != nil {
		log.Fatalf("load ship scales: %v", err)
	}

	pack, problems, err := BuildHullPack(rep, *footprints, scales)
	if err != nil {
		log.Fatalf("build hull pack: %v", err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("create output directory: %v", err)
	}

	// The replay is copied verbatim: the renderer consumes the model the
	// adapter already normalised, and re-encoding here would be a second place
	// for the shape to drift.
	writeFile(filepath.Join(*outDir, rep.BattleID+".json"), raw)

	packJSON, err := json.Marshal(pack)
	if err != nil {
		log.Fatalf("encode hull pack: %v", err)
	}
	writeFile(filepath.Join(*outDir, rep.BattleID+"-hulls.json"), packJSON)

	page, err := RenderPage(rep)
	if err != nil {
		log.Fatalf("render page: %v", err)
	}
	writeFile(filepath.Join(*outDir, rep.BattleID+".html"), page)

	var missing, ambiguous int
	for _, h := range pack {
		if h.Kind == kindMissing {
			missing++
			log.Printf("no footprint art for ship_class %q", h.Ship)
		}
		if h.FrameAmbiguous {
			ambiguous++
			log.Printf("footprint for %q is flagged frame-ambiguous", h.Ship)
		}
	}
	for ship, probs := range problems {
		for _, p := range probs {
			log.Printf("footprint for %q fails the asset contract: %s", ship, p)
		}
	}
	log.Printf("%s: %d participants, %d ship classes, %d without art, %d frame-ambiguous, %d with contract problems",
		rep.BattleID, len(rep.Participants), len(pack), missing, ambiguous, len(problems))
}

// writeFile writes or dies; every caller here treats a write failure as fatal.
func writeFile(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	log.Printf("wrote %s (%d bytes)", path, len(data))
}

// LoadScales reads the catalog hull scale for every ship. Scale drives relative
// draw size, so a scale-1 cobble and a scale-4 junk_convoy share a table
// correctly rather than rendering the same size.
func LoadScales(db *sql.DB) (map[string]int, error) {
	// COALESCE(scale, 0): a NULL scale anywhere in the table would otherwise
	// fail the int scan and abort the whole generator. 0 flows straight into
	// BuildHullPack's existing `scale <= 0 -> defaultScale` fallback, so this
	// is free — it just moves the "unknown scale" handling to one place.
	rows, err := db.Query(`SELECT id, COALESCE(scale, 0) FROM ships`)
	if err != nil {
		return nil, fmt.Errorf("query ships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	scales := make(map[string]int)
	for rows.Next() {
		var id string
		var scale int
		if err := rows.Scan(&id, &scale); err != nil {
			return nil, fmt.Errorf("scan ship: %w", err)
		}
		scales[id] = scale
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ships: %w", err)
	}
	return scales, nil
}
