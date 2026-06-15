package main

import (
	"strings"
	"testing"
)

func TestBuildPortraitPrompt(t *testing.T) {
	p := buildPortraitPrompt("ore-1", "a grizzled ore hauler", "first", "crimson", "laborer", "")
	if !strings.Contains(p, "a grizzled ore hauler") {
		t.Fatal("bio missing from prompt")
	}
	if !strings.Contains(p, portraitCueSuffix) {
		t.Fatal("global style cue missing")
	}
	if !strings.Contains(p, "dark steel and armor-plated") {
		t.Fatal("crimson empire sensibility missing")
	}
	if !strings.Contains(p, "industrial laborer") {
		t.Fatal("laborer archetype role cue missing")
	}
	if !strings.Contains(p, "refined high-end") {
		t.Fatal("first-class formality missing")
	}
	// The global cue must LEAD the bio, so CLIP's 77-token cap can never truncate
	// the framing/medium directives behind a long bio.
	if strings.Index(p, portraitCueSuffix) > strings.Index(p, "a grizzled ore hauler") {
		t.Fatal("global cue must precede the bio")
	}
}

func TestArchetypeDiversifiesGarment(t *testing.T) {
	// Same empire and class, different archetype -> different garment, so a faction
	// no longer collapses into one uniform.
	officer := buildPortraitPrompt("c-1", "a stern commander", "business", "crimson", "officer", "")
	laborer := buildPortraitPrompt("c-2", "a tired hauler", "business", "crimson", "laborer", "")
	if !strings.Contains(officer, "command uniform") {
		t.Fatal("officer role cue missing")
	}
	if strings.Contains(laborer, "command uniform") {
		t.Fatal("laborer should not wear a command uniform")
	}
	// Both still share the crimson sensibility.
	for _, p := range []string{officer, laborer} {
		if !strings.Contains(p, "dark steel and armor-plated") {
			t.Fatal("crimson sensibility should apply regardless of archetype")
		}
	}
}

func TestUnknownArchetypeFallsBackToSpacer(t *testing.T) {
	p := buildPortraitPrompt("z-1", "a clerk", "business", "nebula", "", "")
	if !strings.Contains(p, archetypeGarment["spacer"]) {
		t.Fatal("empty archetype should fall back to the spacer garment")
	}
}

func TestBuildPortraitPromptEmptyBioIsValid(t *testing.T) {
	p := buildPortraitPrompt("empty-1", "", "", "", "", "")
	if strings.TrimSpace(p) == "" {
		t.Fatal("empty bio produced empty prompt")
	}
	if !strings.Contains(p, portraitCueSuffix) {
		t.Fatal("global cue missing on fallback prompt")
	}
}

func TestBuildPortraitPromptKeepsFullBio(t *testing.T) {
	// Bios are no longer truncated (compel handles long prompts), so every word
	// of even a long bio must survive into the prompt.
	longBio := strings.Repeat("word ", 100)
	p := buildPortraitPrompt("long-1", longBio, "business", "nebula", "merchant", "")
	if strings.Count(p, "word") < 100 {
		t.Fatalf("bio was truncated: only %d of 100 words present", strings.Count(p, "word"))
	}
	if !strings.Contains(p, portraitCueSuffix) {
		t.Fatal("global cue missing on long bio")
	}
}

func TestPerformerOverridesEmpire(t *testing.T) {
	// A glam-rock batch should ignore the nebula corporate theme.
	p := buildPortraitPrompt("perf-1", "a glam-rock legend in an impossible costume", "first", "nebula", "merchant", "")
	if !strings.Contains(p, performerAesthetic) {
		t.Fatal("performer override not applied")
	}
	if strings.Contains(p, "corporate") {
		t.Fatal("empire aesthetic leaked past performer override")
	}
}

func TestPerformerArchetypeOverridesEmpire(t *testing.T) {
	// Even without a bio keyword, a performer archetype should override the empire.
	p := buildPortraitPrompt("perf-2", "a quiet figure who lights up a room", "first", "nebula", "performer", "")
	if !strings.Contains(p, performerAesthetic) {
		t.Fatal("performer archetype override not applied")
	}
	if strings.Contains(p, "corporate") {
		t.Fatal("empire aesthetic leaked past performer archetype override")
	}
}

func TestPirateCitizenshipAesthetic(t *testing.T) {
	p := buildPortraitPrompt("pir-1", "a raider in salvaged armor", "business", "pirates", "outlaw", "")
	if !strings.Contains(p, "space pirate") {
		t.Fatal("pirate aesthetic missing")
	}
	first := buildPortraitPrompt("pir-2", "a privateer with letters of marque", "first", "pirates", "outlaw", "")
	if !strings.Contains(first, "flamboyant privateer") {
		t.Fatal("first-class pirate flair missing")
	}
}

func TestUnknownEmpireFallback(t *testing.T) {
	p := buildPortraitPrompt("wan-1", "a wanderer", "economy", "atlantean", "spacer", "")
	if !strings.Contains(p, "practical spacer style") {
		t.Fatal("unknown empire should fall back to a generic spacer style")
	}
}

func TestPhysicalTraitsDeterministicAndVaried(t *testing.T) {
	// Stable per id.
	first, second := physicalTraits("alice"), physicalTraits("alice")
	if first != second {
		t.Fatal("physicalTraits not deterministic for the same id")
	}
	// Different ids should usually differ; across a handful we expect >1 distinct.
	seen := map[string]bool{}
	for _, id := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		seen[physicalTraits(id)] = true
	}
	if len(seen) < 2 {
		t.Fatalf("physicalTraits produced no variety across ids: %v", seen)
	}
	// A trait string carries all three axes (skin, age, build) -> two commas.
	if strings.Count(physicalTraits("x"), ",") < 2 {
		t.Fatalf("physicalTraits missing an axis: %q", physicalTraits("x"))
	}
}

func TestBuildPortraitPromptIncludesTraits(t *testing.T) {
	p := buildPortraitPrompt("zed", "a clerk", "business", "nebula", "official", "")
	if !strings.Contains(p, physicalTraits("zed")) {
		t.Fatal("prompt missing id-derived physical traits")
	}
}

func TestBioGenderNoun(t *testing.T) {
	cases := map[string]string{
		"He runs a shop and his ledger is tidy": "man",
		"She pilots her own freighter":          "woman",
		"They keep to themselves":               "person",
		"He and she argued constantly":          "person", // even mix -> person
		"A quiet traveler with no pronouns":     "person",
		// Dominant pronoun wins despite an incidental opposite one (the Victoria
		// 'Valkyrie' case: she/her ×3 vs an incidental he ×1).
		"She earned her command; he was a rival she outlasted": "woman",
	}
	for bio, want := range cases {
		if got := bioGenderNoun(bio); got != want {
			t.Errorf("bioGenderNoun(%q) = %q; want %q", bio, got, want)
		}
	}
}

func TestPromptUsesBioGender(t *testing.T) {
	woman := buildPortraitPrompt("a", "She is a captain who lost her ship", "first", "crimson", "pilot", "")
	if !strings.Contains(woman, "single woman,") {
		t.Fatalf("feminine bio should yield 'single woman' cue: %q", woman)
	}
	man := buildPortraitPrompt("b", "He is a captain", "first", "crimson", "pilot", "")
	if !strings.Contains(man, "single man,") {
		t.Fatalf("masculine bio should yield 'single man' cue: %q", man)
	}
}

func TestPromptHashStable(t *testing.T) {
	a := promptHash("hello")
	if a != promptHash("hello") {
		t.Fatal("promptHash not stable")
	}
	if a == promptHash("hellp") {
		t.Fatal("promptHash collided")
	}
	const wantHello = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if promptHash("hello") != wantHello {
		t.Fatalf("promptHash(\"hello\") = %q; want %q", promptHash("hello"), wantHello)
	}
}
