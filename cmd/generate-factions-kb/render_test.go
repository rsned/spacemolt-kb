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

func TestPlayerDetailPortraitPrecedence(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("pdet").Funcs(funcs).Parse(playerDetailTmpl))

	// Generated portrait present (no overlay image) -> <img>, no silhouette.
	genP := &Player{
		ID:           "player-xyz",
		Username:     "Atlas",
		PortraitFile: "portrait.png",
		FirstSeenUTC: "2026-01-01T00:00:00Z",
		LastSeenUTC:  "2026-01-02T00:00:00Z",
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, genP); err != nil {
		t.Fatalf("execute: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, `src="portrait.png"`) {
		t.Fatal("expected generated portrait img on player page")
	}
	if strings.Contains(out, `<svg class="silhouette"`) {
		t.Fatal("silhouette should be suppressed when a player portrait exists")
	}
}

func TestPassengerDetailPortraitPrecedence(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("psdet").Funcs(funcs).Parse(passengerDetailTmpl))

	// No overlay, no generated portrait -> silhouette.
	silP := &Passenger{ID: "p1", Slug: "p1", Name: "Lin", Bio: "a fixer"}
	var b1 strings.Builder
	if err := tmpl.Execute(&b1, silP); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b1.String(), `<svg class="silhouette"`) {
		t.Fatal("expected silhouette when no portrait/overlay")
	}
	if !strings.Contains(b1.String(), "a fixer") {
		t.Fatal("expected bio in About")
	}

	// Generated portrait present -> <img>, no silhouette.
	genP := &Passenger{ID: "p2", Slug: "p2", Name: "Bea", PortraitFile: "portrait.png"}
	var b2 strings.Builder
	if err := tmpl.Execute(&b2, genP); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b2.String(), `src="portrait.png"`) {
		t.Fatal("expected generated portrait img")
	}
	if strings.Contains(b2.String(), `<svg class="silhouette"`) {
		t.Fatal("silhouette should be suppressed when a portrait exists")
	}
}

func TestPassengerDetailPromptFootnote(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("psdet").Funcs(funcs).Parse(passengerDetailTmpl))

	// Generated portrait + prompt -> footnote shows the prompt.
	genP := &Passenger{ID: "p1", Slug: "p1", Name: "Bea", PortraitFile: "portrait.png", PortraitPrompt: "a single woman, cinematic portrait"}
	var b strings.Builder
	if err := tmpl.Execute(&b, genP); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "AI portrait prompt:") || !strings.Contains(b.String(), "a single woman, cinematic portrait") {
		t.Fatal("expected the portrait prompt footnote")
	}

	// A contributor overlay image is displayed instead of the generated portrait,
	// so the generated-prompt footnote must be suppressed.
	ovP := &Passenger{
		ID: "p2", Slug: "p2", Name: "Cy", PortraitFile: "portrait.png", PortraitPrompt: "a single man",
		Overlay: &Overlay{ImageFile: "photo.jpg"},
	}
	var b2 strings.Builder
	if err := tmpl.Execute(&b2, ovP); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b2.String(), "AI portrait prompt:") {
		t.Fatal("prompt footnote must be hidden when a contributor overlay image is shown")
	}
}

func TestPassengerIndexLists(t *testing.T) {
	funcs := templateFuncs(time.Unix(0, 0).UTC())
	tmpl := htmltpl.Must(htmltpl.New("psidx").Funcs(funcs).Parse(passengerIndexTmpl))
	ps := []*Passenger{{ID: "p1", Slug: "p1", Name: "Lin", Citizenship: "nebula", Class: "first"}}
	var b strings.Builder
	if err := tmpl.Execute(&b, ps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `href="p1/"`) || !strings.Contains(b.String(), "Lin") {
		t.Fatal("expected passenger link + name in index")
	}
}
