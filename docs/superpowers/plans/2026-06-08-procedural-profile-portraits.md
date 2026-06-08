# Procedural Profile Portraits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every scanned player and ship passenger an automatic profile image — a deterministic inline SVG silhouette for players, and prompt-driven AI portraits (via a pluggable CLI) for passengers — while a contributor-authored overlay always wins.

**Architecture:** All work lives in the `cmd/generate-factions-kb` generator (Go, `html/template`). A shared, deterministic **silhouette builder** produces inline SVG used as the universal no-image fallback for both players and passengers. A new **passenger rendering surface** (index + detail pages) is added, modeled on the existing player pages. An opt-in **`--portraits` step** builds prompts from each passenger's bio/class/citizenship and shells out to a configured image-generation command (`SMKB_PORTRAIT_CMD`), caching results under `overlays/generated/passengers/<id>/` keyed by a prompt hash. The everyday render step merely consumes whatever cached portraits exist. Portrait precedence for a passenger: contributor overlay → generated AI portrait → silhouette.

**Tech Stack:** Go 1.24, `html/template`, `hash/fnv` (deterministic seeding), `crypto/sha256` (prompt hashing), `os/exec` (CLI shell-out), `modernc.org/sqlite` (DB), existing `validateImage`/`copyOverlayImage`/`loadOverlay` helpers.

**Spec:** `docs/superpowers/specs/2026-06-08-procedural-profile-portraits-design.md`

> **Note — one refinement of the spec:** the spec left passenger no-portrait fallback as a "plain banner." This plan uses the **shared silhouette** as that fallback instead (it reuses Phase 1 and gives passengers a real visual identity in Phase 2 before any AI generation runs). Players' no-overlay branch likewise moves from the bare `player-banner` to a silhouette infobox. If you'd rather keep bare banners, only Task 3 and Task 7's template branches change.

---

## File Structure

**New files (in `cmd/generate-factions-kb/`):**
- `silhouette.go` — deterministic seed hash, derived palette, inline SVG silhouette builder. One responsibility: turn `(seed, primary, secondary)` into an `<svg>` string.
- `silhouette_test.go` — determinism + variation tests.
- `passengers.go` — `loadPassengers`, empire color map/lookup, `attachPassengerOverlays`, `attachPassengerPortraits`.
- `passengers_test.go` — loader (in-memory SQLite) + empire-color + portrait-attach tests.
- `prompt.go` — `buildPortraitPrompt`, `promptHash`, style-suffix constant.
- `prompt_test.go` — prompt-builder table tests + hash stability.
- `portraits.go` — `generatePassengerPortraits` + cache helpers + CLI shell-out.
- `portraits_test.go` — cache-skip / regenerate tests using a fake shell command.

**New committed site asset:**
- `kb/passengers/passengers.css` — passenger page styles (mirrors `players.css`), preserved across regen.

**Modified files:**
- `cmd/generate-factions-kb/types.go` — add `Passenger` struct.
- `cmd/generate-factions-kb/render.go` — add `silhouette` template func; rework player-detail no-overlay branch; add `passengerIndexTmpl` + `passengerDetailTmpl`; add Passengers nav link.
- `cmd/generate-factions-kb/main.go` — `--portraits` flag; load/attach/render passengers; wire generation step.
- `overlays/README.md` — document `overlays/generated/` tree + the `SMKB_PORTRAIT_CMD` contract.

---

## PHASE 1 — Player silhouettes

### Task 1: Deterministic seed hash + derived palette

**Files:**
- Create: `cmd/generate-factions-kb/silhouette.go`
- Test: `cmd/generate-factions-kb/silhouette_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/generate-factions-kb/silhouette_test.go
package main

import "testing"

func TestSeedHashStable(t *testing.T) {
	if seedHash("abc") != seedHash("abc") {
		t.Fatal("seedHash not stable for same input")
	}
	if seedHash("abc") == seedHash("abd") {
		t.Fatal("seedHash collided on distinct inputs")
	}
}

func TestDerivePaletteDeterministicAndNonEmpty(t *testing.T) {
	f1, a1 := derivePalette("player-123")
	f2, a2 := derivePalette("player-123")
	if f1 != f2 || a1 != a2 {
		t.Fatalf("derivePalette not deterministic: (%s,%s) vs (%s,%s)", f1, a1, f2, a2)
	}
	if f1 == "" || a1 == "" {
		t.Fatal("derivePalette returned empty color")
	}
	// Different seeds should not all collapse to one field color.
	if g, _ := derivePalette("player-999"); g == f1 {
		t.Fatal("derivePalette gave identical field for different seeds")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestSeedHash|TestDerivePalette' -v`
Expected: FAIL — `undefined: seedHash` / `undefined: derivePalette`.

- [ ] **Step 3: Write minimal implementation**

```go
// cmd/generate-factions-kb/silhouette.go
package main

import (
	"fmt"
	"hash/fnv"
)

// seedHash is a stable 64-bit FNV-1a hash of s, used to derive deterministic
// visual variation from an entity ID.
func seedHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// derivePalette returns (field, accent) CSS colors derived deterministically
// from seed. Used when a profile has no color data of its own. Both are
// hsl() strings, valid as SVG fills.
func derivePalette(seed string) (field, accent string) {
	h := seedHash(seed)
	hue := h % 360
	field = fmt.Sprintf("hsl(%d 45%% 38%%)", hue)
	accent = fmt.Sprintf("hsl(%d 60%% 58%%)", (hue+150)%360)
	return field, accent
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestSeedHash|TestDerivePalette' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/silhouette.go cmd/generate-factions-kb/silhouette_test.go
git commit -m "feat(kb): deterministic seed hash + derived palette for silhouettes"
```

---

### Task 2: Inline SVG silhouette builder

**Files:**
- Modify: `cmd/generate-factions-kb/silhouette.go`
- Test: `cmd/generate-factions-kb/silhouette_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to cmd/generate-factions-kb/silhouette_test.go
import (
	"strings"
	"testing"
)

func TestSilhouetteSVGDeterministic(t *testing.T) {
	a := string(silhouetteSVG("id-1", "#112233", "#445566"))
	b := string(silhouetteSVG("id-1", "#112233", "#445566"))
	if a != b {
		t.Fatal("silhouetteSVG not byte-identical for same input")
	}
	if !strings.Contains(a, "<svg class=\"silhouette\"") {
		t.Fatalf("missing svg root: %s", a)
	}
	if !strings.Contains(a, "#112233") || !strings.Contains(a, "#445566") {
		t.Fatal("provided colors not used in SVG")
	}
}

func TestSilhouetteSVGVariesBySeed(t *testing.T) {
	// Across several seeds we should see more than one distinct SVG.
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		seen[string(silhouetteSVG(id, "", ""))] = true
	}
	if len(seen) < 2 {
		t.Fatalf("silhouettes did not vary across seeds: %d distinct", len(seen))
	}
}

func TestSilhouetteSVGEmptyColorsUsesDerivedPalette(t *testing.T) {
	out := string(silhouetteSVG("id-x", "", ""))
	if !strings.Contains(out, "hsl(") {
		t.Fatal("empty colors should fall back to derived hsl palette, got none")
	}
}
```

(Note: if Task 1's test file does not yet import `strings`, merge the import blocks — there must be a single import block per file.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestSilhouetteSVG -v`
Expected: FAIL — `undefined: silhouetteSVG`.

- [ ] **Step 3: Write minimal implementation**

> **Imports:** Go disallows leaving two separate `import (...)` blocks (goimports/gofumpt will flag it). **Merge** these new imports into the single existing import block from Task 1 so the file has one block: `"fmt"`, `"hash/fnv"`, `htmltpl "html/template"`, `"strings"`.

```go
// append to cmd/generate-factions-kb/silhouette.go (merge imports per note above)
import (
	htmltpl "html/template"
	"strings"
)

// silhouetteFill is the dark crew-silhouette color (head + shoulders).
const silhouetteFill = "hsl(222 24% 10%)"

// silhouetteVariants is the number of distinct visor styles.
const silhouetteVariants = 5

// silhouetteSVG returns a self-contained inline <svg> for a stylized sci-fi
// crew silhouette, deterministic from seed and tinted by primary/secondary.
// Empty primary/secondary fall back to a palette derived from seed. The output
// is trusted HTML: it is built entirely from literals and pre-validated colors.
func silhouetteSVG(seed, primary, secondary string) htmltpl.HTML {
	field, accent := primary, secondary
	if field == "" || accent == "" {
		df, da := derivePalette(seed)
		if field == "" {
			field = df
		}
		if accent == "" {
			accent = da
		}
	}
	h := seedHash(seed)
	variant := h % silhouetteVariants
	hasBadge := (h>>8)&1 == 1

	var b strings.Builder
	b.WriteString(`<svg class="silhouette" viewBox="0 0 100 120" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="generated crew silhouette">`)
	// Background field.
	b.WriteString(`<rect width="100" height="120" fill="` + field + `"/>`)
	// Faint vignette for depth.
	b.WriteString(`<rect width="100" height="120" fill="hsl(0 0% 0%)" opacity="0.12"/>`)
	// Shoulders.
	b.WriteString(`<path d="M12,120 C12,90 30,80 50,80 C70,80 88,90 88,120 Z" fill="` + silhouetteFill + `"/>`)
	// Head.
	b.WriteString(`<circle cx="50" cy="50" r="26" fill="` + silhouetteFill + `"/>`)
	// Visor (variant-dependent), tinted with the accent color.
	b.WriteString(visorMarkup(variant, accent))
	if hasBadge {
		b.WriteString(`<circle cx="68" cy="98" r="4" fill="` + accent + `"/>`)
	}
	b.WriteString(`</svg>`)
	return htmltpl.HTML(b.String())
}

// visorMarkup returns the variant-specific visor element(s), filled with accent.
func visorMarkup(variant uint64, accent string) string {
	switch variant {
	case 0: // horizontal slit
		return `<rect x="34" y="46" width="32" height="8" rx="4" fill="` + accent + `" opacity="0.9"/>`
	case 1: // T-visor
		return `<path d="M36,42 H64 V48 H54 V60 H46 V48 H36 Z" fill="` + accent + `" opacity="0.9"/>`
	case 2: // full curved visor
		return `<path d="M30,48 a20,14 0 0 1 40,0 a20,8 0 0 1 -40,0 Z" fill="` + accent + `" opacity="0.85"/>`
	case 3: // round single eye
		return `<circle cx="50" cy="50" r="9" fill="` + accent + `" opacity="0.9"/>`
	default: // angled visor
		return `<path d="M32,44 L68,52 L66,58 L34,52 Z" fill="` + accent + `" opacity="0.9"/>`
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestSilhouetteSVG -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/silhouette.go cmd/generate-factions-kb/silhouette_test.go
git commit -m "feat(kb): inline SVG sci-fi crew silhouette builder"
```

---

### Task 3: Wire silhouette into the player detail page

**Files:**
- Modify: `cmd/generate-factions-kb/render.go` (templateFuncs `:11`, playerDetailTmpl `:372-432`)
- Test: `cmd/generate-factions-kb/render_test.go` (create)

- [ ] **Step 1: Write the failing test**

```go
// cmd/generate-factions-kb/render_test.go
package main

import (
	htmltpl "html/template"
	"strings"
	"testing"
	"time"
)

func TestPlayerDetailRendersSilhouetteWhenNoOverlayImage(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))
	p := &Player{
		ID:           "player-abc",
		Username:     "Nova",
		FirstSeenUTC: "2026-01-01T00:00:00Z",
		LastSeenUTC:  "2026-01-02T00:00:00Z",
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, p); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `<svg class="silhouette"`) {
		t.Fatal("expected silhouette SVG in no-overlay player page")
	}
	if !strings.Contains(out, "player-abc") {
		t.Fatal("expected player ID in page")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestPlayerDetailRendersSilhouette -v`
Expected: FAIL — `function "silhouette" not defined` (template parse error).

- [ ] **Step 3a: Add the `silhouette` template func**

In `render.go`, inside the `htmltpl.FuncMap{...}` returned by `templateFuncs` (after the `"inline"` entry at `:29`), add:

```go
		"silhouette": func(seed, primary, secondary string) htmltpl.HTML {
			return silhouetteSVG(seed, primary, secondary)
		},
```

- [ ] **Step 3b: Replace the player-detail no-overlay branch**

In `playerDetailTmpl`, replace the entire `{{else}} ... {{end}}` block that currently renders `.player-banner` + `.stat-strip` (lines `:402-422`) with this branch (the `{{if and .Overlay .Overlay.ImageFile}}` overlay branch above it is unchanged):

```gotemplate
{{else}}
        <aside class="infobox"{{if .PrimaryColor}} style="--player-accent:{{.PrimaryColor}}"{{end}}>
            <div class="infobox-title">{{.Username}}</div>
            {{if .FactionTag}}<div class="infobox-subtitle">{{if .FactionSlug}}<a href="../../factions/{{.FactionSlug}}/">{{.FactionTag}}</a>{{else}}{{.FactionTag}}{{end}}</div>{{end}}
            <div class="infobox-silhouette">{{silhouette .ID .PrimaryColor .SecondaryColor}}</div>
            <dl class="infobox-data">
                {{if .ClanTag}}<dt>Clan</dt><dd>{{.ClanTag}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Last seen</dt><dd>{{rel .LastSeenUTC}}</dd>
            </dl>
            <div class="infobox-id">{{.ID}}</div>
        </aside>
        {{if .StatusMessage}}<div class="pb-status">{{inline .StatusMessage}}</div>{{end}}
{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{end}}
```

- [ ] **Step 3c: Add silhouette CSS**

Append to `kb/players/players.css`:

```css
/* Generated placeholder silhouette (shown when no overlay/AI portrait). */
.infobox-silhouette {
    display: block;
    max-width: calc(100% - 1.5rem);
    margin: .85rem auto .4rem;
    border-radius: 6px;
    overflow: hidden;
    line-height: 0;
}
.infobox-silhouette .silhouette { width: 100%; height: auto; display: block; }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestPlayerDetailRendersSilhouette -v`
Expected: PASS.

- [ ] **Step 5: Build, lint, full test**

Run: `go build ./... && go test ./cmd/generate-factions-kb/... && golangci-lint run ./cmd/generate-factions-kb/...`
Expected: build OK, tests PASS, no new lint findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-factions-kb/render.go cmd/generate-factions-kb/render_test.go kb/players/players.css
git commit -m "feat(kb): show generated silhouette on player pages lacking an overlay image"
```

---

## PHASE 2 — Passenger pages (text-first)

### Task 4: Passenger type + empire color lookup + loader

**Files:**
- Modify: `cmd/generate-factions-kb/types.go` (add `Passenger`)
- Create: `cmd/generate-factions-kb/passengers.go`
- Test: `cmd/generate-factions-kb/passengers_test.go`

- [ ] **Step 1: Add the `Passenger` type**

Append to `types.go`:

```go
// Passenger is a ship passenger (citizen) sighted in the game world.
type Passenger struct {
	ID            string
	Slug          string // == ID (citizen_id is already URL-safe)
	Name          string
	Citizenship   string
	EmpireColor   string // resolved from citizenship; "" when unknown
	Bio           string
	Class         string // travel class: "first" / "business"
	FirstSeenUTC  string
	LastSeenUTC   string
	SightingCount int

	Overlay      *Overlay // contributor-authored content, nil when none
	PortraitFile string   // generated AI portrait filename, "" when none
}
```

- [ ] **Step 2: Write the failing test**

```go
// cmd/generate-factions-kb/passengers_test.go
package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEmpireColor(t *testing.T) {
	if got := empireColor("crimson"); got != "#DC143C" {
		t.Fatalf("crimson = %q, want #DC143C", got)
	}
	if got := empireColor("NEBULA"); got != "#00CED1" {
		t.Fatalf("NEBULA = %q, want #00CED1 (case-insensitive)", got)
	}
	if got := empireColor("unknown"); got != "" {
		t.Fatalf("unknown empire = %q, want empty", got)
	}
}

func TestLoadPassengers(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`CREATE TABLE passengers (
		citizen_id TEXT PRIMARY KEY, name TEXT NOT NULL, citizenship TEXT,
		bio TEXT, class TEXT, first_seen_utc TEXT NOT NULL,
		last_seen_utc TEXT NOT NULL, sighting_count INTEGER NOT NULL DEFAULT 1)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO passengers VALUES
		('b_id','Bea','nebula','rich bio','business','2026-01-01T00:00:00Z','2026-01-02T00:00:00Z',3),
		('a_id','Abe','crimson','','first','2026-01-01T00:00:00Z','2026-01-03T00:00:00Z',1)`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := loadPassengers(db)
	if err != nil {
		t.Fatalf("loadPassengers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d passengers, want 2", len(got))
	}
	// Sorted by name (case-insensitive): Abe before Bea.
	if got[0].Name != "Abe" || got[1].Name != "Bea" {
		t.Fatalf("unexpected order: %s, %s", got[0].Name, got[1].Name)
	}
	if got[0].EmpireColor != "#DC143C" {
		t.Fatalf("Abe empire color = %q, want #DC143C", got[0].EmpireColor)
	}
	if got[1].SightingCount != 3 || got[1].Class != "business" {
		t.Fatalf("Bea fields wrong: count=%d class=%s", got[1].SightingCount, got[1].Class)
	}
	if got[0].Slug != "a_id" {
		t.Fatalf("Slug should equal citizen_id, got %q", got[0].Slug)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestEmpireColor|TestLoadPassengers' -v`
Expected: FAIL — `undefined: empireColor` / `undefined: loadPassengers`.

- [ ] **Step 4: Write minimal implementation**

```go
// cmd/generate-factions-kb/passengers.go
package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// empireColors maps lowercase empire (citizenship) names to their theme color.
// Values mirror cmd/generate-items-kb/main.go:351.
var empireColors = map[string]string{
	"solarian": "#FFD700",
	"voidborn": "#9932CC",
	"crimson":  "#DC143C",
	"nebula":   "#00CED1",
	"outerrim": "#2E8B57",
}

// empireColor returns the theme color for a citizenship, or "" when unknown.
func empireColor(citizenship string) string {
	return empireColors[strings.ToLower(strings.TrimSpace(citizenship))]
}

// loadPassengers loads all rows of the passengers table, sorted by name
// (case-insensitive, ID as tiebreaker). A missing/empty table is the caller's
// concern: a missing table surfaces as a query error.
func loadPassengers(db *sql.DB) ([]*Passenger, error) {
	rows, err := db.Query(`SELECT citizen_id, name, citizenship, bio, class,
	                              first_seen_utc, last_seen_utc, sighting_count
	                       FROM passengers`)
	if err != nil {
		return nil, fmt.Errorf("query passengers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*Passenger
	for rows.Next() {
		var id, name string
		var citizenship, bio, class, first, last sql.NullString
		var count sql.NullInt64
		if err := rows.Scan(&id, &name, &citizenship, &bio, &class, &first, &last, &count); err != nil {
			return nil, fmt.Errorf("scan passenger: %w", err)
		}
		out = append(out, &Passenger{
			ID:            id,
			Slug:          id,
			Name:          name,
			Citizenship:   citizenship.String,
			EmpireColor:   empireColor(citizenship.String),
			Bio:           bio.String,
			Class:         class.String,
			FirstSeenUTC:  first.String,
			LastSeenUTC:   last.String,
			SightingCount: int(count.Int64),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate passengers: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li != lj {
			return li < lj
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestEmpireColor|TestLoadPassengers' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-factions-kb/types.go cmd/generate-factions-kb/passengers.go cmd/generate-factions-kb/passengers_test.go
git commit -m "feat(kb): Passenger type, empire color lookup, and passengers loader"
```

---

### Task 5: Passenger overlay + portrait attachment

**Files:**
- Modify: `cmd/generate-factions-kb/passengers.go`
- Test: `cmd/generate-factions-kb/passengers_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to cmd/generate-factions-kb/passengers_test.go
import (
	"os"
	"path/filepath"
)

func TestAttachPassengerPortraitsMissingFileIsSilent(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "nobody", Slug: "nobody"}}
	attachPassengerPortraits(ps, root) // no file on disk
	if ps[0].PortraitFile != "" {
		t.Fatalf("expected empty PortraitFile, got %q", ps[0].PortraitFile)
	}
}

func TestAttachPassengerPortraitsValidImage(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "generated", "passengers", "p1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 1x1 PNG written as portrait.webp name? validateImage checks extension by
	// name AND decodes content. Use a real .png file to keep it simple.
	writeTinyPNG(t, filepath.Join(dir, generatedPortraitName))
	ps := []*Passenger{{ID: "p1", Slug: "p1"}}
	attachPassengerPortraits(ps, root)
	if ps[0].PortraitFile != generatedPortraitName {
		t.Fatalf("PortraitFile = %q, want %q", ps[0].PortraitFile, generatedPortraitName)
	}
}
```

Add this helper at the bottom of `passengers_test.go` (writes a valid 1×1 PNG to the given path, named whatever `generatedPortraitName` is — set `generatedPortraitName` to `"portrait.png"` in Task 5 Step 3 so name-extension validation passes):

```go
func writeTinyPNG(t *testing.T, path string) {
	t.Helper()
	// 1x1 transparent PNG.
	png := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestAttachPassengerPortraits -v`
Expected: FAIL — `undefined: attachPassengerPortraits` / `undefined: generatedPortraitName`.

- [ ] **Step 3: Write minimal implementation**

> **Imports:** Merge these into the single existing import block from Task 4 (don't leave two `import (...)` blocks). Final block: `"database/sql"`, `"errors"`, `"fmt"`, `"log"`, `"os"`, `"path/filepath"`, `"sort"`, `"strings"`.

```go
// append to cmd/generate-factions-kb/passengers.go (merge imports per note above)
import (
	"errors"
	"log"
	"os"
	"path/filepath"
)

// generatedPortraitName is the fixed filename of a generated passenger portrait
// within overlays/generated/passengers/<id>/. PNG keeps the name/extension
// validation in validateImage simple for any backend that emits PNG.
const generatedPortraitName = "portrait.png"

// passengerGeneratedDir is the cache directory for a passenger's generated
// portrait + prompt sidecar, under the overlays root.
func passengerGeneratedDir(root, id string) string {
	return filepath.Join(root, "generated", "passengers", id)
}

// attachPassengerOverlays loads overlays/passengers/<id>/profile.md for each
// passenger and warns about overlay dirs that match no current passenger.
func attachPassengerOverlays(passengers []*Passenger, root string) {
	entityIDs := make(map[string]bool, len(passengers))
	for _, p := range passengers {
		entityIDs[p.ID] = true
		ov, err := loadOverlay(filepath.Join(root, "passengers", p.ID))
		if err != nil {
			log.Printf("warning: passenger overlay %s: %v", p.ID, err)
			continue
		}
		p.Overlay = ov
	}
	warnOrphanOverlays(filepath.Join(root, "passengers"), entityIDs)
}

// attachPassengerPortraits sets PortraitFile for each passenger whose generated
// portrait exists and validates. A missing file is silent; only real validation
// failures warn.
func attachPassengerPortraits(passengers []*Passenger, root string) {
	for _, p := range passengers {
		dir := passengerGeneratedDir(root, p.ID)
		name, err := validateImage(dir, generatedPortraitName)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.Printf("warning: generated portrait %s: %v", p.ID, err)
			}
			continue
		}
		p.PortraitFile = name
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestAttachPassengerPortraits -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/passengers.go cmd/generate-factions-kb/passengers_test.go
git commit -m "feat(kb): passenger overlay + generated-portrait attachment"
```

---

### Task 6: Passenger index + detail templates

**Files:**
- Modify: `cmd/generate-factions-kb/render.go` (add templates; add Passengers nav link)
- Create: `kb/passengers/passengers.css`
- Test: `cmd/generate-factions-kb/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to cmd/generate-factions-kb/render_test.go
func TestPassengerDetailPortraitPrecedence(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("psdet").Funcs(funcs).Parse(passengerDetailTmpl))

	// No overlay, no generated portrait -> silhouette.
	silP := &Passenger{ID: "p1", Slug: "p1", Name: "Lin", Bio: "a fixer"}
	var b1 strings.Builder
	if err := tmpl.Execute(&b1, silP); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b1.String(), `<svg class="silhouette"`) {
		t.Fatal("expected silhouette when no portrait/overlay")
	}
	if !strings.Contains(b1.String(), "a fixer") {
		t.Fatal("expected bio in About")
	}

	// Generated portrait present -> <img>, no silhouette.
	genP := &Passenger{ID: "p2", Slug: "p2", Name: "Bea", PortraitFile: "portrait.png"}
	var b2 strings.Builder
	if err := tmpl.Execute(&b2, genP); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b2.String(), `src="portrait.png"`) {
		t.Fatal("expected generated portrait img")
	}
	if strings.Contains(b2.String(), `<svg class="silhouette"`) {
		t.Fatal("silhouette should be suppressed when a portrait exists")
	}
}

func TestPassengerIndexLists(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("psidx").Funcs(funcs).Parse(passengerIndexTmpl))
	ps := []*Passenger{{ID: "p1", Slug: "p1", Name: "Lin", Citizenship: "nebula", Class: "first"}}
	var b strings.Builder
	if err := tmpl.Execute(&b, ps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `href="p1/"`) || !strings.Contains(b.String(), "Lin") {
		t.Fatal("expected passenger link + name in index")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestPassengerDetail|TestPassengerIndex' -v`
Expected: FAIL — `undefined: passengerDetailTmpl` / `undefined: passengerIndexTmpl`.

- [ ] **Step 3a: Add Passengers nav link**

In `render.go`, append a Passengers link to **both** nav consts:

In `navLinks1` (after the Players line at `:99`):
```go
            <a href="../players/index.html">Players</a>
            <a href="../passengers/index.html">Passengers</a>`
```

In `navLinks2` (after the Players line at `:107`):
```go
            <a href="../../players/index.html">Players</a>
            <a href="../../passengers/index.html">Passengers</a>`
```

- [ ] **Step 3b: Add the passenger templates**

Append to `render.go`:

```go
// --- Passenger index ---
var passengerIndexTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Passengers - Spacemolt KB</title>
    <link rel="stylesheet" href="../smui.css">
    <link rel="stylesheet" href="../passengers/passengers.css">
</head>
<body>
` + siteHeader1 + `
    <main class="container page-content">
        <h2>Passengers</h2>
        <p class="text-muted mt-1">{{len .}} ship passengers sighted in transit.</p>
        <table class="sortable">
            <thead><tr><th class="sortable">Name</th><th class="sortable">Citizenship</th><th class="sortable">Class</th><th class="sortable">Sightings</th></tr></thead>
            <tbody>
{{- range .}}
                <tr>
                    <td><a href="{{.Slug}}/">{{.Name}}</a></td>
                    <td>{{dash .Citizenship}}</td>
                    <td>{{dash .Class}}</td>
                    <td data-sort="{{.SightingCount}}">{{.SightingCount}}</td>
                </tr>
{{- end}}
            </tbody>
        </table>
    </main>
` + themeScript + sortScript + `
</body>
</html>
`

// --- Passenger detail ---
// Portrait precedence: contributor overlay image > generated AI portrait >
// deterministic silhouette.
var passengerDetailTmpl = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Name}} - Passengers - Spacemolt KB</title>
    <link rel="stylesheet" href="../../smui.css">
    <link rel="stylesheet" href="../../passengers/passengers.css">
</head>
<body>
` + siteHeader2 + `
    <main class="container page-content detail-page">
        <aside class="infobox"{{if .EmpireColor}} style="--player-accent:{{.EmpireColor}}"{{end}}>
            <div class="infobox-title">{{.Name}}</div>
            {{if .Citizenship}}<div class="infobox-subtitle">{{.Citizenship}}</div>{{end}}
{{if and .Overlay .Overlay.ImageFile}}
            <img class="infobox-image" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">
            {{if .Overlay.ImageAlt}}<figcaption class="infobox-caption">{{.Overlay.ImageAlt}}</figcaption>{{end}}
{{else if .PortraitFile}}
            <img class="infobox-image" src="{{.PortraitFile}}" alt="Generated portrait of {{.Name}}">
            <figcaption class="infobox-caption">AI-generated placeholder portrait.</figcaption>
{{else}}
            <div class="infobox-silhouette">{{silhouette .ID .EmpireColor ""}}</div>
{{end}}
            <dl class="infobox-data">
                {{if .Class}}<dt>Class</dt><dd>{{.Class}}</dd>{{end}}
                <dt>First seen</dt><dd>{{shortDate .FirstSeenUTC}}</dd>
                <dt>Last seen</dt><dd>{{rel .LastSeenUTC}}</dd>
                <dt>Sightings</dt><dd>{{.SightingCount}}</dd>
            </dl>
            <div class="infobox-id">{{.ID}}</div>
        </aside>
{{if .Bio}}
        <h3>About</h3>
        {{richText "" .Bio}}
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>Profile</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
    </main>
` + themeScript + `
</body>
</html>
`
```

- [ ] **Step 3c: Create `kb/passengers/passengers.css`**

```css
/* Passenger KB pages. Builds on smui.css; reuses the .infobox component. */
.detail-page { font-size: 15px; }
.detail-page h2 { font-size: 26px; }
.detail-page h3 { font-size: 28px; }

/* Generated placeholder silhouette (shown when no overlay/AI portrait). */
.infobox-silhouette {
    display: block;
    max-width: calc(100% - 1.5rem);
    margin: .85rem auto .4rem;
    border-radius: 6px;
    overflow: hidden;
    line-height: 0;
}
.infobox-silhouette .silhouette { width: 100%; height: auto; display: block; }

/* Contributor overlay body (mirrors players.css). */
.overlay-credit { font-size: 12px; font-style: italic; margin: 0 0 .4rem; }
.overlay-body { max-width: 70ch; line-height: 1.65; }
.overlay-body p { margin: .5rem 0; }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestPassengerDetail|TestPassengerIndex' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/render.go kb/passengers/passengers.css cmd/generate-factions-kb/render_test.go
git commit -m "feat(kb): passenger index + detail templates with portrait precedence"
```

---

### Task 7: Wire passengers into `main` (load, attach, render)

**Files:**
- Modify: `cmd/generate-factions-kb/main.go`

- [ ] **Step 1: Add the passengers output dir + templates**

In `main()`, after `systemsDir := "kb/systems"` (`:21`) add:
```go
	passengersOut := "kb/passengers"
```

After the player template parsing (`pDet := ...` at `:67`) add:
```go
	psIdx := htmltpl.Must(htmltpl.New("psidx").Funcs(funcs).Parse(passengerIndexTmpl))
	psDet := htmltpl.Must(htmltpl.New("psdet").Funcs(funcs).Parse(passengerDetailTmpl))
```

- [ ] **Step 2: Load + attach passengers**

After `attachPlayerOverlays(players, overlaysRoot)` (`:61`) add:
```go
	passengers, err := loadPassengers(db)
	if err != nil {
		log.Printf("warning: load passengers: %v (passenger pages skipped)", err)
		passengers = nil
	}
	attachPassengerOverlays(passengers, overlaysRoot)
	attachPassengerPortraits(passengers, overlaysRoot)
```

(Note: `loadPassengers` returning an error — e.g. the table does not exist — is downgraded to a warning here so the rest of the site still builds.)

- [ ] **Step 3: Reset + render the passengers section**

After the `mustResetDir(playersOut, "players.css")` line (`:71`) add:
```go
	mustResetDir(passengersOut, "passengers.css")
```

After the players render loop (ends `:91`) add:
```go
	mustWrite(filepath.Join(passengersOut, "index.html"), psIdx, passengers)
	for _, p := range passengers {
		dir := filepath.Join(passengersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), psDet, p)
		if p.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "passengers", p.ID), p.Overlay.ImageFile, dir)
		}
		if p.PortraitFile != "" {
			copyOverlayImage(passengerGeneratedDir(overlaysRoot, p.ID), p.PortraitFile, dir)
		}
	}
```

- [ ] **Step 4: Update the final log line**

Replace the `log.Printf("generated %d factions and %d players", ...)` line (`:93`) with:
```go
	log.Printf("generated %d factions, %d players, %d passengers", len(factions), len(players), len(passengers))
```

- [ ] **Step 5: Build + run end-to-end against the real DB**

Run:
```bash
go build ./... && \
go run ./cmd/generate-factions-kb /home/robert/spacemolt/spacemolt-knowledge.db && \
ls kb/passengers && head -40 kb/passengers/fixer_dao/index.html
```
Expected: build OK; `kb/passengers/index.html` + four passenger dirs (`fixer_dao`, `embezzler_pratt`, `torna_grendano`, `loan_shark_demme`); each detail page contains an `<svg class="silhouette"` (no AI portraits yet) and the bio under "About".

- [ ] **Step 6: Lint + full test**

Run: `golangci-lint run ./cmd/generate-factions-kb/... && go test ./cmd/generate-factions-kb/...`
Expected: no new findings, tests PASS.

- [ ] **Step 7: Commit (code + regenerated pages)**

```bash
git add cmd/generate-factions-kb/main.go kb/passengers kb/players kb/factions
git commit -m "feat(kb): render passenger index + detail pages; add Passengers nav"
```

---

## PHASE 3 — Passenger AI portraits

### Task 8: Prompt builder + prompt hash

**Files:**
- Create: `cmd/generate-factions-kb/prompt.go`
- Test: `cmd/generate-factions-kb/prompt_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/generate-factions-kb/prompt_test.go
package main

import (
	"strings"
	"testing"
)

func TestBuildPortraitPrompt(t *testing.T) {
	p := buildPortraitPrompt("a grizzled ore hauler", "first", "crimson")
	if !strings.Contains(p, "a grizzled ore hauler") {
		t.Fatal("bio missing from prompt")
	}
	if !strings.Contains(p, "refined affluent attire") {
		t.Fatal("first-class attire cue missing")
	}
	if !strings.Contains(p, "crimson empire") {
		t.Fatal("citizenship theme missing")
	}
	if !strings.Contains(p, portraitStyleSuffix) {
		t.Fatal("style suffix missing")
	}
}

func TestBuildPortraitPromptEmptyBioIsValid(t *testing.T) {
	p := buildPortraitPrompt("", "", "")
	if strings.TrimSpace(p) == "" {
		t.Fatal("empty bio produced empty prompt")
	}
	if !strings.Contains(p, portraitStyleSuffix) {
		t.Fatal("style suffix missing on fallback prompt")
	}
}

func TestPromptHashStable(t *testing.T) {
	a := promptHash("hello")
	if a != promptHash("hello") {
		t.Fatal("promptHash not stable")
	}
	if a == promptHash("hellp") {
		t.Fatal("promptHash collided")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestBuildPortraitPrompt|TestPromptHash' -v`
Expected: FAIL — `undefined: buildPortraitPrompt` / `undefined: promptHash` / `undefined: portraitStyleSuffix`.

- [ ] **Step 3: Write minimal implementation**

```go
// cmd/generate-factions-kb/prompt.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// portraitStyleSuffix is appended to every passenger prompt to keep the gallery
// visually consistent. Tune the whole gallery's look here.
const portraitStyleSuffix = ", character portrait, sci-fi crew member, painterly, dramatic lighting, neutral background, head and shoulders"

// buildPortraitPrompt composes an image prompt from a passenger's bio, travel
// class, and citizenship. Always returns a non-empty prompt.
func buildPortraitPrompt(bio, class, citizenship string) string {
	subject := strings.TrimSpace(bio)
	if subject == "" {
		subject = "a nondescript interstellar traveler"
	}
	var attire string
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "first":
		attire = "refined affluent attire"
	case "business":
		attire = "professional business attire"
	default:
		attire = "practical traveler's attire"
	}
	var theme string
	if empire := strings.ToLower(strings.TrimSpace(citizenship)); empire != "" {
		theme = fmt.Sprintf(", %s empire color theme", empire)
	}
	return subject + ", " + attire + theme + portraitStyleSuffix
}

// promptHash returns a hex SHA-256 of the prompt, used as the cache key for
// regeneration decisions.
func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestBuildPortraitPrompt|TestPromptHash' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/prompt.go cmd/generate-factions-kb/prompt_test.go
git commit -m "feat(kb): passenger portrait prompt builder + prompt hash"
```

---

### Task 9: Portrait cache + pluggable CLI generation

**Files:**
- Create: `cmd/generate-factions-kb/portraits.go`
- Test: `cmd/generate-factions-kb/portraits_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/generate-factions-kb/portraits_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePortraitsInvokesCommandAndCaches(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "a fixer", Class: "first", Citizenship: "nebula"}}
	// Fake backend: write a byte to PORTRAIT_OUT.
	cmd := `printf 'x' > "$PORTRAIT_OUT"`
	generatePassengerPortraits(ps, root, cmd)

	out := filepath.Join(passengerGeneratedDir(root, "p1"), generatedPortraitName)
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected generated portrait at %s: %v", out, err)
	}
	sidecar := filepath.Join(passengerGeneratedDir(root, "p1"), promptSidecarName)
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("expected prompt sidecar: %v", err)
	}
}

func TestGeneratePortraitsSkipsWhenCached(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "a fixer", Class: "first", Citizenship: "nebula"}}
	dir := passengerGeneratedDir(root, "p1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-seed a matching sidecar + an existing portrait so the cache is warm.
	prompt := buildPortraitPrompt("a fixer", "first", "nebula")
	if err := os.WriteFile(filepath.Join(dir, promptSidecarName), []byte(promptHash(prompt)+"\n"+prompt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, generatedPortraitName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Command would create a marker if invoked.
	marker := filepath.Join(root, "invoked.marker")
	cmd := `touch "` + marker + `"`
	generatePassengerPortraits(ps, root, cmd)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command was invoked despite warm cache")
	}
}

func TestGeneratePortraitsNoCommandIsNoop(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "x"}}
	generatePassengerPortraits(ps, root, "") // must not panic, must not create files
	if _, err := os.Stat(passengerGeneratedDir(root, "p1")); err == nil {
		t.Fatal("no-command run should not create cache dirs")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestGeneratePortraits -v`
Expected: FAIL — `undefined: generatePassengerPortraits` / `undefined: promptSidecarName`.

- [ ] **Step 3: Write minimal implementation**

```go
// cmd/generate-factions-kb/portraits.go
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// promptSidecarName stores the prompt + its hash next to a generated portrait.
// First line is the hash; the remainder is the prompt text.
const promptSidecarName = "prompt.txt"

// generatePassengerPortraits builds a prompt for each passenger and invokes the
// configured CLI (cmdLine, run via `sh -c`) for any passenger whose cached
// portrait is missing or whose prompt changed. Empty cmdLine is a no-op. The
// command receives the prompt on stdin and in $PORTRAIT_PROMPT, the target path
// in $PORTRAIT_OUT, and a deterministic $PORTRAIT_SEED; it must write an image
// to $PORTRAIT_OUT.
func generatePassengerPortraits(passengers []*Passenger, root, cmdLine string) {
	if strings.TrimSpace(cmdLine) == "" {
		log.Printf("portrait generation skipped: no command configured (set SMKB_PORTRAIT_CMD)")
		return
	}
	for _, p := range passengers {
		prompt := buildPortraitPrompt(p.Bio, p.Class, p.Citizenship)
		hash := promptHash(prompt)
		dir := passengerGeneratedDir(root, p.ID)
		if portraitExists(dir) && cachedHashMatches(dir, hash) {
			continue
		}
		out := filepath.Join(dir, generatedPortraitName)
		if err := runPortraitCmd(cmdLine, prompt, p.ID, out); err != nil {
			log.Printf("warning: portrait %s: %v", p.ID, err)
			continue
		}
		writePromptSidecar(dir, prompt, hash)
		log.Printf("generated portrait for %s", p.ID)
	}
}

// runPortraitCmd runs cmdLine via `sh -c`, exposing the prompt/out/seed and
// verifying the command produced outPath.
func runPortraitCmd(cmdLine, prompt, id, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	seed := seedHash(id) % 1_000_000_000
	// cmdLine is operator-supplied configuration (SMKB_PORTRAIT_CMD), intentionally
	// executed via the shell so a full backend pipeline can be expressed.
	c := exec.Command("sh", "-c", cmdLine) //nolint:gosec // operator-supplied command by design
	c.Env = append(os.Environ(),
		"PORTRAIT_PROMPT="+prompt,
		"PORTRAIT_OUT="+outPath,
		fmt.Sprintf("PORTRAIT_SEED=%d", seed),
	)
	c.Stdin = strings.NewReader(prompt)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("command did not produce %s: %w", outPath, err)
	}
	return nil
}

// portraitExists reports whether a generated portrait file is present in dir.
func portraitExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, generatedPortraitName))
	return err == nil
}

// cachedHashMatches reports whether dir's sidecar records the given prompt hash.
func cachedHashMatches(dir, hash string) bool {
	data, err := os.ReadFile(filepath.Join(dir, promptSidecarName))
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(first) == hash
}

// writePromptSidecar records the hash (first line) + prompt next to the image.
func writePromptSidecar(dir, prompt, hash string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("warning: sidecar dir %s: %v", dir, err)
		return
	}
	body := hash + "\n" + prompt + "\n"
	if err := os.WriteFile(filepath.Join(dir, promptSidecarName), []byte(body), 0o644); err != nil {
		log.Printf("warning: write sidecar %s: %v", dir, err)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestGeneratePortraits -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/portraits.go cmd/generate-factions-kb/portraits_test.go
git commit -m "feat(kb): pluggable CLI passenger portrait generation with prompt-hash cache"
```

---

### Task 10: `--portraits` flag wiring

**Files:**
- Modify: `cmd/generate-factions-kb/main.go`

- [ ] **Step 1: Add the flag**

Near the top of `main()` (before or just after `flag.Parse()` at `:16`), declare:
```go
	portraitsFlag := flag.Bool("portraits", false, "generate missing/updated passenger AI portraits via SMKB_PORTRAIT_CMD before rendering")
	flag.Parse()
```
(If `flag.Parse()` already exists at `:16`, move the `flag.Bool` line above it and keep a single `flag.Parse()`.)

- [ ] **Step 2: Invoke generation between attach-overlays and attach-portraits**

In the passenger block added in Task 7 Step 2, insert the generation call **between** `attachPassengerOverlays` and `attachPassengerPortraits`:
```go
	attachPassengerOverlays(passengers, overlaysRoot)
	if *portraitsFlag {
		generatePassengerPortraits(passengers, overlaysRoot, os.Getenv("SMKB_PORTRAIT_CMD"))
	}
	attachPassengerPortraits(passengers, overlaysRoot)
```
(`os` is already imported in `main.go`.)

- [ ] **Step 3: Build + verify flag exists, default run unaffected**

Run:
```bash
go build ./... && \
go run ./cmd/generate-factions-kb -h 2>&1 | grep -- -portraits && \
go run ./cmd/generate-factions-kb /home/robert/spacemolt/spacemolt-knowledge.db
```
Expected: build OK; help shows `-portraits`; a normal (no-flag) run still generates the site with silhouettes and makes no model calls.

- [ ] **Step 4: Smoke-test generation with a fake backend**

Run (writes a valid 1×1 PNG to each passenger's cache, then renders):
```bash
SMKB_PORTRAIT_CMD='printf "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89\x00\x00\x00\nIDATx\x9cc\x00\x01\x00\x00\x05\x00\x01\r\n-\xb4\x00\x00\x00\x00IEND\xaeB\x60\x82" > "$PORTRAIT_OUT"' \
go run ./cmd/generate-factions-kb -portraits /home/robert/spacemolt/spacemolt-knowledge.db && \
ls overlays/generated/passengers && \
grep -l 'infobox-image' kb/passengers/*/index.html
```
Expected: `overlays/generated/passengers/<id>/` dirs contain `portrait.png` + `prompt.txt`; passenger detail pages now use `<img class="infobox-image">` instead of the silhouette. (This is only a pipeline smoke test — the 1×1 PNG is a stand-in for a real backend.)

- [ ] **Step 5: Clean up the smoke-test artifacts (do not commit fake images)**

Run: `rm -rf overlays/generated && go run ./cmd/generate-factions-kb /home/robert/spacemolt/spacemolt-knowledge.db`
Expected: passenger pages revert to silhouettes; no `overlays/generated/` tree.

- [ ] **Step 6: Lint + full test**

Run: `golangci-lint run ./cmd/generate-factions-kb/... && go test ./cmd/generate-factions-kb/...`
Expected: no new findings, tests PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-factions-kb/main.go
git commit -m "feat(kb): add opt-in --portraits step for passenger AI portrait generation"
```

---

### Task 11: Document the overlay + portrait contract

**Files:**
- Modify: `overlays/README.md`

- [ ] **Step 1: Append documentation**

Add a section to `overlays/README.md` describing:
- `overlays/passengers/<citizen_id>/profile.md` — human-authored passenger overlays (same `profile.md` schema as players; an `image:` here always wins over a generated portrait).
- `overlays/generated/passengers/<citizen_id>/` — machine-generated cache: `portrait.png` (committed) + `prompt.txt` (hash on line 1, prompt below). **Do not hand-edit;** regenerated by `generate-factions-kb --portraits`.
- The `SMKB_PORTRAIT_CMD` contract: run via `sh -c`; receives the prompt on **stdin** and in `$PORTRAIT_PROMPT`, the output path in `$PORTRAIT_OUT`, and a deterministic `$PORTRAIT_SEED`; must write an image to `$PORTRAIT_OUT`. Example wrapper:

````markdown
```bash
export SMKB_PORTRAIT_CMD='sd --prompt "$PORTRAIT_PROMPT" --seed "$PORTRAIT_SEED" --out "$PORTRAIT_OUT" --width 320 --height 320'
go run ./cmd/generate-factions-kb --portraits ../spacemolt-knowledge.db
```
````
- Note the precedence: contributor overlay image → generated AI portrait → deterministic silhouette; and that images must be ≤320×320 (enforced by `validateImage`).

- [ ] **Step 2: Commit**

```bash
git add overlays/README.md
git commit -m "docs(kb): document passenger overlays, generated portrait cache, and SMKB_PORTRAIT_CMD"
```

---

## Final Verification

- [ ] **Full build, vet, lint, test**

Run: `go build ./... && go vet ./cmd/generate-factions-kb/... && golangci-lint run ./cmd/generate-factions-kb/... && go test ./cmd/generate-factions-kb/...`
Expected: all clean.

- [ ] **End-to-end regen on the real DB**

Run: `go run ./cmd/generate-factions-kb /home/robert/spacemolt/spacemolt-knowledge.db`
Expected log: `generated N factions, M players, 4 passengers`. Spot-check: a no-overlay player page and every passenger page show `<svg class="silhouette">`; the Passengers nav link appears site-wide; the passengers index lists all four with name/citizenship/class/sightings.

- [ ] **Commit the regenerated site**

```bash
git add kb/
git commit -m "chore(kb): regenerate site with player silhouettes + passenger pages"
```
