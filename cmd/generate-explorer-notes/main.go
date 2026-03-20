package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	_ "modernc.org/sqlite"
)

// ExplorerNote is a single discovery note for a POI.
type ExplorerNote struct {
	POIID      string `json:"poi_id"`
	POIName    string `json:"poi_name"`
	POIType    string `json:"poi_type"`
	POIClass   string `json:"poi_class,omitempty"`
	SystemName string `json:"system_name"`
	Note       string `json:"note"`
	Model      string `json:"model"`
	Generated  string `json:"generated"`
}

// NotesFile is the top-level JSON structure.
type NotesFile struct {
	Explorer string          `json:"explorer"`
	Bio      string          `json:"bio"`
	Notes    []*ExplorerNote `json:"notes"`
}

type poiRecord struct {
	ID          string
	Name        string
	Type        string
	Class       string
	Description string
	SystemName  string
	Resources   []string
}

const explorerName = "Captain Vex Moreau"
const explorerBio = "Senior Field Surveyor, Intergalactic Explorer's Guild. 23 years of service. 612 systems cataloged."

const systemPrompt = `You are ghostwriting the field journal of Captain Vex Moreau, a seasoned intergalactic explorer who has surveyed over 600 star systems. Write a single discovery note (2-4 sentences) for the given point of interest.

Character traits:
- Genuinely awed by spectacular phenomena (stellar nurseries, black holes, alien artifacts)
- Wryly amused by the mundane ("another metallic asteroid belt")
- Occasionally personal — references past visits, compares to things seen before
- Scientific curiosity mixed with poetic observation
- Slight weariness about bureaucratic stations and well-trodden trade routes
- Uses humor to cope with the vastness of space

Rules:
- Write ONLY the journal entry paragraph, no headers or labels
- 2-4 sentences, conversational but literate
- If resources are listed, you may reference 1-2 of them naturally (use exact names)
- If there's a description, incorporate its flavor but don't repeat it verbatim
- ONLY reference elements, ores, ices, and gases from the provided resource list — never invent materials
- Each entry should feel distinct — avoid repetitive sentence structures
- For mundane POIs (common asteroid belts, basic stations), lean into dry wit
- For extraordinary POIs (nurseries, relics, anomalies), let the wonder show`

func main() {
	var (
		dbPath   = flag.String("db", "../spacemolt-knowledge.db", "path to knowledge database")
		outFile  = flag.String("out", "data/explorer-notes.json", "output JSON file")
		poiType  = flag.String("type", "", "only generate for this POI type")
		poiID    = flag.String("poi", "", "only generate for this specific POI ID")
		limit    = flag.Int("limit", 0, "limit number of notes to generate (0 = all)")
		model    = flag.String("model", "claude-haiku-4-5-20251001", "Claude model to use")
		dryRun   = flag.Bool("dry-run", false, "print prompts without calling API")
	)
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Load POIs
	pois, err := loadPOIs(db, *poiType, *poiID)
	if err != nil {
		log.Fatalf("load pois: %v", err)
	}
	fmt.Printf("Loaded %d POIs\n", len(pois))

	if *limit > 0 && len(pois) > *limit {
		pois = pois[:*limit]
	}

	// Load existing notes file if it exists (for incremental generation)
	notes := &NotesFile{
		Explorer: explorerName,
		Bio:      explorerBio,
	}
	existingIndex := map[string]int{} // poi_id → index in notes.Notes
	if data, err := os.ReadFile(*outFile); err == nil {
		if err := json.Unmarshal(data, notes); err != nil {
			log.Printf("WARNING: could not parse existing notes file: %v", err)
		} else {
			for i, n := range notes.Notes {
				existingIndex[n.POIID] = i
			}
			fmt.Printf("Loaded %d existing notes\n", len(notes.Notes))
		}
	}

	// Filter out POIs that already have notes (unless regenerating specific POI)
	var toGenerate []poiRecord
	for _, p := range pois {
		if *poiID != "" || existingIndex[p.ID] == 0 {
			// Check it's truly missing (index 0 could be the first entry)
			if *poiID != "" {
				toGenerate = append(toGenerate, p)
			} else if _, exists := existingIndex[p.ID]; !exists {
				toGenerate = append(toGenerate, p)
			}
		}
	}
	fmt.Printf("Need to generate %d new notes\n", len(toGenerate))

	if len(toGenerate) == 0 {
		fmt.Println("Nothing to do.")
		return
	}

	if *dryRun {
		for _, p := range toGenerate {
			fmt.Printf("\n=== %s (%s/%s) in %s ===\n", p.Name, p.Type, p.Class, p.SystemName)
			fmt.Println(buildUserPrompt(p))
		}
		return
	}

	// Create Anthropic client
	client := anthropic.NewClient()

	generated := 0
	for i, p := range toGenerate {
		note, err := generateNote(context.Background(), client, *model, p)
		if err != nil {
			log.Printf("ERROR generating note for %s: %v", p.Name, err)
			continue
		}

		entry := &ExplorerNote{
			POIID:      p.ID,
			POIName:    p.Name,
			POIType:    p.Type,
			POIClass:   p.Class,
			SystemName: p.SystemName,
			Note:       note,
			Model:      *model,
			Generated:  time.Now().Format(time.RFC3339),
		}

		// Update or append
		if idx, exists := existingIndex[p.ID]; exists {
			notes.Notes[idx] = entry
		} else {
			notes.Notes = append(notes.Notes, entry)
			existingIndex[p.ID] = len(notes.Notes) - 1
		}

		generated++
		if generated%10 == 0 {
			fmt.Printf("  Generated %d/%d...\n", generated, len(toGenerate))
			// Save progress periodically
			if err := saveNotes(*outFile, notes); err != nil {
				log.Printf("WARNING: failed to save progress: %v", err)
			}
		}

		// Brief pause to avoid rate limiting
		if i < len(toGenerate)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if err := saveNotes(*outFile, notes); err != nil {
		log.Fatalf("save notes: %v", err)
	}
	fmt.Printf("Done: %d notes generated, %d total in file\n", generated, len(notes.Notes))
}

func generateNote(ctx context.Context, client anthropic.Client, model string, p poiRecord) (string, error) {
	prompt := buildUserPrompt(p)

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 200,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt, Type: "text"},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return "", err
	}

	for _, block := range resp.Content {
		if block.Type == "text" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "", fmt.Errorf("no text in response")
}

func buildUserPrompt(p poiRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "POI: %s\n", p.Name)
	fmt.Fprintf(&b, "Type: %s\n", p.Type)
	if p.Class != "" {
		fmt.Fprintf(&b, "Class: %s\n", p.Class)
	}
	fmt.Fprintf(&b, "System: %s\n", p.SystemName)
	if p.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", p.Description)
	}
	if len(p.Resources) > 0 {
		fmt.Fprintf(&b, "Resources found: %s\n", strings.Join(p.Resources, ", "))
	}
	return b.String()
}

func loadPOIs(db *sql.DB, filterType, filterID string) ([]poiRecord, error) {
	query := `
		SELECT p.id, p.name, p.type, COALESCE(p.class,''), COALESCE(p.description,''), s.name
		FROM pois p
		JOIN systems s ON p.system_id = s.id
		WHERE 1=1
	`
	var args []any
	if filterType != "" {
		query += " AND p.type = ?"
		args = append(args, filterType)
	}
	if filterID != "" {
		query += " AND p.id = ?"
		args = append(args, filterID)
	}
	query += " ORDER BY s.name, p.type, p.name"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var pois []poiRecord
	poiIndex := map[string]int{}
	for rows.Next() {
		var p poiRecord
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Class, &p.Description, &p.SystemName); err != nil {
			return nil, err
		}
		poiIndex[p.ID] = len(pois)
		pois = append(pois, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Load resources
	resRows, err := db.Query(`
		SELECT pr.poi_id, i.name
		FROM poi_resources pr
		JOIN items i ON pr.resource_id = i.id
		ORDER BY pr.poi_id, i.name
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resRows.Close() }()

	for resRows.Next() {
		var poiID, resName string
		if err := resRows.Scan(&poiID, &resName); err != nil {
			return nil, err
		}
		if idx, ok := poiIndex[poiID]; ok {
			pois[idx].Resources = append(pois[idx].Resources, resName)
		}
	}
	return pois, resRows.Err()
}

func saveNotes(path string, notes *NotesFile) error {
	if err := os.MkdirAll("data", 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(notes, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
