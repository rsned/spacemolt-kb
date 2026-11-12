package main

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Body is one point of interest in a system layout.
type Body struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"` // snake_case POI type, e.g. asteroid_belt
	Class string  `json:"class,omitempty"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

// Layout is one system's bodies at one point in time.
type Layout struct {
	Name   string `json:"name"`
	Bodies []Body `json:"bodies"`
}

// Baseline is the saved "before" side: every system as the KB showed it in
// the last regen before the v0.698 rescale.
type Baseline struct {
	Rev     string            `json:"rev"`
	Note    string            `json:"note"`
	Systems map[string]Layout `json:"systems"`
}

var (
	sysNameRe = regexp.MustCompile(`<h2 class="sys-name">([^<]*)`)
	poiTabRe  = regexp.MustCompile(`(?s)Points of Interest \(\d+\)</div>.*?<tbody>(.*?)</tbody>`)
	rowRe     = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	cellRe    = regexp.MustCompile(`(?s)<td[^>]*>(.*?)</td>`)
	tagRe     = regexp.MustCompile(`<[^>]+>`)
	posRe     = regexp.MustCompile(`^\((-?[\d.]+), (-?[\d.]+)\)$`)
)

func cellText(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRe.ReplaceAllString(s, "")))
}

// parseSystemPage reads a generated kb/systems/<id>/index.html: the system
// name and its Points of Interest table (name, type, class, position).
func parseSystemPage(page string) (Layout, error) {
	var l Layout
	if m := sysNameRe.FindStringSubmatch(page); m != nil {
		l.Name = html.UnescapeString(strings.TrimSpace(m[1]))
	}
	tab := poiTabRe.FindStringSubmatch(page)
	if tab == nil {
		return l, nil // a system with no recorded POIs
	}
	for _, r := range rowRe.FindAllStringSubmatch(tab[1], -1) {
		cells := cellRe.FindAllStringSubmatch(r[1], -1)
		if len(cells) < 6 {
			continue
		}
		pos := posRe.FindStringSubmatch(cellText(cells[5][1]))
		if pos == nil {
			continue
		}
		x, err1 := strconv.ParseFloat(pos[1], 64)
		y, err2 := strconv.ParseFloat(pos[2], 64)
		if err1 != nil || err2 != nil {
			return l, fmt.Errorf("bad position %q", cellText(cells[5][1]))
		}
		class := cellText(cells[3][1])
		if class == "—" {
			class = ""
		}
		l.Bodies = append(l.Bodies, Body{
			Name:  cellText(cells[1][1]),
			Type:  strings.ReplaceAll(strings.ToLower(cellText(cells[2][1])), " ", "_"),
			Class: class,
			X:     x, Y: y,
		})
	}
	return l, nil
}

// extractBaseline reads every system page as committed at rev.
func extractBaseline(rev string) (Baseline, error) {
	b := Baseline{Rev: rev, Systems: map[string]Layout{},
		Note: "System layouts as the KB showed them in the last regen before the v0.698.0 real-scale rescale. Positions are rounded to 0.1 AU, as on those pages."}
	out, err := exec.Command("git", "ls-tree", "-r", "--name-only", rev, "kb/systems/").Output()
	if err != nil {
		return b, fmt.Errorf("git ls-tree %s: %w", rev, err)
	}
	for f := range strings.FieldsSeq(string(out)) {
		if path.Base(f) != "index.html" || strings.Count(f, "/") != 3 {
			continue // only kb/systems/<id>/index.html
		}
		page, err := exec.Command("git", "show", rev+":"+f).Output()
		if err != nil {
			return b, fmt.Errorf("git show %s: %w", f, err)
		}
		l, err := parseSystemPage(string(page))
		if err != nil {
			return b, fmt.Errorf("%s: %w", f, err)
		}
		b.Systems[strings.Split(f, "/")[2]] = l
	}
	return b, nil
}

func loadBaseline(p string) (Baseline, error) {
	var b Baseline
	data, err := os.ReadFile(p)
	if err != nil {
		return b, err
	}
	return b, json.Unmarshal(data, &b)
}

func saveBaseline(p string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0o644)
}
