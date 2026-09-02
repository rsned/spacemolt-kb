package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// swarmMaxTicks is the battle-length cap used by --swarm mode. Long grinds
// against capital defenders need more headroom than the calibration default,
// so this is deliberately generous rather than reusing cal.MaxTicks.
const swarmMaxTicks = 4000

func run() error {
	fitA := flag.String("a", "", "fitting spec JSON for side A (required)")
	fitB := flag.String("b", "", "fitting spec JSON for side B (required)")
	runs := flag.Int("runs", 10000, "battles per stance-pair cell (default 300 in --swarm mode, unless set)")
	seed := flag.Uint64("seed", 42, "RNG seed (deterministic output per seed)")
	catalog := flag.String("catalog", "data/combat-sim/catalog", "catalog snapshot dir")
	calPath := flag.String("calibration", "data/combat-sim/calibration.json", "calibration file (missing = built-in defaults)")
	maxTicks := flag.Int("max-ticks", 0, "override calibration max_ticks (0 = keep)")
	jsonOut := flag.String("json", "", "write full per-cell outcome distributions to this file")
	extract := flag.String("extract-fits", "", "battle id: write one FitSpec JSON per participant and exit")
	battles := flag.String("battles", "data/battles", "battle fixture dir (--extract-fits input)")
	outDir := flag.String("out", "data/combat-sim/fits", "output dir (--extract-fits)")
	swarm := flag.String("swarm", "", "attacker hull id: run swarm-crossover mode against --vs and exit")
	vs := flag.String("vs", "", "defender hull id (--swarm)")
	nMax := flag.Int("n-max", 25000, "largest swarm size probed before reporting ∞ (--swarm)")
	swarmJSON := flag.String("swarm-json", "", "write the full Crossing (curve included) as JSON to this file (--swarm)")
	flag.Parse()
	if *swarm != "" {
		// --swarm reuses --runs but wants a lighter default (300 vs the
		// table mode's 10000); only apply it when the user didn't pass
		// --runs explicitly, so an explicit override still wins either way.
		if !flagWasSet("runs") {
			*runs = 300
		}
		cat, err := LoadCatalog(*catalog)
		if err != nil {
			return err
		}
		cal, err := LoadCalibration(*calPath)
		if errors.Is(err, fs.ErrNotExist) {
			cal, err = DefaultCalibration(), nil
		}
		if err != nil {
			return err
		}
		return runSwarmCLI(*swarm, *vs, cat, cal, *nMax, *runs, swarmMaxTicks, *seed, *swarmJSON, os.Stdout)
	}
	if *extract == "" && (*fitA == "" || *fitB == "") {
		flag.Usage()
		return fmt.Errorf("--a and --b are required (or --extract-fits, or --swarm)")
	}
	cat, err := LoadCatalog(*catalog)
	if err != nil {
		return err
	}
	if *extract != "" {
		fits, err := ExtractFits(*extract, *battles, cat, os.Stderr)
		if err != nil {
			return err
		}
		for _, f := range fits {
			path := filepath.Join(*outDir, f.Filename)
			raw, err := json.MarshalIndent(f.Spec, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
				return err
			}
			s := f.Spec.Skills
			fmt.Printf("wrote %s  hull=%s modules=%d skills W%d/G%d/S%d/A%d\n",
				path, f.Spec.Hull, len(f.Spec.Modules),
				s["weapons"], s["gunnery"], s["shields"], s["armor"])
		}
		return nil
	}
	cal, err := LoadCalibration(*calPath)
	if errors.Is(err, fs.ErrNotExist) {
		cal, err = DefaultCalibration(), nil
	}
	if err != nil {
		return err
	}
	if *maxTicks > 0 {
		cal.MaxTicks = *maxTicks
	}
	fa, err := LoadFit(*fitA)
	if err != nil {
		return err
	}
	fb, err := LoadFit(*fitB)
	if err != nil {
		return err
	}
	a, err := Resolve(fa, cat)
	if err != nil {
		return err
	}
	b, err := Resolve(fb, cat)
	if err != nil {
		return err
	}
	cells := RunTable(a, b, cal, *runs, *seed)
	fmt.Print(FormatTable(a, b, cells, cal))
	if *jsonOut != "" {
		raw, err := json.MarshalIndent(cells, "", " ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*jsonOut, raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// flagWasSet reports whether name was explicitly passed on the command line
// (as opposed to left at its default), per flag.Visit's documented scoping
// to only-set flags.
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// runSwarmCLI resolves attackerID (non-capital) and defenderID (capitals
// allowed) from cat, runs Crossover, prints a one-line summary to w, and —
// when jsonOut is non-empty — writes the full Crossing (curve included) as
// indented JSON to that path.
func runSwarmCLI(attackerID, defenderID string, cat *Catalog, cal *Calibration, nMax, runs, maxTicks int, seed uint64, jsonOut string, w io.Writer) error {
	att, err := ResolveHull(attackerID, cat, false)
	if err != nil {
		return err
	}
	def, err := ResolveHull(defenderID, cat, true)
	if err != nil {
		return err
	}
	c := Crossover(att, def, cal, nMax, runs, maxTicks, seed)
	if c.N == 0 {
		_, _ = fmt.Fprintf(w, "%s swarm vs %s: crossover N=∞ within %d\n", attackerID, defenderID, nMax)
	} else {
		_, _ = fmt.Fprintf(w, "%s swarm vs %s: crossover N=%d (P=%.2f), %s kills %d\n",
			attackerID, defenderID, c.N, c.PWin, defenderID, c.MedianKills)
	}
	if jsonOut != "" {
		raw, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(jsonOut, append(raw, '\n'), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "combat-sim:", err)
		os.Exit(1)
	}
}
