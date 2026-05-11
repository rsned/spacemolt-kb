package profilejson

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// CurrentSchemaVersion is the version stamped on Envelopes written by
// Encode. Bumped only when the envelope wire format changes; the inner
// PlanetProfile evolves independently via Go struct-tag additions.
const CurrentSchemaVersion = "1"

// Envelope is the on-disk wrapper around a PlanetProfile. The
// duplicate Type field (envelope.Type vs envelope.Profile.Type) is
// deliberate: the envelope Type is the dispatch key checked by the
// generator before the inner Profile is trusted.
type Envelope struct {
	SchemaVersion string               `json:"schemaVersion"`
	Type          string               `json:"type"`
	Seed          string               `json:"seed"`
	HandTuned     bool                 `json:"handTuned"`
	Profile       *types.PlanetProfile `json:"profile"`
}

// Encode marshals env with canonical formatting: 2-space indent and
// trailing newline. Two calls with equal Envelope values produce
// byte-identical output.
func Encode(env *Envelope) ([]byte, error) {
	if env == nil {
		return nil, fmt.Errorf("profilejson: nil envelope")
	}
	if env.Profile == nil {
		return nil, fmt.Errorf("profilejson: envelope has nil Profile")
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return nil, fmt.Errorf("profilejson: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// Decode parses an envelope from JSON. It first runs Migrate to lift
// older schema versions to the current shape, then validates that
// envelope.Type == envelope.Profile.Type.
func Decode(data []byte) (*Envelope, error) {
	migrated, err := Migrate(data)
	if err != nil {
		return nil, err
	}
	var env Envelope
	if err := json.Unmarshal(migrated, &env); err != nil {
		return nil, fmt.Errorf("profilejson: decode: %w", err)
	}
	if env.Profile == nil {
		return nil, fmt.Errorf("profilejson: envelope missing profile")
	}
	if env.Type != env.Profile.Type {
		return nil, fmt.Errorf(
			"profilejson: envelope type %q != profile.type %q",
			env.Type, env.Profile.Type)
	}
	return &env, nil
}

// Migrate lifts older-schema envelopes to the current shape. Today v1
// is current, so the function is a passthrough that validates the
// schemaVersion field is present and recognized. The first real
// migration ships when v2 is introduced.
func Migrate(data []byte) ([]byte, error) {
	var peek struct {
		SchemaVersion string `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("profilejson: migrate peek: %w", err)
	}
	if peek.SchemaVersion == "" {
		return nil, fmt.Errorf("profilejson: missing schemaVersion")
	}
	if peek.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf(
			"profilejson: unknown schemaVersion %q (want %q)",
			peek.SchemaVersion, CurrentSchemaVersion)
	}
	return data, nil
}
