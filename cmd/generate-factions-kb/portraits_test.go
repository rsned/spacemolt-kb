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
	generatePassengerPortraits(ps, root, cmd, 0)

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
	// No archetypes.json in this tempdir, so the generator classifies p1 as the
	// empty (spacer) archetype — the cached prompt must match that to stay warm.
	prompt := buildPortraitPrompt("p1", "a fixer", "first", "nebula", "", "")
	if err := os.WriteFile(filepath.Join(dir, promptSidecarName), []byte(promptHash(prompt)+"\n"+prompt+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, generatedPortraitName), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "invoked.marker")
	cmd := `touch "` + marker + `"`
	generatePassengerPortraits(ps, root, cmd, 0)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("command was invoked despite warm cache")
	}
}

func TestGeneratePortraitsLimit(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{
		{ID: "p1", Slug: "p1", Bio: "a"},
		{ID: "p2", Slug: "p2", Bio: "b"},
		{ID: "p3", Slug: "p3", Bio: "c"},
	}
	cmd := `printf 'x' > "$PORTRAIT_OUT"`
	generatePassengerPortraits(ps, root, cmd, 2)
	for _, id := range []string{"p1", "p2"} {
		out := filepath.Join(passengerGeneratedDir(root, id), generatedPortraitName)
		if _, err := os.Stat(out); err != nil {
			t.Fatalf("expected portrait for %s under limit=2: %v", id, err)
		}
	}
	if _, err := os.Stat(passengerGeneratedDir(root, "p3")); err == nil {
		t.Fatal("p3 should be skipped under limit=2")
	}
}

func TestSelectPassengers(t *testing.T) {
	ps := []*Passenger{
		{ID: "helan_tarrard", Name: "Helan Tarrard"},
		{ID: "lissette_rillen", Name: "Lissette Rillen"},
		{ID: "ziggy_stardrift", Name: "Ziggy Stardrift"},
		{ID: "the_physician", Name: "The Physician"},
	}

	// Empty spec is a passthrough.
	if got, _ := selectPassengers(ps, ""); len(got) != len(ps) {
		t.Fatalf("empty spec should return all %d; got %d", len(ps), len(got))
	}

	// Exact citizen_id (case-insensitive) and a name substring, both matched, in
	// input order, deduped per passenger.
	got, unmatched := selectPassengers(ps, "ZIGGY_STARDRIFT, rillen , the_physician, ghost")
	if len(got) != 3 {
		t.Fatalf("expected 3 matches; got %d (%v)", len(got), got)
	}
	wantIDs := []string{"lissette_rillen", "ziggy_stardrift", "the_physician"}
	for i, p := range got {
		if p.ID != wantIDs[i] {
			t.Fatalf("match %d: got %s want %s", i, p.ID, wantIDs[i])
		}
	}
	if len(unmatched) != 1 || unmatched[0] != "ghost" {
		t.Fatalf("expected unmatched [ghost]; got %v", unmatched)
	}
}

func TestGeneratePortraitsNoCommandIsNoop(t *testing.T) {
	root := t.TempDir()
	ps := []*Passenger{{ID: "p1", Slug: "p1", Bio: "x"}}
	generatePassengerPortraits(ps, root, "", 0)
	if _, err := os.Stat(passengerGeneratedDir(root, "p1")); err == nil {
		t.Fatal("no-command run should not create cache dirs")
	}
}
