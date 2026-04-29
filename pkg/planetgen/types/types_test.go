package types

import (
	"encoding/json"
	"testing"
)

func TestControlConfigLegacyErosionKey(t *testing.T) {
	raw := []byte(`{"Erosion":{"Amp":1.5,"Freq":2.0,"Octaves":3}}`)
	var cfg ControlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Detail.Amp != 1.5 || cfg.Detail.Freq != 2.0 || cfg.Detail.Octaves != 3 {
		t.Errorf("legacy Erosion key did not populate Detail; got %+v", cfg.Detail)
	}
}

func TestControlConfigDetailKeyWins(t *testing.T) {
	raw := []byte(`{"Detail":{"Amp":2.5},"Erosion":{"Amp":9.9}}`)
	var cfg ControlConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Detail.Amp != 2.5 {
		t.Errorf("Detail should win over Erosion; got Amp=%f", cfg.Detail.Amp)
	}
}
