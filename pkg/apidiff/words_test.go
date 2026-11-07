package apidiff

import (
	"slices"
	"strings"
	"testing"
)

func marked(r DescRow) []string {
	var out []string
	for _, t := range r.Tokens {
		if t.Marked {
			out = append(out, t.Text)
		}
	}
	return out
}

func text(r DescRow) string {
	var w []string
	for _, t := range r.Tokens {
		w = append(w, t.Text)
	}
	return strings.Join(w, " ")
}

func kinds(rows []DescRow) []LineKind {
	var k []LineKind
	for _, r := range rows {
		k = append(k, r.Kind)
	}
	return k
}

func TestDescDiffWordMark(t *testing.T) {
	rows := DescDiff("An owner-written description replaces the text.", "A per-facility description replaces the text.")
	if !slices.Equal(kinds(rows), []LineKind{LineDel, LineAdd}) {
		t.Fatalf("kinds = %v", kinds(rows))
	}
	if got := marked(rows[0]); !slices.Equal(got, []string{"An", "owner-written"}) {
		t.Errorf("del marks = %v", got)
	}
	if got := marked(rows[1]); !slices.Equal(got, []string{"A", "per-facility"}) {
		t.Errorf("add marks = %v", got)
	}
}

func TestDescDiffUnchangedSentencesCollapse(t *testing.T) {
	rows := DescDiff("One stays. Two was here. Three stays.", "One stays. Two is here. Three stays.")
	if want := []LineKind{LineCtx, LineDel, LineAdd, LineCtx}; !slices.Equal(kinds(rows), want) {
		t.Errorf("kinds = %v, want %v", kinds(rows), want)
	}
}

func TestDescDiffTrim(t *testing.T) {
	words := make([]string, 40)
	for i := range words {
		words[i] = "w" + string(rune('a'+i%26)) + strings.Repeat("x", i/26)
	}
	old := strings.Join(words, " ") + "."
	words[20] = "CHANGED"
	rows := DescDiff(old, strings.Join(words, " ")+".")
	add := rows[len(rows)-1]
	if got := text(add); !strings.HasPrefix(got, "… ") || !strings.HasSuffix(got, " …") {
		t.Errorf("add row not trimmed: %q", got)
	}
	if n := len(add.Tokens); n != 6+1+6+2 {
		t.Errorf("add row has %d tokens, want 15: %q", n, text(add))
	}
}

func TestDescDiffLowSimilarity(t *testing.T) {
	rows := DescDiff("Completely unrelated words here.", "Nothing in common at all, really.")
	for _, r := range rows {
		if len(marked(r)) > 0 {
			t.Errorf("low-similarity rewrite should not mark words: %v", marked(r))
		}
	}
}

func TestDescDiffAddedAndRemovedText(t *testing.T) {
	if rows := DescDiff("", "New text."); !slices.Equal(kinds(rows), []LineKind{LineAdd}) {
		t.Errorf("added description kinds = %v", kinds(rows))
	}
	if rows := DescDiff("Old text.", ""); !slices.Equal(kinds(rows), []LineKind{LineDel}) {
		t.Errorf("removed description kinds = %v", kinds(rows))
	}
}
