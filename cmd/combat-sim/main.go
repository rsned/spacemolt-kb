package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func run() error {
	fitA := flag.String("a", "", "fitting spec JSON for side A (required)")
	fitB := flag.String("b", "", "fitting spec JSON for side B (required)")
	runs := flag.Int("runs", 10000, "battles per stance-pair cell")
	seed := flag.Uint64("seed", 42, "RNG seed (deterministic output per seed)")
	catalog := flag.String("catalog", "data/snapshots/latest", "catalog snapshot dir")
	calPath := flag.String("calibration", "data/combat-sim/calibration.json", "calibration file (missing = built-in defaults)")
	maxTicks := flag.Int("max-ticks", 0, "override calibration max_ticks (0 = keep)")
	jsonOut := flag.String("json", "", "write full per-cell outcome distributions to this file")
	flag.Parse()
	if *fitA == "" || *fitB == "" {
		flag.Usage()
		return fmt.Errorf("--a and --b are required")
	}
	cat, err := LoadCatalog(*catalog)
	if err != nil {
		return err
	}
	cal, err := LoadCalibration(*calPath)
	if os.IsNotExist(err) {
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
