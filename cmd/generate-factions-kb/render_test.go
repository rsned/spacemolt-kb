package main

import (
	htmltpl "html/template"
	"strings"
	"testing"
	"time"
)

func TestPlayerDetailRendersSilhouetteWhenNoOverlayImage(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))
	p := &Player{
		ID:           "player-abc",
		Username:     "Nova",
		FirstSeenUTC: "2026-01-01T00:00:00Z",
		LastSeenUTC:  "2026-01-02T00:00:00Z",
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, p); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `<svg class="silhouette"`) {
		t.Fatal("expected silhouette SVG in no-overlay player page")
	}
	if !strings.Contains(out, "player-abc") {
		t.Fatal("expected player ID in page")
	}
}
