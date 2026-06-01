package main

import (
	"database/sql"
	"flag"
	htmltpl "html/template"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	flag.Parse()

	dbPath := "spacemolt-knowledge.db"
	factionsOut := "kb/factions"
	playersOut := "kb/players"
	systemsDir := "kb/systems"

	if args := flag.Args(); len(args) > 0 {
		dbPath = args[0]
	}

	genTime := time.Now().UTC()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Ships first (rosters and player pages both need them).
	shipClasses, shipDetail, err := loadShips(db)
	if err != nil {
		log.Fatalf("load ships: %v", err)
	}
	sysSlugs := knownSystemSlugs(systemsDir)

	factions, err := loadFactions(db, shipClasses)
	if err != nil {
		log.Fatalf("load factions: %v", err)
	}
	factionSlugByID := map[string]string{}
	for _, f := range factions {
		factionSlugByID[f.ID] = f.Slug
	}

	sightings, err := loadSightings(db, sysSlugs)
	if err != nil {
		log.Fatalf("load sightings: %v", err)
	}
	players, err := loadPlayers(db, shipDetail, sightings, factionSlugByID)
	if err != nil {
		log.Fatalf("load players: %v", err)
	}

	funcs := templateFuncs(genTime)
	fIdx := htmltpl.Must(htmltpl.New("fidx").Funcs(funcs).Parse(factionIndexTmpl))
	fDet := htmltpl.Must(htmltpl.New("fdet").Funcs(funcs).Parse(factionDetailTmpl))
	pIdx := htmltpl.Must(htmltpl.New("pidx").Funcs(funcs).Parse(playerIndexTmpl))
	pDet := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))

	// Clean + recreate output dirs, preserving the .css files.
	mustResetDir(factionsOut, "factions.css")
	mustResetDir(playersOut, "players.css")

	mustWrite(filepath.Join(factionsOut, "index.html"), fIdx, factions)
	for _, f := range factions {
		dir := filepath.Join(factionsOut, f.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), fDet, f)
	}

	mustWrite(filepath.Join(playersOut, "index.html"), pIdx, players)
	for _, p := range players {
		dir := filepath.Join(playersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), pDet, p)
	}

	log.Printf("generated %d factions and %d players", len(factions), len(players))
}

// mustResetDir removes generated HTML under dir (recursively for subdirs) but
// keeps the named file (the section CSS) and the dir itself.
func mustResetDir(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			mustMkdir(dir)
			return
		}
		log.Fatalf("read %s: %v", dir, err)
	}
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if err := os.RemoveAll(p); err != nil {
			log.Fatalf("remove %s: %v", p, err)
		}
	}
}

func mustMkdir(dir string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", dir, err)
	}
}

func mustWrite(path string, tmpl *htmltpl.Template, data any) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := tmpl.Execute(f, data); err != nil {
		log.Fatalf("render %s: %v", path, err)
	}
}
