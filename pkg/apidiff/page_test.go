package apidiff

import (
	"regexp"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/internal/kbnav"
)

func TestStepsSkipsBaselineAndEmpty(t *testing.T) {
	a := `{"P":{"type":"object","properties":{"a":{}}}}`
	b := `{"P":{"type":"object","properties":{"a":{},"b":{}}}}`
	c := `{"P":{"type":"object","properties":{"a":{},"b":{},"c":{}}}}`
	snaps := []Snapshot{{"20260210", schemas(t, a)}, {"20260217", schemas(t, b)}, {"20260301", schemas(t, c)}, {"20260302", schemas(t, c)}}
	steps := Steps(snaps, "20260217")
	if len(steps) != 1 || steps[0].From != "20260217" || steps[0].To != "20260301" {
		t.Fatalf("steps = %+v", steps)
	}
}

func sampleStep(t *testing.T) Step {
	return step(t, `{"S":{"type":"object","properties":{"a":{}}},"W":{"type":"object","properties":{"a":{"description":"old words"}}}}`,
		`{"S":{"type":"object","properties":{"a":{},"b":{}}},"W":{"type":"object","properties":{"a":{"description":"new words"}}}}`)
}

func TestRenderReportHeaderAndNav(t *testing.T) {
	out, err := RenderReport(sampleStep(t), "2026-02-01", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{kbnav.Header("../"), `href="2026-02-01.html"`, `data-l="go" aria-pressed="true"`, "may differ from <code>serverapi</code>"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	if strings.Contains(out, "&rarr;</a>") {
		t.Errorf("report has a next link with no next report")
	}
}

func TestRenderReportGroups(t *testing.T) {
	out, err := RenderReport(sampleStep(t), "", "")
	if err != nil {
		t.Fatal(err)
	}
	si, wi := strings.Index(out, "Structural changes"), strings.Index(out, "Wording only")
	if si < 0 || wi < 0 || si > wi {
		t.Errorf("group headings out of order: %d %d", si, wi)
	}
	if cards, links := strings.Count(out, `<article class="card"`), strings.Count(out, `<a class="toc-link" href="#`); cards != 2 || links != 2 {
		t.Errorf("cards = %d, toc links = %d, want 2 and 2", cards, links)
	}
}

func TestRenderIndex(t *testing.T) {
	s1, s2 := sampleStep(t), sampleStep(t)
	s1.To, s2.To = "20260301", "20260401"
	out, err := RenderIndex([]Step{s2, s1})
	if err != nil {
		t.Fatal(err)
	}
	if i, j := strings.Index(out, "2026-04-01"), strings.Index(out, "2026-03-01"); i < 0 || j < 0 || i > j {
		t.Errorf("rows not newest first")
	}
	for _, want := range []string{s1.Counts.Summary(), "History starts at 2026-02-17, when the spec first documented its schemas.", `href="../diffs/index.html"`, kbnav.Header("../")} {
		if !strings.Contains(out, want) {
			t.Errorf("index lacks %q", want)
		}
	}
}

// .page-content zeroes .container's side padding when both sit on <main>,
// which puts the report flush against the window edge.
func TestRenderReportKeepsContainerGutter(t *testing.T) {
	out, err := RenderReport(sampleStep(t), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `class="container page-content"`) || !strings.Contains(out, `<main class="container">`) {
		t.Errorf("report <main> must be a bare container with page-content inside it")
	}
}

func TestRenderReportAnchorsUnique(t *testing.T) {
	a := &Struct{ID: "X#v1", Display: "X (variant 1)", Fields: []Field{{Name: "a"}}}
	b := &Struct{ID: "X#v1", Display: "X (variant 1)", Fields: []Field{{Name: "b"}}}
	s := Step{From: "20260101", To: "20260102", Counts: Counts{Additions: 1, Removals: 1, Structs: 2},
		Diffs: []StructDiff{{Pair: Pair{Old: a}}, {Pair: Pair{New: b}}}}
	out, err := RenderReport(s, "", "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="(s-[^"]*)"`).FindAllStringSubmatch(out, -1) {
		if seen[m[1]] {
			t.Errorf("duplicate anchor %s", m[1])
		}
		seen[m[1]] = true
	}
}
