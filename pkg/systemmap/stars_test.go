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
