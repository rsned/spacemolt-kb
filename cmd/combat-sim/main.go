package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func run() error {
	fitA := flag.String("a", "", "fitting spec JSON for side A (required)")
	fitB := flag.String("b", "", "fitting spec JSON for side B (required)")
	runs := flag.Int("runs", 10000, "battles per stance-pair cell")
	seed := flag.Uint64("seed", 42, "RNG seed (deterministic output per seed)")
	catalog := flag.String("catalog", "data/combat-sim/catalog", "catalog snapshot dir")
	calPath := flag.String("calibration", "data/combat-sim/calibration.json", "calibration file (missing = built-in defaults)")
	maxTicks := flag.Int("max-ticks", 0, "override calibration max_ticks (0 = keep)")
	jsonOut := flag.String("json", "", "write full per-cell outcome distributions to this file")
	extract := flag.String("extract-fits", "", "battle id: write one FitSpec JSON per participant and exit")
	battles := flag.String("battles", "data/battles", "battle fixture dir (--extract-fits input)")
	outDir := flag.String("out", "data/combat-sim/fits", "output dir (--extract-fits)")
	flag.Parse()
	if *extract == "" && (*fitA == "" || *fitB == "") {
		flag.Usage()
		return fmt.Errorf("--a and --b are required (or --extract-fits)")
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

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "combat-sim:", err)
		os.Exit(1)
	}
}
