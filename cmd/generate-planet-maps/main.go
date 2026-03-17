package main

import (
	"database/sql"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"

	_ "modernc.org/sqlite"
)

type planet struct {
	SystemName string
	PlanetName string
	PlanetType string
}

func main() {
	var (
		singleType = flag.String("type", "", "generate only this planet type (for testing)")
		singleSeed = flag.String("seed", "", "planet name to use as seed (for testing)")
		outFile    = flag.String("out", "", "output file path (single image mode)")
		dbPath     = flag.String("db", "../spacemolt-knowledge.db", "path to knowledge database")
		outDir     = flag.String("outdir", "kb/images/planets", "output directory for planet images")
		width      = flag.Int("width", planetgen.DefaultWidth, "image width in pixels")
		height     = flag.Int("height", planetgen.DefaultHeight, "image height in pixels")
		workers    = flag.Int("workers", 8, "number of parallel workers")
	)
	flag.Parse()

	// Single image mode: generate one test image
	if *singleType != "" && *singleSeed != "" {
		out := *outFile
		if out == "" {
			out = fmt.Sprintf("%s_%s.png", *singleType, sanitize(*singleSeed))
		}
		if err := generateSingle(*singleType, *singleSeed, *width, *height, out); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Generated %s (%dx%d)\n", out, *width, *height)
		return
	}

	// Batch mode: generate all planets from database
	start := time.Now()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	planets, err := loadPlanets(db)
	if err != nil {
		log.Fatalf("failed to load planets: %v", err)
	}
	fmt.Printf("Found %d planets across all systems\n", len(planets))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	// Process planets in parallel
	var wg sync.WaitGroup
	ch := make(chan planet, len(planets))
	var generated, skipped int
	var mu sync.Mutex

	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range ch {
				filename := fmt.Sprintf("%s_%s.png", sanitize(p.SystemName), sanitize(p.PlanetName))
				outPath := filepath.Join(*outDir, filename)

				if err := generateSingle(p.PlanetType, p.PlanetName, *width, *height, outPath); err != nil {
					log.Printf("ERROR: %s/%s (%s): %v", p.SystemName, p.PlanetName, p.PlanetType, err)
					mu.Lock()
					skipped++
					mu.Unlock()
					continue
				}

				mu.Lock()
				generated++
				if generated%50 == 0 {
					fmt.Printf("  Generated %d/%d...\n", generated, len(planets))
				}
				mu.Unlock()
			}
		}()
	}

	for _, p := range planets {
		ch <- p
	}
	close(ch)
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("Done: %d generated, %d skipped in %s\n", generated, skipped, elapsed.Round(time.Millisecond))
}

func generateSingle(planetType, planetName string, width, height int, outPath string) error {
	img, err := planetgen.Generate(planetType, planetName, width, height)
	if err != nil {
		return err
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return png.Encode(f, img)
}

func loadPlanets(db *sql.DB) ([]planet, error) {
	rows, err := db.Query(`
		SELECT s.name, p.name, p.class
		FROM pois p
		JOIN systems s ON p.system_id = s.id
		WHERE p.type = 'planet' AND p.class != ''
		ORDER BY s.name, p.name
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var planets []planet
	for rows.Next() {
		var p planet
		if err := rows.Scan(&p.SystemName, &p.PlanetName, &p.PlanetType); err != nil {
			return nil, err
		}
		planets = append(planets, p)
	}
	return planets, rows.Err()
}

func sanitize(name string) string {
	r := strings.NewReplacer(" ", "_", "'", "", "/", "_", "\\", "_")
	return strings.ToLower(r.Replace(name))
}
