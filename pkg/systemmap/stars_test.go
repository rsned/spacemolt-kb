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
		wantLuminosity string
		wantErr        bool
	}{
		{"G2 V with space", "G2 V", "G", "V", false},
		{"G2V without space", "G2V", "G", "V", false},
		{"B3Ia compact", "B3Ia", "B", "Ia", false},
		{"M9 main sequence", "M9V", "M", "V", false},
		{"M9 without luminosity defaults to V", "M9", "M", "V", false},
		{"G without number", "G V", "G", "V", false},
		{"L5 V brown dwarf", "L5 V", "L", "V", false},
		{"L5V brown dwarf compact", "L5V", "L", "V", false},
		{"T8 brown dwarf", "T8", "T", "V", false},
		{"Y2 brown dwarf", "Y2", "Y", "V", false},
		{"V only invalid (no spectral)", "V", "", "", true},
		{"empty string", "", "", "", true},
		{"invalid spectral", "X9 V", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSpectral, gotLuminosity, err := ParseStarClass(tt.class)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseStarClass(%q) error = %v, wantErr %v", tt.class, err, tt.wantErr)
				return
			}
			if gotSpectral != tt.wantSpectral {
				t.Errorf("ParseStarClass(%q) spectral = %q, want %q", tt.class, gotSpectral, tt.wantSpectral)
			}
			if gotLuminosity != tt.wantLuminosity {
				t.Errorf("ParseStarClass(%q) luminosity = %q, want %q", tt.class, gotLuminosity, tt.wantLuminosity)
			}
		})
	}
}
