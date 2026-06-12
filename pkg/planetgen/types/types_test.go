package types

import (
	"encoding/json"
	"math"
	"strings"
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

func TestPlanetProfilePhase7FieldsRoundTrip(t *testing.T) {
	p := PlanetProfile{
		PlateCount:           12,
		OceanicPlateFraction: 0.7,
		PlateConvergentT:     0.75,
		JitterEnabled:        true,
		JitterCellCount:      120,
		JitterRotMax:         math.Pi / 4,
		JitterOffsetMax:      0.1,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got PlanetProfile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PlateCount != p.PlateCount ||
		got.OceanicPlateFraction != p.OceanicPlateFraction ||
		got.PlateConvergentT != p.PlateConvergentT ||
		got.JitterEnabled != p.JitterEnabled ||
		got.JitterCellCount != p.JitterCellCount ||
		got.JitterRotMax != p.JitterRotMax ||
		got.JitterOffsetMax != p.JitterOffsetMax {
		t.Errorf("Phase 7 fields not round-tripped: got %+v", got)
	}
}

func TestCloudConfigJSONRoundTrip(t *testing.T) {
	in := CloudConfig{
		Coverage:       0.45,
		BandLatRad:     0.26,
		Freq:           4,
		Octaves:        4,
		WarpAmp:        0.4,
		StormCount:     5,
		StormRadiusRad: 0.20,
		SunDir:         [3]float64{1, 0.3, 0},
		ShadowGain:     0.5,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out CloudConfig
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if in != out {
		t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", in, out)
	}
}

func TestCivConfigJSONRoundTrip(t *testing.T) {
	in := CivConfig{
		Tier:             0.5,
		SiteMinDistRad:   0.0314,
		SiteMaxDistRad:   0.1047,
		MaxPopulation:    1.0,
		NightLightHue:    0.12,
		AgricultureRatio: 0.4,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out CivConfig
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if in != out {
		t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", in, out)
	}
}

func TestPlanetProfilePhase7JitterDisabledSurvivesRoundTrip(t *testing.T) {
	p := PlanetProfile{
		JitterEnabled: false,
		// All other fields zero — simulates a gas giant profile.
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"JitterEnabled":false`) {
		t.Errorf("JitterEnabled:false must serialize explicitly (no omitempty); got %s", string(b))
	}
	var got PlanetProfile
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.JitterEnabled != false {
		t.Errorf("JitterEnabled did not round-trip as false: %v", got.JitterEnabled)
	}
}

func TestCrustConfigJSONRoundTrip(t *testing.T) {
	in := CrustConfig{
		MajorPlates: 7, MinorPlates: 4, MajorGrowthBias: 4,
		OceanicFraction: 0.45,
		Assembly:        -1, AssemblyWeights: [3]float64{25, 65, 10},
		TargetLandFraction: -1, LandFracLo: 0.22, LandFracHi: 0.38,
		TectonicAge: -1, AgeLo: 0.25, AgeHi: 0.75,
		CratonsMax: 8, ShelfWidthRad: 0.05,
		EdgeNoiseAmp: 0.45, EdgeNoiseFreq: 2.2, EdgeNoiseOctaves: 4,
		PlatformHeight: 0.62, OceanFloorHeight: 0.25,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out CrustConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestCrustSentinelsSurviveZeroAndNegOne(t *testing.T) {
	// Assembly / TargetLandFraction / TectonicAge must round-trip both
	// 0 (pinned supercontinent) and -1 (sample) — they are not omitempty.
	for _, v := range []float64{0, -1} {
		in := CrustConfig{MajorPlates: 5, Assembly: v, TargetLandFraction: v, TectonicAge: v}
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var out CrustConfig
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Assembly != v || out.TargetLandFraction != v || out.TectonicAge != v {
			t.Errorf("sentinel %v did not round-trip: %+v", v, out)
		}
	}
}

func TestTectonicFXConfigJSONRoundTrip(t *testing.T) {
	in := TectonicFXConfig{
		BeltAmp: 0.3, BeltWidthKm: 900, BeltFreq: 3.2, BeltOctaves: 5,
		CordAmp: 0.22, CordWidthKm: 450,
		TrenchDepth: 0.12, TrenchWidthKm: 220,
		ArcAmp: 0.25, ArcWidthKm: 260,
		RidgeAmp: 0.06, RidgeWidthKm: 700,
		RiftDepth: 0.1, RiftWidthKm: 280, RiftShoulder: 0.35,
		TransformAmp: 0.03, TransformWidthKm: 150,
		ActivityFreq: 1.5,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out TectonicFXConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}
