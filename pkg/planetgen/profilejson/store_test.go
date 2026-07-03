package profilejson_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	. "github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

func TestLoadForPlanetAbsent(t *testing.T) {
	dir := t.TempDir()
	env, ok, err := LoadForPlanet(dir, "Nowhere")
	if err != nil {
		t.Fatalf("LoadForPlanet: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing file, got env=%+v", env)
	}
}

func TestLoadForPlanetPresent(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("terran")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "earth",
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "earth.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := LoadForPlanet(dir, "Earth")
	if err != nil {
		t.Fatalf("LoadForPlanet: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true after writing file")
	}
	if got.Type != "terran" || got.Profile.Type != "terran" {
		t.Errorf("envelope decoded incorrectly: %+v", got)
	}
}

func TestLoadForPlanetCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadForPlanet(dir, "broken"); err == nil {
		t.Errorf("expected decode error on corrupt file")
	}
}

func TestLoadForPlanetEmptyRoot(t *testing.T) {
	env, ok, err := LoadForPlanet("", "anything")
	if err != nil {
		t.Fatalf("LoadForPlanet(\"\"): %v", err)
	}
	if ok || env != nil {
		t.Errorf("expected (nil,false,nil) for empty root")
	}
}
