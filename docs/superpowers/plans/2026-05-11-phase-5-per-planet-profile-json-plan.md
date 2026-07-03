# Phase 5 — Per-Planet Profile JSON Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move per-planet rendering parameters from in-code `Profiles[]` defaults to disk-resident JSON files via an envelope wrapper, so any planet can be hand-tuned without editing Go code while preserving determinism and golden-test stability for planets without a JSON file.

**Architecture:** New library package `pkg/planetgen/profilejson/` owns Encode / Decode / Migrate / LoadForPlanet and the file-naming slug. `planetgen.Generate` gets a `resolveProfile` hook that consults the profile root before falling back to `GetProfile`. A thin seeder CLI (`cmd/tools/seed-planet-profiles`) writes byte-stable envelopes for an explicit planet list, used both to bake initial fixtures and as the future bulk-seeder. The dev server (`cmd/planet-explorer`) gains three REST endpoints (list / get / put) and the web UI gains a planet picker plus Save / Save-as-new buttons. The wasm build sets profileRoot to "" so it never touches disk. A CI drift test compares each non-`handTuned: true` committed envelope byte-for-byte against a freshly-Encoded default.

**Tech Stack:** Go 1.24+, `encoding/json`, `net/http`, `net/http/httptest`, vanilla JavaScript (no framework), existing `pkg/planetgen/types` schema.

**Spec reference:** `docs/superpowers/specs/2026-05-07-phase-5-per-planet-profile-json-design.md`.

---

## File Structure

**New files:**
- `pkg/planetgen/profilejson/slug.go` — `PlanetSlug(name)` helper
- `pkg/planetgen/profilejson/slug_test.go`
- `pkg/planetgen/profilejson/envelope.go` — `Envelope` struct, `Encode`, `Decode`, `Migrate`, `CurrentSchemaVersion`
- `pkg/planetgen/profilejson/envelope_test.go`
- `pkg/planetgen/profilejson/migrate_test.go`
- `pkg/planetgen/profilejson/store.go` — `LoadForPlanet(rootDir, name)`
- `pkg/planetgen/profilejson/store_test.go`
- `pkg/planetgen/profilejson/drift_test.go` — CI guard
- `pkg/planetgen/profilejson/doc.go`
- `cmd/tools/seed-planet-profiles/main.go`
- `cmd/tools/seed-planet-profiles/main_test.go`
- `cmd/tools/seed-planet-profiles/README.md`
- `data/planet-profiles/terran_default.json`
- `data/planet-profiles/super_terran_default.json`
- `data/planet-profiles/scorched_default.json`
- `cmd/planet-explorer/main_test.go`

**Modified files:**
- `pkg/planetgen/planetgen.go` — add `profileRoot` var, `SetProfileRoot`, `resolveProfile`; modify `Generate`
- `pkg/planetgen/planetgen_test.go` (create if absent) — integration tests
- `cmd/planet-explorer/main.go` — add `-profiles-dir`, `-readonly` flags + 3 endpoints
- `cmd/planet-explorer/wasm/main.go` — call `planetgen.SetProfileRoot("")` at startup
- `cmd/planet-explorer/web/index.html` — add planet picker, Save, Save-as-new buttons
- `cmd/planet-explorer/web/app.js` — picker population + envelope pack/unpack + PUT
- `cmd/planet-explorer/README.md` — document the picker, Save workflow, profile directory

---

## Chunk 1 — profilejson library

### Task 1: PlanetSlug helper

**Files:**
- Create: `pkg/planetgen/profilejson/doc.go`
- Create: `pkg/planetgen/profilejson/slug.go`
- Create: `pkg/planetgen/profilejson/slug_test.go`

- [ ] **Step 1: Create the package doc file**

```go
// Package profilejson is the on-disk envelope format for
// per-planet PlanetProfile overrides. The envelope wraps the
// existing types.PlanetProfile with schema versioning and dispatch
// metadata; the wasm contract is unchanged because the envelope is
// unwrapped before the profile reaches the renderer.
//
// See docs/superpowers/specs/2026-05-07-phase-5-per-planet-profile-json-design.md.
package profilejson
```

Write to `pkg/planetgen/profilejson/doc.go`.

- [ ] **Step 2: Write the failing test**

```go
package profilejson

import "testing"

func TestPlanetSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple ascii", "Earth", "earth"},
		{"spaces become underscores", "82 Eridani II", "82_eridani_ii"},
		{"hyphens become underscores", "Alpha-Centauri-Bb", "alpha_centauri_bb"},
		{"multiple separators collapse", "  Foo--Bar  ", "foo_bar"},
		{"strip non-alnum", "Wolf 359 ψ", "wolf_359"},
		{"empty stays empty", "", ""},
		{"already-slug passthrough", "terran_default", "terran_default"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlanetSlug(tc.in); got != tc.want {
				t.Errorf("PlanetSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
```

Write to `pkg/planetgen/profilejson/slug_test.go`.

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestPlanetSlug -v`
Expected: FAIL — `undefined: PlanetSlug`.

- [ ] **Step 4: Write minimal implementation**

```go
package profilejson

import (
	"strings"
	"unicode"
)

// PlanetSlug normalizes a planet name into a filename-safe slug
// matching the regex [a-z0-9_]+. Non-alphanumeric runes are
// treated as separators; consecutive separators collapse into a
// single underscore; leading and trailing underscores are
// trimmed.
func PlanetSlug(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	prevSep := true
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) && r < 128, unicode.IsDigit(r) && r < 128:
			b.WriteRune(r)
			prevSep = false
		default:
			if !prevSep {
				b.WriteByte('_')
				prevSep = true
			}
		}
	}
	out := b.String()
	return strings.Trim(out, "_")
}
```

Write to `pkg/planetgen/profilejson/slug.go`.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestPlanetSlug -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/profilejson/
git commit -m "feat(profilejson): PlanetSlug helper for filename-safe slugs"
```

---

### Task 2: Envelope Encode / Decode round-trip

**Files:**
- Create: `pkg/planetgen/profilejson/envelope.go`
- Create: `pkg/planetgen/profilejson/envelope_test.go`

- [ ] **Step 1: Write the failing round-trip test**

```go
package profilejson

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	prof := planetgen.GetProfile("terran")
	if prof == nil {
		t.Fatal("terran profile missing")
	}
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     false,
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Errorf("Encode output missing trailing newline")
	}
	if !strings.Contains(string(data), `"schemaVersion": "1"`) {
		t.Errorf("Encode output missing schemaVersion: %q", string(data[:80]))
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Type != env.Type || got.Seed != env.Seed || got.HandTuned != env.HandTuned {
		t.Errorf("envelope metadata mismatch: got %+v", got)
	}
	if got.Profile == nil || got.Profile.Type != "terran" {
		t.Errorf("profile not decoded correctly")
	}
}

func TestEncodeStable(t *testing.T) {
	prof := planetgen.GetProfile("scorched")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "scorched", Seed: "scorched_default",
		HandTuned: false, Profile: prof,
	}
	a, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("Encode not byte-stable across calls")
	}
}

func TestDecodeRejectsTypeMismatch(t *testing.T) {
	prof := planetgen.GetProfile("scorched") // Profile.Type = scorched
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran", // envelope says terran — mismatch
		Seed:          "x",
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil {
		t.Errorf("Decode accepted envelope/profile type mismatch")
	}
}
```

Write to `pkg/planetgen/profilejson/envelope_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run 'Encode|Decode' -v`
Expected: FAIL — `undefined: Envelope`, `CurrentSchemaVersion`, `Encode`, `Decode`.

- [ ] **Step 3: Write the envelope implementation**

```go
package profilejson

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// CurrentSchemaVersion is the version stamped on Envelopes written by
// Encode. Bumped only when the envelope wire format changes; the inner
// PlanetProfile evolves independently via Go struct-tag additions.
const CurrentSchemaVersion = "1"

// Envelope is the on-disk wrapper around a PlanetProfile. The
// duplicate Type field (envelope.Type vs envelope.Profile.Type) is
// deliberate: the envelope Type is the dispatch key checked by the
// generator before the inner Profile is trusted.
type Envelope struct {
	SchemaVersion string               `json:"schemaVersion"`
	Type          string               `json:"type"`
	Seed          string               `json:"seed"`
	HandTuned     bool                 `json:"handTuned"`
	Profile       *types.PlanetProfile `json:"profile"`
}

// Encode marshals env with canonical formatting: 2-space indent and
// trailing newline. Two calls with equal Envelope values produce
// byte-identical output.
func Encode(env *Envelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("profilejson: nil envelope")
	}
	if env.Profile == nil {
		return nil, fmt.Errorf("profilejson: envelope has nil Profile")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return nil, fmt.Errorf("profilejson: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode parses an envelope from JSON. It first runs Migrate to lift
// older schema versions to the current shape, then validates that
// envelope.Type == envelope.Profile.Type.
func Decode(data []byte) (*Envelope, error) {
	migrated, err := Migrate(data)
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(migrated, &env); err != nil {
		return nil, fmt.Errorf("profilejson: decode: %w", err)
	}
	if env.Profile == nil {
		return nil, fmt.Errorf("profilejson: envelope missing profile")
	}
	if env.Type != env.Profile.Type {
		return nil, fmt.Errorf(
			"profilejson: envelope type %q != profile.type %q",
			env.Type, env.Profile.Type)
	}
	return &env, nil
}

// Migrate lifts older-schema envelopes to the current shape. Today v1
// is current, so the function is a passthrough that validates the
// schemaVersion field is present and recognized. The first real
// migration ships when v2 is introduced.
func Migrate(data []byte) ([]byte, error) {
	var peek struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("profilejson: migrate peek: %w", err)
	}
	if peek.SchemaVersion == "" {
		return nil, fmt.Errorf("profilejson: missing schemaVersion")
	}
	if peek.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf(
			"profilejson: unknown schemaVersion %q (want %q)",
			peek.SchemaVersion, CurrentSchemaVersion)
	}
	return data, nil
}
```

Write to `pkg/planetgen/profilejson/envelope.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -v`
Expected: PASS (3 envelope tests + 7 slug subtests).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/profilejson/envelope.go pkg/planetgen/profilejson/envelope_test.go
git commit -m "feat(profilejson): envelope schema with Encode/Decode/Migrate"
```

---

### Task 3: Migrate edge cases

**Files:**
- Create: `pkg/planetgen/profilejson/migrate_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package profilejson

import "testing"

func TestMigrateMissingVersion(t *testing.T) {
	_, err := Migrate([]byte(`{"type":"terran","seed":"x"}`))
	if err == nil {
		t.Errorf("Migrate accepted envelope with no schemaVersion")
	}
}

func TestMigrateUnknownVersion(t *testing.T) {
	_, err := Migrate([]byte(`{"schemaVersion":"999"}`))
	if err == nil {
		t.Errorf("Migrate accepted unknown schemaVersion")
	}
}

func TestMigrateCurrentVersionPassthrough(t *testing.T) {
	in := []byte(`{"schemaVersion":"` + CurrentSchemaVersion + `","type":"terran"}`)
	out, err := Migrate(in)
	if err != nil {
		t.Fatalf("Migrate(current): %v", err)
	}
	if string(out) != string(in) {
		t.Errorf("Migrate(current) modified payload:\n in: %s\nout: %s", in, out)
	}
}

func TestMigrateBadJSON(t *testing.T) {
	if _, err := Migrate([]byte(`not json`)); err == nil {
		t.Errorf("Migrate accepted invalid JSON")
	}
}
```

Write to `pkg/planetgen/profilejson/migrate_test.go`.

- [ ] **Step 2: Run tests**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestMigrate -v`
Expected: PASS (Migrate was already implemented to handle these in Task 2; this task locks them in with explicit tests).

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/profilejson/migrate_test.go
git commit -m "test(profilejson): lock in Migrate edge cases"
```

---

### Task 4: LoadForPlanet (filesystem layer)

**Files:**
- Create: `pkg/planetgen/profilejson/store.go`
- Create: `pkg/planetgen/profilejson/store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package profilejson

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
)

func TestLoadForPlanetAbsent(t *testing.T) {
	dir := t.TempDir()
	env, ok, err := LoadForPlanet(dir, "Nowhere")
	if err != nil {
		t.Fatalf("LoadForPlanet: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing file, got env=%+v", env)
	}
}

func TestLoadForPlanetPresent(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("terran")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "earth",
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "earth.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadForPlanet(dir, "Earth")
	if err != nil {
		t.Fatalf("LoadForPlanet: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true after writing file")
	}
	if got.Type != "terran" || got.Profile.Type != "terran" {
		t.Errorf("envelope decoded incorrectly: %+v", got)
	}
}

func TestLoadForPlanetCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadForPlanet(dir, "broken"); err == nil {
		t.Errorf("expected decode error on corrupt file")
	}
}

func TestLoadForPlanetEmptyRoot(t *testing.T) {
	env, ok, err := LoadForPlanet("", "anything")
	if err != nil {
		t.Fatalf("LoadForPlanet(\"\"): %v", err)
	}
	if ok || env != nil {
		t.Errorf("expected (nil,false,nil) for empty root")
	}
}
```

Write to `pkg/planetgen/profilejson/store_test.go`.

- [ ] **Step 2: Run tests to verify failure**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestLoadForPlanet -v`
Expected: FAIL — `undefined: LoadForPlanet`.

- [ ] **Step 3: Write the implementation**

```go
package profilejson

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// LoadForPlanet reads the envelope file at rootDir/<PlanetSlug(name)>.json.
// Returns (env, true, nil) on success, (nil, false, nil) when the file
// does not exist, and (nil, false, err) on any other failure (decode
// error, permission denied, etc.). An empty rootDir always returns
// (nil, false, nil) — callers use this to disable disk lookup (e.g. the
// wasm build).
func LoadForPlanet(rootDir, name string) (*Envelope, bool, error) {
	if rootDir == "" {
		return nil, false, nil
	}
	slug := PlanetSlug(name)
	if slug == "" {
		return nil, false, fmt.Errorf("profilejson: empty slug for name %q", name)
	}
	path := filepath.Join(rootDir, slug+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("profilejson: read %s: %w", path, err)
	}
	env, err := Decode(data)
	if err != nil {
		return nil, false, fmt.Errorf("profilejson: decode %s: %w", path, err)
	}
	return env, true, nil
}
```

Write to `pkg/planetgen/profilejson/store.go`.

- [ ] **Step 4: Run tests to verify pass**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/profilejson/store.go pkg/planetgen/profilejson/store_test.go
git commit -m "feat(profilejson): LoadForPlanet filesystem lookup"
```

---

## Chunk 2 — Generator dispatch

### Task 5: Wire resolveProfile into planetgen.Generate

**Files:**
- Modify: `pkg/planetgen/planetgen.go`
- Create: `pkg/planetgen/planetgen_test.go`

- [ ] **Step 1: Write the failing integration tests**

```go
package planetgen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// withProfileRoot temporarily points planetgen at dir and restores the
// previous root on cleanup.
func withProfileRoot(t *testing.T, dir string) {
	t.Helper()
	prev := planetgen.ProfileRootForTest()
	planetgen.SetProfileRoot(dir)
	t.Cleanup(func() { planetgen.SetProfileRoot(prev) })
}

func TestGenerateFallsBackToDefaultsWhenAbsent(t *testing.T) {
	withProfileRoot(t, t.TempDir())
	cm, err := planetgen.Generate("terran", "FallbackPlanet", 32)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cm == nil {
		t.Fatal("Generate returned nil cube map")
	}
}

func TestGenerateUsesProfileFromDisk(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("terran")
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "DiskPlanet",
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diskplanet.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	withProfileRoot(t, dir)
	gotFromDisk, err := planetgen.Generate("terran", "DiskPlanet", 32)
	if err != nil {
		t.Fatalf("Generate (disk): %v", err)
	}

	withProfileRoot(t, t.TempDir())
	gotFromCode, err := planetgen.Generate("terran", "DiskPlanet", 32)
	if err != nil {
		t.Fatalf("Generate (code): %v", err)
	}

	// Both should produce byte-identical output because the on-disk
	// envelope is just a serialization of the in-code default.
	for face := 0; face < 6; face++ {
		if !bytes.Equal(cubeFaceBytes(gotFromDisk, face), cubeFaceBytes(gotFromCode, face)) {
			t.Errorf("face %d differs between disk and code paths", face)
			break
		}
	}
}

func TestGenerateRejectsTypeMismatch(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("scorched")
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "scorched",
		Seed:          "MismatchPlanet",
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mismatchplanet.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	withProfileRoot(t, dir)
	if _, err := planetgen.Generate("terran", "MismatchPlanet", 32); err == nil {
		t.Errorf("expected type-mismatch error, got nil")
	}
}

func cubeFaceBytes(cm *cubemap.CubeMap, face int) []byte {
	img := cm.Faces[face]
	if img == nil {
		return nil
	}
	return img.Pix
}
```

Write to `pkg/planetgen/planetgen_test.go`.

- [ ] **Step 2: Run tests to verify failure**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/ -run TestGenerate -v`
Expected: FAIL — `undefined: SetProfileRoot`, `ProfileRootForTest`.

- [ ] **Step 3: Modify `pkg/planetgen/planetgen.go` to add dispatch**

Replace the entire file contents with:

```go
package planetgen

import (
	"fmt"
	"image"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/render"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/seed"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// DefaultFaceSize is the default cube-map face edge length in pixels.
const DefaultFaceSize = 1024

// DefaultWidth is the default equirect output width in pixels.
const DefaultWidth = 2000

// DefaultHeight is the default equirect output height in pixels.
const DefaultHeight = 1000

// profileRoot is the directory checked for per-planet envelope files.
// Empty string disables disk lookup (used by the wasm build).
var profileRoot = "data/planet-profiles"

// SetProfileRoot overrides the directory consulted by Generate before
// it falls back to GetProfile. Tests pass t.TempDir(); the wasm
// entrypoint passes "".
func SetProfileRoot(dir string) { profileRoot = dir }

// ProfileRootForTest returns the current profileRoot. Intended only for
// tests that need to save and restore the value.
func ProfileRootForTest() string { return profileRoot }

// Generate creates a planet cube map for the given planet type and name.
// The planet name is hashed to produce a deterministic seed. If a
// per-planet envelope file exists under the profile root, its inner
// profile is used instead of the in-code default for that type.
func Generate(planetType, planetName string, faceSize int) (*cubemap.CubeMap, error) {
	profile, err := resolveProfile(planetType, planetName)
	if err != nil {
		return nil, err
	}
	s := hashSeed(planetName)
	switch profile.Renderer {
	case "rocky":
		return render.RenderRocky(profile, s, faceSize), nil
	case "gas_giant":
		return render.RenderGasGiant(profile, s, faceSize), nil
	default:
		return nil, fmt.Errorf("unknown renderer: %s", profile.Renderer)
	}
}

// resolveProfile picks the active PlanetProfile for a (type, name)
// pair: first the on-disk envelope under profileRoot, otherwise the
// in-code GetProfile default. Type-mismatch between envelope and the
// requested type is a hard error.
func resolveProfile(planetType, planetName string) (*types.PlanetProfile, error) {
	if profileRoot != "" {
		env, ok, err := profilejson.LoadForPlanet(profileRoot, planetName)
		if err != nil {
			return nil, err
		}
		if ok {
			if env.Type != planetType {
				return nil, fmt.Errorf(
					"profile type mismatch for %q: file=%q, requested=%q",
					planetName, env.Type, planetType)
			}
			return env.Profile, nil
		}
	}
	p := GetProfile(planetType)
	if p == nil {
		return nil, fmt.Errorf("unknown planet type: %s", planetType)
	}
	return p, nil
}

// GenerateEquirect generates a planet and bakes it to a width×height
// equirectangular RGBA image. Convenience wrapper around Generate +
// cubemap.BakeEquirect.
func GenerateEquirect(planetType, planetName string, width, height int) (*image.RGBA, error) {
	cm, err := Generate(planetType, planetName, DefaultFaceSize)
	if err != nil {
		return nil, err
	}
	return cubemap.BakeEquirect(cm, width, height), nil
}

// hashSeed converts a planet name to a deterministic int64 seed.
// Thin wrapper around seed.Hash retained for in-package callers.
func hashSeed(name string) int64 {
	return seed.Hash(name)
}

// HashSeedPublic is the exported wrapper for external callers
// (e.g. pkg/kbdb). Phase 1+ callers should prefer pkg/planetgen/seed.Hash
// directly.
func HashSeedPublic(name string) int64 {
	return seed.Hash(name)
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/ -run TestGenerate -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 5: Run full planetgen test suite and golden suite to confirm no regression**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/... && go test ./cmd/generate-planet-maps/ -run TestGolden -timeout 120s`
Expected: PASS — `TestGenerate*` passes, existing golden suite still green (no fixtures shipped yet, so `data/planet-profiles/` is empty and the fallback path is identical to pre-change behavior).

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/planetgen.go pkg/planetgen/planetgen_test.go
git commit -m "feat(planetgen): dispatch via profilejson before in-code defaults"
```

---

### Task 6: Disable disk lookup in the wasm build

**Files:**
- Modify: `cmd/planet-explorer/wasm/main.go`

- [ ] **Step 1: Add the SetProfileRoot("") call at startup**

Edit `cmd/planet-explorer/wasm/main.go`. In `func main()`, immediately before the first `js.Global().Set(...)` line, add:

```go
	// Wasm has no filesystem; force planetgen to skip the disk lookup
	// and use the in-code GetProfile defaults exclusively.
	planetgen.SetProfileRoot("")
```

`planetgen` is already imported in this file.

- [ ] **Step 2: Verify the wasm build still compiles**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && GOOS=js GOARCH=wasm go build -o /tmp/discard.wasm ./cmd/planet-explorer/wasm/`
Expected: build clean, no errors. Delete `/tmp/discard.wasm` after.

- [ ] **Step 3: Rebuild the production wasm artifact**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm/`
Expected: clean build, `planet-explorer.wasm` updated.

- [ ] **Step 4: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/planet-explorer/wasm/main.go cmd/planet-explorer/web/planet-explorer.wasm
git commit -m "fix(wasm): SetProfileRoot(\"\") so wasm skips disk lookup"
```

---

## Chunk 3 — Seeder CLI

### Task 7: seed-planet-profiles binary (explicit-list mode)

**Files:**
- Create: `cmd/tools/seed-planet-profiles/main.go`

- [ ] **Step 1: Write the binary**

```go
// Command seed-planet-profiles writes per-planet PlanetProfile
// envelope JSON files to disk for use by pkg/planetgen.
//
// Two modes:
//
//	-planet=<type>:<seed>     (repeatable)  explicit planet list
//	-manifest=<tsv path>      file with one "<type><TAB><name>" per line
//
// Output is byte-stable: rerunning with the same inputs produces
// identical files. By default, refuses to overwrite an existing file;
// pass -force to overwrite (use after a per-archetype default
// changes).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

type planetSpec struct {
	planetType string
	seed       string // also used as the filename stem
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	out := flag.String("out", "data/planet-profiles", "output directory")
	var planets stringSliceFlag
	flag.Var(&planets, "planet", "<type>:<seed> (repeatable)")
	manifest := flag.String("manifest", "", "optional TSV manifest of type<TAB>name lines")
	force := flag.Bool("force", false, "overwrite existing files")
	flag.Parse()

	specs, err := collectSpecs(planets, *manifest)
	if err != nil {
		log.Fatal(err)
	}
	if len(specs) == 0 {
		log.Fatal("no planets specified (use -planet or -manifest)")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *out, err)
	}
	written, skipped := 0, 0
	for _, s := range specs {
		w, err := writeOne(*out, s, *force)
		if err != nil {
			log.Fatalf("write %s: %v", s.seed, err)
		}
		if w {
			written++
		} else {
			skipped++
		}
	}
	fmt.Printf("seed-planet-profiles: wrote %d, skipped %d (out=%s)\n", written, skipped, *out)
}

// collectSpecs merges -planet entries with optional -manifest lines.
// Stable sort by seed for deterministic output ordering.
func collectSpecs(planets []string, manifest string) ([]planetSpec, error) {
	var specs []planetSpec
	for _, p := range planets {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("bad -planet %q: expected <type>:<seed>", p)
		}
		specs = append(specs, planetSpec{planetType: parts[0], seed: parts[1]})
	}
	if manifest != "" {
		f, err := os.Open(manifest)
		if err != nil {
			return nil, fmt.Errorf("open manifest: %w", err)
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("manifest: bad line %q (want type<TAB>name)", line)
			}
			specs = append(specs, planetSpec{planetType: parts[0], seed: parts[1]})
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("manifest read: %w", err)
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].seed < specs[j].seed })
	return specs, nil
}

// writeOne encodes a fresh envelope (handTuned=false, profile from
// GetProfile) and writes it to <out>/<slug>.json. Returns (true, nil)
// if a file was created or overwritten, (false, nil) if the file
// already exists and force is false.
func writeOne(out string, s planetSpec, force bool) (bool, error) {
	prof := planetgen.GetProfile(s.planetType)
	if prof == nil {
		return false, fmt.Errorf("unknown planet type %q", s.planetType)
	}
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          s.planetType,
		Seed:          s.seed,
		HandTuned:     false,
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		return false, err
	}
	slug := profilejson.PlanetSlug(s.seed)
	if slug == "" {
		return false, fmt.Errorf("empty slug for seed %q", s.seed)
	}
	path := filepath.Join(out, slug+".json")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
```

Write to `cmd/tools/seed-planet-profiles/main.go`.

- [ ] **Step 2: Verify the binary compiles**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go build ./cmd/tools/seed-planet-profiles/`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/tools/seed-planet-profiles/main.go
git commit -m "feat(seeder): cmd/tools/seed-planet-profiles binary"
```

---

### Task 8: Seeder integration tests

**Files:**
- Create: `cmd/tools/seed-planet-profiles/main_test.go`

- [ ] **Step 1: Write the tests**

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// TestWriteOneMatchesEncodeDirect verifies that the seeder writes
// byte-identical output to what profilejson.Encode produces against
// GetProfile(type) — i.e., the seeder is a thin wrapper, not a
// reinterpretation of the profile.
func TestWriteOneMatchesEncodeDirect(t *testing.T) {
	dir := t.TempDir()
	spec := planetSpec{planetType: "terran", seed: "terran_default"}
	if _, err := writeOne(dir, spec, false); err != nil {
		t.Fatalf("writeOne: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "terran_default.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := profilejson.Encode(&profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     false,
		Profile:       planetgen.GetProfile("terran"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("seeder output differs from profilejson.Encode of same envelope")
	}
}

// TestWriteOneIdempotent: rerunning produces no changes and no error.
func TestWriteOneIdempotent(t *testing.T) {
	dir := t.TempDir()
	spec := planetSpec{planetType: "scorched", seed: "scorched_default"}
	written1, err := writeOne(dir, spec, false)
	if err != nil || !written1 {
		t.Fatalf("first write: written=%v err=%v", written1, err)
	}
	written2, err := writeOne(dir, spec, false)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if written2 {
		t.Errorf("second write should have skipped existing file")
	}
}

// TestWriteOneForceOverwrites: -force replaces existing content.
func TestWriteOneForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "terran_default.json")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := planetSpec{planetType: "terran", seed: "terran_default"}
	if _, err := writeOne(dir, spec, true); err != nil {
		t.Fatalf("writeOne force: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) == "stale" {
		t.Errorf("force did not overwrite")
	}
}

func TestCollectSpecsRejectsBadPlanet(t *testing.T) {
	if _, err := collectSpecs([]string{"badform"}, ""); err == nil {
		t.Errorf("expected error on malformed -planet entry")
	}
}

func TestCollectSpecsManifest(t *testing.T) {
	dir := t.TempDir()
	mpath := filepath.Join(dir, "list.tsv")
	if err := os.WriteFile(mpath, []byte("# comment\nterran\tEarth\nscorched\tMercury\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	specs, err := collectSpecs(nil, mpath)
	if err != nil {
		t.Fatalf("collectSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	// Sorted by seed alphabetically: Earth, Mercury.
	if specs[0].seed != "Earth" || specs[1].seed != "Mercury" {
		t.Errorf("specs not sorted: %+v", specs)
	}
}
```

Write to `cmd/tools/seed-planet-profiles/main_test.go`.

- [ ] **Step 2: Run tests**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./cmd/tools/seed-planet-profiles/ -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/tools/seed-planet-profiles/main_test.go
git commit -m "test(seeder): byte-stability, idempotency, manifest parsing"
```

---

### Task 9: Bake the three initial fixtures

**Files:**
- Create: `data/planet-profiles/terran_default.json`
- Create: `data/planet-profiles/super_terran_default.json`
- Create: `data/planet-profiles/scorched_default.json`

- [ ] **Step 1: Run the seeder against the real output directory**

Run:
```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
mkdir -p data/planet-profiles
go run ./cmd/tools/seed-planet-profiles/ \
    -out=data/planet-profiles \
    -planet=terran:terran_default \
    -planet=super_terran:super_terran_default \
    -planet=scorched:scorched_default
ls data/planet-profiles/
```
Expected: prints `wrote 3, skipped 0 (out=data/planet-profiles)` and the three files are listed.

- [ ] **Step 2: Verify idempotency by re-running**

Run:
```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
go run ./cmd/tools/seed-planet-profiles/ \
    -out=data/planet-profiles \
    -planet=terran:terran_default \
    -planet=super_terran:super_terran_default \
    -planet=scorched:scorched_default
```
Expected: prints `wrote 0, skipped 3` (existing files left alone).

- [ ] **Step 3: Spot-check one file's header**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && head -8 data/planet-profiles/terran_default.json`
Expected: leading `{` then `"schemaVersion": "1"`, `"type": "terran"`, `"seed": "terran_default"`, `"handTuned": false`, then `"profile": {`.

- [ ] **Step 4: Confirm end-to-end dispatch works**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/ -run TestGenerate -v`
Expected: PASS — `TestGenerateUsesProfileFromDisk` still uses `t.TempDir()`, unaffected by the real fixtures.

- [ ] **Step 5: Confirm golden suite still passes (fixtures must not perturb existing seeds)**

The golden test names (`GoldenTerran`, `GoldenScorched`, `GoldenSuperTerran`) slug to `goldenterran`, `goldenscorched`, `goldensuper_terran` — none of which match the new fixture filenames (`terran_default`, etc.). The golden path will continue to take the fallback branch.

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./cmd/generate-planet-maps/ -run 'TestGolden$' -timeout 120s`
Expected: PASS — no rendering changed.

- [ ] **Step 6: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add data/planet-profiles/
git commit -m "feat(profilejson): seed terran/super_terran/scorched default fixtures"
```

---

### Task 10: CI drift guard test

**Files:**
- Create: `pkg/planetgen/profilejson/drift_test.go`

- [ ] **Step 1: Write the drift test**

```go
package profilejson_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// TestProfilesDoNotDrift walks every committed envelope under
// data/planet-profiles/ and, for each non-hand-tuned file, verifies it
// matches a freshly-encoded envelope built from GetProfile(env.Type)
// with the same seed and HandTuned=false. Hand-tuned files are
// skipped — the maintainer has opted out of drift checking by
// setting handTuned: true.
//
// Failure means an in-code default changed in a way that the on-disk
// envelope no longer reflects. Fix by either:
//   - rerunning `seed-planet-profiles -force` for the affected planets, or
//   - marking the envelope `"handTuned": true` if it is now intentionally divergent.
//
// Runs in <100ms (pure encode + memcmp, no PNG bake).
func TestProfilesDoNotDrift(t *testing.T) {
	// Discover the repo root by walking up until we find go.mod.
	root := repoRoot(t)
	dir := filepath.Join(root, "data", "planet-profiles")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("no %s directory yet", dir)
		}
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		t.Run(e.Name(), func(t *testing.T) {
			committed, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			env, err := profilejson.Decode(committed)
			if err != nil {
				t.Fatalf("decode %s: %v", e.Name(), err)
			}
			if env.HandTuned {
				t.Skipf("hand-tuned, drift check skipped")
			}
			fresh := &profilejson.Envelope{
				SchemaVersion: profilejson.CurrentSchemaVersion,
				Type:          env.Type,
				Seed:          env.Seed,
				HandTuned:     false,
				Profile:       planetgen.GetProfile(env.Type),
			}
			want, err := profilejson.Encode(fresh)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(committed, want) {
				t.Errorf("envelope %s drifted from GetProfile(%q). "+
					"Either rerun `go run ./cmd/tools/seed-planet-profiles -force -planet=%s:%s` "+
					"or mark this file `\"handTuned\": true`.",
					e.Name(), env.Type, env.Type, env.Seed)
			}
		})
	}
}

// repoRoot walks up from the test working directory until it finds
// go.mod. Tests run from the package directory, so we need to escape
// pkg/planetgen/profilejson/ to find data/planet-profiles/.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		if d == filepath.Dir(d) {
			t.Fatalf("go.mod not found from %s", wd)
		}
	}
}
```

Write to `pkg/planetgen/profilejson/drift_test.go`.

- [ ] **Step 2: Run the drift test**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestProfilesDoNotDrift -v`
Expected: PASS — three subtests (one per fixture), all green.

- [ ] **Step 3: Verify it catches drift (mutation test)**

Manually corrupt a fixture and confirm the test fails:

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
cp data/planet-profiles/terran_default.json /tmp/terran_default.json.bak
sed -i 's/"seed": "terran_default"/"seed": "terran_drifted"/' data/planet-profiles/terran_default.json
```

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestProfilesDoNotDrift`
Expected: FAIL.

Restore:
```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
cp /tmp/terran_default.json.bak data/planet-profiles/terran_default.json
rm /tmp/terran_default.json.bak
```

Run again to confirm green: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestProfilesDoNotDrift`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add pkg/planetgen/profilejson/drift_test.go
git commit -m "test(profilejson): CI drift guard for committed envelopes"
```

---

## Chunk 4 — Dev server endpoints

### Task 11: Add /profiles GET, GET-one, PUT endpoints

**Files:**
- Modify: `cmd/planet-explorer/main.go`

- [ ] **Step 1: Replace `cmd/planet-explorer/main.go` with the expanded version**

Replace the entire file contents with:

```go
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
```

- [ ] **Step 2: Verify build**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go build ./cmd/planet-explorer/`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/planet-explorer/main.go
git commit -m "feat(planet-explorer): GET/PUT /profiles endpoints"
```

---

### Task 12: Dev server tests

**Files:**
- Create: `cmd/planet-explorer/main_test.go`

- [ ] **Step 1: Write the tests**

```go
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
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          planetType,
		Seed:          seed,
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
	writeFixture(t, dir, "scorched_default", "scorched", "scorched_default")

	res, err := http.Get(ts.URL + "/profiles")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	var got []string
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	want := []string{"scorched_default", "terran_default"}
	if !equalStrings(got, want) {
		t.Errorf("list = %v, want %v", got, want)
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
		Seed:          "terran_default",
		HandTuned:     true,
		Profile:       planetgen.GetProfile("terran"),
	}
	body, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/terran_default", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		errBody, _ := io.ReadAll(res.Body)
		t.Fatalf("PUT status = %d, body = %s", res.StatusCode, errBody)
	}
	written, err := os.ReadFile(filepath.Join(dir, "terran_default.json"))
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
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/profiles/terran_default", bytes.NewReader(body))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Errorf("expected 409, got %d", res.StatusCode)
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Write to `cmd/planet-explorer/main_test.go`.

- [ ] **Step 2: Run tests**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./cmd/planet-explorer/ -v`
Expected: PASS (8 tests).

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/planet-explorer/main_test.go
git commit -m "test(planet-explorer): /profiles endpoint round-trip and error cases"
```

---

## Chunk 5 — Web UI

### Task 13: Add planet picker, Save, Save-as-new to the HTML

**Files:**
- Modify: `cmd/planet-explorer/web/index.html`

- [ ] **Step 1: Insert the planet picker and save buttons**

Edit `cmd/planet-explorer/web/index.html`. Find this block (around line 33–34):

```html
        </select>
      </label>
      <label>Seed <input id="seed-input" type="text" value="Earth"></label>
```

Replace with:

```html
        </select>
      </label>
      <label>Planet
        <select id="planet-picker">
          <option value="" selected>(none — use type defaults)</option>
        </select>
      </label>
      <label>Seed <input id="seed-input" type="text" value="Earth"></label>
```

Then find the `<button id="export-json-btn">Export JSON</button>` line (around line 50) and append immediately after it:

```html
      <button id="save-profile-btn" disabled title="Save the current slider state to the selected planet's JSON file.">Save</button>
      <button id="save-profile-as-btn" title="Prompt for a new slug and PUT the current slider state.">Save as new…</button>
```

- [ ] **Step 2: Confirm the HTML still parses (no test framework — eyeball it)**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && grep -n 'planet-picker\|save-profile-btn\|save-profile-as-btn' cmd/planet-explorer/web/index.html`
Expected: 3 matches.

- [ ] **Step 3: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/planet-explorer/web/index.html
git commit -m "feat(planet-explorer): planet picker + Save buttons in markup"
```

---

### Task 14: Wire the picker, Save, and Save-as-new in JS

**Files:**
- Modify: `cmd/planet-explorer/web/app.js`

- [ ] **Step 1: Add DOM handles for the new controls**

Edit `cmd/planet-explorer/web/app.js`. Find lines 7–17:

```javascript
const status = $('#status');
const typePicker = $('#type-picker');
const seedInput = $('#seed-input');
const faceSizeSel = $('#face-size');
const renderBtn = $('#render-btn');
const exportBtn = $('#export-json-btn');
const applyBtn = $('#apply-json-btn');
const toggleJitterBtn = $('#toggle-jitter-btn');
const profileTextarea = $('#profile-json');
const cubeCanvas = $('#cube-canvas');
const equirectCanvas = $('#equirect-canvas');
const viewModeSel = $('#view-mode');
```

Add after the `const viewModeSel` line:

```javascript
const planetPicker = $('#planet-picker');
const saveProfileBtn = $('#save-profile-btn');
const saveAsNewBtn = $('#save-profile-as-btn');
let currentSlug = ''; // empty = picker on "(none)"; otherwise selected slug
```

- [ ] **Step 2: Populate the picker on init**

Find `function init()` (around line 30). Replace the entire function with:

```javascript
async function init() {
  status.textContent = 'Loading wasm…';
  const go = new Go();
  const result = await WebAssembly.instantiateStreaming(fetch('wasm'), go.importObject);
  go.run(result.instance);
  wasmReady = true;
  status.textContent = 'Ready';

  await refreshPlanetPicker();
  loadDefaultProfile();
}

// refreshPlanetPicker fetches /profiles, replaces the dropdown
// options, and preserves the current selection if still present.
async function refreshPlanetPicker() {
  try {
    const res = await fetch('/profiles');
    if (!res.ok) throw new Error('list status ' + res.status);
    const slugs = await res.json();
    const previous = planetPicker.value;
    planetPicker.innerHTML = '<option value="">(none — use type defaults)</option>';
    for (const slug of slugs) {
      const opt = document.createElement('option');
      opt.value = slug;
      opt.textContent = slug;
      planetPicker.appendChild(opt);
    }
    if (slugs.includes(previous)) {
      planetPicker.value = previous;
    }
  } catch (e) {
    console.warn('refreshPlanetPicker:', e);
  }
}
```

- [ ] **Step 3: Wire the picker change handler and update Save button enabled state**

Find the existing line:

```javascript
typePicker.addEventListener('change', loadDefaultProfile);
```

Replace with:

```javascript
typePicker.addEventListener('change', () => {
  // Switching the type clears the Planet selection — the type's
  // defaults are now in effect.
  planetPicker.value = '';
  currentSlug = '';
  saveProfileBtn.disabled = true;
  loadDefaultProfile();
});

planetPicker.addEventListener('change', async () => {
  const slug = planetPicker.value;
  if (!slug) {
    currentSlug = '';
    saveProfileBtn.disabled = true;
    loadDefaultProfile();
    return;
  }
  try {
    const res = await fetch('/profiles/' + encodeURIComponent(slug));
    if (!res.ok) throw new Error('GET status ' + res.status);
    const env = await res.json();
    if (!env || !env.profile) throw new Error('malformed envelope');
    // Sync the type-picker to the envelope's type so the rest of the
    // UI (palette previews, etc.) reflects the right archetype.
    typePicker.value = env.type;
    profileTextarea.value = prettifyJSON(JSON.stringify(env.profile));
    try { snapshotOriginal(JSON.parse(profileTextarea.value)); } catch {}
    renderPanels();
    refreshJitterButtonLabel();
    currentSlug = slug;
    saveProfileBtn.disabled = false;
  } catch (e) {
    status.textContent = 'Load failed: ' + e.message;
  }
});

// saveProfile PUTs the current slider state back to the server,
// wrapping it in an envelope and marking handTuned: true (the
// slider edit is by definition a hand-tune).
async function saveProfile(targetSlug) {
  syncKnotsFromDOM();
  let profile;
  try { profile = JSON.parse(profileTextarea.value); }
  catch (e) {
    status.textContent = 'Save failed: invalid profile JSON';
    return false;
  }
  const env = {
    schemaVersion: '1',
    type: typePicker.value,
    seed: targetSlug,
    handTuned: true,
    profile: profile,
  };
  const body = JSON.stringify(env);
  const res = await fetch('/profiles/' + encodeURIComponent(targetSlug), {
    method: 'PUT',
    headers: {'Content-Type': 'application/json'},
    body: body,
  });
  if (!res.ok) {
    const text = await res.text();
    status.textContent = 'Save failed: ' + text;
    return false;
  }
  return true;
}

saveProfileBtn.addEventListener('click', async () => {
  if (!currentSlug) return;
  const ok = await saveProfile(currentSlug);
  if (ok) {
    status.textContent = 'Saved ' + currentSlug;
    setTimeout(() => { if (status.textContent.startsWith('Saved ')) status.textContent = 'Ready'; }, 1500);
  }
});

saveAsNewBtn.addEventListener('click', async () => {
  const input = window.prompt('New slug ([a-z0-9_]+):', '');
  if (input == null) return;
  const slug = input.trim();
  if (!/^[a-z0-9_]+$/.test(slug)) {
    status.textContent = 'Save failed: slug must match [a-z0-9_]+';
    return;
  }
  // If the slug exists in the current picker, confirm overwrite.
  const existing = Array.from(planetPicker.options).map((o) => o.value);
  if (existing.includes(slug)) {
    if (!window.confirm('Overwrite existing profile "' + slug + '"?')) return;
  }
  const ok = await saveProfile(slug);
  if (ok) {
    await refreshPlanetPicker();
    planetPicker.value = slug;
    currentSlug = slug;
    saveProfileBtn.disabled = false;
    status.textContent = 'Saved ' + slug;
    setTimeout(() => { if (status.textContent.startsWith('Saved ')) status.textContent = 'Ready'; }, 1500);
  }
});
```

- [ ] **Step 4: Manual smoke test — start the dev server**

Run in a second terminal:

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm/
go run ./cmd/planet-explorer/
```

In a browser, open `http://localhost:8080/`. Verify:
1. The Planet dropdown lists `scorched_default`, `super_terran_default`, `terran_default`.
2. Select `terran_default`; the profile textarea reloads and Save becomes enabled.
3. Edit a slider (e.g. drop `Civ.Tier`), click Save; status shows "Saved terran_default".
4. `git diff data/planet-profiles/terran_default.json` shows the change and `handTuned: true`.
5. Reload the page, re-select `terran_default`; the slider state matches the saved value.
6. Click "Save as new…", enter `terran_test`, accept the prompt; verify `data/planet-profiles/terran_test.json` exists.
7. Reset by running:

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git checkout -- data/planet-profiles/terran_default.json
rm -f data/planet-profiles/terran_test.json
```

Stop the dev server (Ctrl+C).

- [ ] **Step 5: Commit**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/planet-explorer/web/app.js
git commit -m "feat(planet-explorer): planet picker + Save + Save-as-new wiring"
```

---

## Chunk 6 — Documentation and acceptance

### Task 15: Documentation

**Files:**
- Create: `cmd/tools/seed-planet-profiles/README.md`
- Modify: `cmd/planet-explorer/README.md`
- Modify: `/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md`

- [ ] **Step 1: Write the seeder README**

```markdown
# seed-planet-profiles

Writes per-planet `PlanetProfile` envelope JSON files to disk for use by `pkg/planetgen`. Output is byte-stable: rerunning produces identical bytes.

## Usage

### Explicit list

```
go run ./cmd/tools/seed-planet-profiles \
    -out=data/planet-profiles \
    -planet=terran:terran_default \
    -planet=super_terran:super_terran_default
```

Each `-planet=<type>:<seed>` writes one envelope to `<out>/<slug>.json`, where `<slug>` is the result of `profilejson.PlanetSlug(seed)`.

### Manifest mode

Pass a TSV manifest with one `<type><TAB><name>` per line. Comment lines start with `#`.

```
go run ./cmd/tools/seed-planet-profiles \
    -out=data/planet-profiles \
    -manifest=data/planet-list.tsv
```

### Flags

- `-out` — output directory (default `data/planet-profiles`)
- `-planet` — `<type>:<seed>` (repeatable)
- `-manifest` — path to a TSV manifest
- `-force` — overwrite existing files (default refuses to clobber)

## Idempotency

Without `-force`, the seeder skips any file that already exists. With `-force`, it overwrites, but two `-force` runs with the same inputs produce byte-identical files because `profilejson.Encode` is canonical.

## When to rerun

- After adding a new planet archetype to `pkg/planetgen.Profiles`.
- After tweaking a per-archetype default that an existing envelope mirrors — use `-force`, then commit the diff (the CI drift guard will flag uncommitted drift).

Do **not** rerun against a file whose `handTuned: true` you want to preserve; the drift guard skips hand-tuned files, but `-force` would overwrite the tune.
```

Write to `cmd/tools/seed-planet-profiles/README.md`.

- [ ] **Step 2: Append a "Planet picker / Save" section to the planet-explorer README**

Read `cmd/planet-explorer/README.md` first to find a good insertion point, then append (before the final closing line) a new section:

```markdown
## Planet picker and per-planet save (Phase 5)

The header bar exposes a **Planet** dropdown alongside the existing **Type** dropdown. It lists every JSON file under `data/planet-profiles/` (configurable via `-profiles-dir`).

- Selecting a planet loads its envelope from the server, swaps the slider state to the envelope's `profile`, and enables **Save**.
- **Save** PUTs the current slider state back to the selected slug as `handTuned: true`. The file is overwritten on disk; check `git diff` to review.
- **Save as new…** prompts for a slug (`[a-z0-9_]+`); if the slug already exists, you'll be asked to confirm overwrite.
- Changing the **Type** dropdown clears the Planet selection — you're back to the in-code defaults until you reselect a planet.

`data/planet-profiles/` is normal git-tracked content. Commit hand-tunes alongside any code changes that motivated them.

The `-readonly` flag turns the server into a viewer: PUTs return 405. Useful for demos or shared dev servers.

To bake or refresh the canonical (non-hand-tuned) envelopes, use `cmd/tools/seed-planet-profiles`.
```

- [ ] **Step 3: Append a Phase 5 status section to project memory**

Edit `/home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md`. Append at the end of the file:

```markdown

## Phase 5 (per-planet profile JSON) — 2026-05-11

Status: complete on branch `phase-0/cube-map`. Spec at `docs/superpowers/specs/2026-05-07-phase-5-per-planet-profile-json-design.md`, plan at `docs/superpowers/plans/2026-05-11-phase-5-per-planet-profile-json-plan.md`.

Shipped:
- `pkg/planetgen/profilejson/` library — Envelope schema v1, Encode/Decode/Migrate, LoadForPlanet, PlanetSlug, drift guard test.
- `planetgen.Generate` dispatch hook (`SetProfileRoot`, `resolveProfile`) — consults disk before falling back to `GetProfile`.
- `cmd/tools/seed-planet-profiles` — explicit-list + manifest mode, idempotent.
- Three fixture envelopes: `terran_default.json`, `super_terran_default.json`, `scorched_default.json`.
- `cmd/planet-explorer` REST endpoints: `GET /profiles`, `GET /profiles/<slug>`, `PUT /profiles/<slug>`; `-profiles-dir`, `-readonly` flags.
- Web UI: planet picker dropdown, Save and Save-as-new buttons; envelope packing in JS.
- Wasm build calls `SetProfileRoot("")` so the wasm binary never touches disk.

Out of scope (deferred): bulk seed of 411 kb planets, schema v2 migrations, etag/locking for concurrent edits.
```

- [ ] **Step 4: Verify writes succeeded**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && ls cmd/tools/seed-planet-profiles/README.md && grep -l 'Planet picker' cmd/planet-explorer/README.md && grep -l 'Phase 5 (per-planet profile JSON)' /home/robert/.claude/projects/-home-robert-spacemolt-kb/memory/project_planet_gen.md`
Expected: all three paths print.

- [ ] **Step 5: Commit (memory file is outside the repo so commit only the two README changes)**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git add cmd/tools/seed-planet-profiles/README.md cmd/planet-explorer/README.md
git commit -m "docs(phase-5): seeder README and planet-explorer picker docs"
```

---

### Task 16: Final acceptance gates

**Files:** none — verification only.

- [ ] **Step 1: Full build**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go build ./...`
Expected: clean.

- [ ] **Step 2: Full test suite**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./... -timeout 300s`
Expected: PASS — including new profilejson, planetgen, seeder, and planet-explorer tests.

- [ ] **Step 3: Lint**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && golangci-lint run ./...`
Expected: clean (no new findings).

- [ ] **Step 4: Wasm build**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/`
Expected: clean.

- [ ] **Step 5: Golden zero-diff**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./cmd/generate-planet-maps/ -run TestGolden -timeout 600s`
Expected: PASS — no rendering changed; all goldens still match because the seeded fixtures use planet names that don't collide with golden seeds.

- [ ] **Step 6: Drift guard**

Run: `cd /home/robert/spacemolt/kb-phase-0-cube-map && go test ./pkg/planetgen/profilejson/ -run TestProfilesDoNotDrift -v`
Expected: PASS — three subtests, no skips beyond `t.Skip` for hand-tuned (none in this PR are hand-tuned).

- [ ] **Step 7: Manual dev-server smoke (final pass)**

Same routine as Task 14 step 4: start server, pick `terran_default`, edit a slider, Save, verify diff, restore. End of phase.

- [ ] **Step 8: Push the branch**

```bash
cd /home/robert/spacemolt/kb-phase-0-cube-map
git push
```

---

## Notes for the implementing engineer

- The repo is a worktree at `/home/robert/spacemolt/kb-phase-0-cube-map`, branch `phase-0/cube-map`. All `cd` commands above assume that root.
- `pkg/planetgen.GetProfile` returns the `"unknown"` profile when asked for an unrecognized type — this is fine in production but means `resolveProfile` should keep the explicit `if p == nil` guard for robustness even though `GetProfile` itself can't return nil today.
- `profilejson.Encode` uses `encoding/json`'s default field ordering, which is struct-field-declaration order, not alphabetical. This is byte-stable because Go's `encoding/json` is deterministic for a given struct. Do not switch to `MarshalIndent` with `json.RawMessage` reordering — it would break the drift guard.
- The web build uses no framework. Keep new JS small and idiomatic; reuse `$('#id')` for DOM lookups.
- `cmd/planet-explorer/web/planet-explorer.wasm` is a built artifact committed to the repo. Rebuild it whenever the wasm sources change (`cd /home/robert/spacemolt/kb-phase-0-cube-map && GOOS=js GOARCH=wasm go build -o cmd/planet-explorer/web/planet-explorer.wasm ./cmd/planet-explorer/wasm/`).
