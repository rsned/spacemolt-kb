package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/stationplan"
	"gopkg.in/yaml.v3"
)

// guide is one guides/*.md file: YAML frontmatter plus a markdown body that
// renders below the build steps as reference material.
type guide struct {
	Title   string     `yaml:"title"`
	Summary string     `yaml:"summary"`
	Intro   string     `yaml:"intro"`
	Steps   []stepSpec `yaml:"steps"`

	// MaintenanceCycleTicks is how often facilities consume their upkeep;
	// MaintenanceCycleBasis says where that figure came from. Zero falls back
	// to stationplan.DefaultMaintenanceCycleTicks, labelled "default".
	MaintenanceCycleTicks int    `yaml:"maintenance_cycle_ticks"`
	MaintenanceCycleBasis string `yaml:"maintenance_cycle_basis"`
	// ShowPerDay adds per-day upkeep figures beside the per-cycle ones. Off
	// until the maintenance cycle length is measured; per-cycle is exact.
	ShowPerDay bool `yaml:"show_per_day"`

	slug string // file name without .md
	file string // file name, for error messages
	body string // markdown after the frontmatter
}

// stepSpec is one build step. Facility empty means a non-facility step
// (founding, or a placeholder not yet charted). Alternatives are shown on the
// card but never counted in the totals; Facility is the default.
type stepSpec struct {
	Title        string    `yaml:"title"`
	Facility     string    `yaml:"facility"`
	Alternatives []string  `yaml:"alternatives"`
	Facts        []fact    `yaml:"facts"`
	Notes        string    `yaml:"notes"`
	Measured     *measured `yaml:"measured"`
	// BreakBefore is markdown shown as a divider before this step's card,
	// e.g. where a strict build order gives way to a flexible one.
	BreakBefore string `yaml:"break_before"`
}

// fact is a hand-stated figure for a non-facility step, with its basis
// (measured / doc-quoted / player-reported and a date). Credits, when set,
// count toward the build-credit total.
type fact struct {
	Label   string `yaml:"label"`
	Value   string `yaml:"value"`
	Basis   string `yaml:"basis"`
	Credits int    `yaml:"credits"`
}

// measured is upkeep observed in the game, shown beside the catalog value.
type measured struct {
	Date           string         `yaml:"date"`
	Source         string         `yaml:"source"`
	UpkeepPerCycle map[string]int `yaml:"upkeep_per_cycle"`
}

// loadGuide reads and parses one guide file.
func loadGuide(path, file string) (*guide, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	front, body := splitFrontmatter(raw)
	if front == nil {
		return nil, fmt.Errorf("%s: missing YAML frontmatter", file)
	}
	g := &guide{}
	if err := yaml.Unmarshal(front, g); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if g.Title == "" {
		return nil, fmt.Errorf("%s: frontmatter needs a title", file)
	}
	if g.MaintenanceCycleTicks <= 0 {
		g.MaintenanceCycleTicks = stationplan.DefaultMaintenanceCycleTicks
		g.MaintenanceCycleBasis = "default, player-reported, unconfirmed"
	}
	g.file = file
	g.slug = strings.TrimSuffix(file, ".md")
	g.body = body
	return g, nil
}

// splitFrontmatter separates a leading "---\n...\n---\n" YAML block from the
// markdown body (same contract as cmd/generate-factions-kb's overlays).
func splitFrontmatter(content []byte) (front []byte, body string) {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, s
	}
	rest := s[len("---\n"):]
	head, tail, found := strings.Cut(rest, "\n---\n")
	if !found {
		if strings.HasSuffix(rest, "\n---") {
			return []byte(rest[:len(rest)-len("\n---")]), ""
		}
		return nil, s
	}
	return []byte(head), tail
}
