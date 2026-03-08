# Standalone System Map SVG Tool

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract SVG system-map rendering into a shared package and create a standalone CLI tool that generates system map SVGs from either a `get_system` JSON file or the SQLite knowledge DB.

**Architecture:** Create `pkg/systemmap/` with all rendering logic (currently in `cmd/generate-items-kb/main.go` lines 1000-1392). The new `cmd/system-map/` CLI reads input from JSON or DB, converts to the package's types, and calls the renderer. The existing generator is updated to import the package instead of using local copies.

**Tech Stack:** Go 1.24, `modernc.org/sqlite`, stdlib `encoding/json`

---

### Task 1: Create the `pkg/systemmap/` package with types and rendering

**Files:**
- Create: `pkg/systemmap/systemmap.go`

**Step 1: Create the package with exported types and rendering functions**

Extract these types (from `cmd/generate-items-kb/main.go:148-180`) into the new package with exported names:

```go
// Package systemmap renders SVG system maps from game system data.
package systemmap

// System holds the data needed to render a system map.
type System struct {
	ID             string
	Name           string
	PositionX      float64
	PositionY      float64
	Connections    []Connection
	POIs           []POI
}

// Connection is a jump gate connection to another system.
type Connection struct {
	SystemID string
	Name     string
	Distance int
}

// POI is a point of interest within a system.
type POI struct {
	ID          string
	Name        string
	Type        string
	Description string
	PositionX   float64
	PositionY   float64
}
```

Extract these functions from `cmd/generate-items-kb/main.go` into the package, making `RenderSystemMap` the only exported function:

| Original (local)             | New (pkg)                  | Lines     |
|------------------------------|----------------------------|-----------|
| `renderSystemMap`            | `RenderSystemMap`          | 1000-1273 |
| `computeGateAngle`          | `computeGateAngle`         | 1277-1285 |
| `poiSeed`                   | `poiSeed`                  | 1288-1292 |
| `poiTitle`                  | `poiTitle`                 | 1295-1300 |
| `htmlEscape`                | `htmlEscape`               | 1303-1309 |
| `renderHexagon`             | `renderHexagon`            | 1312-1321 |
| `renderFourPointStar`       | `renderFourPointStar`      | 1324-1338 |
| `generateAsteroidParticles` | `generateAsteroidParticles`| 1341-1367 |
| `generateIceParticles`      | `generateIceParticles`     | 1370-1392 |

Signature change for the exported function:
```go
// RenderSystemMap generates a complete SVG system map.
// When embedded in HTML, the result includes a wrapping <div class="sys-map">.
// When standalone is true, the wrapping div is omitted and a bare <svg> is returned.
// allSystems is used to compute jump gate angles from galaxy coordinates;
// it may be nil if gate directions are not needed.
func RenderSystemMap(sys *System, allSystems map[string]*System, standalone bool) string
```

The `standalone` flag controls whether the outer `<div class="sys-map">` wrapper and CSS class references are included. When `true`, emit a plain `<svg>` with an explicit `width`/`height` and inline the text styling. When `false`, keep existing behavior for the KB HTML pages.

**Step 2: Verify it compiles**

Run: `go build ./pkg/systemmap/`
Expected: success, no errors

**Step 3: Commit**

```bash
git add pkg/systemmap/systemmap.go
git commit -m "Extract system map SVG rendering into pkg/systemmap"
```

---

### Task 2: Update `generate-items-kb` to use the new package

**Files:**
- Modify: `cmd/generate-items-kb/main.go`

**Step 1: Replace local types and functions with package imports**

In `cmd/generate-items-kb/main.go`:

1. Add import: `"github.com/rsned/spacemolt-kb/pkg/systemmap"`
2. Delete the local rendering functions (lines 1000-1392): `renderSystemMap`, `computeGateAngle`, `poiSeed`, `poiTitle`, `htmlEscape`, `renderHexagon`, `renderFourPointStar`, `generateAsteroidParticles`, `generateIceParticles`
3. Keep the local `System`, `SystemPOI`, `SystemConnection` types (they have extra fields like `Bases`, `Resources`, `SecurityStatus` etc. not needed by the renderer)
4. Add a conversion helper:

```go
func toMapSystem(s *System) *systemmap.System {
	ms := &systemmap.System{
		ID:        s.ID,
		Name:      s.Name,
		PositionX: s.PositionX,
		PositionY: s.PositionY,
	}
	for _, c := range s.Connections {
		ms.Connections = append(ms.Connections, systemmap.Connection{
			SystemID: c.SystemID,
			Name:     c.Name,
			Distance: c.Distance,
		})
	}
	for _, p := range s.POIs {
		ms.POIs = append(ms.POIs, systemmap.POI{
			ID:          p.ID,
			Name:        p.Name,
			Type:        p.Type,
			Description: p.Description,
			PositionX:   p.PositionX,
			PositionY:   p.PositionY,
		})
	}
	return ms
}
```

5. Update the template `systemMap` function (around line 912) to convert and call:

```go
"systemMap": func(sys *System) htmltpl.HTML {
	allMap := make(map[string]*systemmap.System, len(sysLookup))
	for k, v := range sysLookup {
		allMap[k] = toMapSystem(v)
	}
	return htmltpl.HTML(systemmap.RenderSystemMap(toMapSystem(sys), allMap, false))
},
```

Note: building `allMap` every call is fine — it's only used during page generation, not hot-path. If desired, build it once before the template loop instead.

**Step 2: Verify the generator produces identical output**

```bash
# Save current output for comparison
cp kb/systems/sol.html /tmp/sol-before.html
go run ./cmd/generate-items-kb/
diff /tmp/sol-before.html kb/systems/sol.html
```

Expected: no diff (identical output)

**Step 3: Commit**

```bash
git add cmd/generate-items-kb/main.go
git commit -m "Use pkg/systemmap in generate-items-kb instead of local rendering"
```

---

### Task 3: Create the standalone `cmd/system-map` CLI tool

**Files:**
- Create: `cmd/system-map/main.go`

**Step 1: Write the CLI tool**

```go
// Command system-map generates an SVG system map from either a get_system
// JSON response file or from the SQLite knowledge database.
//
// Usage:
//   system-map -json path/to/get_system.json          # from JSON
//   system-map -db path/to/knowledge.db -system sol    # from DB
//   system-map -json file.json -o map.svg              # write to file
package main
```

Flags:
- `-json <path>` — read a `get_system` API response JSON file
- `-db <path>` — path to SQLite knowledge DB
- `-system <id>` — system ID to render (required with `-db`)
- `-o <path>` — output file (default: stdout)

JSON parsing: define structs matching the `get_system` response format from `docs/get_system.sol.json`:

```go
type getSystemResponse struct {
	System struct {
		ID             string       `json:"id"`
		Name           string       `json:"name"`
		POIs           []jsonPOI    `json:"pois"`
		Connections    []jsonConn   `json:"connections"`
	} `json:"system"`
}

type jsonPOI struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Position    struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"position"`
}

type jsonConn struct {
	SystemID string `json:"system_id"`
	Name     string `json:"name"`
	Distance int    `json:"distance"`
}
```

DB loading: query the same tables as the generator (`systems`, `pois`, `connections`) but only for the requested system ID.

Both paths convert to `systemmap.System` and call `systemmap.RenderSystemMap(sys, nil, true)` (nil allSystems is fine — gate angles will default to 0, or we can load connected systems from DB if available).

For the DB path, also load connected systems so gate angles are correct:

```go
// Load the target system + its direct neighbors for gate angle computation.
```

**Step 2: Verify it works with JSON input**

```bash
go run ./cmd/system-map/ -json docs/get_system.sol.json > /tmp/sol-map.svg
# Open in browser to visually verify
```

Expected: valid SVG output

**Step 3: Verify it works with DB input**

```bash
go run ./cmd/system-map/ -db /home/robert/spacemolt/spacemolt/data/spacemolt-knowledge.db -system sol > /tmp/sol-db-map.svg
```

Expected: valid SVG output matching the JSON version

**Step 4: Commit**

```bash
git add cmd/system-map/main.go
git commit -m "Add standalone system-map SVG generator tool"
```

---

### Task 4: Run linter and fix any issues

**Step 1: Run golangci-lint**

```bash
golangci-lint run ./pkg/systemmap/ ./cmd/system-map/ ./cmd/generate-items-kb/
```

**Step 2: Fix any findings**

**Step 3: Final commit**

```bash
git add -A
git commit -m "Fix lint findings in system-map tooling"
```
