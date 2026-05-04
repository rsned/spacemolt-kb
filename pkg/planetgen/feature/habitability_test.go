package feature

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/cubemap/seamtest"
	"github.com/rsned/spacemolt-kb/pkg/planetgen/types"
)

// fillF builds a CubeMapF whose every pixel holds v.
func fillF(S int, v float64) *cubemap.CubeMapF {
	cm := cubemap.NewF(S)
	for face := range cubemap.Face(cubemap.NumFaces) {
		for i := range cm.Faces[face] {
			cm.Faces[face][i] = v
		}
	}
	return cm
}

func enabledCivCfg() types.CivConfig {
	return types.CivConfig{Tier: 0.5}
}

// TestHabitabilityDisabledByZeroTier verifies the zero-value gate.
func TestHabitabilityDisabledByZeroTier(t *testing.T) {
	S := 16
	h := fillF(S, 0.4)
	tF := fillF(S, 0.55)
	mF := fillF(S, 0.6)
	got := GenerateHabitability(h, tF, mF, nil, nil, nil, types.CivConfig{}, S)
	if got != nil {
		t.Errorf("GenerateHabitability with Tier=0 returned non-nil; want nil")
	}
}

// TestHabitabilityZeroOnOcean: ocean pixels (height = 0.1) score <0.1.
func TestHabitabilityZeroOnOcean(t *testing.T) {
	S := 16
	h := fillF(S, 0.1)
	// Climate fields kept "uninteresting" so the height term dominates.
	// Use far-from-mu values so gaussian terms are tiny.
	tF := fillF(S, 0.0)
	mF := fillF(S, 0.0)
	hf := GenerateHabitability(h, tF, mF, nil, nil, nil, enabledCivCfg(), S)
	if hf == nil {
		t.Fatalf("GenerateHabitability returned nil for enabled cfg")
	}
	var maxScore float64
	for face := range cubemap.NumFaces {
		for _, v := range hf.Score[face] {
			if v > maxScore {
				maxScore = v
			}
		}
	}
	if maxScore >= 0.1 {
		t.Errorf("ocean habitability max=%.4f; want < 0.1", maxScore)
	}
}

// TestHabitabilityHighOnTemperateLowlands: with height=0.40 (smoothstep
// of (0.30,0.55) at peak), temp=0.55 (μ peak) and humid=0.6 (μ peak),
// score should clearly exceed an ocean baseline.
func TestHabitabilityHighOnTemperateLowlands(t *testing.T) {
	S := 16
	h := fillF(S, 0.40)
	tF := fillF(S, 0.55)
	mF := fillF(S, 0.60)
	hf := GenerateHabitability(h, tF, mF, nil, nil, nil, enabledCivCfg(), S)
	if hf == nil {
		t.Fatalf("GenerateHabitability returned nil for enabled cfg")
	}
	// Sample a single pixel — the field is uniform so any pixel works.
	// 0.4 floor is a comfortable lower bound: with h=0.40 the lowland
	// smoothstep contributes ≈ habWeightLow * smoothstep(0.30, 0.55, 0.40),
	// plus full temp-gaussian + full moist-gaussian peaks; well above the
	// ocean-baseline run in TestHabitabilityZeroOnOcean (≈ 0.002).
	score := hf.Score[cubemap.FacePosZ][0]
	if score <= 0.4 {
		t.Errorf("temperate lowland habitability=%.4f; want > 0.4", score)
	}
	// Sanity: clamped to [0,1].
	if score < 0 || score > 1 {
		t.Errorf("score=%.4f outside [0,1]", score)
	}
}

// TestHabitabilityDeterministic: same inputs → same outputs.
func TestHabitabilityDeterministic(t *testing.T) {
	S := 32
	h := fillF(S, 0.40)
	tF := fillF(S, 0.55)
	mF := fillF(S, 0.60)
	cfg := enabledCivCfg()
	hf1 := GenerateHabitability(h, tF, mF, nil, nil, nil, cfg, S)
	hf2 := GenerateHabitability(h, tF, mF, nil, nil, nil, cfg, S)
	if hf1 == nil || hf2 == nil {
		t.Fatalf("nil HabitabilityField for enabled cfg")
	}
	for face := range cubemap.NumFaces {
		var b1, b2 bytes.Buffer
		if err := binary.Write(&b1, binary.LittleEndian, hf1.Score[face]); err != nil {
			t.Fatalf("encode hf1 face=%d: %v", face, err)
		}
		if err := binary.Write(&b2, binary.LittleEndian, hf2.Score[face]); err != nil {
			t.Fatalf("encode hf2 face=%d: %v", face, err)
		}
		if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
			t.Errorf("face=%d score byte slice mismatch between two runs", face)
		}
	}
}

// TestHabitabilitySeamContinuity: a synthetic uniform fixture is
// trivially seam-continuous; this catches face-local indexing bugs in
// the writer.
func TestHabitabilitySeamContinuity(t *testing.T) {
	S := 64
	h := fillF(S, 0.40)
	tF := fillF(S, 0.55)
	mF := fillF(S, 0.60)
	hf := GenerateHabitability(h, tF, mF, nil, nil, nil, enabledCivCfg(), S)
	if hf == nil {
		t.Fatalf("nil HabitabilityField for enabled cfg")
	}
	cm := &cubemap.CubeMapF{Size: hf.Size, Faces: hf.Score}
	seamtest.AssertSeamContinuity(t, "habitability", cm, 0.05)
}
