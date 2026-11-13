package kbnav

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeaderDepthPrefix(t *testing.T) {
	h1 := Header("../")
	h2 := Header("../../")

	// Title link, search root and the nav script all point at the kb root.
	for _, want := range []string{`<a href="../" style=`, `data-kb-root="../"`, `<script src="../nav.js" defer>`} {
		if !strings.Contains(h1, want) {
			t.Errorf("depth-1 header missing %s:\n%s", want, h1)
		}
	}
	for _, want := range []string{`<a href="../../" style=`, `data-kb-root="../../"`, `<script src="../../nav.js" defer>`} {
		if !strings.Contains(h2, want) {
			t.Errorf("depth-2 header missing %s:\n%s", want, h2)
		}
	}
}

// The header is a fixed shell: menu button, search, theme toggle and an empty
// drawer. Section links must NOT be baked in, or a nav change means rewriting
// every page again.
func TestHeaderIsAFixedShell(t *testing.T) {
	h := Header("../")
	for _, want := range []string{`class="nav-toggle"`, `aria-controls="site-nav"`, `id="site-nav"`, `class="nav-close"`,
		`class="nav-scrim"`, `id="kb-search"`, `id="theme-toggle"`} {
		if !strings.Contains(h, want) {
			t.Errorf("header missing %s", want)
		}
	}
	for _, it := range Items {
		if it.Slug != "" && strings.Contains(h, it.Slug+"/index.html") {
			t.Errorf("header bakes in the %q link; section links belong in nav.js", it.Label)
		}
	}
	if strings.Count(h, "<header") != 1 || strings.Count(h, "</header>") != 1 {
		t.Errorf("header should be a single well-formed element:\n%s", h)
	}
}

func TestNavScriptListsEveryItem(t *testing.T) {
	js := NavScript()
	for _, it := range Items {
		path := it.Path()
		if !strings.Contains(js, `"path": "`+path+`"`) || !strings.Contains(js, `"label": "`+it.Label+`"`) {
			t.Errorf("nav.js missing %q (%s)", it.Label, path)
		}
	}
	if strings.Contains(js, "/*SECTIONS*/") {
		t.Error("section placeholder was not replaced")
	}
}

// TestCommittedNavScriptMatchesItems: kb/nav.js is the only place the drawer's
// links reach the site, so it must match Items. Fix a failure with
// `go run ./cmd/generate-nav` and commit kb/nav.js.
func TestCommittedNavScriptMatchesItems(t *testing.T) {
	got, err := os.ReadFile(filepath.Join(locateKBDir(t), "nav.js")) //nolint:gosec // test-only read of repo output
	if err != nil {
		t.Fatalf("reading kb/nav.js: %v (run go run ./cmd/generate-nav)", err)
	}
	if string(got) != NavScript() {
		t.Error("kb/nav.js does not match kbnav.Items; run `go run ./cmd/generate-nav` and commit kb/nav.js")
	}
}

func TestNavScriptListsChildren(t *testing.T) {
	js := NavScript()
	n := 0
	for _, it := range Items {
		for _, c := range it.Children {
			n++
			if !strings.Contains(js, `"path": "`+c.Path+`"`) || !strings.Contains(js, `"label": "`+c.Label+`"`) {
				t.Errorf("nav.js missing sub-entry %q (%s) under %q", c.Label, c.Path, it.Label)
			}
		}
	}
	if n != 11 {
		t.Errorf("got %d sub-entries, want 11", n)
	}
	if !strings.Contains(js, `"path": "recipes/where.html"`) {
		t.Error("Where Can I Make This? missing from the drawer")
	}
	if !strings.Contains(js, "nav-sub") || !strings.Contains(js, "nav-divider") {
		t.Error("nav.js does not render sub-entries or the divider")
	}
}

// The divider splits the sections that have a card on the home page (Guides
// excepted, by the owner's call) from the other pages.
func TestDividerFollowsHomeCards(t *testing.T) {
	home, err := os.ReadFile(filepath.Join(locateKBDir(t), "index.html")) //nolint:gosec // test-only read of repo output
	if err != nil {
		t.Fatal(err)
	}
	dividers, above, below := 0, map[string]bool{}, map[string]bool{}
	for _, it := range Items {
		if it.Divider {
			dividers++
		}
		if it.Slug == "" {
			continue
		}
		if dividers == 0 {
			above[it.Slug] = true
		} else {
			below[it.Slug] = true
		}
	}
	if dividers != 1 {
		t.Fatalf("want exactly one divider, got %d", dividers)
	}
	for slug := range above {
		if !strings.Contains(string(home), `<a href="`+slug+`/index.html" class="cat"`) {
			t.Errorf("%s is above the divider but has no home page card", slug)
		}
	}
	for slug := range below {
		if slug != "guides" && strings.Contains(string(home), `<a href="`+slug+`/index.html" class="cat"`) {
			t.Errorf("%s has a home page card but is below the divider", slug)
		}
	}
}

// TestItemPagesExist: every drawer entry, including one whose Page overrides
// the section index (the BoM explorer), must point at a page that exists.
func TestItemPagesExist(t *testing.T) {
	kb := locateKBDir(t)
	for _, it := range Items {
		if _, err := os.Stat(filepath.Join(kb, it.Path())); err != nil {
			t.Errorf("%q: %v", it.Label, err)
		}
	}
	explorer := Item{Slug: "build-costs", Label: "BoM Explorer", Page: "build-costs/explorer.html"}
	if explorer.Path() != "build-costs/explorer.html" {
		t.Errorf("Page override ignored: %q", explorer.Path())
	}
	if (Item{Slug: "systems"}).Path() != "systems/index.html" || (Item{}).Path() != "index.html" {
		t.Error("default paths changed")
	}
}

func TestSubEntryPagesExist(t *testing.T) {
	kb := locateKBDir(t)
	for _, it := range Items {
		for _, c := range it.Children {
			if !strings.HasPrefix(c.Path, it.Slug+"/") {
				t.Errorf("%q (%s) is not inside its section %q", c.Label, c.Path, it.Slug)
			}
			if _, err := os.Stat(filepath.Join(kb, c.Path)); err != nil {
				t.Errorf("%q: %v", c.Label, err)
			}
		}
	}
}
