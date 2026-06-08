package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratePortraitsInvokesCommandAndCaches(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "a fixer", Class: "first", Citizenship: "nebula"}}
	cmd := `printf 'x' > "$PORTRAIT_OUT"`
	generatePassengerPortraits(ps, root, cmd)

	out := filepath.Join(passengerGeneratedDir(root, "p1"), generatedPortraitName)
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("expected generated portrait at %s: %v", out, err)
	}
	sidecar := filepath.Join(passengerGeneratedDir(root, "p1"), promptSidecarName)
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("expected prompt sidecar: %v", err)
	}
}

func TestGeneratePortraitsSkipsWhenCached(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "a fixer", Class: "first", Citizenship: "nebula"}}
	dir := passengerGeneratedDir(root, "p1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	prompt := buildPortraitPrompt("a fixer", "first", "nebula")
	if err := os.WriteFile(filepath.Join(dir, promptSidecarName), []byte(promptHash(prompt)+"\n"+prompt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, generatedPortraitName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "invoked.marker")
	cmd := `touch "` + marker + `"`
	generatePassengerPortraits(ps, root, cmd)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command was invoked despite warm cache")
	}
}

func TestGeneratePortraitsNoCommandIsNoop(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "x"}}
	generatePassengerPortraits(ps, root, "")
	if _, err := os.Stat(passengerGeneratedDir(root, "p1")); err == nil {
		t.Fatal("no-command run should not create cache dirs")
	}
}
