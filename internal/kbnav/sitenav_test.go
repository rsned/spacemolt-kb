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
// generate-items-kb and generate-factions-kb, but these three producers still
// carry their own header copy, which is why the site currently shows three
// different nav vintages. Removing an entry here means migrating that
// generator to Header and re-rendering its pages.
//
//	did_you_know/  cmd/generate-stronghold-reach/template.go — its own header,
//	               nav stops at Missions, plus a "Did You Know?" self-link that
//	               is not in Items. Migrating drops that self-link, so it is a
//	               product call rather than a mechanical fix.
//	diffs/         pkg/gamediff/report.go — an older nav still. These are
//	               historical snapshot reports; re-rendering them means a
//	               57-date backfill (see the regeneration runbook), not a rerun.
//	ships/glyphs/  cmd/generate-ship-glyphs/contactsheet.go — deliberately a
//	               breadcrumb header, not the site nav. Arguably correct as is.
var exemptSections = []string{
	"did_you_know/",
	"diffs/",
	"ships/glyphs/",
}

// homePage is hand-maintained and legitimately differs: it omits the Home
// self-link and does not wrap its <h1> in one. TestHomePageCoversNav checks it
// separately rather than exempting it outright — a nav entry added to Items and
// forgotten here is a demonstrated failure mode (commit a2ab6a274 exists to fix
// exactly that for Passengers).
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
		if slashed == homePage || exempt(slashed) {
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

// TestHomePageCoversNav asserts the hand-maintained kb/index.html links every
// section in Items. It deliberately checks link targets rather than exact
// markup, because the home page's header is allowed to differ in shape.
func TestHomePageCoversNav(t *testing.T) {
	kbDir := locateKBDir(t)
	data, err := os.ReadFile(filepath.Join(kbDir, homePage)) //nolint:gosec // test-only read of repo output
	if err != nil {
		t.Fatalf("reading %s: %v", homePage, err)
	}
	body := string(data)
	for _, it := range Items {
		if it.Slug == "" {
			continue // the home page does not link to itself
		}
		want := `href="` + it.Slug + `/index.html"`
		if !strings.Contains(body, want) {
			t.Errorf("kb/%s is missing a nav link for %q (expected %s); "+
				"it is hand-maintained, so adding an entry to Items requires editing it too",
				homePage, it.Label, want)
		}
	}

	// The search box reaches every generated page through Header, but the home
	// page renders its own nav and so silently missed it when search shipped —
	// the same failure mode as a forgotten nav entry, one layer down.
	for _, want := range []string{`id="kb-search"`, `id="kb-search-results"`, `src="search.js"`} {
		if !strings.Contains(body, want) {
			t.Errorf("kb/%s is missing %s: the site search box is in kbnav.Header for "+
				"generated pages, but this page is hand-maintained and needs it copied in",
				homePage, want)
		}
	}
	// A root-level page addresses the index and its results without a prefix.
	if strings.Contains(body, `data-kb-root="`) && !strings.Contains(body, `data-kb-root=""`) {
		t.Errorf("kb/%s: data-kb-root must be empty on a root-level page, "+
			"or search fetches the index from the wrong path", homePage)
	}
}
