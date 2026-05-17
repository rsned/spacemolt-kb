package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// newTestServer wires the two /profiles handlers to a fresh tmp dir
// and returns the server plus its profile dir. readonly toggles the
// equivalent of the -readonly flag.
func newTestServer(t *testing.T, readonly bool) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	mux := http.NewServeMux()
	mux.HandleFunc("/profiles", func(w http.ResponseWriter, r *http.Request) {
		handleList(w, r, dir)
	})
	mux.HandleFunc("/profiles/", func(w http.ResponseWriter, r *http.Request) {
		handleOne(w, r, dir, readonly)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, dir
}

func writeFixture(t *testing.T, dir, slug, planetType, seed string) {
	t.Helper()
	writeFixtureWithName(t, dir, slug, planetType, seed, "")
}

func writeFixtureWithName(t *testing.T, dir, slug, planetType, seed, name string) {
	t.Helper()
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          planetType,
		Seed:          seed,
		Name:          name,
		Profile:       planetgen.GetProfile(planetType),
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProfilesList(t *testing.T) {
	ts, dir := newTestServer(t, false)
	writeFixture(t, dir, "terran_default", "terran", "terran_default")
	writeFixtureWithName(t, dir, "scorched_default", "scorched", "scorched_default", "Mercury Analog")

	res, err := http.Get(ts.URL + "/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var got []profileListItem
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := []profileListItem{
		{Slug: "scorched_default", Name: "Mercury Analog"},
		{Slug: "terran_default"},
	}
	if len(got) != len(want) {
		t.Fatalf("list length = %d, want %d (got=%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("list[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestProfilesListEmpty(t *testing.T) {
	ts, _ := newTestServer(t, false)
	res, err := http.Get(ts.URL + "/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(res.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("empty list returned %q, want %q", body, "[]")
	}
}

func TestProfilesGetOne(t *testing.T) {
	ts, dir := newTestServer(t, false)
	writeFixture(t, dir, "terran_default", "terran", "terran_default")
	res, err := http.Get(ts.URL + "/profiles/terran_default")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	env, err := profilejson.Decode(body)
	if err != nil {
		t.Fatal(err)
	}
	if env.Type != "terran" {
		t.Errorf("envelope type = %q", env.Type)
	}
}

func TestProfilesGetMissing(t *testing.T) {
	ts, _ := newTestServer(t, false)
	res, err := http.Get(ts.URL + "/profiles/nowhere")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

func TestProfilesPutRoundTrip(t *testing.T) {
	ts, dir := newTestServer(t, false)
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "my_planet",
		HandTuned:     true,
		Profile:       planetgen.GetProfile("terran"),
	}
	body, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/my_planet", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(res.Body)
		t.Fatalf("PUT status = %d, body = %s", res.StatusCode, errBody)
	}
	written, err := os.ReadFile(filepath.Join(dir, "my_planet.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, body) {
		t.Errorf("written bytes != PUT body (re-encoding should be byte-stable)")
	}
}

func TestProfilesPutSlugMismatch(t *testing.T) {
	ts, _ := newTestServer(t, false)
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "different_seed", // slug = "different_seed"
		Profile:       planetgen.GetProfile("terran"),
	}
	body, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/my_planet", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", res.StatusCode)
	}
}

// TestProfilesPutBlocksDefaultOverwrite verifies that PUT-ing to any
// slug in defaultProtectedSlugs without ?force=1 returns 409.
func TestProfilesPutBlocksDefaultOverwrite(t *testing.T) {
	ts, _ := newTestServer(t, false)
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     true,
		Profile:       planetgen.GetProfile("terran"),
	}
	body, _ := profilejson.Encode(env)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/terran_default", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 (default protection), got %d", res.StatusCode)
	}
}

// TestProfilesPutDefaultWithForce verifies the ?force=1 escape hatch
// allows the same overwrite to succeed.
func TestProfilesPutDefaultWithForce(t *testing.T) {
	ts, dir := newTestServer(t, false)
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     true,
		Profile:       planetgen.GetProfile("terran"),
	}
	body, _ := profilejson.Encode(env)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/terran_default?force=1", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(res.Body)
		t.Fatalf("PUT with ?force=1: status = %d, body = %s", res.StatusCode, errBody)
	}
	if _, err := os.Stat(filepath.Join(dir, "terran_default.json")); err != nil {
		t.Errorf("expected file written, got %v", err)
	}
}

func TestProfilesPutBadJSON(t *testing.T) {
	ts, _ := newTestServer(t, false)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/anything", strings.NewReader("{not json"))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", res.StatusCode)
	}
}

func TestProfilesPutReadonly(t *testing.T) {
	ts, _ := newTestServer(t, true)
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		Profile:       planetgen.GetProfile("terran"),
	}
	body, _ := profilejson.Encode(env)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/terran_default", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", res.StatusCode)
	}
}