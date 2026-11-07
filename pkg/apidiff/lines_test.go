package apidiff

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
)

var tagRE = regexp.MustCompile(`<[^>]+>`)

func plain(l Line) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(string(l.HTML), ""))), " ")
}

func onlyDiff(t *testing.T, s Step) StructDiff {
	t.Helper()
	if len(s.Diffs) == 0 {
		t.Fatal("no diffs")
	}
	return s.Diffs[0]
}

func find(lines []Line, kind LineKind, text string) bool {
	for _, l := range lines {
		if l.Kind == kind && plain(l) == text {
			return true
		}
	}
	return false
}

func dump(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "%d %q %s\n", l.Kind, plain(l), l.Label)
	}
	return b.String()
}

func TestGoName(t *testing.T) {
	for in, want := range map[string]string{"ally-battle": "AllyBattle", "2fa_code": "F2faCode", "x": "X", "base_id": "BaseID", "xp_gained": "XPGained"} {
		if got := GoName(in); got != want {
			t.Errorf("GoName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStructLinesGoAdd(t *testing.T) {
	d := onlyDiff(t, step(t, `{"E":{"type":"object","properties":{"a":{"type":"string"}}}}`,
		`{"E":{"type":"object","properties":{"a":{"type":"string"},"flagship_bounty":{"type":"integer"}}}}`))
	lines := StructLines(d, "go")
	if !find(lines, LineAdd, "FlagshipBounty int `json:\"flagship_bounty,omitempty\"`") {
		t.Errorf("no added Go field line:\n%s", dump(lines))
	}
}

func TestStructLinesTSNullable(t *testing.T) {
	d := onlyDiff(t, step(t, `{"B":{"type":"object","properties":{"a":{"type":"string"}}}}`,
		`{"B":{"type":"object","properties":{"a":{"type":"string"},"contraband_items":{"type":["array","null"],"items":{"type":"string"}}}}}`))
	if lines := StructLines(d, "ts"); !find(lines, LineAdd, "contraband_items?: string[] | null;") {
		t.Errorf("no nullable TS line:\n%s", dump(lines))
	}
}

func TestStructLinesEnumBlock(t *testing.T) {
	d := onlyDiff(t, step(t, `{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a","b","c","d","e","f","g","h","boss_flagship_copy"]}}}}`,
		`{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a","b","c","d","e","f","g","h"]}}}}`))
	for _, lang := range []Lang{"go", "ts"} {
		lines := StructLines(d, lang)
		var del, fold bool
		for _, l := range lines {
			del = del || (l.Kind == LineDel && strings.Contains(plain(l), `"boss_flagship_copy"`))
			fold = fold || (l.Kind == LineFold && l.Label == "6 unchanged values")
		}
		if !del || !fold {
			t.Errorf("%s: del=%v fold=%v\n%s", lang, del, fold, dump(lines))
		}
	}
}

func TestStructLinesFold(t *testing.T) {
	var props []string
	for i := range 20 {
		props = append(props, fmt.Sprintf(`"a%02d":{"type":"string"}`, i))
	}
	old := `{"P":{"type":"object","properties":{` + strings.Join(props, ",") + `}}}`
	props = append(props, `"a17z":{"type":"string"}`)
	d := onlyDiff(t, step(t, old, `{"P":{"type":"object","properties":{`+strings.Join(props, ",")+`}}}`))
	lines := StructLines(d, "go")
	var kinds []string
	for _, l := range lines {
		switch l.Kind {
		case LineFold:
			kinds = append(kinds, l.Label)
		case LineCtx:
			kinds = append(kinds, "ctx")
		case LineAdd:
			kinds = append(kinds, "add")
		case LineHdr:
			kinds = append(kinds, "hdr")
		}
	}
	want := "hdr 16 unchanged fields ctx ctx add ctx ctx hdr"
	if got := strings.Join(kinds, " "); got != want {
		t.Errorf("layout = %q, want %q", got, want)
	}
}

func TestStructLinesNoFalseDiffFromAlignment(t *testing.T) {
	d := onlyDiff(t, step(t, `{"P":{"type":"object","properties":{"a":{"type":"string"}}}}`,
		`{"P":{"type":"object","properties":{"a":{"type":"string"},"very_long_field_name_here":{"type":"string"}}}}`))
	for _, l := range StructLines(d, "go") {
		if (l.Kind == LineAdd || l.Kind == LineDel) && strings.HasPrefix(plain(l), "A ") {
			t.Errorf("unchanged field a shows as %d", l.Kind)
		}
	}
}

func TestStructLinesEscaping(t *testing.T) {
	d := onlyDiff(t, step(t, `{"P":{"type":"object","properties":{"a":{}}}}`,
		`{"P":{"type":"object","properties":{"a":{},"b":{"type":"string","description":"<script>x</script> */ `+"`tag`"+` & co"}}}}`))
	for _, lang := range []Lang{"go", "ts"} {
		for _, l := range StructLines(d, lang) {
			if strings.Contains(string(l.HTML), "<script") {
				t.Errorf("%s: raw script tag in %q", lang, l.HTML)
			}
		}
	}
}

func TestStructLinesUnion(t *testing.T) {
	base := `"A":{"type":"object","properties":{"a":{}}},"D":{"type":"object","properties":{"d":{}}}`
	d := diffFor(t, step(t, `{`+base+`,"U":{"oneOf":[{"$ref":"#/components/schemas/A"}]}}`,
		`{`+base+`,"U":{"oneOf":[{"$ref":"#/components/schemas/A"},{"$ref":"#/components/schemas/D"}]}}`), "U")
	if lines := StructLines(d, "ts"); !find(lines, LineAdd, "| D") {
		t.Errorf("no added union member:\n%s", dump(lines))
	}
	if lines := StructLines(d, "go"); !find(lines, LineAdd, "// D") {
		t.Errorf("no added Go union member:\n%s", dump(lines))
	}
}

func TestStructLinesEnumRemovedEntirely(t *testing.T) {
	d := onlyDiff(t, step(t, `{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a","b"]}}}}`,
		`{"G":{"type":"object","properties":{"s":{"type":"string"}}}}`))
	for _, lang := range []Lang{"go", "ts"} {
		lines := StructLines(d, lang)
		for _, l := range lines {
			p := plain(l)
			if p == "export type =" || (strings.HasSuffix(p, `= "a"`) && !strings.HasPrefix(p, "GSA GS")) {
				t.Errorf("%s: nameless enum line %q", lang, p)
			}
		}
	}
	ts := StructLines(d, "ts")
	if !find(ts, LineDel, "s?: GS;") || !find(ts, LineAdd, "s?: string;") {
		t.Errorf("enum removal should show the field as retyped:\n%s", dump(ts))
	}
}

func TestStructLinesEnumAddedToField(t *testing.T) {
	d := onlyDiff(t, step(t, `{"G":{"type":"object","properties":{"s":{"type":"string"}}}}`,
		`{"G":{"type":"object","properties":{"s":{"type":"string","enum":["a"]}}}}`))
	ts := StructLines(d, "ts")
	if !find(ts, LineDel, "s?: string;") || !find(ts, LineAdd, "s?: GS;") || !find(ts, LineHdr, "export type GS =") {
		t.Errorf("enum addition should show a del/add pair and a named block:\n%s", dump(ts))
	}
}
