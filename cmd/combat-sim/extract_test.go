package main

import (
	"bytes"
	"path/filepath"
	"reflect"
	"testing"
)

func battlesDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "data", "battles")
}

func fitByFile(t *testing.T, fits []ExtractedFit, name string) FitSpec {
	t.Helper()
	for _, f := range fits {
		if f.Filename == name {
			return f.Spec
		}
	}
	t.Fatalf("no extracted fit named %s", name)
	return FitSpec{}
}

// Golden: fixture 7c044558 has a raw battle log, and both pilots' true
// skills are known out-of-band (get_skills for Artis; MoltenOne derived
// and locked in the README example).
func TestExtractFitsGolden7c044558(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var warn bytes.Buffer
	fits, err := ExtractFits("7c044558c0c39e972fe560110f69ea25", battlesDir(t), cat, &warn)
	if err != nil {
		t.Fatalf("ExtractFits: %v", err)
	}
	if len(fits) != 2 {
		t.Fatalf("got %d fits, want 2", len(fits))
	}

	molten := fitByFile(t, fits, "7c044558c0c39e972fe560110f69ea25_b195177bf33ce1de4d155a57d1ab149e.json")
	if molten.Hull != "broadaxe" {
		t.Errorf("molten hull = %q, want broadaxe", molten.Hull)
	}
	wantMods := []string{"autocannon_ii", "autocannon_ii", "autocannon_ii", "flak_cannon_ii", "armor_plate_ii"}
	if !reflect.DeepEqual(molten.Modules, wantMods) {
		t.Errorf("molten modules = %v, want %v", molten.Modules, wantMods)
	}
	wantSkills := map[string]int{"weapons": 7, "gunnery": 10, "shields": 4, "armor": 0}
	if !reflect.DeepEqual(molten.Skills, wantSkills) {
		t.Errorf("molten skills = %v, want %v", molten.Skills, wantSkills)
	}
	if molten.Name != "MoltenOne (broadaxe)" {
		t.Errorf("molten name = %q", molten.Name)
	}

	artis := fitByFile(t, fits, "7c044558c0c39e972fe560110f69ea25_a50924913cef881c5e4d14257589d9ba.json")
	if artis.Hull != "survey_vessel" {
		t.Errorf("artis hull = %q, want survey_vessel", artis.Hull)
	}
	wantArtisMods := []string{
		"anomaly_detector", "survey_scanner_ii", "cloaking_device_i",
		"shield_booster_iv", "shield_recharger_ii",
		"pulse_laser_iii", "pulse_laser_iii", "ship_scanner_iii",
	}
	if !reflect.DeepEqual(artis.Modules, wantArtisMods) {
		t.Errorf("artis modules = %v, want %v", artis.Modules, wantArtisMods)
	}
	wantArtisSkills := map[string]int{"weapons": 3, "gunnery": 3, "shields": 1, "armor": 0}
	if !reflect.DeepEqual(artis.Skills, wantArtisSkills) {
		t.Errorf("artis skills = %v, want %v", artis.Skills, wantArtisSkills)
	}
}

// Fixture b7847bbc has no .raw.json next to it: every participant still
// gets a fit, skills fall back to all-zero, and the caller is warned.
func TestExtractFitsNoRawFile(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var warn bytes.Buffer
	fits, err := ExtractFits("b7847bbc62a59f67f503ab3c65fb0897", battlesDir(t), cat, &warn)
	if err != nil {
		t.Fatalf("ExtractFits: %v", err)
	}
	if len(fits) != 4 {
		t.Fatalf("got %d fits, want 4 (MoltenOne, 2 drones, VeraLane)", len(fits))
	}
	zero := map[string]int{"weapons": 0, "gunnery": 0, "shields": 0, "armor": 0}
	for _, f := range fits {
		if !reflect.DeepEqual(f.Spec.Skills, zero) {
			t.Errorf("%s skills = %v, want all-zero (no raw log)", f.Filename, f.Spec.Skills)
		}
	}
	if warn.Len() == 0 {
		t.Error("expected a warning about the missing raw log")
	}
}

func TestExtractFitsUnknownBattle(t *testing.T) {
	cat, err := LoadCatalog(catalogDir(t))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if _, err := ExtractFits("doesnotexist", battlesDir(t), cat, &bytes.Buffer{}); err == nil {
		t.Fatal("want error for unknown battle id")
	}
}
