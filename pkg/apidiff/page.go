package apidiff

import (
	"bytes"
	"fmt"
	htmltpl "html/template"
	"regexp"
	"strings"
	"time"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

// Baseline is the first snapshot that documents its schemas; earlier specs
// keep almost everything inline in paths, so no report ends on or before it.
const Baseline = "20260217"

// Steps compares adjacent snapshots and returns the reportable steps, oldest
// first: those ending after baseline with at least one changed struct.
func Steps(snaps []Snapshot, baseline string) []Step {
	var out []Step
	for i := 1; i < len(snaps); i++ {
		if snaps[i].Date <= baseline {
			continue
		}
		if s := CompareStep(snaps[i-1], snaps[i]); s.Counts.Structs > 0 {
			out = append(out, s)
		}
	}
	return out
}

type card struct {
	Anchor, Title string
	Tags          []tag
	Go, TS        []Line
}

type tag struct{ Class, Text string }

var anchorUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func anchor(display string) string {
	return "s-" + strings.Trim(anchorUnsafe.ReplaceAllString(display, "-"), "-")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func tagsFor(d StructDiff) []tag {
	switch {
	case d.Old == nil:
		if n := len(d.New.Fields); n > 0 {
			return []tag{{"new", fmt.Sprintf("New type · %d %s", n, plural(n, "field", "fields"))}}
		}
		n := len(d.New.Members)
		return []tag{{"new", fmt.Sprintf("New type · %d %s", n, plural(n, "member", "members"))}}
	case d.New == nil:
		return []tag{{"del", "Removed type"}}
	}
	var add, rem, eAdd, eRem, retyped, words int
	for _, f := range d.Fields {
		switch {
		case f.Old == nil:
			add++
		case f.New == nil:
			rem++
		default:
			eAdd += len(f.EnumAdded)
			eRem += len(f.EnumRemoved)
			if f.Retyped {
				retyped++
			}
			if f.Reword {
				words++
			}
		}
	}
	if d.DescChanged {
		words++
	}
	var t []tag
	if add > 0 {
		t = append(t, tag{"add", fmt.Sprintf("+%d %s", add, plural(add, "field", "fields"))})
	}
	if rem > 0 {
		t = append(t, tag{"del", fmt.Sprintf("−%d %s", rem, plural(rem, "field", "fields"))})
	}
	if eAdd > 0 {
		t = append(t, tag{"add", fmt.Sprintf("+%d enum %s", eAdd, plural(eAdd, "value", "values"))})
	}
	if eRem > 0 {
		t = append(t, tag{"del", fmt.Sprintf("−%d enum %s", eRem, plural(eRem, "value", "values"))})
	}
	if n := len(d.MembersAdded); n > 0 {
		t = append(t, tag{"add", fmt.Sprintf("+%d %s", n, plural(n, "member", "members"))})
	}
	if n := len(d.MembersRemoved); n > 0 {
		t = append(t, tag{"del", fmt.Sprintf("−%d %s", n, plural(n, "member", "members"))})
	}
	if retyped > 0 {
		t = append(t, tag{"mod", fmt.Sprintf("%d retyped", retyped)})
	}
	if words > 0 {
		t = append(t, tag{"mod", fmt.Sprintf("%d %s", words, plural(words, "description", "descriptions"))})
	}
	return t
}

func longDate(d string) string {
	t, err := time.Parse("20060102", d)
	if err != nil {
		return d
	}
	return t.Format("January 2, 2006")
}

var funcs = htmltpl.FuncMap{
	"iso":  ISODate,
	"long": longDate,
	"sign": func(k LineKind) string {
		switch k {
		case LineAdd:
			return "+"
		case LineDel:
			return "−"
		}
		return " "
	},
	"cls": func(k LineKind) string {
		return map[LineKind]string{LineAdd: "add", LineDel: "del", LineHdr: "hdr"}[k]
	},
	"isFold": func(k LineKind) bool { return k == LineFold },
}

// RenderReport renders one step's page. prev and next are ISO dates of the
// neighbouring reports, or "".
func RenderReport(s Step, prev, next string) (string, error) {
	var structural, wording []card
	for _, d := range s.Diffs {
		c := card{Anchor: anchor(d.Display()), Title: d.Display(), Tags: tagsFor(d), Go: StructLines(d, LangGo), TS: StructLines(d, LangTS)}
		if d.Structural() {
			structural = append(structural, c)
		} else {
			wording = append(wording, c)
		}
	}
	return execute(reportTemplate, struct {
		Step                Step
		Summary             string
		Prev, Next          string
		Structural, Wording []card
		Header              htmltpl.HTML
	}{s, s.Counts.Summary(), prev, next, structural, wording, header()})
}

// RenderIndex renders the section index; steps are newest first.
func RenderIndex(steps []Step) (string, error) {
	return execute(indexTemplate, struct {
		Steps  []Step
		Header htmltpl.HTML
	}{steps, header()})
}

func header() htmltpl.HTML {
	return htmltpl.HTML(kbnav.Header("../")) //nolint:gosec // site header, generated internally
}

func execute(tpl string, data any) (string, error) {
	t, err := htmltpl.New("page").Funcs(funcs).Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return buf.String(), nil
}
