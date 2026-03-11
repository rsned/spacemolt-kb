package systemmap

import "testing"

func TestGetStarColor(t *testing.T) {
	tests := []struct {
		name     string
		spectral string
		want     string
	}{
		{"O type blue", "O", "#a0c8ff"},
		{"G type yellow", "G", "#fff4a0"},
		{"M type red", "M", "#ff8060"},
		{"L type dark red", "L", "#cc4020"},
		{"T type infrared", "T", "#882010"},
		{"Y type near-IR", "Y", "#440a05"},
		{"unknown type defaults to sun color", "X", "#EBCB8B"},
		{"empty string defaults to sun color", "", "#EBCB8B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStarColor(tt.spectral); got != tt.want {
				t.Errorf("GetStarColor(%q) = %q, want %q", tt.spectral, got, tt.want)
			}
		})
	}
}

func TestGetStarColorRefined(t *testing.T) {
	tests := []struct {
		name           string
		spectral       string
		subtype        int
		wantBlended    bool
		expectedColor  string // Optional specific color check
	}{
		{"G5 is pure G", "G", 5, false, "#fff4a0"},
		{"G-1 uses base color", "G", -1, false, "#fff4a0"},
		{"G10 uses base color", "G", 10, false, "#fff4a0"},
		{"Brown dwarfs don't blend", "L", 0, false, "#cc4020"},
		{"Brown dwarfs don't blend T", "T", 9, false, "#882010"},
		{"Brown dwarfs don't blend Y", "Y", 5, false, "#440a05"},
		{"G0 blends toward F", "G", 0, true, ""},
		{"G9 blends toward K", "G", 9, true, ""},
		{"O0 has no hotter to blend", "O", 0, false, "#a0c8ff"},
		{"Y9 has no cooler to blend", "Y", 9, false, "#440a05"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetStarColorRefined(tt.spectral, tt.subtype)
			baseColor := GetStarColor(tt.spectral)

			if tt.wantBlended {
				if got == baseColor {
					t.Errorf("GetStarColorRefined(%q, %d) should blend but got base color", tt.spectral, tt.subtype)
				}
			} else {
				if got != baseColor {
					t.Errorf("GetStarColorRefined(%q, %d) = %q, want base color %q", tt.spectral, tt.subtype, got, baseColor)
				}
			}

			if tt.expectedColor != "" && got != tt.expectedColor {
				t.Errorf("GetStarColorRefined(%q, %d) = %q, want %q", tt.spectral, tt.subtype, got, tt.expectedColor)
			}
		})
	}
}

func TestGetStarSize(t *testing.T) {
	tests := []struct {
		name       string
		luminosity string
		want       float64
	}{
		{"Ia hypergiant", "Ia", 28},
		{"V main sequence", "V", 10},
		{"VII white dwarf", "VII", 6},
		{"unknown defaults to main sequence", "X", 10},
		{"empty defaults to main sequence", "", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetStarSize(tt.luminosity); got != tt.want {
				t.Errorf("GetStarSize(%q) = %v, want %v", tt.luminosity, got, tt.want)
			}
		})
	}
}

func TestParseStarClass(t *testing.T) {
	tests := []struct {
		name           string
		class          string
		wantSpectral   string
		wantSubtype    int
		wantLuminosity string
		wantErr        bool
	}{
		{"G2 V with space", "G2 V", "G", 2, "V", false},
		{"G2V without space", "G2V", "G", 2, "V", false},
		{"B3Ia compact", "B3Ia", "B", 3, "Ia", false},
		{"M9 main sequence", "M9V", "M", 9, "V", false},
		{"M9 without luminosity defaults to V", "M9", "M", 9, "V", false},
		{"G without number defaults to subtype 5", "G V", "G", 5, "V", false},
		{"L5 V brown dwarf", "L5 V", "L", 5, "V", false},
		{"L5V brown dwarf compact", "L5V", "L", 5, "V", false},
		{"T8 brown dwarf", "T8", "T", 8, "V", false},
		{"Y2 brown dwarf", "Y2", "Y", 2, "V", false},
		{"G0 hottest G", "G0 V", "G", 0, "V", false},
		{"G9 coolest G", "G9 V", "G", 9, "V", false},
		{"V only invalid (no spectral)", "V", "", -1, "", true},
		{"empty string", "", "", -1, "", true},
		{"invalid spectral", "X9 V", "", -1, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpectral, gotSubtype, gotLuminosity, err := ParseStarClass(tt.class)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStarClass(%q) error = %v, wantErr %v", tt.class, err, tt.wantErr)
				return
			}
			if gotSpectral != tt.wantSpectral {
				t.Errorf("ParseStarClass(%q) spectral = %q, want %q", tt.class, gotSpectral, tt.wantSpectral)
			}
			if gotSubtype != tt.wantSubtype {
				t.Errorf("ParseStarClass(%q) subtype = %d, want %d", tt.class, gotSubtype, tt.wantSubtype)
			}
			if gotLuminosity != tt.wantLuminosity {
				t.Errorf("ParseStarClass(%q) luminosity = %q, want %q", tt.class, gotLuminosity, tt.wantLuminosity)
			}
		})
	}
}
