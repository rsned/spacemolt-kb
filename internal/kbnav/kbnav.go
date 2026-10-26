// Package kbnav defines the Spacemolt KB site header in exactly one place.
//
// Every generated page carries the same fixed header shell (Header): a menu
// button, the site title, search and the theme toggle, plus an empty
// slide-out drawer. The drawer's section links live only in kb/nav.js
// (NavScript, written by cmd/generate-nav), so adding, removing or reordering
// a section rewrites that one file instead of every page.
package kbnav

import (
	"encoding/json"
	"strings"
)

// Items is the canonical, ordered section list shown in the nav drawer. The
// empty slug is the Home link (it points at the kb root). To change the nav,
// edit this list and run `go run ./cmd/generate-nav`.
var Items = []struct{ Slug, Label string }{
	{"", "Home"},
	{"systems", "Systems"},
	{"items", "Items"},
	{"recipes", "Recipes"},
	{"skills", "Skills"},
	{"ships", "Ships"},
	{"facilities", "Facilities"},
	{"resources", "Resources"},
	{"wildlife", "Wildlife"},
	{"missions", "Missions"},
	{"factions", "Factions"},
	{"players", "Players"},
	{"passengers", "Passengers"},
	{"did_you_know", "Did You Know?"},
	{"guides", "Guides"},
}

// themeBtn is the dark/light toggle; themeScript on each page wires it up.
const themeBtn = `
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>`

// searchBox renders the site-search input and its results dropdown.
//
// data-kb-root carries the page's relative path to the kb root: search.js needs
// it for both the index fetch and every result href, and pages sit at varying
// depths. search-index.json is fetched lazily on first focus, so a page that is
// never searched from never downloads it.
func searchBox(prefix string) string {
	return `
            <div class="kbs">
                <input id="kb-search" type="search" placeholder="Search the KB…  /"
                    autocomplete="off" spellcheck="false" role="combobox"
                    aria-expanded="false" aria-controls="kb-search-results"
                    aria-label="Search the knowledge base" data-kb-root="` + prefix + `">
                <div class="kbs-results" id="kb-search-results" role="listbox" hidden></div>
            </div>
            <script src="` + prefix + `search.js" defer></script>`
}

// Header renders the full <header> bar. prefix is the relative path from the
// page to the kb root — "../" for section index pages (e.g. kb/items/index.html)
// and "../../" for category/detail pages (e.g. kb/items/<cat>/index.html).
// The drawer is empty here; nav.js fills it from Items.
func Header(prefix string) string {
	return `    <header class="site-header">` + menuBtn + `
        <h1><a href="` + prefix + `" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <div class="site-tools">` + searchBox(prefix) + themeBtn + `
        </div>
        <nav class="site-nav" id="site-nav" aria-label="Site sections">
            <div class="site-nav-head">Spacemolt KB<button class="nav-close" type="button" aria-label="Close navigation">×</button></div>
        </nav>
        <div class="nav-scrim"></div>
        <script src="` + prefix + `nav.js" defer></script>
    </header>`
}

// menuBtn opens the nav drawer.
const menuBtn = `
        <button class="nav-toggle" type="button" aria-label="Open navigation" aria-controls="site-nav" aria-expanded="false">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M4 6h16M4 12h16M4 18h16"/></svg>
        </button>`

// NavScript returns the contents of kb/nav.js: the section list from Items
// and the code that renders it into the drawer and opens/closes it.
func NavScript() string {
	type entry struct {
		Path  string `json:"path"`
		Label string `json:"label"`
	}
	entries := make([]entry, 0, len(Items))
	for _, it := range Items {
		path := "index.html" // explicit, so Home also works when browsing from file://
		if it.Slug != "" {
			path = it.Slug + "/index.html"
		}
		entries = append(entries, entry{Path: path, Label: it.Label})
	}
	list, err := json.MarshalIndent(entries, "  ", "  ")
	if err != nil {
		panic(err) // a static list of strings always marshals
	}
	return strings.Replace(navScript, "/*SECTIONS*/", string(list), 1)
}

// navScript is kb/nav.js with the section list left as a placeholder. The kb
// root comes from the script's own URL, so it works at any page depth and from
// file:// as well as the web.
const navScript = `// Generated by cmd/generate-nav from internal/kbnav.Items. Do not edit.
//
// Site nav drawer: renders the section links, opens from the menu button, and
// closes on ×, the scrim or Esc. The section being viewed is marked
// aria-current.
(function () {
  var SECTIONS = /*SECTIONS*/;
  var script = document.currentScript;
  var root = script ? new URL('.', script.src) : new URL('./', location.href);
  var nav = document.getElementById('site-nav');
  var toggle = document.querySelector('.nav-toggle');
  if (!nav || !toggle) return;

  var here = location.pathname.replace(/index\.html$/, '');
  var current = null, currentLen = -1;
  SECTIONS.forEach(function (s) {
    var a = document.createElement('a');
    var url = new URL(s.path, root);
    a.href = url.href;
    a.textContent = s.label;
    nav.appendChild(a);
    var dir = url.pathname.replace(/index\.html$/, '');
    if (here.indexOf(dir) === 0 && dir.length > currentLen) { current = a; currentLen = dir.length; }
  });
  if (current) current.setAttribute('aria-current', 'page');

  function setOpen(open) {
    document.body.classList.toggle('nav-open', open);
    toggle.setAttribute('aria-expanded', open ? 'true' : 'false');
    if (open) {
      var first = nav.querySelector('a');
      if (first) first.focus();
    } else if (nav.contains(document.activeElement)) {
      toggle.focus();
    }
  }
  toggle.addEventListener('click', function () { setOpen(!document.body.classList.contains('nav-open')); });
  nav.querySelector('.nav-close').addEventListener('click', function () { setOpen(false); });
  var scrim = document.querySelector('.nav-scrim');
  if (scrim) scrim.addEventListener('click', function () { setOpen(false); });
  document.addEventListener('keydown', function (e) {
    if (e.key === 'Escape' && document.body.classList.contains('nav-open')) setOpen(false);
  });
})();
`
