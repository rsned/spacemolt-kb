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
		path := "index.html"
		if it.Slug != "" {
			path = it.Slug + "/index.html"
		}
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
