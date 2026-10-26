// Command generate-nav writes kb/nav.js, the site nav drawer, from
// internal/kbnav.Items. Run it after changing the section list; no page needs
// regenerating.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

func main() {
	out := flag.String("out", "kb/nav.js", "path to write the nav script")
	flag.Parse()
	if err := os.WriteFile(*out, []byte(kbnav.NavScript()), 0o644); err != nil { //nolint:gosec // public site asset
		log.Fatal(err)
	}
	log.Printf("wrote %s", *out)
}
