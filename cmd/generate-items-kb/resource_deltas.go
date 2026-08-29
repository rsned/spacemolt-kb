package main

import (
	"database/sql"
	"fmt"
	htmltpl "html/template"
	"log"
	"strings"

	"github.com/rsned/spacemolt-kb/pkg/resourcediff"
)

// resourceSnapshotDir is where generate-resource-diffs keeps its snapshots.
const resourceSnapshotDir = "data/resource-snapshots"

// resourceDeltas is what the Resources page shows about movement since the
// previous regen and since the content-patch baseline. Nil means no earlier
// snapshot exists, and the page renders without deltas.
type resourceDeltas struct {
	PrevDate    string
	BaseDate    string // "" when no baseline applies
	BaseVersion string // content patch the baseline tracks
	Prev, Base  map[string]resourcediff.ResourceDelta
	PrevSummary resourcediff.Summary
	BaseSummary resourcediff.Summary
	Cur         resourcediff.Summary
}

// loadResourceDeltas diffs the live DB against the snapshots on disk. today
// is the regen date (YYYY-MM-DD): the previous snapshot is the newest one
// strictly before it, so running generate-resource-diffs before or after the
// regen on the same day makes no difference.
func loadResourceDeltas(db *sql.DB, snapDir, today string) (*resourceDeltas, error) {
	man, err := resourcediff.LoadManifest(snapDir)
	if err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	prev := man.PreviousBefore(today)
	if prev == nil {
		return nil, nil
	}
	cur, err := resourcediff.FromDB(db)
	if err != nil {
		return nil, fmt.Errorf("snapshot db: %w", err)
	}
	prevSnap, err := resourcediff.LoadSnapshot(snapDir, prev.Date)
	if err != nil {
		return nil, fmt.Errorf("load snapshot %s: %w", prev.Date, err)
	}
	d := &resourceDeltas{PrevDate: prev.Date, Cur: cur.Summary, PrevSummary: prevSnap.Summary}
	d.Prev = resourcediff.Diff(prevSnap, cur).PerResource()

	if base := man.BaselineFor(today); base != nil {
		d.BaseDate = base.Date
		d.BaseVersion = base.ContentVersion
		if base.Date == prev.Date {
			d.Base = d.Prev
			d.BaseSummary = d.PrevSummary
		} else {
			baseSnap, err := resourcediff.LoadSnapshot(snapDir, base.Date)
			if err != nil {
				return nil, fmt.Errorf("load baseline snapshot %s: %w", base.Date, err)
			}
			d.Base = resourcediff.Diff(baseSnap, cur).PerResource()
			d.BaseSummary = baseSnap.Summary
		}
	}
	return d, nil
}

// HasBase reports whether the baseline is a different reference point than
// the previous regen (when they coincide, one delta says it all).
func (d *resourceDeltas) HasBase() bool {
	return d != nil && d.BaseDate != "" && d.BaseDate != d.PrevDate
}

// deltaHTML renders a signed count as a colored span, or nothing for zero.
func deltaHTML(n int, suffix string) string {
	if n == 0 {
		return ""
	}
	class, sign := "up", "+"
	if n < 0 {
		class, sign = "down", "&minus;"
		n = -n
	}
	return fmt.Sprintf(`<span class="delta %s">%s%d%s</span>`, class, sign, n, suffix)
}

// tocHTML is the compact annotation for the jump list and map dropdown:
// the net change since the previous regen, or a "new" tag for a resource
// discovered (or added to the catalog) since the baseline.
func (d *resourceDeltas) tocHTML(resourceID string) string {
	if d == nil {
		return ""
	}
	p := d.Prev[resourceID]
	b := d.Base[resourceID]
	var s string
	if n := p.Net(); n != 0 {
		title := fmt.Sprintf("%+d since last regen (%s)", n, d.PrevDate)
		if d.HasBase() {
			title += fmt.Sprintf("; %+d since patch v%s", b.Net(), d.BaseVersion)
		}
		s = " " + strings.Replace(deltaHTML(n, ""), `<span `, `<span title="`+htmltpl.HTMLEscapeString(title)+`" `, 1)
	}
	if p.Discovered || b.Discovered || p.NewType || b.NewType {
		s += ` <span class="delta new">new</span>`
	}
	return s
}

// tocText is the plain-text form of tocHTML, for the map <option> labels
// where markup is not allowed: " +4", " -2", " +1 new", or "".
func (d *resourceDeltas) tocText(resourceID string) string {
	if d == nil {
		return ""
	}
	p := d.Prev[resourceID]
	b := d.Base[resourceID]
	var s string
	if n := p.Net(); n != 0 {
		s = fmt.Sprintf(" %+d", n)
	}
	if p.Discovered || b.Discovered || p.NewType || b.NewType {
		s += " new"
	}
	return s
}

// badgeHTML is the fuller annotation for a resource section heading.
func (d *resourceDeltas) badgeHTML(resourceID string) string {
	if d == nil {
		return ""
	}
	var parts []string
	if h := deltaHTML(d.Prev[resourceID].Net(), " since "+d.PrevDate); h != "" {
		parts = append(parts, h)
	}
	if d.HasBase() {
		if h := deltaHTML(d.Base[resourceID].Net(), " since v"+d.BaseVersion); h != "" {
			parts = append(parts, h)
		}
	}
	if d.Prev[resourceID].Discovered || d.Base[resourceID].Discovered {
		parts = append(parts, `<span class="delta new">newly discovered</span>`)
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

// summaryHTML annotates a summary card: since the previous regen, and since
// the baseline when that is a different point.
func (d *resourceDeltas) summaryHTML(prev, base, cur int) string {
	if d == nil {
		return ""
	}
	s := deltaHTML(cur-prev, "")
	if d.HasBase() {
		if h := deltaHTML(cur-base, ""); h != "" {
			s += ` <span class="delta-sep">/</span> ` + h
		}
	}
	if s == "" {
		return ""
	}
	return ` <span class="card-delta">` + s + `</span>`
}

// resourceDeltaFuncs exposes the deltas to the page template. Every func is
// safe on a nil receiver, so a first regen renders a plain page.
func resourceDeltaFuncs(d *resourceDeltas) htmltpl.FuncMap {
	return htmltpl.FuncMap{
		"deltaTOC":   func(id string) htmltpl.HTML { return htmltpl.HTML(d.tocHTML(id)) },   //nolint:gosec // generated internally
		"deltaBadge": func(id string) htmltpl.HTML { return htmltpl.HTML(d.badgeHTML(id)) }, //nolint:gosec // generated internally
		"deltaText":  d.tocText,
		"deltaTypes": func() htmltpl.HTML {
			if d == nil {
				return ""
			}
			return htmltpl.HTML(d.summaryHTML(d.PrevSummary.Types, d.BaseSummary.Types, d.Cur.Types)) //nolint:gosec // generated internally
		},
		"deltaDeposits": func() htmltpl.HTML {
			if d == nil {
				return ""
			}
			return htmltpl.HTML(d.summaryHTML(d.PrevSummary.Deposits, d.BaseSummary.Deposits, d.Cur.Deposits)) //nolint:gosec // generated internally
		},
		"deltaExplored": func() htmltpl.HTML {
			if d == nil {
				return ""
			}
			return htmltpl.HTML(d.summaryHTML(d.PrevSummary.Explored, d.BaseSummary.Explored, d.Cur.Explored)) //nolint:gosec // generated internally
		},
	}
}

// resourceDeltasOrNil loads deltas, degrading to none (with a log line)
// rather than failing the page.
func resourceDeltasOrNil(db *sql.DB, snapDir, today string) *resourceDeltas {
	d, err := loadResourceDeltas(db, snapDir, today)
	if err != nil {
		log.Printf("warning: resource deltas unavailable: %v", err)
		return nil
	}
	if d == nil {
		log.Printf("Resources: no earlier snapshot in %s; page rendered without deltas", snapDir)
	}
	return d
}
