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
	portraitsFlag := flag.Bool("portraits", false, "generate missing/updated passenger AI portraits via SMKB_PORTRAIT_CMD before rendering")
	portraitLimit := flag.Int("portrait-limit", 0, "if >0, only (re)generate portraits for the first N passengers (alphabetical) — for quick verification")
	agentPortraitsFlag := flag.Bool("agent-portraits", false, "generate AI portraits for the user's own agents (sighted players only) via SMKB_PORTRAIT_CMD")
	portraitsOnly := flag.Bool("portraits-only", false, "generate portraits then exit without rendering the site (pairs with -portraits/-agent-portraits)")
	agentsDir := flag.String("agents-dir", "../spacemolt/data/agents", "directory of agent personality.json subdirectories (for -agent-portraits)")
	dailySummaryPath := flag.String("daily-summary", "../spacemolt/data/daily-summary.db", "daily-summary.db used to resolve agent_id -> server player_id (for -agent-portraits)")
	flag.Parse()

	dbPath := "spacemolt-knowledge.db"
	factionsOut := "kb/factions"
	playersOut := "kb/players"
	systemsDir := "kb/systems"
	passengersOut := "kb/passengers"

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

	attachFactionOverlays(factions, overlaysRoot)
	attachPlayerOverlays(players, overlaysRoot)
	if *agentPortraitsFlag {
		live := make(map[string]bool, len(players))
		for _, p := range players {
			live[p.ID] = true
		}
		personas, err := loadAgentPersonas(*agentsDir, *dailySummaryPath, live)
		if err != nil {
			log.Printf("warning: load agent personas: %v (agent portraits skipped)", err)
		}
		generateAgentPortraits(personas, overlaysRoot, os.Getenv("SMKB_PORTRAIT_CMD"), *portraitLimit)
	}
	attachPlayerPortraits(players, overlaysRoot)

	passengers, err := loadPassengers(db)
	if err != nil {
		log.Printf("warning: load passengers: %v (passenger pages skipped)", err)
		passengers = nil
	}
	attachPassengerOverlays(passengers, overlaysRoot)
	if *portraitsFlag {
		generatePassengerPortraits(passengers, overlaysRoot, os.Getenv("SMKB_PORTRAIT_CMD"), *portraitLimit)
	}
	attachPassengerPortraits(passengers, overlaysRoot)

	if *portraitsOnly {
		log.Printf("portraits-only: generation complete, skipping site render")
		return
	}

	funcs := templateFuncs(genTime)
	fIdx := htmltpl.Must(htmltpl.New("fidx").Funcs(funcs).Parse(factionIndexTmpl))
	fDet := htmltpl.Must(htmltpl.New("fdet").Funcs(funcs).Parse(factionDetailTmpl))
	pIdx := htmltpl.Must(htmltpl.New("pidx").Funcs(funcs).Parse(playerIndexTmpl))
	pDet := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))
	psIdx := htmltpl.Must(htmltpl.New("psidx").Funcs(funcs).Parse(passengerIndexTmpl))
	psDet := htmltpl.Must(htmltpl.New("psdet").Funcs(funcs).Parse(passengerDetailTmpl))

	// Clean + recreate output dirs, preserving the .css files.
	mustResetDir(factionsOut, "factions.css")
	mustResetDir(playersOut, "players.css")
	mustResetDir(passengersOut, "passengers.css")

	mustWrite(filepath.Join(factionsOut, "index.html"), fIdx, factions)
	for _, f := range factions {
		dir := filepath.Join(factionsOut, f.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), fDet, f)
		if f.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "factions", f.ID), f.Overlay.ImageFile, dir)
		}
	}

	mustWrite(filepath.Join(playersOut, "index.html"), pIdx, players)
	for _, p := range players {
		dir := filepath.Join(playersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), pDet, p)
		if p.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "players", p.ID), p.Overlay.ImageFile, dir)
		}
		if p.PortraitFile != "" {
			copyOverlayImage(playerGeneratedDir(overlaysRoot, p.ID), p.PortraitFile, dir)
		}
	}

	mustWrite(filepath.Join(passengersOut, "index.html"), psIdx, passengers)
	for _, p := range passengers {
		dir := filepath.Join(passengersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), psDet, p)
		if p.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "passengers", p.ID), p.Overlay.ImageFile, dir)
		}
		if p.PortraitFile != "" {
			copyOverlayImage(passengerGeneratedDir(overlaysRoot, p.ID), p.PortraitFile, dir)
		}
	}

	log.Printf("generated %d factions, %d players, %d passengers", len(factions), len(players), len(passengers))
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
