package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// portraitStyleSuffix is appended to every passenger prompt to keep the gallery
// visually consistent. Tune the whole gallery's look here.
const portraitStyleSuffix = ", character portrait, sci-fi crew member, painterly, dramatic lighting, neutral background, head and shoulders"

// buildPortraitPrompt composes an image prompt from a passenger's bio, travel
// class, and citizenship. Always returns a non-empty prompt.
func buildPortraitPrompt(bio, class, citizenship string) string {
	subject := strings.TrimSpace(bio)
	if subject == "" {
		subject = "a nondescript interstellar traveler"
	}
	var attire string
	switch strings.ToLower(strings.TrimSpace(class)) {
	case "first":
		attire = "refined affluent attire"
	case "business":
		attire = "professional business attire"
	default:
		attire = "practical traveler's attire"
	}
	var theme string
	if empire := strings.ToLower(strings.TrimSpace(citizenship)); empire != "" {
		theme = fmt.Sprintf(", %s empire color theme", empire)
	}
	return subject + ", " + attire + theme + portraitStyleSuffix
}

// promptHash returns a hex SHA-256 of the prompt, used as the cache key for
// regeneration decisions.
func promptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}
