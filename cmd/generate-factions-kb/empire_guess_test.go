package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEmpireGuess(t *testing.T) {
	root := t.TempDir()
	genDir := filepath.Join(root, "generated")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{
	  "alpha": {"empire": "crimson", "confidence": 0.99, "name": "Hrothek Hammermund"},
	  "beta":  {"empire": "nebula",  "confidence": 0.80, "name": "Daya Lascor"}
	}`
	if err := os.WriteFile(filepath.Join(genDir, empireGuessFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadEmpireGuess(root)
	if got["alpha"] != "crimson" || got["beta"] != "nebula" {
		t.Fatalf("loadEmpireGuess = %v", got)
	}
}

func TestLoadEmpireGuessMissingFile(t *testing.T) {
	got := loadEmpireGuess(t.TempDir())
	if len(got) != 0 {
		t.Fatalf("missing file should yield empty map, got %v", got)
	}
}

func TestEffectiveCitizenship(t *testing.T) {
	guesses := map[string]string{"new1": "voidborn"}
	cases := []struct {
		name        string
		citizenship string
		id          string
		want        string
	}{
		{"db value wins", "crimson", "new1", "crimson"},
		{"empty falls back to guess", "", "new1", "voidborn"},
		{"whitespace falls back to guess", "  ", "new1", "voidborn"},
		{"empty with no guess stays empty", "", "unknown", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := effectiveCitizenship(c.citizenship, c.id, guesses); got != c.want {
				t.Fatalf("effectiveCitizenship(%q,%q) = %q, want %q", c.citizenship, c.id, got, c.want)
			}
		})
	}
}
