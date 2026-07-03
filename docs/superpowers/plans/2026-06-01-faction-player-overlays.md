# Faction & Player Overlays Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let contributors enrich faction/player KB pages with a logo/portrait, a biography/freeform markdown body, and structured stats via PR-edited files in `overlays/`, which the `generate-factions-kb` generator merges into the pages; plus a seed tool that scaffolds overlay stubs from `personality.json`.

**Architecture:** A new `overlay.go` in `cmd/generate-factions-kb` parses `overlays/<kind>/<hash-id>/profile.md` (YAML frontmatter + markdown body, rendered with goldmark in safe mode), attaches an `*Overlay` to the matched `Faction`/`Player`, and the generator copies the overlay image into each entity's output dir and renders extra sections. A separate `cmd/seed-overlays` binary writes stub overlays from `personality.json`.

**Tech Stack:** Go 1.25, `html/template`, `github.com/yuin/goldmark` (safe markdown), `gopkg.in/yaml.v3` (frontmatter). Module `github.com/rsned/spacemolt-kb`. All commands run from `/home/robert/spacemolt/kb`.

---

## Background the engineer needs

- The generator `cmd/generate-factions-kb` builds `kb/factions/<slug>/index.html` and `kb/players/<slug>/index.html` from `spacemolt-knowledge.db`. Output dirs are wiped and rebuilt each run by `mustResetDir` (in `main.go`), so overlay **source** lives outside the output tree, at repo-root `overlays/`.
- Overlay dirs are keyed by the stable hash ID (`faction_id` / `player_id`) — these are printed on each page (`fb-id` / `pb-id`), so contributors copy them from their page.
- Entity structs are in `cmd/generate-factions-kb/types.go`: `Faction` (fields incl. `ID`, `Slug`, `Members`…) with method `MemberCount()`, and `Player` (fields incl. `ID`, `Slug`, `Username`…).
- Templates are Go string constants in `cmd/generate-factions-kb/render.go`: `factionDetailTmpl` and `playerDetailTmpl`. The faction banner ends with the Charter block; then a `dl.faction-stats`, an `api-note`, then `<h3>Members…`. The player detail has `div.player-banner` then `div.stat-strip` (first/last seen).
- goldmark's default config (no `html.WithUnsafe()`) replaces raw HTML with `<!-- raw HTML omitted -->`, so embedded `<script>` is inert. That is the safety property we rely on.
- Tests follow the existing `cmd/generate-factions-kb/*_test.go` convention (table tests, `t.TempDir()` for fixtures).

## File Structure

```
cmd/generate-factions-kb/
  overlay.go        # Overlay/OverlayStat types, splitFrontmatter, renderMarkdown,
                    # validateImage, loadOverlay, copyOverlayImage, attach/orphan helpers
  overlay_test.go   # unit tests for the above
  types.go          # + Overlay *Overlay field on Faction and Player
  main.go           # load+attach overlays, copy images, warn on orphans
  render.go         # image + Profile + About sections in faction/player detail templates
kb/factions/factions.css   # + .overlay-logo, .overlay-body, .overlay-credit, .overlay-stats
kb/players/players.css      # + .overlay-portrait, .overlay-body, .overlay-credit, .overlay-stats
overlays/
  README.md                 # contributor guide
  factions/<HEXC-id>/profile.md   # one committed example (stats + body, no image)
cmd/seed-overlays/
  main.go           # personality.json -> stub profile.md (skip-if-exists, --dry-run)
  seed.go           # pure match/normalize/stub-render helpers
  seed_test.go
```

---

## Task 1: Dependencies, Overlay types, and frontmatter split (TDD)

**Files:**
- Create: `cmd/generate-factions-kb/overlay.go`
- Test: `cmd/generate-factions-kb/overlay_test.go`

- [ ] **Step 1: Add dependencies**

Run:
```bash
cd /home/robert/spacemolt/kb
go get github.com/yuin/goldmark@latest
go get gopkg.in/yaml.v3@latest
go mod tidy
```
Expected: both modules appear in `go.mod` require block.

- [ ] **Step 2: Write the failing test for splitFrontmatter**

Create `cmd/generate-factions-kb/overlay_test.go`:

```go
package main

import "testing"

func TestSplitFrontmatter(t *testing.T) {
	withFM := "---\nimage: logo.png\n---\n## Body\n\nHello\n"
	front, body := splitFrontmatter([]byte(withFM))
	if string(front) != "image: logo.png" {
		t.Errorf("front = %q", front)
	}
	if body != "## Body\n\nHello\n" {
		t.Errorf("body = %q", body)
	}

	// No frontmatter: everything is body.
	front, body = splitFrontmatter([]byte("just a body\n"))
	if front != nil {
		t.Errorf("expected nil front, got %q", front)
	}
	if body != "just a body\n" {
		t.Errorf("body = %q", body)
	}

	// Unterminated frontmatter: treat whole thing as body, no front.
	front, body = splitFrontmatter([]byte("---\nimage: x\nno closing fence\n"))
	if front != nil {
		t.Errorf("expected nil front for unterminated, got %q", front)
	}

	// CRLF normalized.
	front, _ = splitFrontmatter([]byte("---\r\nimage: a.png\r\n---\r\nbody"))
	if string(front) != "image: a.png" {
		t.Errorf("CRLF front = %q", front)
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestSplitFrontmatter -v`
Expected: FAIL — `undefined: splitFrontmatter`.

- [ ] **Step 4: Implement types + splitFrontmatter**

Create `cmd/generate-factions-kb/overlay.go`:

```go
package main

import (
	htmltpl "html/template"
	"strings"
)

// Overlay is contributor-authored content merged onto a faction or player page.
type Overlay struct {
	ImageFile string        // validated bare filename, or "" when absent/invalid
	ImageAlt  string        // alt text for the image
	Stats     []OverlayStat // ordered structured stats
	BodyHTML  htmltpl.HTML  // goldmark-rendered markdown body (safe), or ""
}

// OverlayStat is one label/value row of an overlay's structured stats.
type OverlayStat struct {
	Label string `yaml:"label"`
	Value string `yaml:"value"`
}

// splitFrontmatter separates a leading "---\n...\n---\n" YAML block from the
// markdown body. Returns nil front when there is no well-formed frontmatter; in
// that case the entire (newline-normalized) input is the body.
func splitFrontmatter(content []byte) (front []byte, body string) {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, s
	}
	rest := s[len("---\n"):]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		// Allow a closing fence at EOF with no trailing newline.
		if strings.HasSuffix(rest, "\n---") {
			return []byte(rest[:len(rest)-len("\n---")]), ""
		}
		return nil, s
	}
	return []byte(rest[:idx]), rest[idx+len("\n---\n"):]
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestSplitFrontmatter -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-factions-kb/overlay.go cmd/generate-factions-kb/overlay_test.go go.mod go.sum
git commit -m "feat(overlays): add Overlay types and frontmatter splitter"
```

---

## Task 2: Safe markdown rendering (TDD)

**Files:**
- Modify: `cmd/generate-factions-kb/overlay.go`
- Test: `cmd/generate-factions-kb/overlay_test.go`

- [ ] **Step 1: Write the failing test**

Append the test below to `cmd/generate-factions-kb/overlay_test.go`. First ensure the file's single import block includes `"strings"` and `"testing"` (add `"strings"` if missing):

```go
func TestRenderMarkdown(t *testing.T) {
	html, err := renderMarkdown("This is **bold** and a [link](http://example.com).")
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, "<strong>bold</strong>") {
		t.Errorf("bold not rendered: %q", s)
	}
	if !strings.Contains(s, `rel="nofollow noopener"`) {
		t.Errorf("link not hardened with nofollow: %q", s)
	}

	// Raw HTML / scripts must be neutralized (goldmark safe mode).
	bad, err := renderMarkdown("ok <script>alert(1)</script> done")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bad), "<script>") {
		t.Errorf("script tag leaked through: %q", bad)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestRenderMarkdown -v`
Expected: FAIL — `undefined: renderMarkdown`.

- [ ] **Step 3: Implement renderMarkdown**

Add to `cmd/generate-factions-kb/overlay.go` (add `"bytes"` and `"github.com/yuin/goldmark"` to the import block):

```go
// goldmark with its default (safe) options: raw HTML is replaced with an
// "omitted" comment rather than passed through, so contributor markdown cannot
// inject scripts.
var markdown = goldmark.New()

// renderMarkdown converts a markdown body to safe HTML and hardens links with
// rel="nofollow noopener".
func renderMarkdown(src string) (htmltpl.HTML, error) {
	var buf bytes.Buffer
	if err := markdown.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	out := strings.ReplaceAll(buf.String(), "<a href=", `<a rel="nofollow noopener" href=`)
	return htmltpl.HTML(out), nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestRenderMarkdown -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/overlay.go cmd/generate-factions-kb/overlay_test.go
git commit -m "feat(overlays): safe goldmark markdown rendering"
```

---

## Task 3: Image validation (TDD)

**Files:**
- Modify: `cmd/generate-factions-kb/overlay.go`
- Test: `cmd/generate-factions-kb/overlay_test.go`

- [ ] **Step 1: Write the failing test**

Append the test below to `cmd/generate-factions-kb/overlay_test.go`. First ensure the file's single import block includes `"os"` and `"path/filepath"` (add them if missing):

```go
func TestValidateImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty name is fine (no image), returns "".
	if got, err := validateImage(dir, ""); err != nil || got != "" {
		t.Errorf("empty: got %q err %v", got, err)
	}
	// Valid file.
	if got, err := validateImage(dir, "logo.png"); err != nil || got != "logo.png" {
		t.Errorf("valid: got %q err %v", got, err)
	}
	// Bad extension.
	if _, err := validateImage(dir, "logo.svg"); err == nil {
		t.Error("expected error for .svg")
	}
	// Path traversal.
	if _, err := validateImage(dir, "../secret.png"); err == nil {
		t.Error("expected error for traversal")
	}
	if _, err := validateImage(dir, "sub/logo.png"); err == nil {
		t.Error("expected error for separator")
	}
	// Missing file.
	if _, err := validateImage(dir, "nope.png"); err == nil {
		t.Error("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestValidateImage -v`
Expected: FAIL — `undefined: validateImage`.

- [ ] **Step 3: Implement validateImage**

Add to `cmd/generate-factions-kb/overlay.go` (add `"fmt"`, `"os"`, `"path/filepath"` to the import block):

```go
var allowedImageExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true,
}

// validateImage checks that name is a bare filename (no separators, no ".."),
// has an allowed raster extension, and exists in dir. Returns the validated
// name, or an error describing why it was rejected. Empty name -> ("", nil).
func validateImage(dir, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("image %q must be a bare filename with no path", name)
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedImageExt[ext] {
		return "", fmt.Errorf("image %q has unsupported extension %q", name, ext)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		return "", fmt.Errorf("image %q not found in overlay dir: %w", name, err)
	}
	return name, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestValidateImage -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/generate-factions-kb/overlay.go cmd/generate-factions-kb/overlay_test.go
git commit -m "feat(overlays): validate overlay image filenames"
```

---

## Task 4: loadOverlay end-to-end (TDD)

**Files:**
- Modify: `cmd/generate-factions-kb/overlay.go`
- Test: `cmd/generate-factions-kb/overlay_test.go`

- [ ] **Step 1: Write the failing test**

Append to `cmd/generate-factions-kb/overlay_test.go`:

```go
func writeOverlay(t *testing.T, dir, profile string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "profile.md"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadOverlay(t *testing.T) {
	// Missing profile.md -> (nil, nil).
	if ov, err := loadOverlay(t.TempDir()); err != nil || ov != nil {
		t.Errorf("missing: ov=%v err=%v", ov, err)
	}

	// Full overlay: frontmatter (image + ordered stats) + body.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeOverlay(t, dir, "---\nimage: logo.png\nimage_alt: Crest\nstats:\n  - label: Homeworld\n    value: Krynn\n  - label: Founded\n    value: 2387\n---\n## Bio\n\nWe **optimize**.\n")
	ov, err := loadOverlay(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ov.ImageFile != "logo.png" || ov.ImageAlt != "Crest" {
		t.Errorf("image fields: %+v", ov)
	}
	if len(ov.Stats) != 2 || ov.Stats[0].Label != "Homeworld" || ov.Stats[1].Value != "2387" {
		t.Errorf("stats: %+v", ov.Stats)
	}
	if !strings.Contains(string(ov.BodyHTML), "<strong>optimize</strong>") {
		t.Errorf("body: %q", ov.BodyHTML)
	}

	// Bad image extension: overlay still returned, image skipped (warn-not-fail).
	dir2 := t.TempDir()
	writeOverlay(t, dir2, "---\nimage: logo.svg\n---\nbody")
	ov2, err := loadOverlay(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if ov2.ImageFile != "" {
		t.Errorf("expected image skipped, got %q", ov2.ImageFile)
	}

	// Body-only (no frontmatter).
	dir3 := t.TempDir()
	writeOverlay(t, dir3, "Just a bio.\n")
	ov3, err := loadOverlay(dir3)
	if err != nil {
		t.Fatal(err)
	}
	if ov3.BodyHTML == "" || len(ov3.Stats) != 0 || ov3.ImageFile != "" {
		t.Errorf("body-only: %+v", ov3)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run TestLoadOverlay -v`
Expected: FAIL — `undefined: loadOverlay`.

- [ ] **Step 3: Implement loadOverlay**

Add to `cmd/generate-factions-kb/overlay.go` (add `"errors"`, `"log"`, and `"gopkg.in/yaml.v3"` to the import block):

```go
// profileFront is the YAML frontmatter shape of a profile.md.
type profileFront struct {
	Image    string        `yaml:"image"`
	ImageAlt string        `yaml:"image_alt"`
	Stats    []OverlayStat `yaml:"stats"`
}

// loadOverlay reads <dir>/profile.md and returns the parsed Overlay. Returns
// (nil, nil) when no profile.md exists. A bad/missing image is a warning (the
// image is dropped, the rest still renders); malformed YAML or markdown is an
// error.
func loadOverlay(dir string) (*Overlay, error) {
	content, err := os.ReadFile(filepath.Join(dir, "profile.md"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	front, body := splitFrontmatter(content)
	var fm profileFront
	if len(front) > 0 {
		if err := yaml.Unmarshal(front, &fm); err != nil {
			return nil, fmt.Errorf("frontmatter: %w", err)
		}
	}
	ov := &Overlay{ImageAlt: fm.ImageAlt, Stats: fm.Stats}
	if img, err := validateImage(dir, fm.Image); err != nil {
		log.Printf("warning: overlay %s: %v (image skipped)", dir, err)
	} else {
		ov.ImageFile = img
	}
	if strings.TrimSpace(body) != "" {
		html, err := renderMarkdown(body)
		if err != nil {
			return nil, fmt.Errorf("markdown: %w", err)
		}
		ov.BodyHTML = html
	}
	return ov, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./cmd/generate-factions-kb/ -run TestLoadOverlay -v`
Expected: PASS.

- [ ] **Step 5: Verify the whole package still type-checks**

Run: `go vet ./cmd/generate-factions-kb/` and `go test ./cmd/generate-factions-kb/`
Expected: clean; all tests pass. Run the golangci-lint tool on the package — note `loadOverlay`/`copyOverlayImage`/attach helpers may show as `unused` until Task 5 wires them in; that is expected. Confirm no OTHER findings.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate-factions-kb/overlay.go cmd/generate-factions-kb/overlay_test.go
git commit -m "feat(overlays): parse profile.md into an Overlay"
```

---

## Task 5: Wire overlays into the generator (attach, orphan-warn, copy image)

**Files:**
- Modify: `cmd/generate-factions-kb/types.go`
- Modify: `cmd/generate-factions-kb/overlay.go`
- Modify: `cmd/generate-factions-kb/main.go`
- Test: `cmd/generate-factions-kb/overlay_test.go`

- [ ] **Step 1: Add the Overlay field to both structs**

In `cmd/generate-factions-kb/types.go`, in the `Faction` struct, after the `Facilities []Facility` line and before the closing `}`:

```go
	Facilities []Facility

	Overlay *Overlay // contributor-authored content, nil when none
}
```

In the `Player` struct, after `Sightings []Sighting`:

```go
	Ships     []ShipSeen
	Sightings []Sighting

	Overlay *Overlay // contributor-authored content, nil when none
}
```

- [ ] **Step 2: Write the failing test for copyOverlayImage and attach/orphan helpers**

Append to `cmd/generate-factions-kb/overlay_test.go`:

```go
func TestCopyOverlayImage(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "logo.png"), []byte("imgbytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	copyOverlayImage(src, "logo.png", dst)
	got, err := os.ReadFile(filepath.Join(dst, "logo.png"))
	if err != nil || string(got) != "imgbytes" {
		t.Errorf("copy: got %q err %v", got, err)
	}
	// Empty name is a no-op (no panic, nothing written).
	copyOverlayImage(src, "", dst)
}

func TestAttachFactionOverlays(t *testing.T) {
	root := t.TempDir()
	// Overlay for faction "abc"; an orphan dir "ghost" with no matching faction.
	mk := func(id, profile string) {
		d := filepath.Join(root, "factions", id)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "profile.md"), []byte(profile), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mk("abc", "body for abc")
	mk("ghost", "orphan body")

	factions := []*Faction{{ID: "abc"}, {ID: "xyz"}}
	attachFactionOverlays(factions, root)
	if factions[0].Overlay == nil {
		t.Error("faction abc should have an overlay")
	}
	if factions[1].Overlay != nil {
		t.Error("faction xyz should have no overlay")
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./cmd/generate-factions-kb/ -run 'TestCopyOverlayImage|TestAttachFactionOverlays' -v`
Expected: FAIL — `undefined: copyOverlayImage` / `undefined: attachFactionOverlays`.

- [ ] **Step 4: Implement the copy + attach + orphan helpers**

Add to `cmd/generate-factions-kb/overlay.go`:

```go
// overlaysRoot is the repo-root source directory holding contributor overlays.
const overlaysRoot = "overlays"

// copyOverlayImage copies name from srcDir to destDir. Empty name is a no-op.
// Failures are warnings (the page still renders without the image).
func copyOverlayImage(srcDir, name, destDir string) {
	if name == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(srcDir, name))
	if err != nil {
		log.Printf("warning: read overlay image %s: %v", name, err)
		return
	}
	if err := os.WriteFile(filepath.Join(destDir, name), data, 0o644); err != nil {
		log.Printf("warning: write overlay image %s: %v", name, err)
	}
}

// attachFactionOverlays loads overlays/factions/<id>/ for each faction and warns
// about overlay dirs that match no current faction.
func attachFactionOverlays(factions []*Faction, root string) {
	matched := map[string]bool{}
	for _, f := range factions {
		ov, err := loadOverlay(filepath.Join(root, "factions", f.ID))
		if err != nil {
			log.Printf("warning: faction overlay %s: %v", f.ID, err)
			continue
		}
		if ov != nil {
			f.Overlay = ov
			matched[f.ID] = true
		}
	}
	warnOrphanOverlays(filepath.Join(root, "factions"), matched)
}

// attachPlayerOverlays loads overlays/players/<id>/ for each player and warns
// about overlay dirs that match no current player.
func attachPlayerOverlays(players []*Player, root string) {
	matched := map[string]bool{}
	for _, p := range players {
		ov, err := loadOverlay(filepath.Join(root, "players", p.ID))
		if err != nil {
			log.Printf("warning: player overlay %s: %v", p.ID, err)
			continue
		}
		if ov != nil {
			p.Overlay = ov
			matched[p.ID] = true
		}
	}
	warnOrphanOverlays(filepath.Join(root, "players"), matched)
}

// warnOrphanOverlays logs any subdirectory of dir whose name was not matched.
func warnOrphanOverlays(dir string, matched map[string]bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // no overlays of this kind yet
	}
	for _, e := range entries {
		if e.IsDir() && !matched[e.Name()] {
			log.Printf("warning: overlay %s matches no current entity", filepath.Join(dir, e.Name()))
		}
	}
}
```

- [ ] **Step 5: Wire into main.go**

In `cmd/generate-factions-kb/main.go`, after the `players, err := loadPlayers(...)` block (after its error check) and before `funcs := templateFuncs(genTime)`, add:

```go
	attachFactionOverlays(factions, overlaysRoot)
	attachPlayerOverlays(players, overlaysRoot)
```

Then update the two write loops to copy the overlay image into each output dir. Replace the faction write loop:

```go
	for _, f := range factions {
		dir := filepath.Join(factionsOut, f.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), fDet, f)
	}
```
with:
```go
	for _, f := range factions {
		dir := filepath.Join(factionsOut, f.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), fDet, f)
		if f.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "factions", f.ID), f.Overlay.ImageFile, dir)
		}
	}
```

Replace the player write loop:

```go
	for _, p := range players {
		dir := filepath.Join(playersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), pDet, p)
	}
```
with:
```go
	for _, p := range players {
		dir := filepath.Join(playersOut, p.Slug)
		mustMkdir(dir)
		mustWrite(filepath.Join(dir, "index.html"), pDet, p)
		if p.Overlay != nil {
			copyOverlayImage(filepath.Join(overlaysRoot, "players", p.ID), p.Overlay.ImageFile, dir)
		}
	}
```

- [ ] **Step 6: Run tests + build + run generator**

Run:
```bash
go test ./cmd/generate-factions-kb/ -v
go build ./cmd/generate-factions-kb/
go run ./cmd/generate-factions-kb ../spacemolt-knowledge.db
```
Expected: tests pass; generator runs and logs `generated N factions and M players` (no overlays exist yet, so no overlay warnings). golangci-lint on the package: 0 issues (helpers are now used).

- [ ] **Step 7: Commit**

```bash
git add cmd/generate-factions-kb/types.go cmd/generate-factions-kb/overlay.go cmd/generate-factions-kb/main.go cmd/generate-factions-kb/overlay_test.go
git commit -m "feat(overlays): attach overlays to entities and copy images on generate"
```

---

## Task 6: Render overlay sections + CSS + example overlay + README

**Files:**
- Modify: `cmd/generate-factions-kb/render.go`
- Modify: `kb/factions/factions.css`
- Modify: `kb/players/players.css`
- Create: `overlays/README.md`
- Create: `overlays/factions/<HEXC-faction_id>/profile.md`

- [ ] **Step 1: Add overlay sections to the faction detail template**

In `cmd/generate-factions-kb/render.go`, in `factionDetailTmpl`, insert the logo `<img>` as the FIRST child of the banner (immediately after the opening `<div class="faction-banner" ...>` line, before the `<h2>`):

```html
            {{if and .Overlay .Overlay.ImageFile}}<img class="overlay-logo" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">{{end}}
```

Then, immediately AFTER the `<p class="api-note">...</p>` line and BEFORE the `<h3>Members ({{.MemberCount}})</h3>` line, insert the Profile and About sections:

```html
{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>About</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
```

- [ ] **Step 2: Add overlay sections to the player detail template**

In `playerDetailTmpl`, insert the portrait as the FIRST child of `<div class="player-banner" ...>` (before the `<h2>`):

```html
            {{if and .Overlay .Overlay.ImageFile}}<img class="overlay-portrait" src="{{.Overlay.ImageFile}}" alt="{{.Overlay.ImageAlt}}">{{end}}
```

Then, immediately AFTER the closing `</div>` of the player banner and BEFORE the `<div class="stat-strip">` line, insert:

```html
{{if and .Overlay .Overlay.Stats}}
        <h3>Profile</h3>
        <dl class="faction-stats overlay-stats">
{{- range .Overlay.Stats}}
            <dt>{{.Label}}</dt><dd>{{.Value}}</dd>
{{- end}}
        </dl>
{{end}}
{{if and .Overlay .Overlay.BodyHTML}}
        <h3>About</h3>
        <p class="overlay-credit text-muted">Community-contributed profile.</p>
        <div class="overlay-body">{{.Overlay.BodyHTML}}</div>
{{end}}
```

(The player template loads only `players.css`; Step 4 adds `.faction-stats`/`.overlay-stats` there. Note `.faction-stats` is already defined in `players.css` from earlier work — verify with `grep faction-stats kb/players/players.css`; if absent, copy the rule from `factions.css`.)

- [ ] **Step 3: Add CSS to factions.css**

Append to `kb/factions/factions.css`:

```css
/* Contributor overlay content. */
.overlay-logo { float: right; max-width: 120px; max-height: 120px; margin: 0 0 .5rem 1rem; border-radius: 8px; }
.overlay-stats { margin-top: .35rem; }
.overlay-credit { font-size: 12px; font-style: italic; margin: 0 0 .4rem; }
.overlay-body { max-width: 70ch; line-height: 1.65; }
.overlay-body h1, .overlay-body h2, .overlay-body h3, .overlay-body h4 { font-size: 1.15rem; margin: 1rem 0 .35rem; }
.overlay-body p { margin: .5rem 0; }
.overlay-body ul, .overlay-body ol { margin: .5rem 0 .5rem 1.25rem; }
```

- [ ] **Step 4: Add CSS to players.css**

Append to `kb/players/players.css`:

```css
/* Contributor overlay content. */
.overlay-portrait { float: right; max-width: 120px; max-height: 120px; margin: 0 0 .5rem 1rem; border-radius: 8px; }
.overlay-stats { margin-top: .35rem; }
.overlay-credit { font-size: 12px; font-style: italic; margin: 0 0 .4rem; }
.overlay-body { max-width: 70ch; line-height: 1.65; }
.overlay-body h1, .overlay-body h2, .overlay-body h3, .overlay-body h4 { font-size: 1.15rem; margin: 1rem 0 .35rem; }
.overlay-body p { margin: .5rem 0; }
.overlay-body ul, .overlay-body ol { margin: .5rem 0 .5rem 1.25rem; }
```

- [ ] **Step 5: Create the contributor README**

Create `overlays/README.md`:

```markdown
# Faction & Player Overlays

Add a logo/portrait, biography, and extra stats to your faction or player page —
content the game does not store. Edit these files and open a PR; on the next KB
regeneration your content appears on your page.

## How to add yours

1. Find your **ID**: open your faction or player page in the KB. The monospace
   hash under the name (e.g. `e3653eac2392899ee0ee1f93a945306d`) is your ID.
2. Create a directory named by that ID:
   - Faction: `overlays/factions/<faction_id>/`
   - Player: `overlays/players/<player_id>/`
3. Add a `profile.md` (see format below). Optionally drop an image (`logo.png` /
   `portrait.jpg`) in the same directory.
4. Open a PR.

## profile.md format

```
---
image: logo.png                # optional; a file in this same directory
image_alt: "Faction crest"     # optional alt text
stats:                         # optional; shown as a Profile table
  - label: Homeworld
    value: Krynn Prime
  - label: Founded (lore)
    value: 2387 AE
---

## Biography

Markdown here — paragraphs, **bold**, lists, [links](https://example.com).
This renders as the "About" section on your page.
```

All parts are optional. Images must be `.png`, `.jpg`, `.jpeg`, `.webp`, or
`.gif` (no SVG), named as a plain filename in the same directory. Raw HTML in the
body is ignored for safety.
```

- [ ] **Step 6: Create one example faction overlay using a real faction_id**

Get a real, current faction_id (HEXC) and create the example:

```bash
HID=$(sqlite3 ../spacemolt-knowledge.db "SELECT faction_id FROM factions WHERE tag='HEXC'")
echo "HEXC id: $HID"
mkdir -p "overlays/factions/$HID"
cat > "overlays/factions/$HID/profile.md" <<'EOF'
---
stats:
  - label: Homeworld
    value: Hex Prime
  - label: Specialty
    value: Distributed computation
  - label: Founded (lore)
    value: 2381 AE
---

## About the Hex Collective

The **Hex Collective** is a guild of builders and optimizers who treat the galaxy
as one large system to be solved. This profile is community-maintained — edit
`overlays/factions/<id>/profile.md` to update it.

### Doctrine

- Transparency in trade
- Knowledge is a shared resource
- Optimize relentlessly
EOF
```

(If the HEXC faction is ever removed from the DB, the generator will log this as an orphan overlay; pick another current faction_id instead.)

- [ ] **Step 7: Build, regenerate, verify rendering**

Run:
```bash
go build ./cmd/generate-factions-kb/
go run ./cmd/generate-factions-kb ../spacemolt-knowledge.db
```
Then verify (HEXC slug is `hexc`):
```bash
grep -c 'overlay-stats\|>About<\|Hex Collective' kb/factions/hexc/index.html   # > 0
grep -c 'overlay-body' kb/factions/hexc/index.html                              # > 0
```
Expected: the Profile table and About section appear on the HEXC page. Open `kb/factions/hexc/index.html` in a browser and confirm the About markdown (heading, bold, list) renders and is placed after the stat block, before Members. A faction WITHOUT an overlay (e.g. `kb/factions/strg/index.html`) shows no Profile/About sections.

- [ ] **Step 8: Lint and commit**

Run the golangci-lint tool on `./cmd/generate-factions-kb/` (expect 0 issues), then:
```bash
git add cmd/generate-factions-kb/render.go kb/factions/factions.css kb/players/players.css overlays/ kb/factions kb/players
git commit -m "feat(overlays): render logo, Profile, and About sections; add README + example"
```

---

## Task 7: Seed tool — cmd/seed-overlays

**Files:**
- Create: `cmd/seed-overlays/seed.go`
- Create: `cmd/seed-overlays/seed_test.go`
- Create: `cmd/seed-overlays/main.go`

- [ ] **Step 1: Write the failing test for the pure helpers**

Create `cmd/seed-overlays/seed_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeName(t *testing.T) {
	if normalizeName("  Foundling Mira ") != "foundling mira" {
		t.Error("name normalize")
	}
}

func TestNormalizeOrg(t *testing.T) {
	if normalizeOrg("The Hex Collective") != "hex collective" {
		t.Error("strip leading 'the'")
	}
	if normalizeOrg("Hex Collective") != "hex collective" {
		t.Error("no-the case")
	}
}

func TestRenderStub(t *testing.T) {
	s := renderStub("A bio about **someone**.", []stubStat{{"Organization", "Hex"}, {"Role", "Acolyte"}})
	if !strings.Contains(s, "label: Organization") || !strings.Contains(s, "value: Hex") {
		t.Errorf("stats missing: %s", s)
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "A bio about **someone**.") {
		t.Errorf("body missing/last: %s", s)
	}
}

func TestWriteStubSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.md")
	if err := os.WriteFile(path, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not dry-run, but file exists -> must skip (no overwrite).
	wrote, err := writeStub(path, "NEW CONTENT", false)
	if err != nil || wrote {
		t.Errorf("should skip existing: wrote=%v err=%v", wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "ORIGINAL" {
		t.Error("existing file was overwritten")
	}

	// Dry-run on a new path -> reports it would write, but writes nothing.
	newPath := filepath.Join(dir, "sub", "profile.md")
	wrote, err = writeStub(newPath, "X", true)
	if err != nil || !wrote {
		t.Errorf("dry-run new: wrote=%v err=%v", wrote, err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("dry-run must not create files")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/seed-overlays/ -v`
Expected: FAIL — undefined helpers / no package.

- [ ] **Step 3: Implement the pure helpers**

Create `cmd/seed-overlays/seed.go`:

```go
// Command seed-overlays scaffolds overlay stubs from agent personality.json data.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type stubStat struct{ Label, Value string }

// normalizeName lowercases and trims a display name for matching.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeOrg normalizes an organization/faction name for matching by
// lowercasing, trimming, and dropping a leading "the ".
func normalizeOrg(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.TrimPrefix(s, "the ")
}

// renderStub builds a profile.md body from a biography and structured stats.
func renderStub(biography string, stats []stubStat) string {
	var b strings.Builder
	b.WriteString("---\n")
	if len(stats) > 0 {
		b.WriteString("stats:\n")
		for _, s := range stats {
			fmt.Fprintf(&b, "  - label: %s\n    value: %s\n", s.Label, s.Value)
		}
	}
	b.WriteString("---\n\n")
	b.WriteString("## Biography\n\n")
	b.WriteString(strings.TrimSpace(biography))
	b.WriteString("\n")
	return b.String()
}

// writeStub writes content to path unless the file already exists. Returns
// whether it (would) write. In dryRun mode it never touches the filesystem.
func writeStub(path, content string, dryRun bool) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil // exists -> never overwrite
	}
	if dryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
```

- [ ] **Step 4: Run to verify the helpers pass**

Run: `go test ./cmd/seed-overlays/ -v`
Expected: PASS.

- [ ] **Step 5: Implement main.go (the orchestration)**

Create `cmd/seed-overlays/main.go`:

```go
package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// personality is the subset of personality.json we read.
type personality struct {
	Name         string `json:"name"`
	Biography    string `json:"biography"`
	Organization string `json:"organization"`
	Role         string `json:"role"`
	SubRole      string `json:"sub_role"`
}

func main() {
	agentsDir := flag.String("agents", "../spacemolt/data/agents", "directory of agent personality dirs")
	dbPath := flag.String("db", "../spacemolt-knowledge.db", "knowledge database path")
	outRoot := flag.String("overlays", "overlays", "overlays output root")
	dryRun := flag.Bool("dry-run", false, "report what would be written without writing")
	flag.Parse()

	db, err := sql.Open("sqlite", *dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// username(normalized) -> player_id
	players := map[string]string{}
	rows, err := db.Query(`SELECT player_id, username FROM seen_players WHERE username NOT LIKE '[%' AND player_id NOT LIKE 'npc%'`)
	if err != nil {
		log.Fatalf("query players: %v", err)
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatalf("scan player: %v", err)
		}
		players[normalizeName(name)] = id
	}
	_ = rows.Close()

	// org(normalized) -> faction_id
	factions := map[string]string{}
	frows, err := db.Query(`SELECT faction_id, name FROM factions`)
	if err != nil {
		log.Fatalf("query factions: %v", err)
	}
	for frows.Next() {
		var id, name string
		if err := frows.Scan(&id, &name); err != nil {
			log.Fatalf("scan faction: %v", err)
		}
		factions[normalizeOrg(name)] = id
	}
	_ = frows.Close()

	entries, err := os.ReadDir(*agentsDir)
	if err != nil {
		log.Fatalf("read agents dir: %v", err)
	}

	playerStubs, factionStubs := 0, 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, ok := readPersonality(filepath.Join(*agentsDir, e.Name(), "personality.json"))
		if !ok {
			continue
		}
		stats := []stubStat{}
		if p.Organization != "" {
			stats = append(stats, stubStat{"Organization", p.Organization})
		}
		if p.Role != "" {
			stats = append(stats, stubStat{"Role", p.Role})
		}
		if p.SubRole != "" {
			stats = append(stats, stubStat{"Sub-role", p.SubRole})
		}
		content := renderStub(p.Biography, stats)

		if pid, ok := players[normalizeName(p.Name)]; ok {
			path := filepath.Join(*outRoot, "players", pid, "profile.md")
			if wrote, err := writeStub(path, content, *dryRun); err != nil {
				log.Printf("warning: %s: %v", path, err)
			} else if wrote {
				playerStubs++
				log.Printf("player stub: %s (%s)", p.Name, pid)
			}
		}
		if fid, ok := factions[normalizeOrg(p.Organization)]; ok && p.Organization != "" {
			path := filepath.Join(*outRoot, "factions", fid, "profile.md")
			if wrote, err := writeStub(path, content, *dryRun); err != nil {
				log.Printf("warning: %s: %v", path, err)
			} else if wrote {
				factionStubs++
				log.Printf("faction stub: %s (%s)", p.Organization, fid)
			}
		}
	}

	verb := "wrote"
	if *dryRun {
		verb = "would write"
	}
	log.Printf("%s %d player and %d faction overlay stubs", verb, playerStubs, factionStubs)
}

func readPersonality(path string) (personality, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return personality{}, false
	}
	var p personality
	if err := json.Unmarshal(data, &p); err != nil {
		return personality{}, false
	}
	if strings.TrimSpace(p.Name) == "" {
		return personality{}, false
	}
	return p, true
}
```

- [ ] **Step 6: Build and dry-run the seed tool**

Run:
```bash
go build ./cmd/seed-overlays/
go run ./cmd/seed-overlays -dry-run
```
Expected: logs `would write N player and M faction overlay stubs` (N is small — ~5 player matches today). Confirm NO files were created: `git status --porcelain overlays/ | grep profile.md` returns nothing.

- [ ] **Step 7: Lint, full test, commit**

Run:
```bash
go test ./cmd/seed-overlays/ ./cmd/generate-factions-kb/
```
Run the golangci-lint tool on `./cmd/seed-overlays/` — expect 0 issues. Then:
```bash
git add cmd/seed-overlays/
git commit -m "feat(overlays): seed-overlays tool scaffolds stubs from personality.json"
```

(Do NOT run the seed tool for real in this task — generating committed stubs is a separate, reviewed step the maintainer runs deliberately.)

---

## Self-Review Notes (for the implementer)

- **Spec coverage:** overlay source layout + hash-ID keying (Tasks 5–6, README); profile.md schema with optional image/stats/body (Tasks 1,3,4 + README); goldmark safe rendering + nofollow (Task 2); image allowlist + no traversal (Task 3); attach + orphan warnings + image copy (Task 5); banner image + Profile + About rendering with community-contributed caption (Task 6); CSS (Task 6); committed example + README (Task 6); seed tool with skip-if-exists + dry-run (Task 7); tests across all units.
- **Type consistency:** `Overlay{ImageFile, ImageAlt, Stats []OverlayStat, BodyHTML}` used identically in `overlay.go`, the struct field `Overlay *Overlay`, and templates (`.Overlay.ImageFile` etc.). `loadOverlay`/`attachFactionOverlays`/`attachPlayerOverlays`/`copyOverlayImage`/`overlaysRoot` names match between `overlay.go` and `main.go`. Seed helpers `normalizeName`/`normalizeOrg`/`renderStub`/`writeStub`/`stubStat` match between `seed.go`, `seed_test.go`, and `main.go`.
- **Determinism:** overlays are read fresh each run; no timestamps involved.

## Out of Scope (follow-ups)

- Links section; SVG support; live personality.json import; logos on the faction landing cards; overlay content on index pages.
