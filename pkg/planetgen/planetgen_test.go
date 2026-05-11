package planetgen_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

// withProfileRoot temporarily points planetgen at dir and restores the
// previous root on cleanup.
func withProfileRoot(t *testing.T, dir string) {
	t.Helper()
	prev := planetgen.ProfileRootForTest()
	planetgen.SetProfileRoot(dir)
	t.Cleanup(func() { planetgen.SetProfileRoot(prev) })
}

func TestGenerateFallsBackToDefaultsWhenAbsent(t *testing.T) {
	withProfileRoot(t, t.TempDir())
	cm, err := planetgen.Generate("terran", "FallbackPlanet", 32)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if cm == nil {
		t.Fatal("Generate returned nil cube map")
	}
}

func TestGenerateUsesProfileFromDisk(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("terran")
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "DiskPlanet",
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diskplanet.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	withProfileRoot(t, dir)
	gotFromDisk, err := planetgen.Generate("terran", "DiskPlanet", 32)
	if err != nil {
		t.Fatalf("Generate (disk): %v", err)
	}

	withProfileRoot(t, t.TempDir())
	gotFromCode, err := planetgen.Generate("terran", "DiskPlanet", 32)
	if err != nil {
		t.Fatalf("Generate (code): %v", err)
	}

	// Both should produce byte-identical output because the on-disk
	// envelope is just a serialization of the in-code default.
	for face := 0; face < 6; face++ {
		if !bytes.Equal(cubeFaceBytes(gotFromDisk, face), cubeFaceBytes(gotFromCode, face)) {
			t.Errorf("face %d differs between disk and code paths", face)
			break
		}
	}
}

func TestGenerateRejectsTypeMismatch(t *testing.T) {
	dir := t.TempDir()
	prof := planetgen.GetProfile("scorched")
	env := &profilejson.Envelope{
		SchemaVersion: profilejson.CurrentSchemaVersion,
		Type:          "scorched",
		Seed:          "MismatchPlanet",
		Profile:       prof,
	}
	data, err := profilejson.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mismatchplanet.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	withProfileRoot(t, dir)
	if _, err := planetgen.Generate("terran", "MismatchPlanet", 32); err == nil {
		t.Errorf("expected type-mismatch error, got nil")
	}
}

func cubeFaceBytes(cm *cubemap.CubeMap, face int) []byte {
	pix := cm.Faces[face]
	if pix == nil {
		return nil
	}
	out := make([]byte, 0, len(pix)*4)
	for _, c := range pix {
		out = append(out, c.R, c.G, c.B, c.A)
	}
	return out
}
