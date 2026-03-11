package systemmap

import "testing"

func TestRenderSystemMapWithClassifications(t *testing.T) {
	allSystems := map[string]*System{
		"test-system": {
			ID:   "test-system",
			Name: "Test System",
			POIs: []POI{
				{
					ID:        "star-1",
					Type:      "sun",
					Name:      "Test Star",
					Class:     "G5 V", // Use G5 for pure color (no blending)
					PositionX: 0,
					PositionY: 0,
				},
				{
					ID:        "planet-1",
					Type:      "planet",
					Name:      "Test Planet",
					Class:     "terran",
					PositionX: 1,
					PositionY: 0,
				},
			},
		},
	}

	sys := allSystems["test-system"]
	output := RenderSystemMap(sys, allSystems, true)

	// Check that output contains expected elements
	expectedStrings := []string{
		`G5 V`,                   // Star classification in label
		`#fff4a0`,                // G-type pure color (G5 has no blending)
		`#4a9c6d`,                // Terran planet color
		`Test Star`,              // Star name
		`Test Planet`,            // Planet name
		`xmlns="http://www.w3.org/2000/svg"`, // Standalone SVG
	}

	for _, expected := range expectedStrings {
		if !contains(output, expected) {
			t.Errorf("RenderSystemMap output missing expected string: %q", expected)
		}
	}
}

func TestRenderSystemMapInvalidClassifications(t *testing.T) {
	allSystems := map[string]*System{
		"test-system": {
			ID:   "test-system",
			Name: "Test System",
			POIs: []POI{
				{
					ID:        "star-1",
					Type:      "sun",
					Name:      "Invalid Star",
					Class:     "X9 Z", // Invalid
					PositionX: 0,
					PositionY: 0,
				},
			},
		},
	}

	sys := allSystems["test-system"]
	output := RenderSystemMap(sys, allSystems, true)

	// Should still render with default styling
	if !contains(output, `Invalid Star`) {
		t.Error("RenderSystemMap should render star even with invalid class")
	}
	if !contains(output, `#EBCB8B`) {
		t.Error("Invalid class should use default sun color")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
