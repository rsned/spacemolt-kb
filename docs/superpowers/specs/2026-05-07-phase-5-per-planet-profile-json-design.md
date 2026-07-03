# Phase 5 — Per-Planet Profile JSON

**Status:** Design complete, awaiting review.
**Master plan reference:** `docs/plans/2026-04-25-planet-gen-master-plan.md` §9.

## Goal

Move per-planet rendering parameters from in-code defaults to disk-resident JSON files so any planet can be hand-tuned without editing Go code, while preserving determinism and golden-test stability for planets without a JSON file.

## Scope decisions

- **Infrastructure-complete + small starter set.** This phase ships the seed tool, generator dispatch, slider integration, CI drift guard, and three fixture envelopes. Mass-seeding the 411 kb planets is a separate future PR.
- **Envelope wrapper file format** (not inline fields on `PlanetProfile`, not sidecar `.meta.json`). Keeps the wasm contract unchanged.
- **`HandTuned` flag, slider sets it on Save.** Simple boolean; CI guard skips files where it's true.
- **No "diff against type default" UI.** `git diff` is the canonical diff view.
- **Library-with-thin-CLI packaging.** Single source of truth in `pkg/planetgen/profilejson/`; cmd binaries and the dev server import it.

## Architecture

### Packages and binaries

```
pkg/planetgen/profilejson/         (NEW)
  envelope.go                      Envelope struct, Encode/Decode
  migrate.go                       Migrate(jsonBytes) → ([]byte, error)
  slug.go                          PlanetSlug(name) → "82_eridani_82_eridani_ii"
  store.go                         LoadForPlanet(rootDir, name) (*Envelope, bool, error)
  envelope_test.go, slug_test.go, migrate_test.go, store_test.go, drift_test.go

pkg/planetgen/planetgen.go         (MODIFIED)
  Generate calls profilejson.LoadForPlanet first, falls back to GetProfile

cmd/tools/seed-planet-profiles/    (NEW)
  main.go                          ~80 LOC; walks a planet list, calls profilejson.Encode

cmd/planet-explorer/main.go        (MODIFIED)
  Adds GET /profiles, GET /profiles/<slug>, PUT /profiles/<slug>; -profiles-dir, -readonly flags

cmd/planet-explorer/wasm/main.go   (UNCHANGED — wasm contract preserved)

cmd/planet-explorer/web/app.js     (MODIFIED)
  Planet picker dropdown, Open/Save/Save-as-new wiring; envelope packing in JS

data/planet-profiles/              (NEW data dir)
  terran_default.json
  super_terran_default.json
  scorched_default.json
```

### Data flow at generation time

1. `Generate(planetType, planetName, faceSize)` calls `profilejson.LoadForPlanet(profileRoot, planetName)`.
2. If a file exists: decode, migrate, validate `envelope.Type == planetType`, use `envelope.Profile` as the active profile.
3. If absent: fall back to `GetProfile(planetType)` (current behavior).
4. Seed derivation is unchanged: `seed := hashSeed(planetName)`. The envelope's `Seed` field is metadata for the CI guard, not a rendering input.

### Data flow in the slider

1. Page load: `GET /profiles` → list of slugs → populate planet picker dropdown.
2. User picks a planet: `GET /profiles/<slug>` → JS unwraps envelope → pushes inner profile to wasm via the existing `planetExplorerGenerate(profileJSON, ...)` entry point.
3. User clicks Save: JS wraps current slider state in an envelope (`handTuned: true`), `PUT /profiles/<slug>` → server writes file via `profilejson.Encode`.

The wasm contract is unchanged. Envelope packing/unpacking lives in the dev server and JS.

## Envelope schema

```go
const CurrentSchemaVersion = "1"

type Envelope struct {
    SchemaVersion string                 `json:"schemaVersion"`
    Type          string                 `json:"type"`
    Seed          string                 `json:"seed"`
    HandTuned     bool                   `json:"handTuned"`
    Profile       *types.PlanetProfile   `json:"profile"`
}

func Encode(env *Envelope) ([]byte, error)
func Decode(data []byte) (*Envelope, error)  // calls Migrate first
func Migrate(data []byte) ([]byte, error)    // v1 no-op; switch on peeked schemaVersion
```

`Encode` produces canonically-formatted output: 2-space indent, sorted keys via `encoding/json` field order, trailing newline. Two `Encode` calls with equal `Envelope` values are byte-identical.

`Decode` validates that `envelope.Type == envelope.Profile.Type` and errors if they disagree. The duplication is deliberate: the envelope's `Type` is the dispatch key (used by the CI guard), while `Profile.Type` is the rendering knob already in `PlanetProfile`.

`Seed` is the canonical string passed to `Generate(planetType, planetName, ...)`. For seeded fixtures it equals the filename stem.

### Wire format example

```json
{
  "schemaVersion": "1",
  "type": "terran",
  "seed": "terran_default",
  "handTuned": false,
  "profile": {
    "type": "terran",
    "controlFields": { ... },
    "civ": { "tier": 0.5, ... }
  }
}
```

## Generator dispatch

```go
// pkg/planetgen/planetgen.go

var profileRoot = "data/planet-profiles"

func SetProfileRoot(dir string) { profileRoot = dir }

func Generate(planetType, planetName string, faceSize int) (*cubemap.CubeMap, error) {
    profile, err := resolveProfile(planetType, planetName)
    if err != nil {
        return nil, err
    }
    seed := hashSeed(planetName)
    return renderRocky(profile, seed, faceSize), nil
}

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
```

`profileRoot` defaults to `"data/planet-profiles"` and is overridable via `SetProfileRoot`. Tests use `t.TempDir()`. The wasm build sets it to `""` (no disk lookup; always defaults).

Type-mismatch is a hard error, not a silent override.

`Generate`'s exported signature is unchanged. Existing callers — including the kb's batch-generate path for 411 planets — pick up the new behavior automatically once JSON files appear.

## Seeder CLI

`cmd/tools/seed-planet-profiles/main.go`, ~80 LOC. Two modes:

```bash
# Mode 1: explicit list (initial fixtures + CI guard)
seed-planet-profiles -out=data/planet-profiles \
    -planet=terran:terran_default \
    -planet=super_terran:super_terran_default \
    -planet=scorched:scorched_default

# Mode 2: manifest-driven (deferred; for future bulk seed of kb planets)
seed-planet-profiles -out=data/planet-profiles -manifest=data/planet-list.tsv
```

Each `-planet=type:seed` entry becomes one envelope: `Type=type`, `Seed=seed`, `HandTuned=false`, `Profile=GetProfile(type)`, written to `<out>/<seed>.json`.

The manifest path expects a TSV with `type<TAB>name` per line. Implementation lands now (so the path is exercised by tests) but no manifest file ships in this PR.

**Idempotent.** Output is sorted by slug, pretty-printed, stable. Re-running produces byte-identical files.

**Overwrite policy.** Default refuses to overwrite existing files (so a hand-tuned file isn't clobbered). `-force` overwrites; used for manual re-seeding after a per-archetype default changes. The CI drift guard does not invoke the seeder — it does an in-process `Encode` comparison and never writes to disk.

## Slider integration

### Server (`cmd/planet-explorer/main.go`)

| Endpoint | Behavior |
|---|---|
| `GET /profiles` | Returns JSON array of slug strings from `data/planet-profiles/`. |
| `GET /profiles/<slug>` | Returns the raw envelope file. 404 if absent. |
| `PUT /profiles/<slug>` | Decodes body as Envelope; validates `schemaVersion` and `type == profile.type`; writes via `profilejson.Encode` (canonical formatting). 400 on invalid JSON, 409 if URL slug doesn't match envelope content. |

New flags: `-profiles-dir` (default `data/planet-profiles`), `-readonly` (rejects PUTs with 405).

### Client (`cmd/planet-explorer/web/app.js` + `index.html`)

- New "Planet" `<select>` next to the existing "Type" dropdown. Populated from `GET /profiles` on page load. First option is `(none — use type defaults)`.
- Selecting a planet: `GET /profiles/<slug>` → unwrap envelope → push `envelope.profile` into existing slider state via the same code path `planetExplorerDefaultProfile` already feeds.
- Changing the Type dropdown clears the Planet selection.
- "Save" button: wraps current slider state in an envelope (`handTuned: true`), `PUT /profiles/<currentSlug>`. Disabled when no Planet is selected. Briefly displays "Saved" on success.
- "Save as new…" button: always enabled. Prompts for a slug (regex `[a-z0-9_]+`, validated client-side); if the slug already exists in the picker, prompts to confirm overwrite. `PUT /profiles/<newSlug>`. On success, refreshes the picker and selects the new entry.

Concurrency is last-write-wins; no etags. Single-user dev tool.

## CI drift guard

`pkg/planetgen/profilejson/drift_test.go` (sketch in § 6 of brainstorm). For every committed envelope file:

1. Read and decode the file.
2. If `HandTuned == true`, skip.
3. Build a fresh envelope from `GetProfile(env.Type)` with the same `Seed` and `HandTuned: false`.
4. `Encode` the fresh envelope and `bytes.Equal` against the committed file content.
5. On mismatch: error with a message instructing the user to either set `handTuned: true` or rerun `seed-planet-profiles`.

Runs in <100ms. No PNG bake.

## Initial fixtures

Three files committed at PR time, all `handTuned: false`:

| File | Type | Seed | Why |
|---|---|---|---|
| `terran_default.json` | terran | `terran_default` | Civ.Tier=0.5; densest profile; exercises the only non-zero civ pipeline |
| `super_terran_default.json` | super_terran | `super_terran_default` | Civ.Tier=0.3; second civ-bearing archetype |
| `scorched_default.json` | scorched | `scorched_default` | No-civ, no-cloud, no-water archetype; sparsest non-trivial profile |

Filenames double as the slug. Seeds match the filename stem so that running `cmd/generate-planet-maps -type=terran -name=terran_default` triggers dispatch end-to-end without depending on a real kb planet name.

## Testing strategy

Five test surfaces:

1. **`pkg/planetgen/profilejson/`** — unit tests for `Encode`/`Decode` round-trip, `PlanetSlug` table-driven cases, `Migrate` v1 no-op + unknown-version error + missing-version error, `LoadForPlanet` absent + present + decode error. Fast.
2. **`pkg/planetgen/planetgen_test.go`** — integration:
   - `TestGenerateUsesProfileFromDisk`: write envelope to `t.TempDir()`, `SetProfileRoot(tmp)`, generate, byte-equal to direct profile path.
   - `TestGenerateFallsBackToDefaultsWhenAbsent`: empty temp dir, identical to current `GetProfile("terran")` path.
   - `TestGenerateRejectsTypeMismatch`: file says `scorched`, request `terran`, expect error.
3. **`pkg/planetgen/profilejson/drift_test.go`** — the CI guard.
4. **`cmd/tools/seed-planet-profiles/main_test.go`** — run binary against `t.TempDir()`, verify expected files, verify byte-equal to `profilejson.Encode(GetProfile(type))`. Idempotency: re-run, zero changes.
5. **`cmd/planet-explorer/main_test.go`** — `httptest.Server` round-trip: `GET /profiles`, `GET`, `PUT`, type-mismatch returns 409, bad JSON returns 400, `-readonly` rejects with 405.

## Acceptance gates

- `go build ./...`, `go test ./...`, `golangci-lint run ./...` — clean.
- `GOOS=js GOARCH=wasm go build ./pkg/planetgen/... ./cmd/planet-explorer/wasm/` — clean.
- `TestGolden` re-run — zero diff (no rendering changed).
- Drift guard passes.
- Manual: dev server, pick `terran_default` from picker, tweak a slider, Save, observe file changed on disk, reload, observe slider state matches.

## Documentation

- `cmd/planet-explorer/README.md` — add a section explaining the planet picker, Save / Save-as-new workflow, and that `data/planet-profiles/` files are normal git-tracked content.
- `cmd/tools/seed-planet-profiles/README.md` (new) — usage examples, both modes, idempotency note.
- Memory note: append a Phase 5 status section to `project_planet_gen.md`.

## Out of scope

- Bulk seeding of the 411 kb planets (separate future PR).
- "Diff against type default" UI.
- Static-hosting save fallback (clipboard/download). Dev-server-only.
- Concurrent-edit safety (etags, locking). Single-user tool.
- Schema v2 migrations. Plumbing exists; first real migration lands when v2 ships.
- Orbital signatures, Phase 4 Tier C polish, anything outside profile-JSON plumbing.
