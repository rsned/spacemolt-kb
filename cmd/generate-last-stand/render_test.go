package main

import (
	"strings"
	"testing"
)

func TestRenderPage(t *testing.T) {
	cat, cal := loadForTest(t)
	m := BuildMatrix(cat, cal, []string{"prospect", "opus_magna"}, starterColumnIDs(), 25000, 20, 4000)
	html, err := RenderPage(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<title", "smui.css", "Opus Magna", "no capital weapon bonus", "id=\"matrix\"",
		"how easily most hulls fall", "id=\"ls-low-end\"",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	// The interactive data must be embedded for client-side sort/filter.
	if !strings.Contains(html, "MATRIX_DATA") {
		t.Fatal("embedded matrix JSON missing")
	}
}
