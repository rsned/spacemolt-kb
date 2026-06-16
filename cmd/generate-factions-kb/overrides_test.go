package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPortraitOverrideMissingAndMalformed(t *testing.T) {
	dir := t.TempDir()
	// Missing file -> zero value.
	if got := loadPortraitOverride(dir); got != (PortraitOverride{}) {
		t.Fatalf("missing override should be zero value, got %+v", got)
	}
	// Malformed JSON -> zero value (no panic).
	if err := os.WriteFile(filepath.Join(dir, overrideFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadPortraitOverride(dir); got != (PortraitOverride{}) {
		t.Fatalf("malformed override should be zero value, got %+v", got)
	}
}

func TestLoadPortraitOverrideRoundTrip(t *testing.T) {
	dir := t.TempDir()
	body := `{"archetype":"medic","gender":"woman","bio_append":"keeps a brass locket"}`
	if err := os.WriteFile(filepath.Join(dir, overrideFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ov := loadPortraitOverride(dir)
	if ov.Archetype != "medic" || ov.Gender != "woman" || ov.BioAppend != "keeps a brass locket" {
		t.Fatalf("override not parsed: %+v", ov)
	}
}

func TestOverrideArchetypeBeatsClassifier(t *testing.T) {
	// Classifier says spacer; override says medic -> medic garment wins.
	p := buildPortraitPrompt("x", "a quiet traveler", "first", "crimson", "spacer",
		PortraitOverride{Archetype: "medic"})
	if !strings.Contains(p, archetypeGarment["medic"]) {
		t.Fatalf("override archetype (medic) not applied: %q", p)
	}
	if strings.Contains(p, archetypeGarment["spacer"]) {
		t.Fatalf("classifier spacer garment leaked past the override: %q", p)
	}
}

func TestOverrideGenderBeatsBio(t *testing.T) {
	// Masculine bio, but a gender override forces a woman.
	p := buildPortraitPrompt("x", "He runs a tidy shop and his ledger is neat", "first", "nebula", "merchant",
		PortraitOverride{Gender: "woman"})
	if !strings.Contains(p, "single woman,") {
		t.Fatalf("gender override not applied: %q", p)
	}
	// An invalid gender falls through to bio inference.
	p2 := buildPortraitPrompt("x", "He runs a shop", "first", "nebula", "merchant",
		PortraitOverride{Gender: "robot"})
	if !strings.Contains(p2, "single man,") {
		t.Fatalf("invalid gender should fall through to bio inference: %q", p2)
	}
}

func TestOverrideAppearanceAndSpecies(t *testing.T) {
	// Explicit appearance replaces the id-hashed physical traits.
	p := buildPortraitPrompt("x", "a traveler", "first", "voidborn", "spacer",
		PortraitOverride{Appearance: "deep teal grown chitin"})
	if !strings.Contains(p, "deep teal grown chitin") {
		t.Fatalf("appearance override missing: %q", p)
	}
	if strings.Contains(p, physicalTraits("x")) {
		t.Fatalf("appearance override should replace default traits: %q", p)
	}
	// A known species injects its descriptor when no appearance is set.
	p2 := buildPortraitPrompt("x", "a traveler", "first", "voidborn", "spacer",
		PortraitOverride{Species: "voidborn"})
	if !strings.Contains(p2, speciesAppearance["voidborn"]) {
		t.Fatalf("species descriptor missing: %q", p2)
	}
	// Appearance wins over species when both are present.
	p3 := buildPortraitPrompt("x", "a traveler", "first", "voidborn", "spacer",
		PortraitOverride{Appearance: "obsidian carapace", Species: "voidborn"})
	if !strings.Contains(p3, "obsidian carapace") || strings.Contains(p3, speciesAppearance["voidborn"]) {
		t.Fatalf("appearance should win over species: %q", p3)
	}
}

func TestOverrideBioAppendAndVisualCue(t *testing.T) {
	p := buildPortraitPrompt("x", "a wandering trader", "first", "nebula", "merchant",
		PortraitOverride{BioAppend: "wears a worn brass locket", VisualCue: "holding a glowing data-slate"})
	if !strings.Contains(p, "a wandering trader. wears a worn brass locket") {
		t.Fatalf("bio_append not appended as a sentence: %q", p)
	}
	if !strings.Contains(p, "holding a glowing data-slate") {
		t.Fatalf("visual_cue not injected: %q", p)
	}
}

func TestEmptyOverrideMatchesNoOverride(t *testing.T) {
	// The zero-value override must reproduce the pre-override prompt exactly, so the
	// migration off portrait_overrides.json never busts a cached portrait.
	a := buildPortraitPrompt("p1", "a fixer", "first", "nebula", "merchant", PortraitOverride{})
	want := portraitCue(bioGenderNoun("a fixer")) + ", " + physicalTraits("p1") + ", " +
		passengerAesthetic("a fixer", "first", "nebula", "merchant") + ". a fixer"
	if a != want {
		t.Fatalf("empty override changed the prompt:\n got %q\nwant %q", a, want)
	}
}

func TestAgentArchetypeOverride(t *testing.T) {
	// An archetype override selects the garment directly for an agent.
	p := buildAgentPortraitPrompt("pid", "a steady hand", "solarian", "Trader",
		PortraitOverride{Archetype: "medic"})
	if !strings.Contains(p, archetypeGarment["medic"]) {
		t.Fatalf("agent archetype override not applied: %q", p)
	}
}
