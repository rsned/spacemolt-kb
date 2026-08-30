package wildlife

import (
	"strings"
	"testing"
)

const loreFixture = `# Wildlife lore

Preamble that must be ignored. **Bold words** here are not entries.

## Part 1 — the known roster

**Belt-Grazer** *(asteroid_belt · grazer · ranchable)*
The baseline stock animal of the belts, and the reason "grazer" means
anything out here.
- **Changed:** lungs traded for sealed fermentation stomachs; skin for
  a molting pressure-carapace.
- **Feeds:** rasps oxide crusts straight off the rock.
- **Defends:** nothing but armor and arithmetic.

**Rainbow Leviathan** *(asteroid_belt · predator)*
*Codex (scanned, v0.571.0): "The grandest of them all, it
fires the carapace."*
The grandest of the void-leviathans.
- **Changed:** the lobster plan scaled to cruiser size; a light-
  fracturing lattice.
- **Feeds:** ranges off its own belts.
- **Defends:** attacks — it fires the carapace.

---

## Part 2 — exotic form hypotheses

**Not A Species** *(void · hypothesis)*
Should not be parsed.
- **Changed:** nope.
`

func TestParseLore(t *testing.T) {
	entries := ParseLore([]byte(loreFixture))
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(entries), entries)
	}
	e, ok := entries.Lookup("Belt-Grazer")
	if !ok {
		t.Fatal("Belt-Grazer missing")
	}
	if e.Name != "Belt-Grazer" || len(e.Tags) != 3 || e.Tags[2] != "ranchable" {
		t.Errorf("header = %+v", e)
	}
	if !strings.HasPrefix(e.Intro, "The baseline stock animal") || !strings.HasSuffix(e.Intro, "anything out here.") || strings.Contains(e.Intro, "\n") {
		t.Errorf("intro = %q", e.Intro)
	}
	if e.Changed != "lungs traded for sealed fermentation stomachs; skin for a molting pressure-carapace." {
		t.Errorf("changed = %q", e.Changed)
	}
	if e.Feeds != "rasps oxide crusts straight off the rock." || e.Defends != "nothing but armor and arithmetic." {
		t.Errorf("feeds/defends = %q / %q", e.Feeds, e.Defends)
	}
	// Lookup is tolerant of case and punctuation differences.
	if _, ok := entries.Lookup("rainbow leviathan"); !ok {
		t.Error("case-insensitive lookup failed")
	}
	rl, _ := entries.Lookup("Rainbow Leviathan")
	if rl.Changed != "the lobster plan scaled to cruiser size; a light-fracturing lattice." {
		t.Errorf("hyphen rejoin: %q", rl.Changed)
	}
	if rl.Codex != "The grandest of them all, it fires the carapace." {
		t.Errorf("codex quote = %q", rl.Codex)
	}
	if rl.Intro != "The grandest of the void-leviathans." {
		t.Errorf("intro must exclude the codex block: %q", rl.Intro)
	}
	if e.Codex != "" {
		t.Errorf("no codex block -> empty, got %q", e.Codex)
	}
	if _, ok := entries.Lookup("Not A Species"); ok {
		t.Error("Part 2 entries must not be parsed")
	}
}
