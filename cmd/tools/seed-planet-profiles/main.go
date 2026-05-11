// Command seed-planet-profiles writes per-planet PlanetProfile
// envelope JSON files to disk for use by pkg/planetgen.
//
// Two modes:
//
//	-planet=<type>:<seed>     (repeatable)  explicit planet list
//	-manifest=<tsv path>      file with one "<type><TAB><name>" per line
//
// Output is byte-stable: rerunning with the same inputs produces
// identical files. By default, refuses to overwrite an existing file;
// pass -force to overwrite (use after a per-archetype default
// changes).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

type planetSpec struct {
	planetType string
	seed       string // also used as the filename stem
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error { *s = append(*s, v); return nil }

func main() {
	out := flag.String("out", "data/planet-profiles", "output directory")
	var planets stringSliceFlag
	flag.Var(&planets, "planet", "<type>:<seed> (repeatable)")
	manifest := flag.String("manifest", "", "optional TSV manifest of type<TAB>name lines")
	force := flag.Bool("force", false, "overwrite existing files")
	flag.Parse()

	specs, err := collectSpecs(planets, *manifest)
	if err != nil {
		log.Fatal(err)
	}
	if len(specs) == 0 {
		log.Fatal("no planets specified (use -planet or -manifest)")
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatalf("mkdir %s: %v", *out, err)
	}
	written, skipped := 0, 0
	for _, s := range specs {
		w, err := writeOne(*out, s, *force)
		if err != nil {
			log.Fatalf("write %s: %v", s.seed, err)
		}
		if w {
			written++
		} else {
			skipped++
		}
	}
	fmt.Printf("seed-planet-profiles: wrote %d, skipped %d (out=%s)\n", written, skipped, *out)
}

// collectSpecs merges -planet entries with optional -manifest lines.
// Stable sort by seed for deterministic output ordering.
func collectSpecs(planets []string, manifest string) ([]planetSpec, error) {
	var specs []planetSpec
	for _, p := range planets {
		parts := strings.SplitN(p, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("bad -planet %q: expected <type>:<seed>", p)
		}
		specs = append(specs, planetSpec{planetType: parts[0], seed: parts[1]})
	}
	if manifest != "" {
		f, err := os.Open(manifest)
		if err != nil {
			return nil, fmt.Errorf("open manifest: %w", err)
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("manifest: bad line %q (want type<TAB>name)", line)
			}
			specs = append(specs, planetSpec{planetType: parts[0], seed: parts[1]})
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("manifest read: %w", err)
		}
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].seed < specs[j].seed })
	return specs, nil
}

// writeOne encodes a fresh envelope (handTuned=false, profile from
// GetProfile) and writes it to <out>/<slug>.json. Returns (true, nil)
// if a file was created or overwritten, (false, nil) if the file
// already exists and force is false.
func writeOne(out string, s planetSpec, force bool) (bool, error) {
	prof := planetgen.GetProfile(s.planetType)
	if prof == nil {
		return false, fmt.Errorf("unknown planet type %q", s.planetType)
	}
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          s.planetType,
		Seed:          s.seed,
		HandTuned:     false,
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		return false, err
	}
	slug := profilejson.PlanetSlug(s.seed)
	if slug == "" {
		return false, fmt.Errorf("empty slug for seed %q", s.seed)
	}
	path := filepath.Join(out, slug+".json")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
