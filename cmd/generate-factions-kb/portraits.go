package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// promptSidecarName stores the prompt + its hash next to a generated portrait.
// First line is the hash; the remainder is the prompt text.
const promptSidecarName = "prompt.txt"

// generatePassengerPortraits builds a prompt for each passenger and invokes the
// configured CLI (cmdLine, run via `sh -c`) for any passenger whose cached
// portrait is missing or whose prompt changed. Empty cmdLine is a no-op. The
// command receives the prompt on stdin and in $PORTRAIT_PROMPT, the target path
// in $PORTRAIT_OUT, and a deterministic $PORTRAIT_SEED; it must write an image
// to $PORTRAIT_OUT. A positive limit restricts generation to the first limit
// passengers (already sorted by name) for quick verification runs.
func generatePassengerPortraits(passengers []*Passenger, root, cmdLine string, limit int) {
	if strings.TrimSpace(cmdLine) == "" {
		log.Printf("portrait generation skipped: no command configured (set SMKB_PORTRAIT_CMD)")
		return
	}
	if limit > 0 && limit < len(passengers) {
		log.Printf("portrait generation limited to first %d of %d passengers", limit, len(passengers))
		passengers = passengers[:limit]
	}
	archetypes := loadArchetypes(root)
	empireGuess := loadEmpireGuess(root)
	for _, p := range passengers {
		ov := loadPortraitOverride(filepath.Join(root, "passengers", p.ID))
		// Passengers imported without a citizenship fall back to a name-inferred
		// empire for styling only (DB / visible faction stay unknown).
		cit := effectiveCitizenship(p.Citizenship, p.ID, empireGuess)
		prompt := buildPortraitPrompt(p.ID, p.Bio, p.Class, cit, archetypes[p.ID], ov)
		hash := promptHash(prompt)
		dir := passengerGeneratedDir(root, p.ID)
		if portraitExists(dir) && cachedHashMatches(dir, hash) {
			continue
		}
		out := filepath.Join(dir, generatedPortraitName)
		if err := runPortraitCmd(cmdLine, prompt, p.ID, out); err != nil {
			log.Printf("warning: portrait %s: %v", p.ID, err)
			continue
		}
		writePromptSidecar(dir, prompt, hash)
		log.Printf("generated portrait for %s", p.ID)
	}
}

// selectPassengers filters passengers to those matching any token in spec, a
// comma-separated list where each token matches a passenger by case-insensitive
// citizen_id equality or a case-insensitive substring of the name. An empty spec
// returns passengers unchanged. Tokens that match no passenger are returned in
// unmatched so the caller can warn about typos. Order follows the input slice
// (each passenger appears at most once).
func selectPassengers(passengers []*Passenger, spec string) (selected []*Passenger, unmatched []string) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return passengers, nil
	}
	var tokens []string
	for t := range strings.SplitSeq(spec, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			tokens = append(tokens, t)
		}
	}
	matched := make(map[string]bool, len(tokens))
	for _, p := range passengers {
		id := strings.ToLower(p.ID)
		name := strings.ToLower(p.Name)
		for _, t := range tokens {
			if id == t || (name != "" && strings.Contains(name, t)) {
				selected = append(selected, p)
				matched[t] = true
				break
			}
		}
	}
	for _, t := range tokens {
		if !matched[t] {
			unmatched = append(unmatched, t)
		}
	}
	return selected, unmatched
}

// runPortraitCmd runs cmdLine via `sh -c`, exposing the prompt/out/seed and
// verifying the command produced outPath.
func runPortraitCmd(cmdLine, prompt, id, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	seed := seedHash(id) % 1_000_000_000
	// cmdLine is operator-supplied configuration (SMKB_PORTRAIT_CMD), intentionally
	// executed via the shell so a full backend pipeline can be expressed.
	c := exec.Command("sh", "-c", cmdLine) //nolint:gosec // operator-supplied command by design
	c.Env = append(os.Environ(),
		"PORTRAIT_PROMPT="+prompt,
		"PORTRAIT_OUT="+outPath,
		fmt.Sprintf("PORTRAIT_SEED=%d", seed),
	)
	c.Stdin = strings.NewReader(prompt)
	c.Stdout = os.Stderr
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("command did not produce %s: %w", outPath, err)
	}
	return nil
}

// portraitExists reports whether a generated portrait file is present in dir.
func portraitExists(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, generatedPortraitName))
	return err == nil
}

// cachedHashMatches reports whether dir's sidecar records the given prompt hash.
func cachedHashMatches(dir, hash string) bool {
	data, err := os.ReadFile(filepath.Join(dir, promptSidecarName))
	if err != nil {
		return false
	}
	first, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(first) == hash
}

// writePromptSidecar records the hash (first line) + prompt next to the image.
func writePromptSidecar(dir, prompt, hash string) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("warning: sidecar dir %s: %v", dir, err)
		return
	}
	body := hash + "\n" + prompt + "\n"
	if err := os.WriteFile(filepath.Join(dir, promptSidecarName), []byte(body), 0o644); err != nil {
		log.Printf("warning: write sidecar %s: %v", dir, err)
	}
}
