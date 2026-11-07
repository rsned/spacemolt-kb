// Command generate-api-diffs renders kb/api-diffs/: one page per OpenAPI
// snapshot step showing what changed in the API's structs, as Go or
// TypeScript diffs, plus an index. Every run rebuilds the whole section from
// the dated specs and deletes report pages that no longer belong.
//
// Usage:
//
//	generate-api-diffs [-docs ../spacemolt/server_docs] [-output kb/api-diffs]
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/rsned/spacemolt-kb/pkg/apidiff"
)

var reportName = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\.html$`)

func main() {
	docs := flag.String("docs", "../spacemolt/server_docs", "server_docs directory holding openapi.YYYYMMDD.json and archive/*.json.gz")
	out := flag.String("output", "kb/api-diffs", "output directory")
	flag.Parse()

	snaps, err := apidiff.LoadSnapshots(*docs)
	if err != nil {
		log.Fatal(err)
	}
	steps := apidiff.Steps(snaps, apidiff.Baseline)
	if err := os.MkdirAll(*out, 0o755); err != nil { //nolint:gosec // public site directory
		log.Fatal(err)
	}
	written := map[string]bool{"index.html": true}
	for i, s := range steps {
		prev, next := "", ""
		if i > 0 {
			prev = apidiff.ISODate(steps[i-1].To)
		}
		if i+1 < len(steps) {
			next = apidiff.ISODate(steps[i+1].To)
		}
		page, err := apidiff.RenderReport(s, prev, next)
		if err != nil {
			log.Fatal(err)
		}
		name := apidiff.ISODate(s.To) + ".html"
		write(filepath.Join(*out, name), page)
		written[name] = true
	}
	newest := slices.Clone(steps)
	slices.Reverse(newest)
	index, err := apidiff.RenderIndex(newest)
	if err != nil {
		log.Fatal(err)
	}
	write(filepath.Join(*out, "index.html"), index)

	entries, err := os.ReadDir(*out)
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range entries {
		if reportName.MatchString(e.Name()) && !written[e.Name()] {
			if err := os.Remove(filepath.Join(*out, e.Name())); err != nil {
				log.Fatal(err)
			}
		}
	}
	log.Printf("api-diffs: %d reports from %d snapshots → %s", len(steps), len(snaps), *out)
}

func write(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // public site page
		log.Fatal(err)
	}
}
