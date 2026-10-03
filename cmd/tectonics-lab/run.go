package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/tectonics"
)

type runOpts struct {
	Planet    string
	Seed      int64
	Archetype string
	Out       string
	Sets      []string
	Face      int
	Steps     int
	Progress  func(step, steps int)
}

// setFlag collects repeated -set Name=value flags.
type setFlag []string

func (s *setFlag) String() string     { return strings.Join(*s, ",") }
func (s *setFlag) Set(v string) error { *s = append(*s, v); return nil }

func runBundle(o runOpts) (string, error) {
	p, err := tectonics.DefaultParams(o.Archetype)
	if err != nil {
		return "", err
	}
	if o.Face > 0 {
		p.Face = o.Face
	}
	if o.Steps > 0 {
		p.Steps = o.Steps
	}
	for _, kv := range o.Sets {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return "", fmt.Errorf("-set %q: want Name=value", kv)
		}
		if err := p.Set(k, v); err != nil {
			return "", err
		}
	}
	seed, slug := o.Seed, fmt.Sprintf("seed-%d", o.Seed)
	if o.Planet != "" {
		seed, slug = tectonics.SeedForPlanet(o.Planet), o.Planet
	}
	dir := filepath.Join(o.Out, slug+"-"+o.Archetype)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	m := tectonics.NewManifest(p, seed, o.Planet)
	err = tectonics.Run(p, seed, func(f tectonics.Frame) error {
		fi, err := tectonics.WriteFrame(dir, f)
		if err != nil {
			return err
		}
		m.Frames = append(m.Frames, fi)
		if o.Progress != nil {
			o.Progress(f.Step, p.Steps)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return dir, tectonics.WriteManifest(dir, m)
}
