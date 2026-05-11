// Command planet-explorer hosts the web-based parameter explorer for
// the planet generator. It serves static assets from web/ and the
// compiled Wasm binary, exposing a UI for tuning PlanetProfile values
// interactively. See cmd/planet-explorer/README.md for build steps.
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

func main() {
	addr := flag.String("addr", ":8080", "address to listen on")
	webDir := flag.String("web", "cmd/planet-explorer/web", "path to web assets directory")
	wasmPath := flag.String("wasm", "cmd/planet-explorer/web/planet-explorer.wasm", "path to compiled wasm binary")
	profilesDir := flag.String("profiles-dir", "data/planet-profiles", "directory of per-planet envelope JSON files")
	readonly := flag.Bool("readonly", false, "reject PUTs (treat /profiles/<slug> as read-only)")
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

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(abs)))
	mux.HandleFunc("/wasm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/wasm")
		http.ServeFile(w, r, *wasmPath)
	})
	mux.HandleFunc("/profiles", func(w http.ResponseWriter, r *http.Request) {
		handleList(w, r, *profilesDir)
	})
	mux.HandleFunc("/profiles/", func(w http.ResponseWriter, r *http.Request) {
		handleOne(w, r, *profilesDir, *readonly)
	})

	log.Printf("planet-explorer dev server reachable at:")
	for _, url := range listenURLs(*addr) {
		log.Printf("  %s", url)
	}
	log.Printf("serving web assets from: %s", abs)
	log.Printf("serving wasm from: %s", *wasmPath)
	log.Printf("serving profiles from: %s (readonly=%v)", *profilesDir, *readonly)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// handleList serves GET /profiles — a JSON array of available slugs
// (filename stems). Always returns a JSON array, never null.
func handleList(w http.ResponseWriter, r *http.Request, dir string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	slugs := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		slugs = append(slugs, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(slugs)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(slugs)
}

// handleOne serves GET / PUT /profiles/<slug>. PUT validates that the
// envelope content matches the URL slug (rejecting 409 otherwise).
func handleOne(w http.ResponseWriter, r *http.Request, dir string, readonly bool) {
	slug := strings.TrimPrefix(r.URL.Path, "/profiles/")
	if slug == "" || strings.ContainsAny(slug, "/\\") {
		http.Error(w, "bad slug", http.StatusBadRequest)
		return
	}
	if profilejson.PlanetSlug(slug) != slug {
		http.Error(w, "slug must match [a-z0-9_]+", http.StatusBadRequest)
		return
	}
	path := filepath.Join(dir, slug+".json")
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	case http.MethodPut:
		if readonly {
			http.Error(w, "server is read-only", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		env, err := profilejson.Decode(body)
		if err != nil {
			http.Error(w, "invalid envelope: "+err.Error(), http.StatusBadRequest)
			return
		}
		// URL slug must match the envelope seed's slug — the picker
		// and Save-as-new flow both PUT to the slug they advertise,
		// so a mismatch is almost certainly a client bug.
		if profilejson.PlanetSlug(env.Seed) != slug {
			http.Error(w,
				"slug in URL does not match envelope seed",
				http.StatusConflict)
			return
		}
		// Re-encode rather than echoing body bytes so the on-disk file
		// is always in canonical form (sorted keys, 2-space indent,
		// trailing newline).
		out, err := profilejson.Encode(env)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(path, out, 0o644); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		if strings.HasPrefix(ip.String(), "fe80:") {
			continue
		}
		out = append(out, "http://"+net.JoinHostPort(ip.String(), port))
	}
	return out
}
