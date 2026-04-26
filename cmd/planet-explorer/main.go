// Command planet-explorer hosts the web-based parameter explorer for
// the planet generator. It serves static assets from web/ and the
// compiled Wasm binary, exposing a UI for tuning PlanetProfile values
// interactively. See cmd/planet-explorer/README.md for build steps.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	webDir := flag.String("web", "cmd/planet-explorer/web", "path to web assets directory")
	wasmPath := flag.String("wasm", "cmd/planet-explorer/web/planet-explorer.wasm", "path to compiled wasm binary")
	flag.Parse()

	abs, err := filepath.Abs(*webDir)
	if err != nil {
		log.Fatalf("resolve web dir: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("web dir %s not found: %v", abs, err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(abs)))
	mux.HandleFunc("/wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, *wasmPath)
	})

	log.Printf("planet-explorer dev server: http://localhost%s", *addr)
	log.Printf("serving web assets from: %s", abs)
	log.Printf("serving wasm from: %s", *wasmPath)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
