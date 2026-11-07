package apidiff

import (
	"regexp"
	"strings"
)

// LineKind is how a rendered line takes part in a diff.
type LineKind int

// Line kinds.
const (
	LineCtx  LineKind = iota // unchanged context
	LineAdd                  // added (green)
	LineDel                  // removed (red)
	LineHdr                  // structural line, never folded: type headers, braces
	LineFold                 // a folded run of unchanged lines
)

// Token is one word of a description; Marked words changed.
type Token struct {
	Text   string
	Marked bool
}

// DescRow is one side of a description change, before wrapping. A LineCtx row
// stands for unchanged sentences and renders as "…".
type DescRow struct {
	Kind   LineKind
	Tokens []Token
}

// trimContext is how many unchanged words are kept either side of a change
// inside a long sentence.
const trimContext = 6

// markSimilarity is the word similarity below which a rewrite is shown whole,
// without word marks.
const markSimilarity = 0.35

var sentenceEnd = regexp.MustCompile(`([.!?;:])\s+`)

func sentences(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Split(sentenceEnd.ReplaceAllString(s, "$1\x00"), "\x00")
}

// DescDiff shows what changed between two descriptions: unchanged sentences
// collapse to one context row, changed ones become a removed and an added row
// with the changed words marked.
func DescDiff(old, new string) []DescRow {
	a, b := sentences(old), sentences(new)
	var rows []DescRow
	for _, op := range diffOps(a, b) {
		if op.equal {
			if len(rows) == 0 || rows[len(rows)-1].Kind != LineCtx {
				rows = append(rows, DescRow{Kind: LineCtx})
			}
			continue
		}
		x := strings.Fields(strings.Join(a[op.i1:op.i2], " "))
		y := strings.Fields(strings.Join(b[op.j1:op.j2], " "))
		mx, my := tokens(x), tokens(y)
		if len(x) > 0 && len(y) > 0 {
			wops := diffOps(x, y)
			if similarity(wops, len(x), len(y)) >= markSimilarity {
				for _, w := range wops {
					if w.equal {
						continue
					}
					for i := w.i1; i < w.i2; i++ {
						mx[i].Marked = true
					}
					for j := w.j1; j < w.j2; j++ {
						my[j].Marked = true
					}
				}
				p, q := commonEnds(x, y)
				mx, my = trim(mx, p, q), trim(my, p, q)
			}
		}
		if len(mx) > 0 {
			rows = append(rows, DescRow{Kind: LineDel, Tokens: mx})
		}
		if len(my) > 0 {
			rows = append(rows, DescRow{Kind: LineAdd, Tokens: my})
		}
	}
	return rows
}

func tokens(words []string) []Token {
	t := make([]Token, len(words))
	for i, w := range words {
		t[i] = Token{Text: w}
	}
	return t
}

func commonEnds(x, y []string) (p, q int) {
	n := min(len(x), len(y))
	for p < n && x[p] == y[p] {
		p++
	}
	for q < n-p && x[len(x)-1-q] == y[len(y)-1-q] {
		q++
	}
	return p, q
}

// trim keeps trimContext words either side of the changed middle.
func trim(t []Token, p, q int) []Token {
	st := max(0, p-trimContext)
	en := len(t) - max(0, q-trimContext)
	var out []Token
	if st > 0 {
		out = append(out, Token{Text: "…"})
	}
	out = append(out, t[st:en]...)
	if en < len(t) {
		out = append(out, Token{Text: "…"})
	}
	return out
}

type op struct {
	equal          bool
	i1, i2, j1, j2 int
}

// diffOps is a longest-common-subsequence diff of two string slices, as
// runs of equal and changed elements.
func diffOps(a, b []string) []op {
	n, m := len(a), len(b)
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	var ops []op
	push := func(eq bool, i1, i2, j1, j2 int) {
		if k := len(ops) - 1; k >= 0 && ops[k].equal == eq {
			ops[k].i2, ops[k].j2 = i2, j2
			return
		}
		ops = append(ops, op{eq, i1, i2, j1, j2})
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			push(true, i, i+1, j, j+1)
			i, j = i+1, j+1
		case j < m && (i == n || l[i][j+1] >= l[i+1][j]):
			push(false, i, i, j, j+1)
			j++
		default:
			push(false, i, i+1, j, j)
			i++
		}
	}
	return ops
}

func similarity(ops []op, n, m int) float64 {
	same := 0
	for _, o := range ops {
		if o.equal {
			same += o.i2 - o.i1
		}
	}
	return 2 * float64(same) / float64(n+m)
}
