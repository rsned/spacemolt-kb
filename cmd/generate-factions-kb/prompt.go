package main

import (
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"regexp"
	"strings"
)

// portraitCueSuffix is the framing/medium half of the leading cue, shared by
// every passenger. Tune the gallery's shared look here. It pins a single
// rendering style (cinematic photoreal) so the gallery doesn't drift between
// comic-book and realistic looks the way a bare "painterly" cue did. The full
// cue is built by portraitCue, which injects a bio-derived gender noun first —
// SDXL has a strong male prior, so without an explicit "woman"/"man" cue
// feminine-pronoun bios render as men.
const portraitCueSuffix = "head and shoulders, cinematic photorealistic portrait, sharp detailed features, dramatic studio lighting, plain neutral background, highly detailed, realistic skin texture"

// portraitCue leads every prompt: "solo character portrait of a single <gender>,
// <framing/medium>". gender is "man", "woman", or "person".
func portraitCue(gender string) string {
	return "solo character portrait of a single " + gender + ", " + portraitCueSuffix
}

var (
	masculinePronoun = regexp.MustCompile(`(?i)\b(he|him|his|himself)\b`)
	femininePronoun  = regexp.MustCompile(`(?i)\b(she|her|hers|herself)\b`)
)

// bioGenderNoun infers a gender noun from a bio's pronouns by frequency: the
// dominant pronoun set wins when it clearly outnumbers the other (at least 2x),
// else "person" (no pronouns, or a genuine mix). Counting rather than mere
// presence matters because a bio about a woman often contains an incidental
// "he"/"his" about someone else — a presence test would mask her as ambiguous
// and SDXL's male prior would then render a man (e.g. Victoria 'Valkyrie',
// she/her ×15 vs he ×3, was rendering male).
func bioGenderNoun(bio string) string {
	m := len(masculinePronoun.FindAllString(bio, -1))
	f := len(femininePronoun.FindAllString(bio, -1))
	switch {
	case m == 0 && f == 0:
		return "person"
	case f > m && f >= 2*m:
		return "woman"
	case m > f && m >= 2*f:
		return "man"
	default:
		return "person"
	}
}

// Bios are no longer length-capped: the gen_portrait.py backend encodes prompts
// with compel, which chunks past CLIP's 77-token window instead of truncating,
// so the full bio contributes. (Set PORTRAIT_NO_COMPEL=1 there to fall back to
// truncated encoding.)

// empireStyle maps a citizenship to a palette/cut/material *sensibility* — not a
// full outfit. The archetype supplies the actual garment; the empire only tints
// it. This keeps a crimson refinery laborer and a crimson officer both regimented
// in palette without both collapsing into the same dress-uniform general (the old
// "<empire> theme" cue did exactly that). Unknown empires fall back to a generic
// spacer sensibility. Crimson is a militaristic *society*, not only its military:
// a regimented muted palette and crisp straight cuts, never frills or ruffles.
var empireStyle = map[string]string{
	"solarian": "naturalistic contemporary Earth-centered fashion, finely detailed refined tailoring",
	"nebula":   "sleek corporate palette, clean minimal lines",
	"crimson":  "regimented muted color palette, crisp straight cuts with clean sharp edges, plain and unadorned with no frills ruffles flourishes or hoops, disciplined bearing",
	"outerrim": "scavenger chic, salvaged and patched makeshift materials, rugged and worn",
	"voidborn": "ethereal pale high-collared materials, ascetic refined alien elegance",
}

// archetypeGarment maps a role archetype (classified from the bio by
// tools/classify_archetypes.py via local Ollama) to a garment/role phrase.
// Layered over an empire sensibility this diversifies a faction by occupation so
// not every crimson citizen reads as a senior officer in dress uniform. Unknown
// or empty archetypes fall back to the "spacer" garment.
var archetypeGarment = map[string]string{
	"laborer":    "in sturdy utilitarian work clothes",
	"officer":    "in a formal military uniform",
	"merchant":   "in a sharp business outfit",
	"official":   "in tidy administrative attire with subtle insignia",
	"technician": "in a practical technical jumpsuit with a tool harness",
	"pilot":      "in a flight jacket and pilot's rig",
	"medic":      "in clean clinical medical attire",
	"outlaw":     "in worn nondescript layered clothing",
	"spiritual":  "in ceremonial robes",
	"aristocrat": "in opulently tailored finery",
	"spacer":     "in practical spacer attire",
}

// classFormality scales the styling by travel class.
var classFormality = map[string]string{
	"first":    "refined high-end",
	"business": "sharp professional",
	"economy":  "practical well-worn",
}

// pirateAesthetic styles the "pirates" citizenship as modern, gritty raiders
// rather than historical wooden-ship buccaneers.
const pirateAesthetic = "modern gritty space pirate, lean and wiry, coiled and watchful like a cornered cat about to pounce, shifty-eyed, worn scavenged gear"

// performerAesthetic overrides the empire theme for vivid stage characters so a
// glam-rock legend is not flattened into corporate attire.
const performerAesthetic = "flamboyant theatrical stage performer in an extravagant glamorous costume, bold expressive showmanship"

// performerCues are bio keywords that trigger the performer override.
var performerCues = []string{
	"glam", "rock star", "rockstar", "rock legend", "pop star", "popstar",
	"diva", "performer", "showman", "cabaret", "celebrity", "costume",
	"stage idol", "pop idol",
}

// skinTones, ageBrackets, and builds give each passenger a deterministic,
// gender-neutral physical description so the gallery doesn't collapse into one
// default face per empire. Gender is left to the bio.
var skinTones = []string{
	"deep ebony skin, Central African features",
	"warm brown skin, West African features",
	"East Asian features, light tan skin",
	"South Asian features, medium brown skin",
	"olive Mediterranean skin",
	"pale fair Nordic skin",
	"sun-tanned Latino features",
	"Middle Eastern features, light brown skin",
	"Southeast Asian features, golden-brown skin",
	"freckled light skin",
	"Pacific Islander features, brown skin",
	"mixed-race features, medium brown skin",
}

var ageBrackets = []string{
	"youthful in their twenties",
	"in their thirties",
	"early middle-aged",
	"middle-aged with some gray",
	"older and graying",
	"elderly and weathered",
}

var builds = []string{
	"lean build",
	"stocky build",
	"broad-shouldered and heavyset",
	"slender build",
	"average build",
	"tall and wiry",
	"short and round-faced",
	"athletic build",
}

// physicalTraits returns a deterministic "skin, age, build" descriptor for id.
// Each axis uses an independent salt so the three don't correlate (a skin tone
// isn't always paired with the same age/build).
func physicalTraits(id string) string {
	return pickTrait(skinTones, "skin", id) + ", " +
		pickTrait(ageBrackets, "age", id) + ", " +
		pickTrait(builds, "build", id)
}

// pickTrait deterministically selects an element of pool for a given salt+id.
func pickTrait(pool []string, salt, id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(salt + ":" + id))
	return pool[h.Sum32()%uint32(len(pool))]
}

// buildPortraitPrompt composes an image prompt from a passenger's stable id,
// bio, travel class, citizenship, and bio-derived role archetype. The global cue
// leads, then an aesthetic chosen from (in priority order) a performer, the
// pirate citizenship, or a class-scaled empire-sensibility + archetype-garment
// combination, then id-derived physical traits (skin/age/build), then the full
// bio. Always returns a non-empty prompt.
func buildPortraitPrompt(id, bio, class, citizenship, archetype string) string {
	cls := strings.ToLower(strings.TrimSpace(class))
	cit := strings.ToLower(strings.TrimSpace(citizenship))
	arch := strings.ToLower(strings.TrimSpace(archetype))
	// The FLUX backend encodes the whole prompt through T5, so the full bio
	// counts (no CLIP 77-token truncation): lead with the style cue, then
	// physical appearance and the styling aesthetic, then the free-text bio as a
	// trailing sentence. Order still puts the portrait-defining cue first so the
	// SDXL fallback (CLIP-only) degrades gracefully.
	prompt := portraitCue(bioGenderNoun(bio)) + ", " +
		physicalTraits(id) + ", " + passengerAesthetic(bio, cls, cit, arch)
	if subject := strings.TrimSpace(bio); subject != "" {
		prompt += ". " + subject
	}
	return prompt
}

// passengerAesthetic picks the styling phrase: a performer wins (bio cue or
// archetype), then the pirate citizenship, then a class-scaled empire sensibility
// layered with an archetype garment (with generic fallbacks for unknown empire or
// archetype). cls, cit, and arch must already be lowercased/trimmed.
func passengerAesthetic(bio, cls, cit, arch string) string {
	if arch == "performer" || hasPerformerCue(bio) {
		return performerAesthetic
	}
	if cit == "pirates" {
		if cls == "first" {
			return pirateAesthetic + " with a flamboyant privateer streak"
		}
		return pirateAesthetic
	}
	empire, ok := empireStyle[cit]
	if !ok {
		empire = "practical spacer style"
	}
	garment, ok := archetypeGarment[arch]
	if !ok {
		garment = archetypeGarment["spacer"]
	}
	formality := classFormality[cls]
	if formality == "" {
		formality = "practical"
	}
	// Garment precedes the empire sensibility so it stays inside CLIP's 77-token
	// window: some empire styles (notably crimson) are long, and if they came
	// first they truncated the garment clause, leaving subjects bare-shouldered.
	// The empire adjectives are lower-priority and fine to lose to truncation.
	return formality + ", " + garment + ", " + empire
}

// hasPerformerCue reports whether bio mentions a flamboyant stage persona.
func hasPerformerCue(bio string) bool {
	b := strings.ToLower(bio)
	for _, k := range performerCues {
		if strings.Contains(b, k) {
			return true
		}
	}
	return false
}

// promptHash returns a hex SHA-256 of the prompt, used as the cache key for
// regeneration decisions.
func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}
