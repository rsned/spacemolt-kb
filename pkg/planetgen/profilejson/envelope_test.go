package profilejson_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen"
	. "github.com/rsned/spacemolt-kb/pkg/planetgen/profilejson"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	prof := planetgen.GetProfile("terran")
	if prof == nil {
		t.Fatal("terran profile missing")
	}
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		HandTuned:     false,
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) {
		t.Errorf("Encode output missing trailing newline")
	}
	if !strings.Contains(string(data), `"schemaVersion": "1"`) {
		t.Errorf("Encode output missing schemaVersion: %q", string(data[:80]))
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Type != env.Type || got.Seed != env.Seed || got.HandTuned != env.HandTuned {
		t.Errorf("envelope metadata mismatch: got %+v", got)
	}
	if got.Profile == nil || got.Profile.Type != "terran" {
		t.Errorf("profile not decoded correctly")
	}
}

func TestEncodeStable(t *testing.T) {
	prof := planetgen.GetProfile("scorched")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "scorched", Seed: "scorched_default",
		HandTuned: false, Profile: prof,
	}
	a, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("Encode not byte-stable across calls")
	}
}

// TestEncodeOmitsEmptyNameNotes verifies that an envelope with empty
// Name and Notes serializes WITHOUT those keys, so seeder-produced
// default fixtures stay byte-stable against the drift guard.
func TestEncodeOmitsEmptyNameNotes(t *testing.T) {
	prof := planetgen.GetProfile("terran")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "terran_default",
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"name"`) {
		t.Errorf("Encode emitted empty name key:\n%s", data[:200])
	}
	if strings.Contains(string(data), `"notes"`) {
		t.Errorf("Encode emitted empty notes key:\n%s", data[:200])
	}
}

// TestEncodeDecodeRoundTripWithNameNotes verifies Name and Notes
// survive a round-trip when set.
func TestEncodeDecodeRoundTripWithNameNotes(t *testing.T) {
	prof := planetgen.GetProfile("terran")
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran",
		Seed:          "82_eridani_ii",
		Name:          "Cradle of Life",
		Notes:         "Bumped Civ.Tier and dropped polar cap size; see issue #421.",
		HandTuned:     true,
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Name != env.Name {
		t.Errorf("Name = %q, want %q", got.Name, env.Name)
	}
	if got.Notes != env.Notes {
		t.Errorf("Notes = %q, want %q", got.Notes, env.Notes)
	}
}

func TestDecodeRejectsTypeMismatch(t *testing.T) {
	prof := planetgen.GetProfile("scorched") // Profile.Type = scorched
	env := &Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Type:          "terran", // envelope says terran — mismatch
		Seed:          "x",
		Profile:       prof,
	}
	data, err := Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil {
		t.Errorf("Decode accepted envelope/profile type mismatch")
	}
}
