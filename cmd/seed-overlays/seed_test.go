package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNormalizeName(t *testing.T) {
	if normalizeName("  Foundling Mira ") != "foundling mira" {
		t.Error("name normalize")
	}
}

func TestNormalizeOrg(t *testing.T) {
	if normalizeOrg("The Hex Collective") != "hex collective" {
		t.Error("strip leading 'the'")
	}
	if normalizeOrg("Hex Collective") != "hex collective" {
		t.Error("no-the case")
	}
}

func TestRenderStub(t *testing.T) {
	s := renderStub("A bio about **someone**.", []stubStat{{"Organization", "Hex"}, {"Role", "Acolyte"}})
	if !strings.Contains(s, `label: "Organization"`) || !strings.Contains(s, `value: "Hex"`) {
		t.Errorf("stats missing: %s", s)
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "A bio about **someone**.") {
		t.Errorf("body missing/last: %s", s)
	}
}

func TestRenderStubEmptyStats(t *testing.T) {
	s := renderStub("A simple bio.", nil)
	if !strings.Contains(s, "---") {
		t.Errorf("expected frontmatter fences: %s", s)
	}
	if !strings.HasSuffix(strings.TrimSpace(s), "A simple bio.") {
		t.Errorf("bio missing from output: %s", s)
	}
}

func TestRenderStubValidYAML(t *testing.T) {
	s := renderStub("Body.", []stubStat{{"Organization", "Merchants: United"}})

	// Extract frontmatter between the first "---\n" and the closing "\n---".
	after, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		t.Fatalf("no opening fence: %s", s)
	}
	fmEnd := strings.Index(after, "\n---")
	if fmEnd < 0 {
		t.Fatalf("no closing fence: %s", s)
	}
	fm := after[:fmEnd]

	var out struct {
		Stats []struct {
			Label string `yaml:"label"`
			Value string `yaml:"value"`
		} `yaml:"stats"`
	}
	if err := yaml.Unmarshal([]byte(fm), &out); err != nil {
		t.Fatalf("yaml parse error: %v\nfrontmatter was:\n%s", err, fm)
	}
	if len(out.Stats) != 1 {
		t.Fatalf("expected 1 stat, got %d", len(out.Stats))
	}
	if out.Stats[0].Value != "Merchants: United" {
		t.Errorf("value round-trip failed: got %q", out.Stats[0].Value)
	}
}

func TestWriteStubSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.md")
	if err := os.WriteFile(path, []byte("ORIGINAL"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Not dry-run, but file exists -> must skip (no overwrite).
	wrote, err := writeStub(path, "NEW CONTENT", false)
	if err != nil || wrote {
		t.Errorf("should skip existing: wrote=%v err=%v", wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "ORIGINAL" {
		t.Error("existing file was overwritten")
	}

	// Dry-run on a new path -> reports it would write, but writes nothing.
	newPath := filepath.Join(dir, "sub", "profile.md")
	wrote, err = writeStub(newPath, "X", true)
	if err != nil || !wrote {
		t.Errorf("dry-run new: wrote=%v err=%v", wrote, err)
	}
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Error("dry-run must not create files")
	}
}
