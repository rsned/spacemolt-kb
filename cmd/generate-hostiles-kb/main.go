// Command generate-hostiles-kb renders kb/hostiles/: hostile NPCs agents have
// met, from the knowledge database's hostile_types, hostile_sightings and
// hostile_attacks tables (filled by the game client from get_nearby and
// battle logs). Two subsections: Frontier (repeaters in uncharted space) and
// Pirates. Creatures stay in the Wildlife section. Run from the kb repo root.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database")
	outDir := flag.String("out", "kb/hostiles", "output directory")
	flag.Parse()

	db, err := sql.Open("sqlite", "file:"+*dbPath+"?mode=ro")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	types, err := loadTypes(db)
	if err != nil {
		log.Fatal(err)
	}
	if err := writePages(*outDir, types); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote hostiles: %d types (%d frontier, %d pirate) → %s\n",
		len(types), len(ofKind(types, "repeater")), len(ofKind(types, "pirate")), *outDir)
}

// writePages renders the landing page and the two subsections.
func writePages(outDir string, types []*HostileType) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	frontier := Page{Title: "Frontier",
		Intro: "Repeaters: hostile units met in the uncharted frontier systems beyond the charted map. They fight in groups and so far deal energy damage.",
		Empty: "No repeaters recorded yet.",
		Types: ofKind(types, "repeater")}
	pirates := Page{Title: "Pirates",
		Intro: "Pirate stronghold crews: hostile NPC ships flying for the pirate strongholds.",
		Empty: "No pirate data has been recorded yet. The game client captures hostile stats from get_nearby and battle logs; pirate capture is still to come.",
		Types: ofKind(types, "pirate")}
	for name, data := range map[string]any{
		"frontier.html": map[string]any{"Title": frontier.Title, "Header": header(), "P": frontier},
		"pirates.html":  map[string]any{"Title": pirates.Title, "Header": header(), "P": pirates},
	} {
		out, err := render(listTmpl, data)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, name), out, 0o644); err != nil {
			return err
		}
	}
	idx, err := render(indexTmpl, map[string]any{"Title": "Hostiles", "Header": header(),
		"Frontier": summary(frontier), "Pirates": summary(pirates)})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "index.html"), idx, 0o644)
}
