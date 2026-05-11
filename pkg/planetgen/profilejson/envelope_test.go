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
