package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// empireGuessFileName is the artifact written by tools/infer_empire.py under the
// overlays root: overlays/generated/empire_guess.json. It maps a passenger's
// citizen_id to a name-inferred empire for passengers imported without a
// citizenship field. The guess feeds ONLY the portrait prompt's empire styling
// (see effectiveCitizenship); it is never written back to the DB or shown as the
// passenger's visible faction.
const empireGuessFileName = "empire_guess.json"

// loadEmpireGuess reads the empire-guess artifact under root/generated/. A
// missing or unreadable file yields an empty map, so the inference is an
// optional enrichment — passengers without a guess keep the generic styling.
func loadEmpireGuess(root string) map[string]string {
	path := filepath.Join(root, "generated", empireGuessFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]struct {
		Empire string `json:"empire"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for id, v := range raw {
		out[id] = v.Empire
	}
	return out
}

// effectiveCitizenship returns the citizenship to use for portrait styling: the
// authoritative DB value when present, otherwise the name-inferred guess for id
// (empty if there is none). The DB value always wins, so labeled passengers are
// never affected by the inference.
func effectiveCitizenship(citizenship, id string, guesses map[string]string) string {
	if strings.TrimSpace(citizenship) != "" {
		return citizenship
	}
	return guesses[id]
}
