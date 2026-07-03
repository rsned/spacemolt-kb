// Package kbnav defines the Spacemolt KB site header in exactly one place.
//
// The top nav bar is baked into every generated page, but its definition lives
// here so a nav change is a single edit (the Items list below) instead of the
// half-dozen duplicated header constants this package replaced. Both KB
// generators (generate-items-kb and generate-factions-kb) render their headers
// via Header, so the bar is identical site-wide.
package kbnav

import "strings"

// Items is the canonical, ordered top-nav list. The empty slug is the Home link
// (it points at the kb root). To add/remove/reorder a nav entry, edit this list.
var Items = []struct{ Slug, Label string }{
	{"", "Home"},
	{"systems", "Systems"},
	{"items", "Items"},
	{"recipes", "Recipes"},
	{"skills", "Skills"},
	{"ships", "Ships"},
	{"facilities", "Facilities"},
	{"resources", "Resources"},
	{"missions", "Missions"},
	{"factions", "Factions"},
	{"players", "Players"},
	{"passengers", "Passengers"},
}

// themeBtn is the dark/light toggle; themeScript on each page wires it up.
const themeBtn = `
            <button class="theme-toggle" id="theme-toggle" aria-label="Toggle theme">
                <svg class="icon-sun" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
                <svg class="icon-moon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
            </button>`

// Header renders the full <header> bar. prefix is the relative path from the
// page to the kb root — "../" for section index pages (e.g. kb/items/index.html)
// and "../../" for category/detail pages (e.g. kb/items/<cat>/index.html).
func Header(prefix string) string {
	var nav strings.Builder
	for _, it := range Items {
		href := prefix
		if it.Slug != "" {
			href += it.Slug + "/index.html"
		}
		nav.WriteString("\n            <a href=\"" + href + "\">" + it.Label + "</a>")
	}
	return `    <header class="site-header">
        <h1><a href="` + prefix + `" style="color:inherit;text-decoration:none">Spacemolt KB</a></h1>
        <nav>` + nav.String() + themeBtn + `
        </nav>
    </header>`
}
