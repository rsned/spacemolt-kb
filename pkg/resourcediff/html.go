package resourcediff

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// The Resources page is our own generator's output (see
// cmd/generate-items-kb/resources.go), so its structure is fixed and a
// handful of anchored patterns read it back reliably.
var (
	reSection = regexp.MustCompile(`(?s)<div id="[^"]*" class="resource-section"[^>]*>\s*<h3>(.*?)</h3>(.*?)\n        </div>`)
	reDetails = regexp.MustCompile(`<a href="\.\./items/([^/]+)/([^/]+)\.html">Details</a>`)
	reRow     = regexp.MustCompile(`(?s)<tr>\s*(<td>.*?)</tr>`)
	reCell    = regexp.MustCompile(`(?s)<td( class="[^"]*")?>(.*?)</td>`)
	reSysLink = regexp.MustCompile(`<a href="\.\./systems/([^/]+)/index\.html">(.*?)</a>`)
	reCode    = regexp.MustCompile(`<code>(.*?)</code>`)
	reCardNum = regexp.MustCompile(`(?s)<div class="num">([^<]*)</div>\s*<div class="label">([^<]*)</div>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
)

// FromHTML parses a generated kb/resources/index.html into a Snapshot. The
// caller sets Date and ServerVersion.
func FromHTML(page []byte) (*Snapshot, error) {
	src := string(page)
	snap := &Snapshot{Source: SourceHTML}

	for _, m := range reCardNum.FindAllStringSubmatch(src, -1) {
		val := strings.TrimSpace(m[1])
		n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSuffix(val, "%"), ",", ""))
		switch strings.TrimSpace(m[2]) {
		case "Resource Types":
			snap.Summary.Types = n
		case "Total Deposits":
			snap.Summary.Deposits = n
		case "Star Systems":
			snap.Summary.Systems = n
		case "Systems Explored":
			snap.Summary.Explored = n
		}
	}

	sections := reSection.FindAllStringSubmatch(src, -1)
	if len(sections) == 0 {
		return nil, errors.New("no resource sections found: not a generated Resources page?")
	}
	for _, sec := range sections {
		heading, body := sec[1], sec[2]
		det := reDetails.FindStringSubmatch(heading)
		if det == nil {
			return nil, fmt.Errorf("resource section without a Details link: %q", stripTags(heading))
		}
		rt := ResourceType{Category: det[1], ID: det[2]}
		// The name is the heading text before the badge.
		name := heading
		if i := strings.Index(name, "<span"); i >= 0 {
			name = name[:i]
		}
		rt.Name = html.UnescapeString(strings.TrimSpace(name))
		snap.Types = append(snap.Types, rt)

		for _, row := range reRow.FindAllStringSubmatch(body, -1) {
			cells := reCell.FindAllStringSubmatch(row[1], -1)
			if len(cells) != 12 && len(cells) != 13 {
				return nil, fmt.Errorf("%s: deposit row has %d cells, want 12 or 13", rt.ID, len(cells))
			}
			d, err := parseRow(cells)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rt.ID, err)
			}
			if d.ResourceID != rt.ID {
				return nil, fmt.Errorf("%s: row lists resource %s", rt.ID, d.ResourceID)
			}
			snap.Deposits = append(snap.Deposits, d)
		}
	}
	snap.normalize()
	return snap, nil
}

// parseRow decodes the cells of a deposit row: system, system id, station,
// poi, poi id, hidden, resource id, richness, max amount, remaining,
// depletion, [supported power,] last updated tick. The max-amount cell only
// counts as a capacity when it is marked class="cap"; unmarked values are
// the page's "highest observed" estimate.
func parseRow(cells [][]string) (Deposit, error) {
	var d Deposit
	cell := func(i int) string { return cells[i][2] }
	sys := reSysLink.FindStringSubmatch(cell(0))
	if sys == nil {
		return d, fmt.Errorf("no system link in %q", cell(0))
	}
	d.SystemID = sys[1]
	d.SystemName = html.UnescapeString(sys[2])
	d.Station = strings.Contains(cell(2), "badge-green")
	d.POIName = html.UnescapeString(strings.TrimSpace(stripTags(cell(3))))
	d.POIID = codeText(cell(4))
	d.Hidden = strings.Contains(cell(5), "badge-yellow")
	d.ResourceID = codeText(cell(6))
	if d.POIID == "" || d.ResourceID == "" {
		return d, fmt.Errorf("missing poi/resource id in row for %s", d.SystemID)
	}
	var err error
	if d.Richness, err = parseInt(cell(7)); err != nil {
		return d, fmt.Errorf("richness: %w", err)
	}
	if strings.Contains(cells[8][1], "cap") {
		if d.MaxRemaining, err = parseInt(cell(8)); err != nil {
			return d, fmt.Errorf("max amount: %w", err)
		}
	}
	if d.Remaining, err = parseInt(cell(9)); err != nil {
		return d, fmt.Errorf("remaining: %w", err)
	}
	if tick := strings.TrimSpace(stripTags(cell(len(cells) - 1))); tick != "-" {
		if d.LastTick, err = parseInt(tick); err != nil {
			return d, fmt.Errorf("tick: %w", err)
		}
	}
	return d, nil
}

func codeText(cell string) string {
	if m := reCode.FindStringSubmatch(cell); m != nil {
		return html.UnescapeString(m[1])
	}
	return ""
}

func stripTags(s string) string { return reTag.ReplaceAllString(s, "") }

func parseInt(s string) (int, error) {
	s = strings.ReplaceAll(strings.TrimSpace(stripTags(s)), ",", "")
	return strconv.Atoi(s)
}
