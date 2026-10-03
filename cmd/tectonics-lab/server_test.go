package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerRunListAndFiles(t *testing.T) {
	data, web := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<html>viewer</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(newServer(data, web).Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/run", "application/json",
		strings.NewReader(`{"seed":3,"archetype":"arid","face":16,"steps":2,"sets":["KeyframeEvery=1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("run status %d", res.StatusCode)
	}
	var started struct{ Job string }
	_ = json.NewDecoder(res.Body).Decode(&started)
	_ = res.Body.Close()

	var st struct {
		State string
		Step  int
		Slug  string
		Error string
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != "done" && st.State != "error" && time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/jobs/" + started.Job)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(r.Body).Decode(&st)
		_ = r.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if st.State != "done" || st.Slug != "seed-3-arid" || st.Step != 2 {
		t.Fatalf("job %+v", st)
	}

	r, _ := http.Get(ts.URL + "/api/bundles")
	var list []struct {
		Slug   string
		Frames int
	}
	_ = json.NewDecoder(r.Body).Decode(&list)
	_ = r.Body.Close()
	if len(list) != 1 || list[0].Slug != "seed-3-arid" || list[0].Frames != 3 {
		t.Fatalf("bundles %+v", list)
	}
	for _, path := range []string{"/bundles/seed-3-arid/manifest.json", "/bundles/seed-3-arid/frames/0002/plate.png", "/"} {
		r, err := http.Get(ts.URL + path)
		if err != nil || r.StatusCode != 200 {
			t.Errorf("GET %s: %v %v", path, err, r)
		}
		if r != nil {
			_ = r.Body.Close()
		}
	}
	r, _ = http.Post(ts.URL+"/api/run", "application/json", strings.NewReader(`{"seed":1,"archetype":"jovian","face":16,"steps":1}`))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("jovian run status %d", r.StatusCode)
	}
	_ = r.Body.Close()
}

func TestServerSurvivesRunPanic(t *testing.T) {
	prev := runBundleFn
	runBundleFn = func(runOpts) (string, error) { panic("boom") }
	t.Cleanup(func() { runBundleFn = prev })

	data, web := t.TempDir(), t.TempDir()
	ts := httptest.NewServer(newServer(data, web).Handler())
	defer ts.Close()

	res, err := http.Post(ts.URL+"/api/run", "application/json",
		strings.NewReader(`{"seed":9,"archetype":"arid","face":16,"steps":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("run status %d", res.StatusCode)
	}
	var started struct{ Job string }
	_ = json.NewDecoder(res.Body).Decode(&started)
	_ = res.Body.Close()

	var st struct {
		State string
		Error string
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != "done" && st.State != "error" && time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/jobs/" + started.Job)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(r.Body).Decode(&st)
		_ = r.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if st.State != "error" || !strings.Contains(st.Error, "panic") {
		t.Fatalf("job %+v", st)
	}

	r2, err := http.Post(ts.URL+"/api/run", "application/json",
		strings.NewReader(`{"seed":9,"archetype":"arid","face":16,"steps":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if r2.StatusCode != http.StatusAccepted {
		t.Errorf("second run status %d, want 202 (server must survive the panic)", r2.StatusCode)
	}
	var started2 struct{ Job string }
	_ = json.NewDecoder(r2.Body).Decode(&started2)
	_ = r2.Body.Close()

	// Wait for the second job's goroutine to finish (it panics too, since
	// runBundleFn is still the injected fake) before the test returns and
	// t.Cleanup restores runBundleFn out from under it.
	var st2 struct{ State string }
	deadline = time.Now().Add(30 * time.Second)
	for st2.State != "done" && st2.State != "error" && time.Now().Before(deadline) {
		r, err := http.Get(ts.URL + "/api/jobs/" + started2.Job)
		if err != nil {
			t.Fatal(err)
		}
		_ = json.NewDecoder(r.Body).Decode(&st2)
		_ = r.Body.Close()
		time.Sleep(50 * time.Millisecond)
	}
	if st2.State == "" {
		t.Fatalf("second job never reached a terminal state: %+v", st2)
	}
}
