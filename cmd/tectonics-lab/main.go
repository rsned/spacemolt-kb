// Command tectonics-lab bakes tectonic-plate timelines into keyframe bundles
// and serves a time-slider viewer for them.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

func usage() {
	fmt.Fprintf(os.Stderr, `usage:
  tectonics-lab run  -planet <id> | -seed <n>  -archetype <%s> [-face 256] [-steps 150] [-set Name=value]... [-out data/tectonics]
  tectonics-lab serve [-addr localhost:8091] [-data data/tectonics] [-web cmd/tectonics-lab/web]
`, strings.Join(tectonics.Archetypes(), "|"))
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		var o runOpts
		var sets setFlag
		fs.StringVar(&o.Planet, "planet", "", "planet game id (seeds the run)")
		fs.Int64Var(&o.Seed, "seed", 0, "raw master seed when no -planet")
		fs.StringVar(&o.Archetype, "archetype", "terran", "planet archetype")
		fs.StringVar(&o.Out, "out", "data/tectonics", "bundle root")
		fs.IntVar(&o.Face, "face", 0, "cube face size (default from archetype)")
		fs.IntVar(&o.Steps, "steps", 0, "time steps (default from archetype)")
		fs.Var(&sets, "set", "override a knob, Name=value (repeatable)")
		_ = fs.Parse(os.Args[2:])
		o.Sets = sets
		o.Progress = func(step, steps int) {
			if step%10 == 0 || step == steps {
				log.Printf("step %d/%d", step, steps)
			}
		}
		dir, err := runBundle(o)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("bundle written to %s", dir)
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", "localhost:8091", "listen address (use :8091 to listen on all interfaces)")
		data := fs.String("data", "data/tectonics", "bundle root")
		web := fs.String("web", "cmd/tectonics-lab/web", "viewer assets")
		_ = fs.Parse(os.Args[2:])
		log.Fatal(serve(*addr, *data, *web))
	default:
		usage()
	}
}
