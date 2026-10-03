package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

type job struct {
	ID    string `json:"job"`
	State string `json:"state"`
	Step  int    `json:"step"`
	Steps int    `json:"steps"`
	Slug  string `json:"slug"`
	Error string `json:"error,omitempty"`
}

type server struct {
	data, web string
	mu        sync.Mutex
	jobs      map[string]*job
	running   bool
}

func newServer(data, web string) *server {
	return &server{data: data, web: web, jobs: map[string]*job{}}
}

type bundleInfo struct {
	Slug      string `json:"slug"`
	Archetype string `json:"archetype"`
	Planet    string `json:"planet"`
	Seed      int64  `json:"seed"`
	Face      int    `json:"face"`
	Steps     int    `json:"steps"`
	Frames    int    `json:"frames"`
}

func (s *server) listBundles() []bundleInfo {
	entries, _ := os.ReadDir(s.data)
	var out []bundleInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := tectonics.ReadManifest(filepath.Join(s.data, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, bundleInfo{Slug: e.Name(), Archetype: m.Archetype, Planet: m.Planet,
			Seed: m.Seed, Face: m.Face, Steps: m.Steps, Frames: len(m.Frames)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	if out == nil {
		out = []bundleInfo{}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) startRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Planet    string   `json:"planet"`
		Seed      int64    `json:"seed"`
		Archetype string   `json:"archetype"`
		Face      int      `json:"face"`
		Steps     int      `json:"steps"`
		Sets      []string `json:"sets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// validate params up front so bad requests fail synchronously
	p, err := tectonics.DefaultParams(req.Archetype)
	if err == nil {
		for _, kv := range req.Sets {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				err = fmt.Errorf("-set %q: want Name=value", kv)
				break
			}
			if err = p.Set(k, v); err != nil {
				break
			}
		}
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a run is already in progress"})
		return
	}
	s.running = true
	j := &job{ID: fmt.Sprintf("%d", time.Now().UnixNano()), State: "running"}
	s.jobs[j.ID] = j
	s.mu.Unlock()

	go func() {
		dir, err := runBundle(runOpts{Planet: req.Planet, Seed: req.Seed, Archetype: req.Archetype,
			Out: s.data, Sets: req.Sets, Face: req.Face, Steps: req.Steps,
			Progress: func(step, steps int) {
				s.mu.Lock()
				j.Step, j.Steps = step, steps
				s.mu.Unlock()
			}})
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = false
		if err != nil {
			j.State, j.Error = "error", err.Error()
			return
		}
		j.State, j.Slug = "done", filepath.Base(dir)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"job": j.ID})
}

func (s *server) jobStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	j, ok := s.jobs[r.PathValue("id")]
	var cp job
	if ok {
		cp = *j
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such job"})
		return
	}
	writeJSON(w, http.StatusOK, cp)
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/bundles", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.listBundles()) })
	mux.HandleFunc("GET /api/archetypes", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, tectonics.Archetypes()) })
	mux.HandleFunc("POST /api/run", s.startRun)
	mux.HandleFunc("GET /api/jobs/{id}", s.jobStatus)
	mux.Handle("GET /bundles/", noCache(http.StripPrefix("/bundles/", http.FileServer(http.Dir(s.data)))))
	mux.Handle("/", noCache(http.FileServer(http.Dir(s.web))))
	return mux
}

func serve(addr, data, web string) error {
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	fmt.Printf("tectonics-lab viewer on http://localhost%s (bundles: %s)\n", addr, data)
	srv := &http.Server{Addr: addr, Handler: newServer(data, web).Handler(), ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
