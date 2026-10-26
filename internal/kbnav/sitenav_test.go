package kbnav

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// headerMarker identifies a page that renders the shared site header. Pages
// without it are hand-maintained (kb/build-costs/**, kb/market, kb/galaxy-map,
// kb/ships/blueprints) and are deliberately out of scope.
const headerMarker = `<header class="site-header">`

// exemptSections are page trees whose header is NOT produced by this package.
// Each is a real gap, not a permanent carve-out — the nav is shared by
// generate-items-kb and generate-factions-kb, but these producers still
// carry their own header copy, which is why the site currently shows three
// different nav vintages. Removing an entry here means migrating that
// generator to Header and re-rendering its pages.
//
//	diffs/         pkg/gamediff/report.go — an older nav still. These are
//	               historical snapshot reports; re-rendering them means a
//	               57-date backfill (see the regeneration runbook), not a rerun.
//	ships/glyphs/  cmd/generate-ship-glyphs/contactsheet.go — deliberately a
//	               breadcrumb header, not the site nav. Arguably correct as is.
var exemptSections = []string{
	"diffs/",
	"ships/glyphs/",
}

// homePage is hand-maintained but carries the same header shell as every
// generated page, so TestGeneratedPagesMatchNav checks it too.
const homePage = "index.html"

// maxReported bounds the failure output: a desync is usually site-wide, and
// printing 14k paths buries the one fact you need.
const maxReported = 10

// TestGeneratedPagesMatchNav guards the invariant that makes this package
// worth having: every committed page's nav is what Header currently renders.
//
// The nav is baked into ~14k generated pages at build time and those pages are
// committed, so "one definition" only holds in the instant after a full regen.
// A partial regen, or a regen from a branch with a different Items list,
// silently leaves committed pages advertising a nav this package no longer
// emits — with nothing to detect it. That is not hypothetical: on 2026-08-23
// main carried 13,970 pages linking a "Battles" entry that Items did not
// contain and whose target page did not exist on main, so the live site had a
// 404 in its primary nav. kbnav_test.go could not catch it, because it tests
// what Header returns and never looks at what the generators wrote.
//
// Fix a failure by regenerating (both generators — generate-items-kb covers
// items/systems/ships/recipes/skills/facilities/missions, generate-factions-kb
// covers factions/players/passengers) and committing the result.
func TestGeneratedPagesMatchNav(t *testing.T) {
	kbDir := locateKBDir(t)

	// Header's prefix is the relative path back to the kb root, so it is a
	// pure function of directory depth: kb/index.html -> "",
	// kb/items/index.html -> "../", kb/items/weapon/foo.html -> "../../".
	wantByDepth := map[int]string{}
	headerFor := func(depth int) string {
		if h, ok := wantByDepth[depth]; ok {
			return h
		}
		h := Header(strings.Repeat("../", depth))
		wantByDepth[depth] = h
		return h
	}

	var checked, skipped int
	var bad []string
	err := filepath.WalkDir(kbDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".html") {
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // test-only walk of repo output
		if err != nil {
			return err
		}
		body := string(data)
		if !strings.Contains(body, headerMarker) {
			skipped++
			return nil
		}
		rel, err := filepath.Rel(kbDir, path)
		if err != nil {
			return err
		}
		slashed := filepath.ToSlash(rel)
		if exempt(slashed) {
			skipped++
			return nil
		}
		checked++
		depth := strings.Count(rel, string(filepath.Separator))
		if !strings.Contains(body, headerFor(depth)) {
			bad = append(bad, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", kbDir, err)
	}
	if checked == 0 {
		t.Fatalf("no pages with a site header found under %s — the walk is looking in the wrong place", kbDir)
	}
	t.Logf("checked %d pages against kbnav.Items (%d skipped: hand-maintained or exempt sections)", checked, skipped)

	if len(bad) > 0 {
		shown := bad
		if len(shown) > maxReported {
			shown = shown[:maxReported]
		}
		t.Errorf("%d of %d generated pages have a nav that does not match kbnav.Items.\n"+
			"Regenerate with both generators and commit the result.\nFirst %d:\n  %s",
			len(bad), checked, len(shown), strings.Join(shown, "\n  "))
	}
}

// exempt reports whether a kb-relative path belongs to a tree this package
// does not render the header for.
func exempt(rel string) bool {
	for _, s := range exemptSections {
		if strings.HasPrefix(rel, s) {
			return true
		}
	}
	return false
}

// locateKBDir walks up from the test's working directory to the repo root and
// returns its kb/ directory, skipping the test when the generated site is not
// present (so the package remains testable in isolation).
func locateKBDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "kb")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Confirm it is the generated site, not some other "kb" dir.
			if _, err := os.Stat(filepath.Join(candidate, "index.html")); err == nil {
				return candidate
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("generated kb/ site not found; skipping site-wide nav check")
		}
		dir = parent
	}
}

// TestHomePageLinksGuides: the home page has a Guides section card (the nav
// drawer link comes from nav.js).
func TestHomePageLinksGuides(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(locateKBDir(t), homePage)) //nolint:gosec // test-only read of repo output
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `href="guides/index.html"`) {
		t.Errorf("kb/%s has no Guides card linking guides/index.html", homePage)
	}
}
