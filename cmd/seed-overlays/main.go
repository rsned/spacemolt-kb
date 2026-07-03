package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// personality is the subset of personality.json we read.
type personality struct {
	Name         string `json:"name"`
	Biography    string `json:"biography"`
	Organization string `json:"organization"`
	Role         string `json:"role"`
	SubRole      string `json:"sub_role"`
}

func main() {
	agentsDir := flag.String("agents", "../spacemolt/data/agents", "directory of agent personality dirs")
	dbPath := flag.String("db", "/home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db", "knowledge database path")
	outRoot := flag.String("overlays", "overlays", "overlays output root")
	dryRun := flag.Bool("dry-run", false, "report what would be written without writing")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// username(normalized) -> player_id
	players := map[string]string{}
	rows, err := db.Query(`SELECT player_id, username FROM seen_players WHERE username NOT LIKE '[%' AND player_id NOT LIKE 'npc%'`)
	if err != nil {
		log.Fatalf("query players: %v", err)
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatalf("scan player: %v", err)
		}
		players[normalizeName(name)] = id
	}
	_ = rows.Close()

	// org(normalized) -> faction_id
	factions := map[string]string{}
	frows, err := db.Query(`SELECT faction_id, name FROM factions`)
	if err != nil {
		log.Fatalf("query factions: %v", err)
	}
	for frows.Next() {
		var id, name string
		if err := frows.Scan(&id, &name); err != nil {
			log.Fatalf("scan faction: %v", err)
		}
		factions[normalizeOrg(name)] = id
	}
	_ = frows.Close()

	entries, err := os.ReadDir(*agentsDir)
	if err != nil {
		log.Fatalf("read agents dir: %v", err)
	}

	playerStubs, factionStubs := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, ok := readPersonality(filepath.Join(*agentsDir, e.Name(), "personality.json"))
		if !ok {
			continue
		}
		stats := []stubStat{}
		if p.Organization != "" {
			stats = append(stats, stubStat{"Organization", p.Organization})
		}
		if p.Role != "" {
			stats = append(stats, stubStat{"Role", p.Role})
		}
		if p.SubRole != "" {
			stats = append(stats, stubStat{"Sub-role", p.SubRole})
		}
		content := renderStub(p.Biography, stats)

		if pid, ok := players[normalizeName(p.Name)]; ok {
			path := filepath.Join(*outRoot, "players", pid, "profile.md")
			if wrote, err := writeStub(path, content, *dryRun); err != nil {
				log.Printf("warning: %s: %v", path, err)
			} else if wrote {
				playerStubs++
				log.Printf("player stub: %s (%s)", p.Name, pid)
			}
		}
		if fid, ok := factions[normalizeOrg(p.Organization)]; ok && p.Organization != "" {
			path := filepath.Join(*outRoot, "factions", fid, "profile.md")
			if wrote, err := writeStub(path, content, *dryRun); err != nil {
				log.Printf("warning: %s: %v", path, err)
			} else if wrote {
				factionStubs++
				log.Printf("faction stub: %s (%s)", p.Organization, fid)
			}
		}
	}

	verb := "wrote"
	if *dryRun {
		verb = "would write"
	}
	log.Printf("%s %d player and %d faction overlay stubs", verb, playerStubs, factionStubs)
}

func readPersonality(path string) (personality, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return personality{}, false
	}
	var p personality
	if err := json.Unmarshal(data, &p); err != nil {
		return personality{}, false
	}
	if strings.TrimSpace(p.Name) == "" {
		return personality{}, false
	}
	return p, true
}
