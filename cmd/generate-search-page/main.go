// Command generate-search-page writes kb/search.html, the full results view the
// nav search box links to when a query matches more than the dropdown shows.
//
// It is its own command rather than a committed static file so the header comes
// from kbnav like every other page — a nav change stays a single edit, and
// internal/kbnav's site-wide test covers this page too.
//
// The page ships no results of its own: kb/search.js reads ?q= and renders
// against the same index and the same ranking the dropdown uses.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

const page = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Search - Spacemolt KB</title>
    <link rel="stylesheet" href="smui.css">
</head>
<body>
%s
    <main class="container page-content">
      <div class="breadcrumb"><a href="./">Home</a> / Search</div>
      <h2>Search</h2>
      <p class="text-muted mt-1" id="kbs-page-q">Loading the index&hellip;</p>
      <div id="kbs-page-results" class="mt-2"></div>
    </main>
</body>
</html>
`

func main() {
	out := flag.String("out", "kb/search.html", "output path")
	flag.Parse()

	// Root-level page, so the prefix to the kb root is empty.
	html := fmt.Sprintf(page, kbnav.Header(""))
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, []byte(html), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("generate-search-page: wrote %s (%d bytes)\n", *out, len(html))
}
