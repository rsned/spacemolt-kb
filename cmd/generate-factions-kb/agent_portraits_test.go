package main

import (
	"strings"
	"testing"
)

func TestBuildAgentPortraitPromptEmpireAndRole(t *testing.T) {
	// Crimson engineer -> regimented empire sensibility + technician garment.
	p := buildAgentPortraitPrompt("pid-1", "she keeps the reactors running", "crimson", "Engineer", PortraitOverride{})
	for _, want := range []string{
		portraitCueSuffix,
		"solo character portrait of a single woman",
		"dark steel and armor-plated",
		"practical technical jumpsuit",
		"she keeps the reactors running",
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("crimson engineer prompt missing %q in %q", want, p)
		}
	}
}

func TestBuildAgentPortraitPromptFighterGarment(t *testing.T) {
	// Fighters use rugged tactical gear, not the "officer" formal military
	// uniform (which renders as an authoritarian junta officer).
	p := buildAgentPortraitPrompt("pid-f", "she is a hardened combat veteran", "nebula", "Fighter", PortraitOverride{})
	if !strings.Contains(p, "practical tactical combat gear") {
		t.Fatalf("fighter should use tactical combat gear: %q", p)
	}
	if strings.Contains(p, "command uniform") {
		t.Fatalf("fighter should NOT use the officer command-uniform garment: %q", p)
	}
	if !strings.Contains(p, "single woman") {
		t.Fatalf("she-dominant bio should render a woman: %q", p)
	}
}

func TestBuildAgentPortraitPromptSynthetic(t *testing.T) {
	p := buildAgentPortraitPrompt("pid-2", "Unit SAR-7 patrols Solarian space", "solarian", "Assist", PortraitOverride{})
	if !strings.Contains(p, syntheticAesthetic) {
		t.Fatalf("synthetic agent prompt missing android cue: %q", p)
	}
	if !strings.Contains(p, "single android") {
		t.Fatalf("synthetic agent should use android gender noun: %q", p)
	}
	// Human physical traits and an empire garment must NOT appear for a drone.
	if strings.Contains(p, "in a ") || strings.Contains(p, "skin,") {
		t.Fatalf("synthetic agent should skip human garment/physical traits: %q", p)
	}
}

func TestBuildAgentPortraitPromptPirate(t *testing.T) {
	p := buildAgentPortraitPrompt("pid-3", "he raids the shipping lanes", "outerrim", "Pirate", PortraitOverride{})
	if !strings.Contains(p, pirateAesthetic) {
		t.Fatalf("pirate role should use the pirate aesthetic: %q", p)
	}
}

func TestBuildAgentPortraitPromptUnknownRoleFallsBack(t *testing.T) {
	// Unknown role -> spacer garment; unknown empire -> practical spacer style.
	p := buildAgentPortraitPrompt("pid-4", "a wanderer", "frontier", "Wanderer", PortraitOverride{})
	if !strings.Contains(p, archetypeGarment["spacer"]) {
		t.Fatalf("unknown role should fall back to the spacer garment: %q", p)
	}
	if !strings.Contains(p, "practical spacer style") {
		t.Fatalf("unknown empire should fall back to practical spacer style: %q", p)
	}
}

func TestBuildAgentPortraitPromptPerformerOverridesEmpire(t *testing.T) {
	p := buildAgentPortraitPrompt("pid-5", "a glam rock legend of the stations", "crimson", "Trader", PortraitOverride{})
	if !strings.Contains(p, performerAesthetic) {
		t.Fatalf("a performer bio should override the empire styling: %q", p)
	}
	if strings.Contains(p, "dark steel and armor-plated") {
		t.Fatalf("performer should not also carry the crimson empire cue: %q", p)
	}
}

func TestIsSyntheticRole(t *testing.T) {
	if !isSyntheticRole("Assist") || !isSyntheticRole(" assist ") {
		t.Fatal("Assist role should be synthetic")
	}
	for _, r := range []string{"Engineer", "DataService", "Salvager", ""} {
		if isSyntheticRole(r) {
			t.Fatalf("%q should not be synthetic", r)
		}
	}
}
