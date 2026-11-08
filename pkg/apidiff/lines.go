package apidiff

import (
	"fmt"
	htmltpl "html/template"
	"strconv"
	"strings"
	"unicode"
)

// Lang selects the rendered form.
type Lang string

// Languages.
const (
	LangGo Lang = "go"
	LangTS Lang = "ts"
)

// Line is one rendered line of a struct card.
type Line struct {
	Kind   LineKind
	HTML   htmltpl.HTML // escaped, with <span class="k|t|n|s|c"> and <mark> markup
	Folded []Line       // LineFold only
	Label  string       // LineFold only
	unit   string       // "field", "value" or "member": what a folded run counts
}

const (
	commentWidth = 84
	foldContext  = 2
	foldMin      = 3
)

var initialisms = map[string]string{"id": "ID", "url": "URL", "xp": "XP", "api": "API", "poi": "POI", "hp": "HP", "npc": "NPC"}

// GoName turns a JSON name into an exported Go identifier.
func GoName(s string) string {
	var b strings.Builder
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if up, ok := initialisms[strings.ToLower(p)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	out := b.String()
	if out == "" {
		return "X"
	}
	if unicode.IsDigit(rune(out[0])) {
		return "F" + out
	}
	return out
}

// typeName turns a struct ID into a type name: "P.rewards" → "PRewards",
// "P.list[]" → "PList", "P#v1" → "PV1". Schema names are kept as they are.
func typeName(id string) string {
	id = strings.NewReplacer("[]", "", "{}", "").Replace(id)
	parts := strings.Split(id, ".")
	var b strings.Builder
	for i, p := range parts {
		head, v, hasV := strings.Cut(p, "#v")
		if i == 0 {
			b.WriteString(head)
		} else {
			b.WriteString(GoName(head))
		}
		if hasV {
			b.WriteString("V" + v)
		}
	}
	return b.String()
}

func esc(s string) string { return htmltpl.HTMLEscapeString(s) }

func span(class, s string) string { return `<span class="` + class + `">` + esc(s) + `</span>` }

type renderer struct {
	lang  Lang
	owner string // type name of the struct being rendered
	nameW int
	typeW int
}

func (r renderer) typeStr(t TypeRef, enumType string) string {
	if r.lang == LangGo {
		return r.goType(t, enumType)
	}
	s := r.tsType(t, enumType)
	if t.Nullable {
		s += " | null"
	}
	return s
}

func (r renderer) goType(t TypeRef, enumType string) string {
	if enumType != "" {
		return enumType
	}
	switch t.Kind {
	case KindScalar:
		return map[string]string{"integer": "int", "number": "float64", "boolean": "bool", "string": "string"}[t.Scalar]
	case KindArray:
		return "[]" + r.goType(elem(t), "")
	case KindMap:
		return "map[string]" + r.goType(elem(t), "")
	case KindRef, KindNested:
		return typeName(t.Ref)
	}
	return "any"
}

func (r renderer) tsType(t TypeRef, enumType string) string {
	if enumType != "" {
		return enumType
	}
	switch t.Kind {
	case KindScalar:
		return map[string]string{"integer": "number", "number": "number", "boolean": "boolean", "string": "string"}[t.Scalar]
	case KindArray:
		e := r.tsType(elem(t), "")
		if elem(t).Nullable {
			e = "(" + e + " | null)"
		}
		return e + "[]"
	case KindMap:
		return "Record<string, " + r.tsType(elem(t), "") + ">"
	case KindRef, KindNested:
		return typeName(t.Ref)
	}
	return "unknown"
}

func elem(t TypeRef) TypeRef {
	if t.Elem == nil {
		return TypeRef{Kind: KindAny}
	}
	return *t.Elem
}

func (r renderer) enumType(f *Field) string {
	if len(f.Enum) == 0 {
		return ""
	}
	return r.owner + GoName(f.Name)
}

// code renders a field's declaration line.
func (r renderer) code(f *Field) string {
	t := r.typeStr(f.Type, r.enumType(f))
	if r.lang == LangGo {
		if !f.Required && f.Type.Kind == KindRef && !strings.HasPrefix(t, "*") {
			t = "*" + t
		}
		tag := "`json:\"" + f.Name
		if !f.Required {
			tag += ",omitempty"
		}
		tag += "\"`"
		line := "    " + span("n", pad(GoName(f.Name), r.nameW)) + " " + span("t", pad(t, r.typeW)) + " " + span("s", tag)
		if f.Type.Nullable {
			// Go types can't say a slice, map or value may be null; the
			// TypeScript form shows it as "| null".
			line += " " + span("c", "// nullable")
		}
		return line
	}
	q := ""
	if !f.Required {
		q = "?"
	}
	return "    " + span("n", f.Name+q) + ": " + span("t", t) + ";"
}

func pad(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// comment wraps tokens into "// " lines at commentWidth.
func comment(indent string, toks []Token) []string {
	var lines []string
	var cur strings.Builder
	width := 0
	flush := func() {
		if width > 0 {
			h := strings.ReplaceAll(cur.String(), "</mark> <mark>", " ")
			lines = append(lines, indent+`<span class="c">// `+h+`</span>`)
		}
		cur.Reset()
		width = 0
	}
	for _, t := range toks {
		if width > 0 && width+1+len(t.Text) > commentWidth {
			flush()
		}
		if width > 0 {
			cur.WriteString(" ")
			width++
		}
		if t.Marked {
			cur.WriteString("<mark>" + esc(t.Text) + "</mark>")
		} else {
			cur.WriteString(esc(t.Text))
		}
		width += len(t.Text)
	}
	flush()
	return lines
}

func plainComment(indent, s string) []string { return comment(indent, tokens(strings.Fields(s))) }

// StructLines renders a struct diff in lang, folded.
func StructLines(d StructDiff, lang Lang) []Line {
	cur := d.New
	if cur == nil {
		cur = d.Old
	}
	r := renderer{lang: lang, owner: typeName(cur.ID)}
	for _, fc := range d.Fields {
		for _, f := range []*Field{fc.Old, fc.New} {
			if f == nil {
				continue
			}
			r.nameW = max(r.nameW, len(GoName(f.Name)))
			t := r.goType(f.Type, r.enumType(f))
			if !f.Required && f.Type.Kind == KindRef {
				t = "*" + t
			}
			r.typeW = max(r.typeW, len(t))
		}
	}
	whole := LineHdr
	switch {
	case d.Old == nil:
		whole = LineAdd
	case d.New == nil:
		whole = LineDel
	}
	var out []Line
	add := func(k LineKind, h, unit string) {
		if whole != LineHdr {
			k = whole // a new or removed struct is wholly green or red
		}
		out = append(out, Line{Kind: k, HTML: htmltpl.HTML(h), unit: unit}) //nolint:gosec // built from escaped parts
	}
	if d.DescChanged {
		for _, row := range DescDiff(d.Old.Desc, d.New.Desc) {
			if row.Kind == LineCtx {
				add(LineCtx, `<span class="c">// …</span>`, "")
				continue
			}
			for _, h := range comment("", row.Tokens) {
				add(row.Kind, h, "")
			}
		}
	}
	if len(cur.Fields) > 0 || len(d.Fields) > 0 {
		if lang == LangGo {
			add(LineHdr, span("k", "type")+" "+span("t", r.owner)+" "+span("k", "struct")+" {", "")
		} else {
			add(LineHdr, span("k", "export interface")+" "+span("t", r.owner)+" {", "")
		}
		var enums []FieldChange
		for _, fc := range d.Fields {
			switch {
			case fc.Old == nil:
				for _, h := range plainComment("    ", fc.New.Desc) {
					add(LineAdd, h, "")
				}
				add(LineAdd, r.code(fc.New), "field")
			case fc.New == nil:
				for _, h := range plainComment("    ", fc.Old.Desc) {
					add(LineDel, h, "")
				}
				add(LineDel, r.code(fc.Old), "field")
			default:
				if fc.Reword {
					for _, row := range DescDiff(fc.Old.Desc, fc.New.Desc) {
						if row.Kind == LineCtx {
							add(LineCtx, `    <span class="c">// …</span>`, "")
							continue
						}
						for _, h := range comment("    ", row.Tokens) {
							add(row.Kind, h, "")
						}
					}
				}
				if fc.Retyped {
					add(LineDel, r.code(fc.Old), "field")
					add(LineAdd, r.code(fc.New), "field")
				} else {
					add(LineCtx, r.code(fc.New), "field")
				}
				if len(fc.EnumAdded) > 0 || len(fc.EnumRemoved) > 0 {
					enums = append(enums, fc)
				}
			}
		}
		add(LineHdr, "}", "")
		for _, fc := range enums {
			out = append(out, r.enumBlock(fc)...)
		}
	}
	if len(cur.Members) > 0 || len(d.MembersAdded) > 0 || len(d.MembersRemoved) > 0 {
		if len(out) > 0 {
			add(LineHdr, "", "")
		}
		if lang == LangGo {
			add(LineHdr, `<span class="c">// `+esc(r.owner)+` is one of:</span>`, "")
		} else {
			add(LineHdr, span("k", "export type")+" "+span("t", r.owner)+" =", "")
		}
		member := func(k LineKind, name string) {
			if lang == LangGo {
				add(k, `<span class="c">//   `+esc(name)+`</span>`, "member")
			} else {
				add(k, "    | "+span("t", name), "member")
			}
		}
		if d.Old == nil || d.New == nil {
			for _, m := range cur.Members {
				member(whole, m)
			}
		} else {
			for _, m := range d.MembersKept {
				member(LineCtx, m)
			}
			for _, m := range d.MembersRemoved {
				member(LineDel, m)
			}
			for _, m := range d.MembersAdded {
				member(LineAdd, m)
			}
		}
		if lang == LangTS {
			add(LineHdr, ";", "")
		}
	}
	return fold(out)
}

func (r renderer) enumBlock(fc FieldChange) []Line {
	side := fc.New
	if len(side.Enum) == 0 {
		side = fc.Old // the enum was removed: name the block after the old type
	}
	t := r.enumType(side)
	lines := []Line{{Kind: LineHdr, HTML: ""}}
	if r.lang == LangGo {
		lines = append(lines, Line{Kind: LineHdr, HTML: htmltpl.HTML(span("k", "const") + " (")}) //nolint:gosec // escaped parts
	} else {
		lines = append(lines, Line{Kind: LineHdr, HTML: htmltpl.HTML(span("k", "export type") + " " + span("t", t) + " =")}) //nolint:gosec // escaped parts
	}
	removed := map[string]bool{}
	for _, v := range fc.EnumRemoved {
		removed[v] = true
	}
	added := map[string]bool{}
	for _, v := range fc.EnumAdded {
		added[v] = true
	}
	order := append([]string{}, fc.Old.Enum...)
	order = append(order, fc.EnumAdded...)
	for _, v := range order {
		k := LineCtx
		switch {
		case removed[v]:
			k = LineDel
		case added[v]:
			k = LineAdd
		}
		var h string
		if r.lang == LangGo {
			name := v
			if u, err := strconv.Unquote(v); err == nil {
				name = u
			}
			h = "    " + span("n", t+GoName(name)) + " " + span("t", t) + " = " + span("s", v)
		} else {
			h = "    | " + span("s", v)
		}
		lines = append(lines, Line{Kind: k, HTML: htmltpl.HTML(h), unit: "value"}) //nolint:gosec // escaped parts
	}
	if r.lang == LangGo {
		lines = append(lines, Line{Kind: LineHdr, HTML: ")"})
	} else {
		lines = append(lines, Line{Kind: LineHdr, HTML: ";"})
	}
	return lines
}

// fold collapses runs of unchanged lines more than foldContext lines from a
// change into one LineFold, when the run is at least foldMin lines long.
func fold(lines []Line) []Line {
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.Kind != LineCtx {
			keep[i] = true
		}
		if l.Kind == LineAdd || l.Kind == LineDel {
			for j := max(0, i-foldContext); j <= min(len(lines)-1, i+foldContext); j++ {
				keep[j] = true
			}
		}
	}
	var out []Line
	for i := 0; i < len(lines); {
		if keep[i] {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && !keep[j] {
			j++
		}
		run := lines[i:j]
		if len(run) < foldMin {
			out = append(out, run...)
		} else {
			out = append(out, Line{Kind: LineFold, Folded: run, Label: foldLabel(run)})
		}
		i = j
	}
	return out
}

func foldLabel(run []Line) string {
	counts := map[string]int{}
	for _, l := range run {
		if l.unit != "" {
			counts[l.unit]++
		}
	}
	unit, n := "line", len(run)
	for _, u := range []string{"field", "value", "member"} {
		if counts[u] > 0 {
			unit, n = u, counts[u]
			break
		}
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d unchanged %s", n, unit)
}
