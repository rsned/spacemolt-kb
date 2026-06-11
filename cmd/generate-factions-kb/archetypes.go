package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// archetypesFileName is the cache written by tools/classify_archetypes.py under
// the overlays root: overlays/generated/archetypes.json. It maps a passenger's
// citizen_id to a role archetype (plus the bio hash used for cache validity).
const archetypesFileName = "archetypes.json"

// loadArchetypes reads the archetype cache under root/generated/. A missing file
// yields an empty map (portraits then fall back to the generic "spacer" garment),
// so archetype classification is an optional enrichment, not a hard dependency.
func loadArchetypes(root string) map[string]string {
	path := filepath.Join(root, "generated", archetypesFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]struct {
		Archetype string `json:"archetype"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(raw))
	for id, v := range raw {
		out[id] = v.Archetype
	}
	return out
}
