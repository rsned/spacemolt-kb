package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func main() {
	outDir := "/tmp/land-ratio-test"
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	types := []string{"terran", "super_terran", "oceanic", "lava_world"}
	landPcts := []int{25, 30, 35, 40, 45, 50, 55, 60, 65, 70, 75, 80, 85}

	for _, tp := range types {
		fmt.Printf("Generating %s...\n", tp)
		for _, pct := range landPcts {
			// Clone the profile and adjust ocean level
			orig := planetgen.GetProfile(tp)
			p := *orig
			p.OceanLevel = 1.0 - float64(pct)/100.0

			seed := planetgen.HashSeedPublic(fmt.Sprintf("ratio_test_%s_%d", tp, pct))
			img := planetgen.RenderRocky(&p, seed, 600, 300)

			filename := fmt.Sprintf("%s_land%d.png", tp, pct)
			f, err := os.Create(filepath.Join(outDir, filename))
			if err != nil {
				panic(err)
			}
			if err := png.Encode(f, img); err != nil {
				_ = f.Close()
				panic(err)
			}
			_ = f.Close()
		}
	}

	// Generate HTML page
	html := `<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>Land Ratio Test</title>
<style>
body { background: #111; color: #ccc; font-family: system-ui; padding: 20px; }
h1 { text-align: center; }
h2 { margin-top: 30px; color: #aaa; }
.row { display: flex; flex-wrap: wrap; gap: 8px; }
.cell { text-align: center; }
.cell img { width: 300px; height: 150px; border-radius: 4px; }
.cell .label { font-size: 0.8em; color: #888; margin-top: 2px; }
</style></head><body>
<h1>Land/Liquid Ratio Test Grid</h1>
`
	for _, tp := range types {
		html += fmt.Sprintf("<h2>%s</h2>\n<div class=\"row\">\n", tp)
		for _, pct := range landPcts {
			filename := fmt.Sprintf("%s_land%d.png", tp, pct)
			html += fmt.Sprintf("  <div class=\"cell\"><img src=\"%s\"><div class=\"label\">%d%% land</div></div>\n", filename, pct)
		}
		html += "</div>\n"
	}
	html += "</body></html>"

	if err := os.WriteFile(filepath.Join(outDir, "index.html"), []byte(html), 0o644); err != nil {
		panic(err)
	}

	fmt.Printf("Done! Open: file://%s/index.html\n", outDir)
	fmt.Println("Or serve: python3 -m http.server 9124 --directory", outDir)
}
