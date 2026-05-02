// Command planet-explorer hosts the web-based parameter explorer for
// the planet generator. It serves static assets from web/ and the
// compiled Wasm binary, exposing a UI for tuning PlanetProfile values
// interactively. See cmd/planet-explorer/README.md for build steps.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	webDir := flag.String("web", "cmd/planet-explorer/web", "path to web assets directory")
	wasmPath := flag.String("wasm", "cmd/planet-explorer/web/planet-explorer.wasm", "path to compiled wasm binary")
	useSwatch := flag.Bool("use_swatch_mode", true, "render rocky planets via the fast 2D flat path (default); --use_swatch_mode=false renders the full cube-sphere pipeline so sphere-global stages (plates, jitter cells, JFA coastal) are exercised")
	flag.Parse()

	abs, err := filepath.Abs(*webDir)
	if err != nil {
		log.Fatalf("resolve web dir: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		log.Fatalf("web dir %s not found: %v", abs, err)
	}
	indexPath := filepath.Join(abs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		log.Fatalf("index.html not found at %s: %v", indexPath, err)
	}

	fileServer := http.FileServer(http.Dir(abs))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			b, err := os.ReadFile(indexPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			configScript := fmt.Sprintf(
				`<script>window.PLANET_EXPLORER_CONFIG = {useSwatchMode: %t};</script>`,
				*useSwatch,
			)
			out := strings.Replace(string(b), "<!--PLANET_EXPLORER_CONFIG-->", configScript, 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(out))
			return
		}
		fileServer.ServeHTTP(w, r)
	})
	mux.HandleFunc("/wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, *wasmPath)
	})

	log.Printf("planet-explorer dev server reachable at:")
	for _, url := range listenURLs(*addr) {
		log.Printf("  %s", url)
	}
	log.Printf("serving web assets from: %s", abs)
	log.Printf("serving wasm from: %s", *wasmPath)
	log.Printf("use_swatch_mode: %t", *useSwatch)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// listenURLs builds the list of HTTP URLs the dev server is reachable at,
// given a flag-provided -addr like ":8080" or "0.0.0.0:8080".  When the
// host portion is empty/0.0.0.0/[::], the server binds to every interface
// on the host, so we enumerate non-loopback addresses for LAN access.
func listenURLs(addr string) []string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return []string{"http://" + addr}
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		return []string{"http://" + net.JoinHostPort(host, port)}
	}
	out := []string{"http://localhost:" + port}
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range ifaces {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() {
			continue
		}
		ip := ipnet.IP
		if ip4 := ip.To4(); ip4 != nil {
			out = append(out, "http://"+net.JoinHostPort(ip4.String(), port))
			continue
		}
		// IPv6: skip link-local (fe80::/10); they won't resolve from LAN peers.
		if strings.HasPrefix(ip.String(), "fe80:") {
			continue
		}
		out = append(out, "http://"+net.JoinHostPort(ip.String(), port))
	}
	return out
}
