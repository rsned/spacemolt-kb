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
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"

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
		outFile    = flag.String("out", "", "output equirect PNG path (single image mode)")
		dbPath     = flag.String("db", "../spacemolt-knowledge.db", "path to knowledge database")
		outDir     = flag.String("outdir", "kb/images/planets", "output directory for planet images")
		systemName = flag.String("system", "", "regenerate only planets in this system (batch mode subset)")
		faceSize   = flag.Int("face", planetgen.DefaultFaceSize, "cube-map face size in pixels")
		eqWidth    = flag.Int("width", planetgen.DefaultWidth, "equirect bake width in pixels")
		eqHeight   = flag.Int("height", planetgen.DefaultHeight, "equirect bake height in pixels")
		workers    = flag.Int("workers", 8, "number of parallel workers")
	)
	flag.Parse()

	// Single image mode: generate one test image
	if *singleType != "" && *singleSeed != "" {
		out := *outFile
		if out == "" {
			out = fmt.Sprintf("%s_%s.png", *singleType, sanitize(*singleSeed))
		}
		if err := generateSingle(*singleType, *singleSeed, *faceSize, *eqWidth, *eqHeight, out); err != nil {
			log.Fatal(err)
		}
		cubePath := cubePathFor(out)
		fmt.Printf("Generated %s + %s (face %d, equirect %dx%d)\n",
			out, cubePath, *faceSize, *eqWidth, *eqHeight)
		return
	}

	// Batch mode: generate all planets from database
	start := time.Now()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	planets, err := loadPlanets(db, *systemName)
	if err != nil {
		log.Fatalf("failed to load planets: %v", err)
	}
	if *systemName != "" {
		fmt.Printf("Found %d planets in system %q\n", len(planets), *systemName)
	} else {
		fmt.Printf("Found %d planets across all systems\n", len(planets))
	}

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

				if err := generateSingle(p.PlanetType, p.PlanetName, *faceSize, *eqWidth, *eqHeight, outPath); err != nil {
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

func generateSingle(planetType, planetName string, faceSize, equirectW, equirectH int, equirectPath string) error {
	cm, err := planetgen.Generate(planetType, planetName, faceSize)
	if err != nil {
		return err
	}

	// Derive the cube-map path from the equirect path: insert ".cube" before
	// the extension. foo.png → foo.cube.png. If no extension, append .cube.png.
	cubePath := cubePathFor(equirectPath)
	if err := cubemap.WriteCrossPNG(cm, cubePath); err != nil {
		return err
	}

	// Cloud overlay: archetypes with non-zero Cloud.Coverage produce a
	// separate <name>.clouds.cube.png companion to the main cube-map.
	// RenderCloudCubeMap returns nil when clouds are disabled (gas
	// giants, scorched, lava, etc.) — that's expected, not an error.
	if profile := planetgen.GetProfile(planetType); profile != nil {
		if cloudCM := render.RenderCloudCubeMap(profile, planetgen.HashSeedPublic(planetName), faceSize); cloudCM != nil {
			cloudPath := strings.TrimSuffix(cubePath, ".cube.png") + ".clouds.cube.png"
			if cloudPath == cubePath {
				cloudPath = cubePath + ".clouds"
			}
			if err := cubemap.WriteCrossPNG(cloudCM, cloudPath); err != nil {
				return err
			}
		}
	}

	img := cubemap.BakeEquirect(cm, equirectW, equirectH)
	f, err := os.Create(equirectPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	return png.Encode(f, img)
}

func loadPlanets(db *sql.DB, systemName string) ([]planet, error) {
	const baseQuery = `
		SELECT s.name, p.name, p.class
		FROM pois p
		JOIN systems s ON p.system_id = s.id
		WHERE p.type = 'planet' AND p.class != ''`
	var (
		rows *sql.Rows
		err  error
	)
	if systemName != "" {
		rows, err = db.Query(baseQuery+" AND s.name = ? ORDER BY p.name", systemName)
	} else {
		rows, err = db.Query(baseQuery + " ORDER BY s.name, p.name")
	}
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

// cubePathFor derives the cube-map output path from the equirect path.
// foo/bar.png → foo/bar.cube.png; foo (no extension) → foo.cube.png.
func cubePathFor(equirectPath string) string {
	if ext := filepath.Ext(equirectPath); ext != "" {
		return equirectPath[:len(equirectPath)-len(ext)] + ".cube" + ext
	}
	return equirectPath + ".cube.png"
}
