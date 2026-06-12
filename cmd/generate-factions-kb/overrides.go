package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// portraitOverridesFileName is a HAND-CURATED file at the overlays root mapping a
// passenger's citizen_id to an explicit visual cue to inject into the portrait
// prompt. It exists because some distinctive traits live only in narrative bio
// prose (e.g. "lost the eye to a ricochet and keeps the patch") that T5 reads but
// the diffusion model will not render without a concrete visual instruction
// ("wearing a black eyepatch over one eye"). Editing this file + re-running
// portrait generation regenerates only the affected passengers (their prompt
// hash changes; everyone else stays cached).
const portraitOverridesFileName = "portrait_overrides.json"

// loadPortraitOverrides reads overlays/portrait_overrides.json. A missing or
// malformed file yields an empty map, so overrides are an optional enrichment.
func loadPortraitOverrides(root string) map[string]string {
	data, err := os.ReadFile(filepath.Join(root, portraitOverridesFileName))
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]string{}
	}
	return raw
}
