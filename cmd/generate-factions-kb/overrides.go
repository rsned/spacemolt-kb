package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// overrideFileName is a HAND-AUTHORED, per-entity file living beside an entity's
// other overlay content (overlays/passengers/<citizen_id>/override.json or
// overlays/players/<player_id>/override.json). It is the single authoritative
// source for portrait corrections and survives DB re-scrapes and classifier
// reruns: every field wins over the DB-derived or machine-classified value, so a
// scrape that changes a bio can never silently revert a manual fix. All fields
// are optional; an absent file or absent field falls through to default behavior.
// Editing it changes the affected entity's prompt hash, so only that portrait
// regenerates. See overlays/README.md for the documented format.
const overrideFileName = "override.json"

// PortraitOverride holds optional per-entity portrait overrides. The zero value
// (no file / empty fields) means "no override" — default behavior throughout.
type PortraitOverride struct {
	Archetype  string `json:"archetype"`  // beats the classifier (archetypes.json)
	Gender     string `json:"gender"`     // man|woman|person|android; beats bioGenderNoun
	Appearance string `json:"appearance"` // beats physicalTraits (skin/age/build/ethnicity)
	Species    string `json:"species"`    // alien-form nudge (e.g. "voidborn"); else human
	BioAppend  string `json:"bio_append"` // one extra sentence appended to the bio
	VisualCue  string `json:"visual_cue"` // prominent cue injected after appearance
}

// loadPortraitOverride reads <dir>/override.json. A missing or malformed file
// yields the zero value, so overrides are an optional enrichment.
func loadPortraitOverride(dir string) PortraitOverride {
	var ov PortraitOverride
	data, err := os.ReadFile(filepath.Join(dir, overrideFileName))
	if err != nil {
		return ov
	}
	_ = json.Unmarshal(data, &ov) // malformed -> zero value, treated as no override
	return ov
}

// validGenders are the gender nouns a gender override may set; anything else is
// ignored (falls through to bio-pronoun inference).
var validGenders = map[string]bool{"man": true, "woman": true, "person": true, "android": true}

// speciesAppearance maps a known non-human species to a physical descriptor used
// in place of the human skin/age/build traits. An unknown species value is used
// verbatim as the descriptor.
var speciesAppearance = map[string]string{
	"voidborn": "a grown crystalline-organic Voidborn being, pale semi-translucent cultivated skin, faint glowing subdermal lattice",
}

// gender returns the override gender when it is a recognized noun, else the
// bio-pronoun inference.
func (ov PortraitOverride) gender(bio string) string {
	g := strings.ToLower(strings.TrimSpace(ov.Gender))
	if validGenders[g] {
		return g
	}
	return bioGenderNoun(bio)
}

// physical returns the physical-appearance clause: an explicit appearance wins,
// then a species descriptor, then the id-hashed default traits.
func (ov PortraitOverride) physical(id string) string {
	if a := strings.TrimSpace(ov.Appearance); a != "" {
		return a
	}
	if s := strings.ToLower(strings.TrimSpace(ov.Species)); s != "" {
		if d, ok := speciesAppearance[s]; ok {
			return d
		}
		return strings.TrimSpace(ov.Species)
	}
	return physicalTraits(id)
}

// archetypeOr returns the override archetype when set, else the classified one.
func (ov PortraitOverride) archetypeOr(classified string) string {
	if a := strings.TrimSpace(ov.Archetype); a != "" {
		return a
	}
	return classified
}

// bioWithAppend returns the trimmed bio with the override's extra sentence
// appended (separated by ". "), or just the trimmed bio when no append is set.
func (ov PortraitOverride) bioWithAppend(bio string) string {
	b := strings.TrimSpace(bio)
	ap := strings.TrimSpace(ov.BioAppend)
	if ap == "" {
		return b
	}
	if b == "" {
		return ap
	}
	return strings.TrimRight(b, ". ") + ". " + ap
}
